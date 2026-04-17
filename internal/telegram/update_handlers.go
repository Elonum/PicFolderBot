package telegram

import (
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"PicFolderBot/internal/service"
)

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
		b.setSession(chatID, state)
		return err
	}
	if state != nil && state.Awaiting == "search_product_query" && msg.Text != "" {
		state.SearchField = "product"
		state.SearchQuery = strings.TrimSpace(msg.Text)
		state.PageProduct = 0
		b.setSession(chatID, state)
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
		b.setSession(chatID, state)
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
		state.Product, state.Color, state.Section = resolved, "", ""
		state.PageColor, state.PageSection = 0, 0
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
		state.Color, state.Section = resolved, ""
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
	state.Product, state.Color, state.Section = product, color, section
	state.PageColor, state.PageSection = 0, 0
	return true, b.continueUploadFlow(chatID, state)
}

func parseFullPathInput(input string) (product, color, section string, ok bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", "", false
	}
	sepFields := splitBySeparators(input)
	if len(sepFields) >= 3 {
		return sepFields[0], sepFields[1], strings.Join(sepFields[2:], " "), true
	}
	ws := strings.Fields(input)
	if len(ws) >= 3 {
		return ws[0], ws[1], strings.Join(ws[2:], " "), true
	}
	return "", "", "", false
}

