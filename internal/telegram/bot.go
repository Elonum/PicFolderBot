package telegram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
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
)

type flowAPI interface {
	ParseCaption(caption string) parser.ParsedInput
	ListProducts() ([]string, error)
	ListColors(product string) ([]string, error)
	ListSections(product, color string) ([]string, error)
	UploadImage(payload service.UploadPayload) (string, error)
	CreateFolderAtLevel(level, product, color, section, newFolder string) (string, error)
}

type sessionMode string

const (
	modeUpload sessionMode = "upload"
)

type sessionState struct {
	Mode       sessionMode
	Product    string
	Color      string
	Section    string
	AddLevel   string
	FileID     string
	FileName   string
	FileMIME   string
	FileBytes  []byte
	Awaiting   string
	LastAction time.Time
}

type Bot struct {
	api      *tgbotapi.BotAPI
	flow     flowAPI
	sessions map[int64]*sessionState
	mu       sync.RWMutex
}

func NewBot(token string, flow flowAPI) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(strings.TrimSpace(token))
	if err != nil {
		return nil, err
	}
	return &Bot{
		api:      api,
		flow:     flow,
		sessions: make(map[int64]*sessionState),
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
			return b.send(chatID, "Отправьте фото с подписью (товар/цвет/папка) или нажмите /upload для пошагового сценария.\nДля создания папок используйте кнопку +Добавить папку в списках.\nРазрешенные форматы: "+allowedFormatsText)
		case "upload":
			b.setSession(chatID, &sessionState{Mode: modeUpload, Awaiting: "product", LastAction: time.Now()})
			return b.askProduct(chatID)
		case "createfolder":
			b.setSession(chatID, &sessionState{Mode: modeUpload, Awaiting: "product", LastAction: time.Now()})
			if err := b.send(chatID, "Создание папки доступно кнопкой +Добавить папку на каждом уровне."); err != nil {
				return err
			}
			return b.askProduct(chatID)
		default:
			return b.send(chatID, "Неизвестная команда. Используйте /help")
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
		state.LastAction = time.Now()
		if newFolder == "" {
			return b.send(chatID, "Введите непустое имя новой папки.")
		}
		level := state.AddLevel
		target, err := b.flow.CreateFolderAtLevel(level, state.Product, state.Color, state.Section, newFolder)
		if err != nil {
			return b.send(chatID, "Не удалось создать папку: "+humanError(err))
		}
		state.AddLevel = level
		if err = b.send(chatID, "Папка создана: "+target); err != nil {
			return err
		}
		err = b.refreshLevel(chatID, state)
		state.AddLevel = ""
		return err
	}

	state = b.getSession(chatID)
	if state != nil && state.Mode == modeUpload && state.Awaiting == "photo" {
		return b.send(chatID, "Ожидаю изображение. Разрешенные форматы: "+allowedFormatsText)
	}

	return b.send(chatID, "Отправьте фото с подписью или используйте /upload для пошаговой загрузки.")
}

func (b *Bot) handlePhoto(msg *tgbotapi.Message) error {
	chatID := msg.Chat.ID
	file := msg.Photo[len(msg.Photo)-1]

	fileURL, err := b.api.GetFileDirectURL(file.FileID)
	if err != nil {
		return b.send(chatID, "Не удалось получить файл из Telegram.")
	}

	content, mimeType, err := downloadFile(fileURL)
	if err != nil {
		return b.send(chatID, "Не удалось скачать изображение.")
	}
	if !isAllowedImageMIME(mimeType) {
		return b.send(chatID, "Неподдерживаемый формат изображения. Разрешенные форматы: "+allowedFormatsText)
	}

	fileName := buildFileName(fmt.Sprintf("img_%d", time.Now().Unix()), mimeType)

	state := b.getSession(chatID)
	if state == nil {
		parsed := b.flow.ParseCaption(msg.Caption)
		state = &sessionState{
			Mode:       modeUpload,
			Product:    parsed.Product,
			Color:      parsed.Color,
			Section:    parsed.Section,
			FileID:     file.FileID,
			FileName:   fileName,
			FileMIME:   mimeType,
			FileBytes:  content,
			LastAction: time.Now(),
		}
		b.setSession(chatID, state)
	} else {
		state.FileID = file.FileID
		state.FileName = fileName
		state.FileMIME = mimeType
		state.FileBytes = content
		state.LastAction = time.Now()
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
		return b.send(chatID, "Неподдерживаемый формат файла. Разрешенные форматы: "+allowedFormatsText)
	}

	fileURL, err := b.api.GetFileDirectURL(doc.FileID)
	if err != nil {
		return b.send(chatID, "Не удалось получить файл из Telegram.")
	}

	content, mimeType, err := downloadFile(fileURL)
	if err != nil {
		return b.send(chatID, "Не удалось скачать изображение.")
	}
	if !isAllowedImageMIME(mimeType) && !isAllowedImageExtension(doc.FileName) {
		return b.send(chatID, "Неподдерживаемый формат файла. Разрешенные форматы: "+allowedFormatsText)
	}

	fileName := buildFileName(doc.FileName, mimeType)

	state := b.getSession(chatID)
	if state == nil {
		parsed := b.flow.ParseCaption(msg.Caption)
		state = &sessionState{
			Mode:       modeUpload,
			Product:    parsed.Product,
			Color:      parsed.Color,
			Section:    parsed.Section,
			FileID:     doc.FileID,
			FileName:   fileName,
			FileMIME:   mimeType,
			FileBytes:  content,
			LastAction: time.Now(),
		}
		b.setSession(chatID, state)
	} else {
		state.FileID = doc.FileID
		state.FileName = fileName
		state.FileMIME = mimeType
		state.FileBytes = content
		state.LastAction = time.Now()
	}

	return b.continueUploadFlow(chatID, state)
}

