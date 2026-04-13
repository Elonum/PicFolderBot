package telegram

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"PicFolderBot/internal/parser"
	"PicFolderBot/internal/service"
)

const (
	telegramDownloadTimeout = 30 * time.Second
	maxImageBytes           = 20 << 20 // 20MB
	listPageSize            = 8
	maxButtonLabelRunes     = 26
	albumFlushDelay         = 1200 * time.Millisecond
)

type flowAPI interface {
	ParseCaption(caption string) parser.ParsedInput
	RootDisplayName() string
	ListProducts() ([]string, error)
	ListColors(product string) ([]string, error)
	ListSections(product, color string) ([]string, error)
	UploadImage(payload service.UploadPayload) (string, error)
	CreateFolderAtLevel(level, product, color, section, newFolder string) (string, error)
}

type sessionState struct {
	Product     string
	Color       string
	Section     string
	AddLevel    string
	SearchQuery string
	SearchField string
	PageProduct int
	PageColor   int
	PageSection int
	FileID      string
	FileName    string
	FileMIME    string
	FileBytes   []byte
	ValueMap    map[string]string
	Awaiting    string
}

type Bot struct {
	api      *tgbotapi.BotAPI
	flow     flowAPI
	rootName string
	sessions map[int64]*sessionState
	albums   map[string]*albumBuffer
	mu       sync.RWMutex
	albumsMu sync.Mutex
}

type albumItem struct {
	Filename string
	MimeType string
	Content  []byte
}

type albumBuffer struct {
	ChatID       int64
	MediaGroupID string
	Product      string
	Color        string
	Section      string
	Items        []albumItem
	Timer        *time.Timer
	Notified     bool
}

func NewBot(token string, flow flowAPI) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(strings.TrimSpace(token))
	if err != nil {
		return nil, err
	}
	_, _ = api.Request(tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "upload", Description: "Начать пошаговую загрузку"},
		tgbotapi.BotCommand{Command: "search", Description: "Быстрый поиск товаров/цветов"},
		tgbotapi.BotCommand{Command: "help", Description: "Показать справку"},
		tgbotapi.BotCommand{Command: "cancel", Description: "Отменить текущее действие"},
	))
	return &Bot{
		api:      api,
		flow:     flow,
		rootName: flow.RootDisplayName(),
		sessions: make(map[int64]*sessionState),
		albums:   make(map[string]*albumBuffer),
	}, nil
}

func (b *Bot) Run(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	updates := b.api.GetUpdatesChan(u)

	log.Printf("telegram bot started: @%s", b.api.Self.UserName)
	for {
		select {
		case <-ctx.Done():
			b.api.StopReceivingUpdates()
			return nil
		case upd := <-updates:
			if err := b.handleUpdate(upd); err != nil {
				log.Printf("update error: %v", err)
			}
		}
	}
}

func (b *Bot) handleUpdate(upd tgbotapi.Update) error {
	if upd.CallbackQuery != nil {
		return b.handleCallback(upd.CallbackQuery)
	}

	if upd.Message == nil {
		return nil
	}

	msg := upd.Message
	chatID := msg.Chat.ID

	if msg.IsCommand() {
		switch msg.Command() {
		case "start", "help":
			return b.sendWelcome(chatID)
		case "upload":
			b.setSession(chatID, &sessionState{Awaiting: "product"})
			return b.askProduct(chatID)
		case "search":
			state := b.getSession(chatID)
			if state == nil {
				state = &sessionState{}
				b.setSession(chatID, state)
			}
			return b.sendSearchMenu(chatID, state)
		case "cancel":
			b.clearSession(chatID)
			return b.send(chatID, "🛑 Действие отменено. Нажмите /upload, чтобы начать заново.")
		default:
			return b.send(chatID, "❓ Неизвестная команда. Доступно: /upload, /search, /help, /cancel")
		}
	}

	if msg.Photo != nil {
		return b.handlePhoto(msg)
	}
	if msg.Document != nil {
		return b.handleDocument(msg)
	}

	state := b.getSession(chatID)
	if state != nil && state.Awaiting == "new_folder_name" && msg.Text != "" {
		newFolder := strings.TrimSpace(msg.Text)
		if newFolder == "" {
			return b.send(chatID, "⚠️ Введите непустое имя новой папки.")
		}
		level := state.AddLevel
		target, err := b.flow.CreateFolderAtLevel(level, state.Product, state.Color, state.Section, newFolder)
		if err != nil {
			return b.send(chatID, "❌ Не удалось создать папку: "+humanError(err))
		}
		state.AddLevel = level
		if err = b.send(chatID, "✅ Папка создана:\n"+target); err != nil {
			return err
		}
		err = b.refreshLevel(chatID, state)
		state.AddLevel = ""
		return err
	}
	if state != nil && state.Awaiting == "search_product_query" && msg.Text != "" {
		state.SearchField = "product"
		state.SearchQuery = strings.TrimSpace(msg.Text)
		state.PageProduct = 0
		return b.askProduct(chatID)
	}
	if state != nil && state.Awaiting == "search_color_query" && msg.Text != "" {
		if strings.TrimSpace(state.Product) == "" {
			state.Awaiting = "product"
			return b.send(chatID, "⚠️ Сначала выберите товар, затем выполните поиск по цветам.")
		}
		state.SearchField = "color"
		state.SearchQuery = strings.TrimSpace(msg.Text)
		state.PageColor = 0
		return b.askColor(chatID, state.Product)
	}
	if state != nil && msg.Text != "" {
		handled, err := b.handlePathTextInput(chatID, state, msg.Text)
		if err != nil {
			return err
		}
		if handled {
			return nil
		}
	}

	state = b.getSession(chatID)
	if state != nil && state.Awaiting == "photo" {
		return b.send(chatID, "🖼️ Ожидаю изображение.\nРазрешенные форматы: "+allowedFormatsText)
	}
	if state != nil && state.Awaiting == "post_upload_choice" {
		return b.sendWithKeyboard(chatID, "📤 Загрузить еще в этот же раздел?", "", nil, "", "section", 0, 0,
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ Да", "post|same|yes"),
				tgbotapi.NewInlineKeyboardButtonData("🧭 Изменить путь", "post|change|path"),
			),
		)
	}

	return b.send(chatID, "ℹ️ Отправьте фото с подписью или используйте /upload для пошаговой загрузки.")
}

