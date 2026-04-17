package telegram

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"PicFolderBot/internal/service"
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
		b.setSession(chatID, state)
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
		b.setSession(chatID, state)
	case "add":
		if len(parts) != 3 || parts[2] != "new" {
			return nil
		}
		state.AddLevel, state.Awaiting = parts[1], "new_folder_name"
		b.setSession(chatID, state)
		return b.sendWithKeyboard(chatID, "📁 Введите название новой папки:", "", nil, "", "", 0, msgID,
			tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("↩️ Назад", fmt.Sprintf("back|%s|stay", parts[1]))),
		)
	case "search":
		if len(parts) != 3 || parts[2] != "start" {
			return nil
		}
		if parts[1] == "product" {
			state.Awaiting = "search_product_query"
			b.setSession(chatID, state)
			return b.send(chatID, "🔎 Введите часть названия товара для поиска:")
		}
		if parts[1] == "color" {
			if strings.TrimSpace(state.Product) == "" {
				state.Awaiting = "product"
				return b.send(chatID, "⚠️ Сначала выберите товар, затем выполните поиск по цветам.")
			}
			state.Awaiting = "search_color_query"
			b.setSession(chatID, state)
			return b.send(chatID, "🔎 Введите часть названия цвета для поиска:")
		}
		return nil
	case "show":
		if len(parts) != 3 || parts[2] != "list" {
			return nil
		}
		switch parts[1] {
		case "product":
			state.Awaiting = "product"
			b.setSession(chatID, state)
			return b.askProduct(chatID, msgID)
		case "color":
			if strings.TrimSpace(state.Product) == "" {
				state.Awaiting = "product"
				b.setSession(chatID, state)
				return b.askProduct(chatID, msgID)
			}
			state.Awaiting = "color"
			b.setSession(chatID, state)
			return b.askColor(chatID, state.Product, msgID)
		case "section":
			if strings.TrimSpace(state.Product) == "" || strings.TrimSpace(state.Color) == "" {
				state.Awaiting = "product"
				b.setSession(chatID, state)
				return b.askProduct(chatID, msgID)
			}
			state.Awaiting = "section"
			b.setSession(chatID, state)
			return b.askSection(chatID, state.Product, state.Color, msgID)
		default:
			return nil
		}
	case "post":
		if len(parts) != 3 {
			return nil
		}
		if parts[1] == "change" && parts[2] == "path" {
			state.Section, state.Color = "", ""
			state.UploadLevel = ""
			state.PageColor, state.PageSection = 0, 0
			state.Awaiting = "product"
			b.setSession(chatID, state)
			return b.askProduct(chatID)
		}
		return nil
	case "recent":
		// recent|open|x OR recent|use|<idx>
		if len(parts) != 3 {
			return nil
		}
		if parts[1] == "open" {
			return b.sendRecentMenu(chatID)
		}
		if parts[1] == "use" {
			idx, err := parsePositiveInt(parts[2])
			if err != nil {
				return nil
			}
			items := b.recent.List(chatID)
			if idx < 0 || idx >= len(items) {
				return nil
			}
			it := items[idx]
			// Apply as progressive full path input for safe fallback.
			text := strings.TrimSpace(it.Product + ", " + it.Color + ", " + it.Section)
			state.UploadLevel = strings.TrimSpace(it.Level)
			if state.UploadLevel == "" {
				state.UploadLevel = service.LevelSection
			}
			b.setSession(chatID, state)
			_, _ = b.tryApplyFullPathInput(chatID, state, text)
			return nil
		}
		return nil
	case "refresh":
		if len(parts) != 3 {
			return nil
		}
		switch parts[1] {
		case "product":
			b.flow.InvalidateProducts()
			return b.askProduct(chatID, msgID)
		case "color":
			b.flow.InvalidateColors(state.Product)
			return b.askColor(chatID, state.Product, msgID)
		case "section":
			b.flow.InvalidateSections(state.Product, state.Color)
			return b.askSection(chatID, state.Product, state.Color, msgID)
		default:
			return nil
		}
	case "home":
		if len(parts) != 3 || parts[1] != "go" {
			return nil
		}
		b.clearSession(chatID)
		return b.sendOrEditText(chatID, b.welcomeText(), msgID)
	case "save":
		if len(parts) != 3 || parts[2] != "here" {
			return nil
		}
		level := parts[1]
		switch level {
		case "product":
			if strings.TrimSpace(state.Product) == "" {
				return b.send(chatID, "⚠️ Сначала выберите товар.")
			}
			state.UploadLevel = "product"
		case "color":
			if strings.TrimSpace(state.Product) == "" || strings.TrimSpace(state.Color) == "" {
				return b.send(chatID, "⚠️ Сначала выберите товар и цвет.")
			}
			state.UploadLevel = "color"
		case "section":
			if strings.TrimSpace(state.Product) == "" || strings.TrimSpace(state.Color) == "" || strings.TrimSpace(state.Section) == "" {
				return b.send(chatID, "⚠️ Сначала выберите товар, цвет и раздел.")
			}
			state.UploadLevel = "section"
		default:
			return nil
		}
		state.Awaiting = "photo"
		b.setSession(chatID, state)
		return b.sendOrEditText(chatID, "📥 Отправьте фото — сохраню в текущую папку.", msgID)
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
			b.setSession(chatID, state)
			return b.askProduct(chatID, msgID)
		case "color":
			state.PageColor = stepPage(page, parts[2])
			b.setSession(chatID, state)
			return b.askColor(chatID, state.Product, msgID)
		case "section":
			state.PageSection = stepPage(page, parts[2])
			b.setSession(chatID, state)
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
		state.UploadLevel = ""
		state.PageColor, state.PageSection = 0, 0
		if state.SearchField == "product" {
			state.SearchField, state.SearchQuery = "", ""
		}
	case "color":
		state.Color, state.Section = value, ""
		state.UploadLevel = ""
		state.PageSection = 0
		if state.SearchField == "color" {
			state.SearchField, state.SearchQuery = "", ""
		}
	case "section":
		state.Section = value
		state.UploadLevel = ""
	}
}

func (b *Bot) handleBackAction(chatID int64, msgID int, state *sessionState, step, mode string) error {
	if mode == "stay" {
		state.AddLevel = ""
		state.UploadLevel = ""
		switch step {
		case "product":
			state.Awaiting = "product"
			b.setSession(chatID, state)
			return b.askProduct(chatID, msgID)
		case "color":
			state.Awaiting = "color"
			b.setSession(chatID, state)
			return b.askColor(chatID, state.Product, msgID)
		case "section":
			state.Awaiting = "section"
			b.setSession(chatID, state)
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
		state.UploadLevel = ""
		state.PageColor, state.PageSection = 0, 0
		b.setSession(chatID, state)
		return b.askProduct(chatID, msgID)
	case "section":
		state.Section = ""
		state.UploadLevel = ""
		state.PageSection = 0
		b.setSession(chatID, state)
		return b.askColor(chatID, state.Product, msgID)
	default:
		return nil
	}
}
