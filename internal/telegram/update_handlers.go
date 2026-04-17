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
				if err := b.send(chatID, msgPendingAlbumExists); err != nil {
					return err
				}
				return b.continueUploadFlow(chatID, state)
			}
			b.setSession(chatID, &sessionState{Awaiting: awaitingProduct})
			return b.askProduct(chatID)
		case "search":
			state := b.getSession(chatID)
			if state == nil {
				state = &sessionState{}
				b.setSession(chatID, state)
			}
			return b.sendSearchMenu(chatID, state)
		case "recent":
			return b.sendRecentMenu(chatID)
		case "cancel":
			b.clearSession(chatID)
			return b.send(chatID, msgCanceled)
		default:
			return b.send(chatID, msgUnknownCommand)
		}
	}

	if msg.Photo != nil {
		return b.handlePhoto(msg)
	}
	if msg.Document != nil {
		return b.handleDocument(msg)
	}

	state := b.getSession(chatID)
	if state != nil && state.Awaiting == awaitingNewFolderName && msg.Text != "" {
		newFolder := strings.TrimSpace(msg.Text)
		if newFolder == "" {
			return b.send(chatID, msgNewFolderEmptyName)
		}
		level := state.AddLevel
		target, err := b.flow.CreateFolderAtLevel(level, state.Product, state.Color, state.Section, newFolder)
		if err != nil {
			return b.send(chatID, msgFolderCreateError(err))
		}
		state.AddLevel = level
		if err = b.send(chatID, msgFolderCreated(target)); err != nil {
			return err
		}
		// Invalidate cached listing for this level so the new folder appears immediately.
		switch level {
		case service.LevelProduct:
			b.flow.InvalidateProducts()
		case service.LevelColor:
			b.flow.InvalidateColors(state.Product)
		case service.LevelSection:
			b.flow.InvalidateSections(state.Product, state.Color)
		}
		err = b.refreshLevel(chatID, state)
		state.AddLevel = ""
		b.setSession(chatID, state)
		return err
	}
	if state != nil && state.Awaiting == awaitingRenameSingle && msg.Text != "" {
		typed := strings.TrimSpace(msg.Text)
		if typed == "" {
			return b.send(chatID, msgRenameSingleEmpty)
		}
		state.FileName = applyRenameInput(state.FileName, typed, state.FileMIME)
		state.Awaiting = awaitingUploading
		b.setSession(chatID, state)
		level := strings.TrimSpace(state.UploadLevel)
		if level == "" {
			level = service.LevelSection
		}
		return b.enqueueSingleUpload(chatID, level, state, 0)
	}
	if state != nil && state.Awaiting == awaitingRenameAlbum && msg.Text != "" {
		typed := strings.TrimSpace(msg.Text)
		if typed == "" {
			return b.send(chatID, msgRenameAlbumEmpty)
		}
		key := strings.TrimSpace(state.PendingAlbumKey)
		if key == "" {
			state.Awaiting = awaitingSection
			b.setSession(chatID, state)
			return b.send(chatID, msgPendingAlbumHasNoKey)
		}
		b.renameAlbumFilenames(key, typed)
		state.Awaiting = awaitingUploading
		b.setSession(chatID, state)
		go b.flushAlbum(key)
		return b.send(chatID, msgRenameAlbumApplied)
	}
	if state != nil && state.Awaiting == awaitingSearchProduct && msg.Text != "" {
		// Allow full-path input even inside /search query.
		if ok, err := b.tryApplyFullPathInput(chatID, state, msg.Text); ok || err != nil {
			return err
		}
		query := strings.TrimSpace(msg.Text)
		products, err := b.flow.ListProducts()
		if err != nil {
			return b.send(chatID, msgListProductsError(err))
		}
		res := resolveTypedOptionSmart(products, query)
		if res.Value != "" {
			state.Product, state.Color, state.Section = res.Value, "", ""
			state.SearchField, state.SearchQuery = "", ""
			state.PageColor, state.PageSection = 0, 0
			state.Awaiting = awaitingColor
			b.setSession(chatID, state)
			go b.prefetchColors(res.Value)
			return b.askColor(chatID, res.Value)
		}
		state.SearchField = "product"
		state.SearchQuery = query
		state.PageProduct = 0
		b.setSession(chatID, state)
		if len(res.Suggestions) > 0 {
			return b.sendResolveHint(chatID, "product", res, msgResolveNoExactMatch)
		}
		return b.askProduct(chatID)
	}
	if state != nil && state.Awaiting == awaitingSearchColor && msg.Text != "" {
		if strings.TrimSpace(state.Product) == "" {
			state.Awaiting = awaitingProduct
			return b.send(chatID, msgSearchColorNeedProduct)
		}
		// Allow full-path input even inside /search query.
		if ok, err := b.tryApplyFullPathInput(chatID, state, msg.Text); ok || err != nil {
			return err
		}
		query := strings.TrimSpace(msg.Text)
		colors, err := b.flow.ListColors(state.Product)
		if err != nil {
			return b.send(chatID, msgListColorsError(err))
		}
		res := resolveTypedOptionSmart(colors, query)
		if res.Value != "" {
			state.Color, state.Section = res.Value, ""
			state.SearchField, state.SearchQuery = "", ""
			state.PageSection = 0
			state.Awaiting = awaitingSection
			b.setSession(chatID, state)
			go b.prefetchSections(state.Product, res.Value)
			return b.askSection(chatID, state.Product, res.Value)
		}
		state.SearchField = "color"
		state.SearchQuery = query
		state.PageColor = 0
		b.setSession(chatID, state)
		if len(res.Suggestions) > 0 {
			return b.sendResolveHint(chatID, "color", res, msgResolveNoExactMatch)
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
	if state != nil && state.Awaiting == awaitingPhoto {
		return b.send(chatID, msgWaitPhoto)
	}
	// Sticky mode: no dedicated "post success" state is required.
	return b.send(chatID, msgDefaultHint)
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
			return true, b.send(chatID, msgListProductsError(err))
		}
		res := resolveTypedOptionSmart(options, value)
		if res.Value == "" {
			return true, b.sendResolveHint(chatID, "product", res, msgResolveProductAmbiguous)
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
			return true, b.send(chatID, msgListColorsError(err))
		}
		res := resolveTypedOptionSmart(options, value)
		if res.Value == "" {
			return true, b.sendResolveHint(chatID, "color", res, msgResolveColorAmbiguous)
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
			return true, b.send(chatID, msgListSectionsError(err))
		}
		res := resolveTypedOptionSmart(options, value)
		if res.Value == "" {
			return true, b.sendResolveHint(chatID, "section", res, msgResolveSectionAmbiguous)
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
		return true, b.send(chatID, msgListProductsError(err))
	}
	productRes := resolveTypedOptionSmart(products, productRaw)
	if productRes.Value == "" {
		state.Awaiting = awaitingProduct
		b.setSession(chatID, state)
		return true, b.sendResolveHint(chatID, "product", productRes, msgResolvePathProductAmbiguous)
	}
	product := productRes.Value
	state.Product, state.Color, state.Section = product, "", ""
	state.PageColor, state.PageSection = 0, 0
	b.setSession(chatID, state)
	colors, err := b.flow.ListColors(product)
	if err != nil {
		return true, b.send(chatID, msgListColorsError(err))
	}
	colorRes := resolveTypedOptionSmart(colors, colorRaw)
	if colorRes.Value == "" {
		state.Awaiting = awaitingColor
		b.setSession(chatID, state)
		return true, b.sendResolveHint(chatID, "color", colorRes, msgResolvePathColorAmbiguous)
	}
	color := colorRes.Value
	state.Color, state.Section = color, ""
	state.PageSection = 0
	b.setSession(chatID, state)
	sections, err := b.flow.ListSections(product, color)
	if err != nil {
		return true, b.send(chatID, msgListSectionsError(err))
	}
	sectionRes := resolveTypedOptionSmart(sections, sectionRaw)
	if sectionRes.Value == "" {
		state.Awaiting = awaitingSection
		b.setSession(chatID, state)
		return true, b.sendResolveHint(chatID, "section", sectionRes, msgResolvePathSectionAmbiguous)
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
		return b.send(chatID, baseText+msgResolveHintNoSuggestions)
	}
	text := baseText + msgResolveHintHasSuggestions
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
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(btnEditQuery, fmt.Sprintf("search|%s|start", field))),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(btnShowList, fmt.Sprintf("show|%s|list", field))),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(btnHome, "home|go|x")),
	}
	return b.sendWithKeyboard(chatID, text, field, res.Suggestions, "", backStep, 0, 0, extra...)
}

