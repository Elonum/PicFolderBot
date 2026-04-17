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
			state := b.getSession(chatID)
			if state == nil {
				state = &sessionState{}
			}
			if strings.TrimSpace(state.PendingAlbumKey) != "" {
				if strings.TrimSpace(state.UploadLevel) == "" {
					state.UploadLevel = service.LevelSection
				}
				b.setSession(chatID, state)
				if err := b.send(chatID, "📦 Уже есть ожидающий пакет файлов. Выберите путь — загружу автоматически."); err != nil {
					return err
				}
				return b.continueUploadFlow(chatID, state)
			}
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
		// Allow full-path input even inside /search query.
		if ok, err := b.tryApplyFullPathInput(chatID, state, msg.Text); ok || err != nil {
			return err
		}
		query := strings.TrimSpace(msg.Text)
		products, err := b.flow.ListProducts()
		if err != nil {
			return b.send(chatID, "❌ Не удалось получить список товаров:\n"+humanError(err))
		}
		res := resolveTypedOptionSmart(products, query)
		if res.Value != "" {
			state.Product, state.Color, state.Section = res.Value, "", ""
			state.SearchField, state.SearchQuery = "", ""
			state.PageColor, state.PageSection = 0, 0
			state.Awaiting = "color"
			b.setSession(chatID, state)
			go b.prefetchColors(res.Value)
			return b.askColor(chatID, res.Value)
		}
		state.SearchField = "product"
		state.SearchQuery = query
		state.PageProduct = 0
		b.setSession(chatID, state)
		if len(res.Suggestions) > 0 {
			return b.sendResolveHint(chatID, "product", res, "🔎 Не нашел точное совпадение. Возможно, вы имели в виду:")
		}
		return b.askProduct(chatID)
	}
	if state != nil && state.Awaiting == "search_color_query" && msg.Text != "" {
		if strings.TrimSpace(state.Product) == "" {
			state.Awaiting = "product"
			return b.send(chatID, "⚠️ Сначала выберите товар, затем выполните поиск по цветам.")
		}
		// Allow full-path input even inside /search query.
		if ok, err := b.tryApplyFullPathInput(chatID, state, msg.Text); ok || err != nil {
			return err
		}
		query := strings.TrimSpace(msg.Text)
		colors, err := b.flow.ListColors(state.Product)
		if err != nil {
			return b.send(chatID, "❌ Не удалось получить список цветов:\n"+humanError(err))
		}
		res := resolveTypedOptionSmart(colors, query)
		if res.Value != "" {
			state.Color, state.Section = res.Value, ""
			state.SearchField, state.SearchQuery = "", ""
			state.PageSection = 0
			state.Awaiting = "section"
			b.setSession(chatID, state)
			go b.prefetchSections(state.Product, res.Value)
			return b.askSection(chatID, state.Product, res.Value)
		}
		state.SearchField = "color"
		state.SearchQuery = query
		state.PageColor = 0
		b.setSession(chatID, state)
		if len(res.Suggestions) > 0 {
			return b.sendResolveHint(chatID, "color", res, "🔎 Не нашел точное совпадение. Возможно, вы имели в виду:")
		}
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
		res := resolveTypedOptionSmart(options, value)
		if res.Value == "" {
			return true, b.sendResolveHint(chatID, "product", res, "⚠️ Не удалось однозначно определить папку товара.")
		}
		state.Product, state.Color, state.Section = res.Value, "", ""
		state.PageColor, state.PageSection = 0, 0
		b.setSession(chatID, state)
		go b.prefetchColors(res.Value)
		return true, b.continueUploadFlow(chatID, state)
	case "color":
		if strings.TrimSpace(state.Product) == "" {
			return true, b.askProduct(chatID)
		}
		options, err := b.flow.ListColors(state.Product)
		if err != nil {
			return true, b.send(chatID, "❌ Не удалось получить список цветов:\n"+humanError(err))
		}
		res := resolveTypedOptionSmart(options, value)
		if res.Value == "" {
			return true, b.sendResolveHint(chatID, "color", res, "⚠️ Не удалось однозначно определить папку цвета.")
		}
		state.Color, state.Section = res.Value, ""
		state.PageSection = 0
		b.setSession(chatID, state)
		go b.prefetchSections(state.Product, res.Value)
		return true, b.continueUploadFlow(chatID, state)
	case "section":
		if strings.TrimSpace(state.Product) == "" || strings.TrimSpace(state.Color) == "" {
			return true, b.askProduct(chatID)
		}
		options, err := b.flow.ListSections(state.Product, state.Color)
		if err != nil {
			return true, b.send(chatID, "❌ Не удалось получить список разделов:\n"+humanError(err))
		}
		res := resolveTypedOptionSmart(options, value)
		if res.Value == "" {
			return true, b.sendResolveHint(chatID, "section", res, "⚠️ Не удалось однозначно определить папку раздела.")
		}
		state.Section = res.Value
		b.setSession(chatID, state)
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
	// Resolve full path progressively and preserve successful levels.
	// If some level is wrong/ambiguous, stop there and suggest options for that level.
	products, err := b.flow.ListProducts()
	if err != nil {
		return true, b.send(chatID, "❌ Не удалось получить список товаров:\n"+humanError(err))
	}
	productRes := resolveTypedOptionSmart(products, productRaw)
	if productRes.Value == "" {
		state.Awaiting = "product"
		b.setSession(chatID, state)
		return true, b.sendResolveHint(chatID, "product", productRes, "⚠️ Товар из полного пути не найден или неоднозначен.")
	}
	product := productRes.Value
	state.Product, state.Color, state.Section = product, "", ""
	state.PageColor, state.PageSection = 0, 0
	b.setSession(chatID, state)
	colors, err := b.flow.ListColors(product)
	if err != nil {
		return true, b.send(chatID, "❌ Не удалось получить список цветов:\n"+humanError(err))
	}
	colorRes := resolveTypedOptionSmart(colors, colorRaw)
	if colorRes.Value == "" {
		state.Awaiting = "color"
		b.setSession(chatID, state)
		return true, b.sendResolveHint(chatID, "color", colorRes, "⚠️ Товар найден. Цвет из полного пути не найден или неоднозначен.")
	}
	color := colorRes.Value
	state.Color, state.Section = color, ""
	state.PageSection = 0
	b.setSession(chatID, state)
	sections, err := b.flow.ListSections(product, color)
	if err != nil {
		return true, b.send(chatID, "❌ Не удалось получить список разделов:\n"+humanError(err))
	}
	sectionRes := resolveTypedOptionSmart(sections, sectionRaw)
	if sectionRes.Value == "" {
		state.Awaiting = "section"
		b.setSession(chatID, state)
		return true, b.sendResolveHint(chatID, "section", sectionRes, "⚠️ Товар и цвет найдены. Раздел из полного пути не найден или неоднозначен.")
	}
	section := sectionRes.Value
	state.Product, state.Color, state.Section = product, color, section
	state.PageColor, state.PageSection = 0, 0
	b.setSession(chatID, state)
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

func (b *Bot) sendResolveHint(chatID int64, field string, res optionResolution, baseText string) error {
	if len(res.Suggestions) == 0 {
		return b.send(chatID, baseText+"\nВведите точнее или выберите кнопку из списка.")
	}
	text := baseText + "\nВарианты ниже помогут выбрать быстрее."
	backStep := ""
	switch field {
	case "product":
		backStep = "product"
	case "color":
		backStep = "color"
	case "section":
		backStep = "section"
	}
	extra := [][]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("✏️ Изменить запрос", fmt.Sprintf("search|%s|start", field))),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("📋 Показать список", fmt.Sprintf("show|%s|list", field))),
	}
	return b.sendWithKeyboard(chatID, text, field, res.Suggestions, "", backStep, 0, 0, extra...)
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
		if strings.TrimSpace(state.UploadLevel) == "" {
			state.UploadLevel = level
		}
		b.setSession(buf.ChatID, state)
		b.albumStore.Set(key, buf)
		_ = b.promptPendingAlbumPath(buf.ChatID, state)
		return
	}
	delete(b.albums, key)
	b.albumStore.Delete(key)
	b.albumsMu.Unlock()

	product, color, section := strings.TrimSpace(buf.Product), strings.TrimSpace(buf.Color), strings.TrimSpace(buf.Section)
	success, fail := 0, 0
	savedFolder := ""
	results := make([]<-chan uploadResult, 0, len(buf.Items))
	for _, it := range buf.Items {
		payload := service.UploadPayload{
			Product:  product,
			Color:    color,
			Section:  section,
			Filename: it.Filename,
			MimeType: it.MimeType,
			Content:  it.Content,
		}
		results = append(results, b.uploader.submit(level, payload))
	}
	for _, ch := range results {
		res := <-ch
		if res.Err != nil {
			fail++
			continue
		}
		if savedFolder == "" {
			savedFolder = folderFromTarget(res.Target)
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

func (b *Bot) promptPendingAlbumPath(chatID int64, state *sessionState) error {
	if strings.TrimSpace(state.UploadLevel) == "" {
		state.UploadLevel = service.LevelSection
	}
	state.Awaiting = nextAwaitingForLevel(state.UploadLevel, state)
	b.setSession(chatID, state)
	if err := b.send(chatID, "⚠️ Для пакетной загрузки выберите путь — и я автоматически загружу уже отправленные файлы."); err != nil {
		return err
	}
	return b.continueUploadFlow(chatID, state)
}

func nextAwaitingForLevel(level string, state *sessionState) string {
	if strings.TrimSpace(state.Product) == "" {
		return "product"
	}
	if level != service.LevelProduct && strings.TrimSpace(state.Color) == "" {
		return "color"
	}
	if level == service.LevelSection && strings.TrimSpace(state.Section) == "" {
		return "section"
	}
	return "photo"
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