func (b *Bot) handlePhoto(msg *tgbotapi.Message) error {
	chatID := msg.Chat.ID
	file := msg.Photo[len(msg.Photo)-1]

	fileURL, err := b.api.GetFileDirectURL(file.FileID)
	if err != nil {
		return b.send(chatID, "❌ Не удалось получить файл из Telegram.")
	}

	content, mimeType, err := downloadFile(fileURL)
	if err != nil {
		return b.send(chatID, "❌ Не удалось скачать изображение.")
	}
	if !isAllowedImageMIME(mimeType) {
		return b.send(chatID, "⚠️ Неподдерживаемый формат изображения.\nРазрешенные форматы: "+allowedFormatsText)
	}

	fileName := buildFileName(fmt.Sprintf("img_%d", time.Now().Unix()), mimeType)
	if msg.MediaGroupID != "" {
		product, color, section := b.resolveUploadContext(chatID, msg.Caption)
		return b.enqueueAlbumItem(chatID, msg.MediaGroupID, albumItem{
			Filename: fileName,
			MimeType: mimeType,
			Content:  content,
		}, product, color, section)
	}

	state := b.getSession(chatID)
	if state == nil {
		parsed := b.flow.ParseCaption(msg.Caption)
		state = &sessionState{
			Product:   parsed.Product,
			Color:     parsed.Color,
			Section:   parsed.Section,
			FileID:    file.FileID,
			FileName:  fileName,
			FileMIME:  mimeType,
			FileBytes: content,
		}
		b.setSession(chatID, state)
	} else {
		state.FileID = file.FileID
		state.FileName = fileName
		state.FileMIME = mimeType
		state.FileBytes = content
	}

	return b.continueUploadFlow(chatID, state)
}

func (b *Bot) handleDocument(msg *tgbotapi.Message) error {
	chatID := msg.Chat.ID
	doc := msg.Document
	if doc == nil {
		return nil
	}
	if !isAllowedImageMIME(doc.MimeType) && !isAllowedImageExtension(doc.FileName) {
		return b.send(chatID, "⚠️ Неподдерживаемый формат файла.\nРазрешенные форматы: "+allowedFormatsText)
	}

	fileURL, err := b.api.GetFileDirectURL(doc.FileID)
	if err != nil {
		return b.send(chatID, "❌ Не удалось получить файл из Telegram.")
	}

	content, mimeType, err := downloadFile(fileURL)
	if err != nil {
		return b.send(chatID, "❌ Не удалось скачать изображение.")
	}
	if !isAllowedImageMIME(mimeType) && !isAllowedImageExtension(doc.FileName) {
		return b.send(chatID, "⚠️ Неподдерживаемый формат файла.\nРазрешенные форматы: "+allowedFormatsText)
	}

	fileName := buildFileName(doc.FileName, mimeType)
	if msg.MediaGroupID != "" {
		product, color, section := b.resolveUploadContext(chatID, msg.Caption)
		return b.enqueueAlbumItem(chatID, msg.MediaGroupID, albumItem{
			Filename: fileName,
			MimeType: mimeType,
			Content:  content,
		}, product, color, section)
	}

	state := b.getSession(chatID)
	if state == nil {
		parsed := b.flow.ParseCaption(msg.Caption)
		state = &sessionState{
			Product:   parsed.Product,
			Color:     parsed.Color,
			Section:   parsed.Section,
			FileID:    doc.FileID,
			FileName:  fileName,
			FileMIME:  mimeType,
			FileBytes: content,
		}
		b.setSession(chatID, state)
	} else {
		state.FileID = doc.FileID
		state.FileName = fileName
		state.FileMIME = mimeType
		state.FileBytes = content
	}

	return b.continueUploadFlow(chatID, state)
}

