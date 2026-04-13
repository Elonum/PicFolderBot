package telegram

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) handleCallback(cb *tgbotapi.CallbackQuery) error {
	chatID, msgID := cb.Message.Chat.ID, cb.Message.MessageID
	defer b.api.Request(tgbotapi.NewCallback(cb.ID, ""))

	parts := strings.Split(cb.Data, "|")
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
		b.applySetSelection(state, parts[1], parts[2])
	case "pick":
		if len(parts) != 3 || parts[2] != "set" {
			return nil
		}
		payload := state.ValueMap[parts[1]]
		chunks := strings.SplitN(payload, "|", 2)
		if len(chunks) != 2 {
			return nil
		}
		b.applySetSelection(state, chunks[0], chunks[1])
	case "add":
		if len(parts) != 3 || parts[2] != "new" {
			return nil
		}
		state.AddLevel, state.Awaiting = parts[1], "new_folder_name"
		return b.sendWithKeyboard(chatID, "📁 Введите название новой папки:", "", nil, "", "", 0, msgID,
			tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("↩️ Назад", fmt.Sprintf("back|%s|stay", parts[1]))),
		)
	case "search":
		if len(parts) != 3 || parts[2] != "start" {
			return nil
		}
		if parts[1] == "product" {
			state.Awaiting = "search_product_query"
			return b.send(chatID, "🔎 Введите часть названия товара для поиска:")
		}
		if parts[1] == "color" {
			if strings.TrimSpace(state.Product) == "" {
				state.Awaiting = "product"
				return b.send(chatID, "⚠️ Сначала выберите товар, затем выполните поиск по цветам.")
			}
			state.Awaiting = "search_color_query"
			return b.send(chatID, "🔎 Введите часть названия цвета для поиска:")
		}
		return nil
	case "post":
		if len(parts) != 3 {
			return nil
		}
		if parts[1] == "same" && parts[2] == "yes" {
			state.Awaiting = "photo"
			return b.send(chatID, "📸 Отправьте следующее изображение в этот же раздел.")
		}
		if parts[1] == "change" && parts[2] == "path" {
			state.Section, state.Color = "", ""
			state.PageColor, state.PageSection = 0, 0
			state.Awaiting = "product"
			return b.askProduct(chatID)
		}
		return nil
	case "nav":
		if len(parts) != 4 {
			return nil
		}
		page, err := parsePositiveInt(parts[3])
		if err != nil {
			return nil
		}
		switch parts[1] {
		case "product":
			state.PageProduct = stepPage(page, parts[2])
			return b.askProduct(chatID, msgID)
		case "color":
			state.PageColor = stepPage(page, parts[2])
			return b.askColor(chatID, state.Product, msgID)
		case "section":
			state.PageSection = stepPage(page, parts[2])
			return b.askSection(chatID, state.Product, state.Color, msgID)
		default:
			return nil
		}
	case "back":
		if len(parts) != 3 {
			return nil
		}
		return b.handleBackAction(chatID, msgID, state, parts[1], parts[2])
	case "noop":
		return nil
	default:
		return nil
	}
	return b.continueUploadFlow(chatID, state, msgID)
}

func (b *Bot) applySetSelection(state *sessionState, field, value string) {
	switch field {
	case "product":
		state.Product, state.Color, state.Section = value, "", ""
		state.PageColor, state.PageSection = 0, 0
		if state.SearchField == "product" {
			state.SearchField, state.SearchQuery = "", ""
		}
	case "color":
		state.Color, state.Section = value, ""
		state.PageSection = 0
		if state.SearchField == "color" {
			state.SearchField, state.SearchQuery = "", ""
		}
	case "section":
		state.Section = value
	}
}

func (b *Bot) handleBackAction(chatID int64, msgID int, state *sessionState, step, mode string) error {
	if mode == "stay" {
		state.AddLevel = ""
		switch step {
		case "product":
			state.Awaiting = "product"
			return b.askProduct(chatID, msgID)
		case "color":
			state.Awaiting = "color"
			return b.askColor(chatID, state.Product, msgID)
		case "section":
			state.Awaiting = "section"
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
		state.Color, state.Section = "", ""
		state.PageColor, state.PageSection = 0, 0
		return b.askProduct(chatID, msgID)
	case "section":
		state.Section = ""
		state.PageSection = 0
		return b.askColor(chatID, state.Product, msgID)
	default:
		return nil
	}
}