func splitBySeparators(input string) []string {
	replacer := strings.NewReplacer("\n", ",", ";", ",", "/", ",")
	raw := strings.Split(replacer.Replace(input), ",")
	out := make([]string, 0, len(raw))
	for _, part := range raw {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func resolveTypedOption(options []string, input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	normInput := normalizeLookup(input)
	for _, opt := range options {
		if strings.EqualFold(strings.TrimSpace(opt), input) {
			return opt
		}
		if normalizeLookup(opt) == normInput {
			return opt
		}
		if strings.Contains(normalizeLookup(opt), normInput) || strings.Contains(normInput, normalizeLookup(opt)) {
			return opt
		}
	}
	return ""
}

func normalizeLookup(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.ReplaceAll(v, "ё", "е")
	v = strings.ReplaceAll(v, "-", " ")
	v = strings.ReplaceAll(v, "_", " ")
	return strings.Join(strings.Fields(v), " ")
}

func (b *Bot) enqueueAlbumItem(chatID int64, groupID string, item albumItem, product, color, section, uploadLevel string) error {
	key := fmt.Sprintf("%d:%s", chatID, groupID)
	b.albumsMu.Lock()
	buf := b.albums[key]
	if buf == nil {
		buf = &albumBuffer{ChatID: chatID, MediaGroupID: groupID}
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
	if buf.UploadLevel == "" && uploadLevel != "" {
		buf.UploadLevel = uploadLevel
	}
	buf.Items = append(buf.Items, item)
	if buf.Timer != nil {
		buf.Timer.Stop()
	}
	buf.Timer = time.AfterFunc(albumFlushDelay, func() { b.flushAlbum(key) })
	needNotify := !buf.Notified
	if needNotify {
		buf.Notified = true
	}
	b.albumsMu.Unlock()
	state := b.getSession(chatID)
	if state == nil {
		state = &sessionState{}
		b.setSession(chatID, state)
	}
	state.PendingAlbumKey = key
	b.setSession(chatID, state)
	b.albumStore.Set(key, buf)
	if needNotify {
		return b.send(chatID, "📦 Получено несколько файлов. Загружаю одним пакетом...")
	}
	return nil
}

func (b *Bot) flushAlbum(key string) {
	b.albumsMu.Lock()
	buf := b.albums[key]
	if buf == nil {
		buf = b.albumStore.Get(key)
	}
	if buf == nil {
		b.albumsMu.Unlock()
		return
	}
	b.fillAlbumPathFromSession(buf)
	level := strings.TrimSpace(buf.UploadLevel)
	if level == "" {
		level = service.LevelSection
	}
	if b.isAlbumPathMissing(level, buf.Product, buf.Color, buf.Section) {
		b.albumsMu.Unlock()
		state := b.getSession(buf.ChatID)
		if state == nil {
			state = &sessionState{}
			b.setSession(buf.ChatID, state)
		}
		state.PendingAlbumKey = key
		b.setSession(buf.ChatID, state)
		b.albumStore.Set(key, buf)
		_ = b.send(buf.ChatID, "⚠️ Для пакетной загрузки выберите путь, и я автоматически загружу уже отправленные файлы.")
		return
	}
	delete(b.albums, key)
	b.albumStore.Delete(key)
	b.albumsMu.Unlock()

	product, color, section := strings.TrimSpace(buf.Product), strings.TrimSpace(buf.Color), strings.TrimSpace(buf.Section)
	success, fail := 0, 0
	savedFolder := ""
	for _, it := range buf.Items {
		target, err := b.flow.UploadImageAtLevel(level, service.UploadPayload{
			Product: product, Color: color, Section: section, Filename: it.Filename, MimeType: it.MimeType, Content: it.Content,
		})
		if err != nil {
			fail++
			continue
		}
		if savedFolder == "" {
			savedFolder = folderFromTarget(target)
		}
		success++
	}
	result := fmt.Sprintf("✅ Пакетная загрузка завершена.\nУспешно: %d\nС ошибками: %d", success, fail)
	if savedFolder != "" {
		result += "\nСохранено в:\n" + savedFolder
	}
	state := b.getSession(buf.ChatID)
	if state != nil && state.PendingAlbumKey == key {
		state.PendingAlbumKey = ""
		b.setSession(buf.ChatID, state)
	}
	_ = b.sendWithKeyboard(buf.ChatID, result+"\n\n📤 Загрузить еще в этот же раздел?", "", nil, "", "section", 0, 0,
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Да", "post|same|yes"),
			tgbotapi.NewInlineKeyboardButtonData("🧭 Изменить путь", "post|change|path"),
		),
	)
}

func (b *Bot) processPendingAlbumIfReady(chatID int64, state *sessionState) (bool, error) {
	key := strings.TrimSpace(state.PendingAlbumKey)
	if key == "" {
		return false, nil
	}
	b.albumsMu.Lock()
	buf := b.albums[key]
	if buf == nil {
		buf = b.albumStore.Get(key)
	}
	if buf == nil {
		b.albumsMu.Unlock()
		state.PendingAlbumKey = ""
		b.setSession(chatID, state)
		return false, nil
	}
	b.fillAlbumPathFromSession(buf)
	level := strings.TrimSpace(buf.UploadLevel)
	if level == "" {
		level = service.LevelSection
	}
	missing := b.isAlbumPathMissing(level, buf.Product, buf.Color, buf.Section)
	b.albumsMu.Unlock()
	if missing {
		return false, nil
	}
	go b.flushAlbum(key)
	return true, b.send(chatID, "⏳ Путь выбран. Загружаю ранее отправленный пакет...")
}

func (b *Bot) fillAlbumPathFromSession(buf *albumBuffer) {
	state := b.getSession(buf.ChatID)
	if state == nil {
		return
	}
	if strings.TrimSpace(buf.Product) == "" {
		buf.Product = state.Product
	}
	if strings.TrimSpace(buf.Color) == "" {
		buf.Color = state.Color
	}
	if strings.TrimSpace(buf.Section) == "" {
		buf.Section = state.Section
	}
	if strings.TrimSpace(buf.UploadLevel) == "" {
		buf.UploadLevel = state.UploadLevel
	}
}

func (b *Bot) isAlbumPathMissing(level, product, color, section string) bool {
	product = strings.TrimSpace(product)
	color = strings.TrimSpace(color)
	section = strings.TrimSpace(section)
	if product == "" {
		return true
	}
	if level != service.LevelProduct && color == "" {
		return true
	}
	return level == service.LevelSection && section == ""
}

func folderFromTarget(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	idx := strings.LastIndex(target, "/")
	if idx <= 0 {
		return target
	}
	return target[:idx]
}