func (b *Bot) continueUploadFlow(chatID int64, state *sessionState, editMessageID ...int) error {
	if state.Product == "" {
		state.Awaiting = "product"
		return b.askProduct(chatID, editMessageID...)
	}
	if state.Color == "" {
		state.Awaiting = "color"
		return b.askColor(chatID, state.Product, editMessageID...)
	}
	if state.Section == "" {
		state.Awaiting = "section"
		return b.askSection(chatID, state.Product, state.Color, editMessageID...)
	}
	if len(state.FileBytes) == 0 {
		state.Awaiting = "photo"
		return b.sendWithKeyboard(chatID, "📸 Теперь отправьте фото в выбранную папку.\n"+b.pathHint(state.Product, state.Color, state.Section), "", nil, "", "section", 0, extractEditID(editMessageID...))
	}

	target, err := b.flow.UploadImage(service.UploadPayload{
		Product:  state.Product,
		Color:    state.Color,
		Section:  state.Section,
		Filename: state.FileName,
		MimeType: state.FileMIME,
		Content:  state.FileBytes,
	})
	if err != nil {
		return b.send(chatID, "❌ Ошибка загрузки в Яндекс.Диск:\n"+humanError(err))
	}

	state.FileID = ""
	state.FileName = ""
	state.FileMIME = ""
	state.FileBytes = nil
	state.Awaiting = "post_upload_choice"
	return b.sendWithKeyboard(chatID, "✅ Готово. Изображение сохранено:\n"+target+"\n\n📤 Загрузить еще в этот же раздел?", "", nil, "", "section", 0, extractEditID(editMessageID...),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Да", "post|same|yes"),
			tgbotapi.NewInlineKeyboardButtonData("🧭 Изменить путь", "post|change|path"),
		),
	)
}