func (b *Bot) sendRecentMenu(chatID int64) error {
	items := b.recent.List(chatID)
	if len(items) == 0 {
		return b.send(chatID, msgRecentEmpty)
	}
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(items)+1)
	for i, it := range items {
		parts := make([]string, 0, 3)
		if v := strings.TrimSpace(it.Product); v != "" {
			parts = append(parts, v)
		}
		if v := strings.TrimSpace(it.Color); v != "" {
			parts = append(parts, v)
		}
		if v := strings.TrimSpace(it.Section); v != "" {
			parts = append(parts, v)
		}
		label := strings.Join(parts, " / ")
		if label == "" {
			label = msgRecentPathUndefined
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(trimButtonLabel(label), fmt.Sprintf("recent|use|%d", i)),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(btnHome, "home|go|x")))
	msg := tgbotapi.NewMessage(chatID, msgRecentTitle)
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	return b.sendWithRetry(msg)
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
		return b.send(chatID, msgBatchReceived)
	}
	return nil
}

func (b *Bot) flushAlbum(key string) {
	if !b.tryMarkAlbumFlushing(key) {
		return
	}
	defer b.unmarkAlbumFlushing(key)

	b.albumsMu.Lock()
	buf := b.albums[key]
	if buf == nil {
		buf = b.albumStore.Get(key)
	}
	if buf == nil {
		b.albumsMu.Unlock()
		return
	}
	// Guard: if we already asked for rename for this album, do not spam the chat
	// on repeated timer flushes/callback-triggered flushes.
	if st := b.getSession(buf.ChatID); st != nil {
		if st.Awaiting == awaitingRenameAlbum && strings.TrimSpace(st.PendingAlbumKey) == key {
			b.albumsMu.Unlock()
			return
		}
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
	// If we are uploading now, skip rename prompt and proceed.
	if st := b.getSession(buf.ChatID); st != nil && st.Awaiting == awaitingUploading && strings.TrimSpace(st.PendingAlbumKey) == key {
		// proceed
	} else if level == service.LevelSection && isTitularSectionName(buf.Section) {
		// Ask once for the whole album before uploading.
		state := b.getSession(buf.ChatID)
		if state == nil {
			state = &sessionState{}
		}
		state.PendingAlbumKey = key
		state.Awaiting = awaitingRenameAlbum
		b.setSession(buf.ChatID, state)
		b.albumStore.Set(key, buf)
		b.albumsMu.Unlock()
		_ = b.sendWithKeyboard(buf.ChatID,
			msgRenameAlbumPrompt,
			"", nil, "", "section", 0, 0,
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(btnSkip, "rename|album|skip"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(btnBack, "back|section|stay"),
				tgbotapi.NewInlineKeyboardButtonData(btnChangePath, "post|change|path"),
			),
		)
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
	result := msgBatchResult(success, fail, savedFolder)
	state := b.getSession(buf.ChatID)
	if state != nil && state.PendingAlbumKey == key {
		state.PendingAlbumKey = ""
		b.setSession(buf.ChatID, state)
	}
	// Remember successful path for quick reuse.
	if success > 0 {
		b.recent.Push(buf.ChatID, RecentPath{
			Product: product,
			Color:   color,
			Section: section,
			Level:   level,
		})
	}
	state = b.getSession(buf.ChatID)
	if state != nil {
		state.Awaiting = awaitingPhoto
		b.setSession(buf.ChatID, state)
	}
	_ = b.sendCustomKeyboard(buf.ChatID, msgBatchUploadSuccess(result), uploadSuccessRows(), 0)
}

func (b *Bot) tryMarkAlbumFlushing(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	b.flushMu.Lock()
	defer b.flushMu.Unlock()
	if _, ok := b.flushing[key]; ok {
		return false
	}
	b.flushing[key] = struct{}{}
	return true
}

func (b *Bot) unmarkAlbumFlushing(key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	b.flushMu.Lock()
	delete(b.flushing, key)
	b.flushMu.Unlock()
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
	return true, b.send(chatID, msgAlbumPathSelectedFlushing)
}

func (b *Bot) promptPendingAlbumPath(chatID int64, state *sessionState) error {
	if strings.TrimSpace(state.UploadLevel) == "" {
		state.UploadLevel = service.LevelSection
	}
	state.Awaiting = nextAwaitingForLevel(state.UploadLevel, state)
	b.setSession(chatID, state)
	if err := b.send(chatID, msgAlbumChoosePath); err != nil {
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

func applyRenameInput(originalFileName string, typed string, mimeType string) string {
	typed = strings.TrimSpace(typed)
	if typed == "" {
		return originalFileName
	}
	// User input replaces the filename. If no extension is provided, preserve the original extension.
	clean := strings.TrimSpace(typed)
	clean = strings.ReplaceAll(clean, "/", "_")
	clean = strings.ReplaceAll(clean, "\\", "_")
	if strings.Contains(clean, ".") {
		return buildFileName(clean, mimeType)
	}
	ext := inferExtension(mimeType)
	if dot := strings.LastIndex(strings.TrimSpace(originalFileName), "."); dot > 0 {
		ext = strings.TrimSpace(originalFileName)[dot:]
	}
	return buildFileName(clean+ext, mimeType)
}

func (b *Bot) renameAlbumFilenames(key string, newBase string) {
	newBase = strings.TrimSpace(newBase)
	if newBase == "" {
		return
	}
	b.albumsMu.Lock()
	defer b.albumsMu.Unlock()
	buf := b.albums[key]
	if buf == nil {
		buf = b.albumStore.Get(key)
	}
	if buf == nil {
		return
	}
	for i := range buf.Items {
		// Apply base name to all files; keep each file's extension and add index suffix for uniqueness.
		ext := inferExtension(buf.Items[i].MimeType)
		if dot := strings.LastIndex(buf.Items[i].Filename, "."); dot > 0 {
			ext = buf.Items[i].Filename[dot:]
		}
		buf.Items[i].Filename = buildFileName(fmt.Sprintf("%s_%02d%s", newBase, i+1, ext), buf.Items[i].MimeType)
	}
	b.albumStore.Set(key, buf)
}
