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
	case cbActionSet:
		if len(parts) != 3 {
			return nil
		}
		b.applySetSelection(state, parts[1], parts[2])
		b.setSession(chatID, state)
	case cbActionPick:
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
	case cbActionAdd:
		if len(parts) != 3 || parts[2] != "new" {
			return nil
		}
		state.AddLevel, state.Awaiting = parts[1], awaitingNewFolderName
		b.setSession(chatID, state)
		return b.sendWithKeyboard(chatID, msgAskNewFolderName, "", nil, "", "", 0, msgID,
			tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(btnBack, fmt.Sprintf("back|%s|stay", parts[1]))),
		)
	case cbActionSearch:
		if len(parts) != 3 || parts[2] != "start" {
			return nil
		}
		if parts[1] == "product" {
			state.Awaiting = awaitingSearchProduct
			b.setSession(chatID, state)
			return b.send(chatID, msgSearchPromptProduct)
		}
		if parts[1] == "color" {
			if strings.TrimSpace(state.Product) == "" {
				state.Awaiting = awaitingProduct
				return b.send(chatID, msgSearchColorNeedProduct)
			}
			state.Awaiting = awaitingSearchColor
			b.setSession(chatID, state)
			return b.send(chatID, msgSearchPromptColor)
		}
		return nil
	case cbActionShow:
		if len(parts) != 3 || parts[2] != "list" {
			return nil
		}
		switch parts[1] {
		case "product":
			state.Awaiting = awaitingProduct
			b.setSession(chatID, state)
			return b.askProduct(chatID, msgID)
		case "color":
			if strings.TrimSpace(state.Product) == "" {
				state.Awaiting = awaitingProduct
				b.setSession(chatID, state)
				return b.askProduct(chatID, msgID)
			}
			state.Awaiting = awaitingColor
			b.setSession(chatID, state)
			return b.askColor(chatID, state.Product, msgID)
		case "section":
			if strings.TrimSpace(state.Product) == "" || strings.TrimSpace(state.Color) == "" {
				state.Awaiting = awaitingProduct
				b.setSession(chatID, state)
				return b.askProduct(chatID, msgID)
			}
			state.Awaiting = awaitingSection
			b.setSession(chatID, state)
			return b.askSection(chatID, state.Product, state.Color, msgID)
		default:
			return nil
		}
	case cbActionPost:
		if len(parts) != 3 {
			return nil
		}
		if parts[1] == "change" && parts[2] == "path" {
			state.Section, state.Color = "", ""
			state.UploadLevel = ""
			state.PageColor, state.PageSection = 0, 0
			state.Awaiting = awaitingProduct
			b.setSession(chatID, state)
			return b.askProduct(chatID)
		}
		return nil
	case cbActionRename:
		if len(parts) != 3 {
			return nil
		}
		switch parts[1] {
		case "single":
			if parts[2] == "skip" {
				level := strings.TrimSpace(state.UploadLevel)
				if level == "" {
					level = service.LevelSection
				}
				state.Awaiting = awaitingUploading
				b.setSession(chatID, state)
				return b.enqueueSingleUpload(chatID, level, state, msgID)
			}
		case "album":
			if parts[2] == "skip" {
				key := strings.TrimSpace(state.PendingAlbumKey)
				if key == "" {
					return b.send(chatID, msgPendingAlbumHasNoKey)
				}
				state.Awaiting = awaitingUploading
				b.setSession(chatID, state)
				go b.flushAlbum(key)
				return b.sendOrEditText(chatID, msgBatchInProgress, msgID)
			}
		}
		return nil
	case cbActionRecent:
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
			handled, err := b.tryApplyFullPathInput(chatID, state, text)
			if err != nil {
				return err
			}
			if !handled {
				return b.send(chatID, msgRecentPathApplyFailed)
			}
			return nil
		}
		return nil
	case cbActionRefresh:
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
	case cbActionHome:
		if len(parts) != 3 || parts[1] != "go" {
			return nil
		}
		b.clearSession(chatID)
		return b.sendOrEditText(chatID, b.welcomeText(), msgID)
	case cbActionSave:
		if len(parts) != 3 || parts[2] != "here" {
			return nil
		}
		level := parts[1]
		switch level {
		case "product":
			if strings.TrimSpace(state.Product) == "" {
				return b.send(chatID, msgSelectProductFirst)
			}
			state.UploadLevel = "product"
		case "color":
			if strings.TrimSpace(state.Product) == "" || strings.TrimSpace(state.Color) == "" {
				return b.send(chatID, msgSelectProductColorFirst)
			}
			state.UploadLevel = "color"
		case "section":
			if strings.TrimSpace(state.Product) == "" || strings.TrimSpace(state.Color) == "" || strings.TrimSpace(state.Section) == "" {
				return b.send(chatID, msgSelectProductColorSectionFirst)
			}
			state.UploadLevel = "section"
		default:
			return nil
		}
		state.Awaiting = awaitingPhoto
		b.setSession(chatID, state)
		return b.sendOrEditText(chatID, msgSendPhotoToCurrentFolder, msgID)
	case cbActionNav:
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
	case cbActionBack:
		if len(parts) != 3 {
			return nil
		}
		return b.handleBackAction(chatID, msgID, state, parts[1], parts[2])
	case cbActionNoop:
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
		// "stay" means: go back to the list on the same level without
		// wiping already selected upper levels.
		state.AddLevel = ""
		state.UploadLevel = ""
		target := b.resolveBackTarget(step, state)
		state.Awaiting = target
		b.setSession(chatID, state)
		return b.renderStep(chatID, msgID, state, target)
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
		return b.renderStep(chatID, msgID, state, "product")
	case "section":
		state.Section = ""
		state.UploadLevel = ""
		state.PageSection = 0
		b.setSession(chatID, state)
		return b.renderStep(chatID, msgID, state, "color")
	default:
		return nil
	}
}

func (b *Bot) resolveBackTarget(preferred string, state *sessionState) string {
	switch preferred {
	case "product":
		return "product"
	case "color":
		if strings.TrimSpace(state.Product) == "" {
			return "product"
		}
		return "color"
	case "section":
		if strings.TrimSpace(state.Product) == "" {
			return "product"
		}
		if strings.TrimSpace(state.Color) == "" {
			return "color"
		}
		return "section"
	default:
		if strings.TrimSpace(state.Product) == "" {
			return "product"
		}
		if strings.TrimSpace(state.Color) == "" {
			return "color"
		}
		return "section"
	}
}

func (b *Bot) renderStep(chatID int64, msgID int, state *sessionState, step string) error {
	switch step {
	case "product":
		return b.askProduct(chatID, msgID)
	case "color":
		return b.askColor(chatID, state.Product, msgID)
	case "section":
		return b.askSection(chatID, state.Product, state.Color, msgID)
	default:
		return b.askProduct(chatID, msgID)
	}
}