func (b *Bot) handleCallback(cb *tgbotapi.CallbackQuery) error {
	chatID := cb.Message.Chat.ID
	msgID := cb.Message.MessageID
	data := cb.Data
	defer b.api.Request(tgbotapi.NewCallback(cb.ID, ""))

	parts := strings.Split(data, "|")
	if len(parts) < 3 {
		return nil
	}

	state := b.getSession(chatID)
	if state == nil {
		state = &sessionState{}
		b.setSession(chatID, state)
	}

	switch parts[0] {
	case "set":
		if len(parts) != 3 {
			return nil
		}
		field, value := parts[1], parts[2]
		switch field {
		case "product":
			state.Product = value
			state.Color = ""
			state.Section = ""
			state.PageColor = 0
			state.PageSection = 0
			if state.SearchField == "product" {
				state.SearchField = ""
				state.SearchQuery = ""
			}
		case "color":
			state.Color = value
			state.Section = ""
			state.PageSection = 0
			if state.SearchField == "color" {
				state.SearchField = ""
				state.SearchQuery = ""
			}
		case "section":
			state.Section = value
		default:
			return nil
		}
	case "add":
		if len(parts) != 3 {
			return nil
		}
		level := parts[1]
		if parts[2] != "new" {
			return nil
		}
		state.AddLevel = level
		state.Awaiting = "new_folder_name"
		return b.sendWithKeyboard(chatID, "📁 Введите название новой папки:", "", nil, "", "", 0, msgID,
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("↩️ Назад", fmt.Sprintf("back|%s|stay", level)),
			),
		)
	case "search":
		if len(parts) != 3 || parts[2] != "start" {
			return nil
		}
		switch parts[1] {
		case "product":
			state.Awaiting = "search_product_query"
			return b.send(chatID, "🔎 Введите часть названия товара для поиска:")
		case "color":
			if strings.TrimSpace(state.Product) == "" {
				state.Awaiting = "product"
				return b.send(chatID, "⚠️ Сначала выберите товар, затем выполните поиск по цветам.")
			}
			state.Awaiting = "search_color_query"
			return b.send(chatID, "🔎 Введите часть названия цвета для поиска:")
		default:
			return nil
		}
	case "post":
		if len(parts) != 3 {
			return nil
		}
		if parts[1] == "same" && parts[2] == "yes" {
			state.Awaiting = "photo"
			return b.send(chatID, "📸 Отправьте следующее изображение в этот же раздел.")
		}
		if parts[1] == "change" && parts[2] == "path" {
			state.Section = ""
			state.Color = ""
			state.PageColor = 0
			state.PageSection = 0
			state.Awaiting = "product"
			return b.askProduct(chatID)
		}
		return nil
	case "pick":
		if len(parts) != 3 || parts[2] != "set" || state == nil {
			return nil
		}
		payload := state.ValueMap[parts[1]]
		if payload == "" {
			return nil
		}
		chunks := strings.SplitN(payload, "|", 2)
		if len(chunks) != 2 {
			return nil
		}
		field, value := chunks[0], chunks[1]
		switch field {
		case "product":
			state.Product = value
			state.Color = ""
			state.Section = ""
			state.PageColor = 0
			state.PageSection = 0
			if state.SearchField == "product" {
				state.SearchField = ""
				state.SearchQuery = ""
			}
		case "color":
			state.Color = value
			state.Section = ""
			state.PageSection = 0
			if state.SearchField == "color" {
				state.SearchField = ""
				state.SearchQuery = ""
			}
		case "section":
			state.Section = value
		default:
			return nil
		}
	case "nav":
		if len(parts) != 4 {
			return nil
		}
		field := parts[1]
		dir := parts[2]
		page, err := parsePositiveInt(parts[3])
		if err != nil {
			return nil
		}
		switch field {
		case "product":
			state.PageProduct = stepPage(page, dir)
			return b.askProduct(chatID, msgID)
		case "color":
			state.PageColor = stepPage(page, dir)
			return b.askColor(chatID, state.Product, msgID)
		case "section":
			state.PageSection = stepPage(page, dir)
			return b.askSection(chatID, state.Product, state.Color, msgID)
		default:
			return nil
		}
	case "back":
		if len(parts) != 3 {
			return nil
		}
		step := parts[1]
		mode := parts[2]
		if mode == "stay" {
			switch step {
			case "product":
				state.Awaiting = "product"
				state.AddLevel = ""
				return b.askProduct(chatID, msgID)
			case "color":
				state.Awaiting = "color"
				state.AddLevel = ""
				return b.askColor(chatID, state.Product, msgID)
			case "section":
				state.Awaiting = "section"
				state.AddLevel = ""
				return b.askSection(chatID, state.Product, state.Color, msgID)
			default:
				return nil
			}
		}
		switch step {
		case "product":
			b.clearSession(chatID)
			return b.sendOrEditText(chatID, b.welcomeText(), msgID)
		case "color":
			state.Color = ""
			state.Section = ""
			state.PageColor = 0
			state.PageSection = 0
			return b.askProduct(chatID, msgID)
		case "section":
			state.Section = ""
			state.PageSection = 0
			return b.askColor(chatID, state.Product, msgID)
		default:
			return nil
		}
	case "noop":
		return nil
	default:
		return nil
	}
	return b.continueUploadFlow(chatID, state, msgID)
}

func (b *Bot) askProduct(chatID int64, editMessageID ...int) error {
	options, err := b.flow.ListProducts()
	if err != nil {
		return b.send(chatID, "❌ Не удалось получить список товаров:\n"+humanError(err))
	}
	if len(options) == 0 {
		return b.sendWithKeyboard(chatID, "📦 Список товаров пуст.\n"+b.pathHint("", "", "")+"\nСоздайте первую папку товара кнопкой ниже.", "product", options, service.LevelProduct, "product", 0, extractEditID(editMessageID...))
	}
	state := b.getSession(chatID)
	page := 0
	if state != nil {
		page = state.PageProduct
		options = filterOptions(options, state.SearchField == "product", state.SearchQuery)
	}
	text := "📦 Выберите товар:\n" + b.pathHint("", "", "") + "\n✍️ Можно ввести название текстом или полный путь."
	if state != nil && state.SearchField == "product" && strings.TrimSpace(state.SearchQuery) != "" {
		text += "\n🔎 Поиск: " + state.SearchQuery
	}
	return b.sendWithKeyboard(chatID, text, "product", options, service.LevelProduct, "product", page, extractEditID(editMessageID...))
}

