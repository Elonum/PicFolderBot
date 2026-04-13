package telegram

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"PicFolderBot/internal/service"
)

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
	return b.sendWithRetry(tgbotapi.NewMessage(chatID, text))
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
		if v = strings.TrimSpace(v); v == "" {
			continue
		}
		pair = append(pair, tgbotapi.NewInlineKeyboardButtonData(trimButtonLabel(v), b.callbackForValue(state, field, v)))
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
	ctrl := make([]tgbotapi.InlineKeyboardButton, 0, 2)
	if backStep != "" {
		ctrl = append(ctrl, tgbotapi.NewInlineKeyboardButtonData("↩️ Назад", fmt.Sprintf("back|%s|go", backStep)))
	}
	if addLevel != "" {
		ctrl = append(ctrl, tgbotapi.NewInlineKeyboardButtonData("➕ Добавить папку", fmt.Sprintf("add|%s|new", addLevel)))
	}
	if len(ctrl) > 0 {
		rows = append(rows, ctrl)
	}
	if field == "product" {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("🔎 Поиск товара", "search|product|start")))
	}
	if field == "color" {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("🔎 Поиск цвета", "search|color|start")))
	}
	rows = append(rows, extraRows...)
	if len(rows) == 0 {
		return b.sendOrEditText(chatID, text, editMessageID)
	}

	markup := tgbotapi.NewInlineKeyboardMarkup(rows...)
	if editMessageID > 0 {
		return b.sendWithRetry(tgbotapi.NewEditMessageTextAndMarkup(chatID, editMessageID, text, markup))
	}
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = markup
	return b.sendWithRetry(msg)
}

func (b *Bot) sendOrEditText(chatID int64, text string, editMessageID int) error {
	if editMessageID > 0 {
		return b.sendWithRetry(tgbotapi.NewEditMessageText(chatID, editMessageID, text))
	}
	return b.send(chatID, text)
}

func (b *Bot) refreshLevel(chatID int64, state *sessionState) error {
	switch state.AddLevel {
	case service.LevelProduct:
		state.Awaiting, state.PageProduct = "product", 0
		return b.askProduct(chatID)
	case service.LevelColor:
		state.Awaiting, state.PageColor = "color", 0
		return b.askColor(chatID, state.Product)
	case service.LevelSection:
		state.Awaiting, state.PageSection = "section", 0
		return b.askSection(chatID, state.Product, state.Color)
	default:
		state.Awaiting = ""
		return b.send(chatID, "Папка создана.")
	}
}

func (b *Bot) sendWelcome(chatID int64) error {
	return b.send(chatID, b.welcomeText())
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
	return b.sendWithRetry(msg)
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