func (b *Bot) continueUploadFlow(chatID int64, state *sessionState) error {
	if state.Product == "" {
		state.Awaiting = "product"
		return b.askProduct(chatID)
	}
	if state.Color == "" {
		state.Awaiting = "color"
		return b.askColor(chatID, state.Product)
	}
	if state.Section == "" {
		state.Awaiting = "section"
		return b.askSection(chatID, state.Product, state.Color)
	}
	if len(state.FileBytes) == 0 {
		state.Awaiting = "photo"
		return b.send(chatID, "Теперь отправьте фото.")
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
		return b.send(chatID, "Ошибка загрузки в Яндекс.Диск: "+humanError(err))
	}

	b.clearSession(chatID)
	return b.send(chatID, "Готово. Изображение сохранено: "+target)
}

func (b *Bot) handleCallback(cb *tgbotapi.CallbackQuery) error {
	chatID := cb.Message.Chat.ID
	data := cb.Data
	defer b.api.Request(tgbotapi.NewCallback(cb.ID, "OK"))

	parts := strings.SplitN(data, "|", 3)
	if len(parts) != 3 {
		return nil
	}

	state := b.getSession(chatID)
	if state == nil {
		state = &sessionState{Mode: modeUpload, LastAction: time.Now()}
		b.setSession(chatID, state)
	}

	switch parts[0] {
	case "set":
		field := parts[1]
		value := parts[2]
		switch field {
		case "product":
			state.Product = value
		case "color":
			state.Color = value
		case "section":
			state.Section = value
		default:
			return nil
		}
	case "add":
		level := parts[1]
		if parts[2] != "new" {
			return nil
		}
		state.AddLevel = level
		state.Awaiting = "new_folder_name"
		state.LastAction = time.Now()
		return b.send(chatID, "Введите название новой папки:")
	default:
		return nil
	}
	state.LastAction = time.Now()

	return b.continueUploadFlow(chatID, state)
}

func (b *Bot) askProduct(chatID int64) error {
	options, err := b.flow.ListProducts()
	if err != nil {
		return b.send(chatID, "Не удалось получить список товаров: "+humanError(err))
	}
	if len(options) == 0 {
		return b.send(chatID, "Список товаров пуст. Проверьте корневую папку на Яндекс.Диске.")
	}
	return b.sendWithKeyboard(chatID, "Выберите товар:", "product", options, service.LevelProduct)
}

func (b *Bot) askColor(chatID int64, product string) error {
	options, err := b.flow.ListColors(product)
	if err != nil {
		return b.send(chatID, "Не удалось получить список цветов: "+humanError(err))
	}
	if len(options) == 0 {
		return b.send(chatID, "Для выбранного товара нет папок цветов.")
	}
	return b.sendWithKeyboard(chatID, "Выберите цвет:", "color", options, service.LevelColor)
}

func (b *Bot) askSection(chatID int64, product, color string) error {
	options, err := b.flow.ListSections(product, color)
	if err != nil {
		return b.send(chatID, "Не удалось получить список разделов: "+humanError(err))
	}
	if len(options) == 0 {
		return b.sendWithKeyboard(chatID, "В этой папке цвета пока нет разделов. Нажмите +Добавить папку, чтобы создать нужный раздел.", "section", options, service.LevelSection)
	}
	return b.sendWithKeyboard(chatID, "Выберите папку раздела:", "section", options, service.LevelSection)
}

func (b *Bot) send(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	_, err := b.api.Send(msg)
	return err
}

func (b *Bot) sendWithKeyboard(chatID int64, text, field string, values []string, addLevel string) error {
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		data := fmt.Sprintf("set|%s|%s", field, v)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(v, data)))
	}
	if addLevel != "" {
		addBtn := tgbotapi.NewInlineKeyboardButtonData("+Добавить папку", fmt.Sprintf("add|%s|new", addLevel))
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(addBtn))
	}

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	_, err := b.api.Send(msg)
	return err
}

func (b *Bot) refreshLevel(chatID int64, state *sessionState) error {
	switch state.AddLevel {
	case service.LevelProduct:
		state.Awaiting = "product"
		return b.askProduct(chatID)
	case service.LevelColor:
		state.Awaiting = "color"
		return b.askColor(chatID, state.Product)
	case service.LevelSection:
		state.Awaiting = "section"
		return b.askSection(chatID, state.Product, state.Color)
	default:
		state.Awaiting = ""
		return b.send(chatID, "Папка создана.")
	}
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