func (b *Bot) askColor(chatID int64, product string, editMessageID ...int) error {
	options, err := b.flow.ListColors(product)
	if err != nil {
		return b.send(chatID, "❌ Не удалось получить список цветов:\n"+humanError(err))
	}
	if len(options) == 0 {
		return b.sendWithKeyboard(chatID, "🎨 Для выбранного товара пока нет папок цветов.\n"+b.pathHint(product, "", "")+"\nСоздайте первую кнопкой ниже.", "color", options, service.LevelColor, "color", 0, extractEditID(editMessageID...))
	}
	state := b.getSession(chatID)
	page := 0
	if state != nil {
		page = state.PageColor
		options = filterOptions(options, state.SearchField == "color", state.SearchQuery)
	}
	text := "🎨 Выберите цвет:\n" + b.pathHint(product, "", "") + "\n✍️ Можно ввести название текстом или полный путь."
	if state != nil && state.SearchField == "color" && strings.TrimSpace(state.SearchQuery) != "" {
		text += "\n🔎 Поиск: " + state.SearchQuery
	}
	return b.sendWithKeyboard(chatID, text, "color", options, service.LevelColor, "color", page, extractEditID(editMessageID...))
}

func (b *Bot) askSection(chatID int64, product, color string, editMessageID ...int) error {
	options, err := b.flow.ListSections(product, color)
	if err != nil {
		return b.send(chatID, "❌ Не удалось получить список разделов:\n"+humanError(err))
	}
	if len(options) == 0 {
		return b.sendWithKeyboard(chatID, "🗂️ В этой папке цвета пока нет разделов.\n"+b.pathHint(product, color, "")+"\nСоздайте нужный раздел кнопкой ниже.", "section", options, service.LevelSection, "section", 0, extractEditID(editMessageID...))
	}
	state := b.getSession(chatID)
	page := 0
	if state != nil {
		page = state.PageSection
	}
	return b.sendWithKeyboard(chatID, "🗂️ Выберите раздел:\n"+b.pathHint(product, color, "")+"\n✍️ Можно ввести название текстом или полный путь.", "section", options, service.LevelSection, "section", page, extractEditID(editMessageID...))
}

func (b *Bot) send(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	_, err := b.api.Send(msg)
	return err
}

func (b *Bot) sendWithKeyboard(chatID int64, text, field string, values []string, addLevel string, backStep string, page int, editMessageID int, extraRows ...[]tgbotapi.InlineKeyboardButton) error {
	state := b.getSession(chatID)
	if state != nil && state.ValueMap == nil {
		state.ValueMap = map[string]string{}
	}
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(values))
	visible, hasPrev, hasNext, currentPage := paginate(values, page, listPageSize)
	totalPages := 0
	if len(values) > 0 {
		totalPages = (len(values)-1)/listPageSize + 1
	}
	pair := make([]tgbotapi.InlineKeyboardButton, 0, 2)
	for _, v := range visible {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		data := b.callbackForValue(state, field, v)
		pair = append(pair, tgbotapi.NewInlineKeyboardButtonData(trimButtonLabel(v), data))
		if len(pair) == 2 {
			rows = append(rows, pair)
			pair = make([]tgbotapi.InlineKeyboardButton, 0, 2)
		}
	}
	if len(pair) > 0 {
		rows = append(rows, pair)
	}
	if hasPrev || hasNext {
		navRow := make([]tgbotapi.InlineKeyboardButton, 0, 3)
		if hasPrev {
			navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", fmt.Sprintf("nav|%s|prev|%d", field, currentPage)))
		}
		navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("Стр. %d/%d", currentPage+1, maxInt(totalPages, 1)), "noop|page|x"))
		if hasNext {
			navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("➡️ Далее", fmt.Sprintf("nav|%s|next|%d", field, currentPage)))
		}
		rows = append(rows, navRow)
	}
	controls := make([]tgbotapi.InlineKeyboardButton, 0, 2)
	if backStep != "" {
		controls = append(controls, tgbotapi.NewInlineKeyboardButtonData("↩️ Назад", fmt.Sprintf("back|%s|go", backStep)))
	}
	if addLevel != "" {
		controls = append(controls, tgbotapi.NewInlineKeyboardButtonData("➕ Добавить папку", fmt.Sprintf("add|%s|new", addLevel)))
	}
	if len(controls) > 0 {
		rows = append(rows, controls)
	}
	if field == "product" {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔎 Поиск товара", "search|product|start"),
		))
	}
	if field == "color" {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔎 Поиск цвета", "search|color|start"),
		))
	}
	rows = append(rows, extraRows...)
	if len(rows) == 0 {
		return b.sendOrEditText(chatID, text, editMessageID)
	}

	markup := tgbotapi.NewInlineKeyboardMarkup(rows...)
	if editMessageID > 0 {
		edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, editMessageID, text, markup)
		_, err := b.api.Send(edit)
		return err
	}
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = markup
	_, err := b.api.Send(msg)
	return err
}

func (b *Bot) sendOrEditText(chatID int64, text string, editMessageID int) error {
	if editMessageID > 0 {
		edit := tgbotapi.NewEditMessageText(chatID, editMessageID, text)
		_, err := b.api.Send(edit)
		return err
	}
	return b.send(chatID, text)
}

func (b *Bot) resolveUploadContext(chatID int64, caption string) (string, string, string) {
	state := b.getSession(chatID)
	var product, color, section string
	if state != nil {
		product = strings.TrimSpace(state.Product)
		color = strings.TrimSpace(state.Color)
		section = strings.TrimSpace(state.Section)
	}
	parsed := b.flow.ParseCaption(caption)
	if product == "" {
		product = strings.TrimSpace(parsed.Product)
	}
	if color == "" {
		color = strings.TrimSpace(parsed.Color)
	}
	if section == "" {
		section = strings.TrimSpace(parsed.Section)
	}
	return product, color, section
}

func (b *Bot) enqueueAlbumItem(chatID int64, groupID string, item albumItem, product, color, section string) error {
	key := fmt.Sprintf("%d:%s", chatID, groupID)

	b.albumsMu.Lock()
	buf := b.albums[key]
	if buf == nil {
		buf = &albumBuffer{
			ChatID:       chatID,
			MediaGroupID: groupID,
		}
		b.albums[key] = buf
	}
	if buf.Product == "" && product != "" {
		buf.Product = product
	}
	if buf.Color == "" && color != "" {
		buf.Color = color
	}
	if buf.Section == "" && section != "" {
		buf.Section = section
	}
	buf.Items = append(buf.Items, item)
	if buf.Timer != nil {
		buf.Timer.Stop()
	}
	buf.Timer = time.AfterFunc(albumFlushDelay, func() {
		b.flushAlbum(key)
	})
	needNotify := !buf.Notified
	if needNotify {
		buf.Notified = true
	}
	b.albumsMu.Unlock()

	if needNotify {
		return b.send(chatID, "📦 Получено несколько файлов. Загружаю одним пакетом...")
	}
	return nil
}

func (b *Bot) flushAlbum(key string) {
	b.albumsMu.Lock()
	buf := b.albums[key]
	if buf == nil {
		b.albumsMu.Unlock()
		return
	}
	delete(b.albums, key)
	b.albumsMu.Unlock()

	product := strings.TrimSpace(buf.Product)
	color := strings.TrimSpace(buf.Color)
	section := strings.TrimSpace(buf.Section)
	if product == "" || color == "" || section == "" {
		_ = b.send(buf.ChatID, "⚠️ Для пакетной загрузки сначала выберите путь (товар/цвет/раздел), затем отправьте файлы.")
		return
	}

	success := 0
	fail := 0
	for _, it := range buf.Items {
		_, err := b.flow.UploadImage(service.UploadPayload{
			Product:  product,
			Color:    color,
			Section:  section,
			Filename: it.Filename,
			MimeType: it.MimeType,
			Content:  it.Content,
		})
		if err != nil {
			fail++
			continue
		}
		success++
	}

	result := fmt.Sprintf("✅ Пакетная загрузка завершена.\nУспешно: %d\nС ошибками: %d", success, fail)
	_ = b.sendWithKeyboard(buf.ChatID, result+"\n\n📤 Загрузить еще в этот же раздел?", "", nil, "", "section", 0, 0,
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Да", "post|same|yes"),
			tgbotapi.NewInlineKeyboardButtonData("🧭 Изменить путь", "post|change|path"),
		),
	)
}

func (b *Bot) refreshLevel(chatID int64, state *sessionState) error {
	switch state.AddLevel {
	case service.LevelProduct:
		state.Awaiting = "product"
		state.PageProduct = 0
		return b.askProduct(chatID)
	case service.LevelColor:
		state.Awaiting = "color"
		state.PageColor = 0
		return b.askColor(chatID, state.Product)
	case service.LevelSection:
		state.Awaiting = "section"
		state.PageSection = 0
		return b.askSection(chatID, state.Product, state.Color)
	default:
		state.Awaiting = ""
		return b.send(chatID, "Папка создана.")
	}
}

func (b *Bot) sendWelcome(chatID int64) error {
	return b.send(chatID, b.welcomeText())
}

func (b *Bot) handlePathTextInput(chatID int64, state *sessionState, input string) (bool, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return false, nil
	}
	if ok, err := b.tryApplyFullPathInput(chatID, state, value); ok || err != nil {
		return true, err
	}

	switch state.Awaiting {
	case "product":
		options, err := b.flow.ListProducts()
		if err != nil {
			return true, b.send(chatID, "❌ Не удалось получить список товаров:\n"+humanError(err))
		}
		resolved := resolveTypedOption(options, value)
		if resolved == "" {
			return true, b.send(chatID, "⚠️ Такой папки товара нет. Выберите из списка или введите точное название.")
		}
		state.Product = resolved
		state.Color = ""
		state.Section = ""
		state.PageColor = 0
		state.PageSection = 0
		return true, b.continueUploadFlow(chatID, state)
	case "color":
		if strings.TrimSpace(state.Product) == "" {
			return true, b.askProduct(chatID)
		}
		options, err := b.flow.ListColors(state.Product)
		if err != nil {
			return true, b.send(chatID, "❌ Не удалось получить список цветов:\n"+humanError(err))
		}
		resolved := resolveTypedOption(options, value)
		if resolved == "" {
			return true, b.send(chatID, "⚠️ Такой папки цвета нет. Выберите из списка или введите точное название.")
		}
		state.Color = resolved
		state.Section = ""
		state.PageSection = 0
		return true, b.continueUploadFlow(chatID, state)
	case "section":
		if strings.TrimSpace(state.Product) == "" || strings.TrimSpace(state.Color) == "" {
			return true, b.askProduct(chatID)
		}
		options, err := b.flow.ListSections(state.Product, state.Color)
		if err != nil {
			return true, b.send(chatID, "❌ Не удалось получить список разделов:\n"+humanError(err))
		}
		resolved := resolveTypedOption(options, value)
		if resolved == "" {
			return true, b.send(chatID, "⚠️ Такой папки раздела нет. Выберите из списка или введите точное название.")
		}
		state.Section = resolved
		return true, b.continueUploadFlow(chatID, state)
	default:
		return false, nil
	}
}

func (b *Bot) tryApplyFullPathInput(chatID int64, state *sessionState, input string) (bool, error) {
	productRaw, colorRaw, sectionRaw, ok := parseFullPathInput(input)
	if !ok {
		return false, nil
	}

	products, err := b.flow.ListProducts()
	if err != nil {
		return true, b.send(chatID, "❌ Не удалось получить список товаров:\n"+humanError(err))
	}
	product := resolveTypedOption(products, productRaw)
	if product == "" {
		return true, b.send(chatID, "⚠️ Товар из полного пути не найден. Проверьте ввод.")
	}

	colors, err := b.flow.ListColors(product)
	if err != nil {
		return true, b.send(chatID, "❌ Не удалось получить список цветов:\n"+humanError(err))
	}
	color := resolveTypedOption(colors, colorRaw)
	if color == "" {
		return true, b.send(chatID, "⚠️ Цвет из полного пути не найден для выбранного товара.")
	}

	sections, err := b.flow.ListSections(product, color)
	if err != nil {
		return true, b.send(chatID, "❌ Не удалось получить список разделов:\n"+humanError(err))
	}
	section := resolveTypedOption(sections, sectionRaw)
	if section == "" {
		return true, b.send(chatID, "⚠️ Раздел из полного пути не найден для выбранного цвета.")
	}

	state.Product = product
	state.Color = color
	state.Section = section
	state.PageColor = 0
	state.PageSection = 0

	return true, b.continueUploadFlow(chatID, state)
}

func (b *Bot) welcomeText() string {
	return "👋 Добро пожаловать в PicFolderBot.\n\n" +
		"Что умею:\n" +
		"• Помогаю выбрать товар → цвет → раздел\n" +
		"• Загружаю изображение в выбранную папку\n" +
		"• Создаю папки кнопкой ➕ на нужном уровне\n\n" +
		"🚀 Нажмите /upload, чтобы начать.\n" +
		"🔎 Для больших каталогов используйте /search.\n" +
		"🖼️ Форматы: " + allowedFormatsText
}

func (b *Bot) sendSearchMenu(chatID int64, state *sessionState) error {
	rows := [][]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("🔎 Поиск товара", "search|product|start")),
	}
	if strings.TrimSpace(state.Product) != "" {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("🔎 Поиск цвета в выбранном товаре", "search|color|start")))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("↩️ Назад", "back|product|go")))

	msg := tgbotapi.NewMessage(chatID, "🔎 Выберите режим поиска:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	_, err := b.api.Send(msg)
	return err
}

func extractEditID(values ...int) int {
	if len(values) == 0 {
		return 0
	}
	return values[0]
}

func (b *Bot) getSession(chatID int64) *sessionState {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.sessions[chatID]
}

func (b *Bot) setSession(chatID int64, state *sessionState) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sessions[chatID] = state
}

func (b *Bot) clearSession(chatID int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.sessions, chatID)
}

func downloadFile(url string) ([]byte, string, error) {
	client := &http.Client{Timeout: telegramDownloadTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", errors.New("telegram file endpoint error")
	}

	content, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(content) > maxImageBytes {
		return nil, "", errors.New("file is too large")
	}
	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "image/jpeg"
	}
	return content, mimeType, nil
}

func humanError(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	text = strings.TrimPrefix(text, "Get ")
	text = strings.TrimSpace(text)
	if text == "" {
		return "неизвестная ошибка"
	}
	return text
}

func inferExtension(mimeType string) string {
	return extensionByMIME(mimeType)
}

func paginate(values []string, page int, pageSize int) ([]string, bool, bool, int) {
	if pageSize <= 0 {
		pageSize = listPageSize
	}
	total := len(values)
	if total == 0 {
		return nil, false, false, 0
	}
	maxPage := (total - 1) / pageSize
	if page < 0 {
		page = 0
	}
	if page > maxPage {
		page = maxPage
	}

	start := page * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}
	return values[start:end], page > 0, page < maxPage, page
}

func stepPage(page int, dir string) int {
	switch dir {
	case "prev":
		if page > 0 {
			return page - 1
		}
		return 0
	case "next":
		return page + 1
	default:
		return page
	}
}

func parsePositiveInt(v string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		return 0, errors.New("invalid int")
	}
	return n, nil
}

func (b *Bot) pathHint(product, color, section string) string {
	root := strings.TrimSpace(b.rootName)
	if root == "" {
		root = "disk"
	}
	parts := []string{root}
	if p := strings.TrimSpace(product); p != "" {
		parts = append(parts, p)
	}
	if c := strings.TrimSpace(color); c != "" {
		parts = append(parts, c)
	}
	if s := strings.TrimSpace(section); s != "" {
		parts = append(parts, s)
	}
	return "📁 Путь: " + strings.Join(parts, " / ")
}

func filterOptions(values []string, enabled bool, query string) []string {
	if !enabled {
		return values
	}
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return values
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if strings.Contains(strings.ToLower(v), query) {
			out = append(out, v)
		}
	}
	return out
}

func resolveTypedOption(options []string, input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	for _, opt := range options {
		if strings.EqualFold(strings.TrimSpace(opt), input) {
			return opt
		}
	}
	return ""
}

func parseFullPathInput(input string) (product, color, section string, ok bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", "", false
	}

	// Preferred explicit separators: comma/semicolon/slash/newline.
	sepFields := splitBySeparators(input)
	if len(sepFields) >= 3 {
		return sepFields[0], sepFields[1], strings.Join(sepFields[2:], " "), true
	}

	// Fallback: whitespace-separated path "product color section words...".
	ws := strings.Fields(input)
	if len(ws) >= 3 {
		return ws[0], ws[1], strings.Join(ws[2:], " "), true
	}
	return "", "", "", false
}

func splitBySeparators(input string) []string {
	replacer := strings.NewReplacer("\n", ",", ";", ",", "/", ",")
	input = replacer.Replace(input)
	raw := strings.Split(input, ",")
	out := make([]string, 0, len(raw))
	for _, part := range raw {
		p := strings.TrimSpace(part)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func trimButtonLabel(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return v
	}
	runes := []rune(v)
	if len(runes) <= maxButtonLabelRunes {
		return v
	}
	return string(runes[:maxButtonLabelRunes-1]) + "…"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (b *Bot) callbackForValue(state *sessionState, field string, value string) string {
	data := fmt.Sprintf("set|%s|%s", field, value)
	if len(data) <= 64 {
		return data
	}
	if state == nil {
		return "noop|long|x"
	}
	if state.ValueMap == nil {
		state.ValueMap = map[string]string{}
	}
	token := fmt.Sprintf("%x", crc32.ChecksumIEEE([]byte(field+"|"+value)))
	state.ValueMap[token] = field + "|" + value
	return "pick|" + token + "|set"
}

func buildFileName(base string, mimeType string) string {
	base = strings.TrimSpace(filepath.Base(base))
	base = strings.ReplaceAll(base, "/", "_")
	base = strings.ReplaceAll(base, "\\", "_")
	if base == "" {
		base = fmt.Sprintf("img_%d", time.Now().Unix())
	}
	ext := strings.ToLower(filepath.Ext(base))
	if ext == "" || !isAllowedImageExtension(base) {
		base = strings.TrimSuffix(base, filepath.Ext(base))
		base += inferExtension(mimeType)
	}
	return base
}
