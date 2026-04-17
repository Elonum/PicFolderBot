package telegram

import (
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"PicFolderBot/internal/service"
)

func (b *Bot) handlePhoto(msg *tgbotapi.Message) error {
	chatID := msg.Chat.ID
	if len(msg.Photo) == 0 {
		return b.send(chatID, "⚠️ Не удалось прочитать изображение. Отправьте фото еще раз.")
	}
	file := msg.Photo[len(msg.Photo)-1]

	fileURL, err := b.api.GetFileDirectURL(file.FileID)
	if err != nil {
		return b.send(chatID, "❌ Не удалось получить файл из Telegram.")
	}
	content, mimeType, err := downloadFile(fileURL)
	if err != nil {
		return b.send(chatID, "❌ Не удалось скачать изображение.")
	}
	// Telegram message.Photo is always an image payload selected via "Send an image".
	// Some Telegram CDN responses can expose generic MIME values, so we force safe image fallback.
	if !isAllowedImageMIME(mimeType) {
		mimeType = "image/jpeg"
	}
	fileName := buildFileName(fmt.Sprintf("img_%d", time.Now().Unix()), mimeType)
	if msg.MediaGroupID != "" {
		p, c, s, lvl := b.resolveUploadContext(chatID, msg.Caption)
		return b.enqueueAlbumItem(chatID, msg.MediaGroupID, albumItem{Filename: fileName, MimeType: mimeType, Content: content}, p, c, s, lvl)
	}

	state := b.getSession(chatID)
	if state == nil {
		parsed := b.flow.ParseCaption(msg.Caption)
		state = &sessionState{
			Product: parsed.Product, Color: parsed.Color, Section: parsed.Section,
			FileID: file.FileID, FileName: fileName, FileMIME: mimeType, FileBytes: content,
		}
		b.setSession(chatID, state)
	} else {
		state.FileID, state.FileName, state.FileMIME, state.FileBytes = file.FileID, fileName, mimeType, content
		b.setSession(chatID, state)
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
		p, c, s, lvl := b.resolveUploadContext(chatID, msg.Caption)
		return b.enqueueAlbumItem(chatID, msg.MediaGroupID, albumItem{Filename: fileName, MimeType: mimeType, Content: content}, p, c, s, lvl)
	}

	state := b.getSession(chatID)
	if state == nil {
		parsed := b.flow.ParseCaption(msg.Caption)
		state = &sessionState{
			Product: parsed.Product, Color: parsed.Color, Section: parsed.Section,
			FileID: doc.FileID, FileName: fileName, FileMIME: mimeType, FileBytes: content,
		}
		b.setSession(chatID, state)
	} else {
		state.FileID, state.FileName, state.FileMIME, state.FileBytes = doc.FileID, fileName, mimeType, content
		b.setSession(chatID, state)
	}
	return b.continueUploadFlow(chatID, state)
}

func (b *Bot) continueUploadFlow(chatID int64, state *sessionState, editMessageID ...int) error {
	level := strings.TrimSpace(state.UploadLevel)
	if level == "" {
		level = service.LevelSection
	}

	if state.Product == "" {
		state.Awaiting = "product"
		b.setSession(chatID, state)
		return b.askProduct(chatID, editMessageID...)
	}
	if level != service.LevelProduct && state.Color == "" {
		state.Awaiting = "color"
		b.setSession(chatID, state)
		return b.askColor(chatID, state.Product, editMessageID...)
	}
	if level == service.LevelSection && state.Section == "" {
		state.Awaiting = "section"
		b.setSession(chatID, state)
		return b.askSection(chatID, state.Product, state.Color, editMessageID...)
	}
	if handled, err := b.processPendingAlbumIfReady(chatID, state); handled || err != nil {
		return err
	}
	if len(state.FileBytes) == 0 {
		state.Awaiting = "photo"
		b.setSession(chatID, state)
		return b.sendWithKeyboard(chatID, "📸 Теперь отправьте фото в выбранную папку.\n"+b.pathHint(state.Product, state.Color, state.Section), "", nil, "", "section", 0, extractEditID(editMessageID...))
	}
	return b.enqueueSingleUpload(chatID, level, state, extractEditID(editMessageID...))
}

func (b *Bot) resolveUploadContext(chatID int64, caption string) (string, string, string, string) {
	state := b.getSession(chatID)
	var product, color, section, level string
	if state != nil {
		product, color, section = strings.TrimSpace(state.Product), strings.TrimSpace(state.Color), strings.TrimSpace(state.Section)
		level = strings.TrimSpace(state.UploadLevel)
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
	return product, color, section, level
}

func (b *Bot) enqueueSingleUpload(chatID int64, level string, state *sessionState, editMessageID int) error {
	if b.uploader == nil {
		return b.send(chatID, "❌ Ошибка: uploader не инициализирован.")
	}
	payload := service.UploadPayload{
		Product:  state.Product,
		Color:    state.Color,
		Section:  state.Section,
		Filename: state.FileName,
		MimeType: state.FileMIME,
		Content:  state.FileBytes,
	}
	// Clear file bytes immediately to keep session small and UI responsive.
	state.FileID, state.FileName, state.FileMIME, state.FileBytes = "", "", "", nil
	state.Awaiting = "uploading"
	b.setSession(chatID, state)

	if err := b.sendOrEditText(chatID, "⏳ Загружаю в Яндекс.Диск…", editMessageID); err != nil {
		return err
	}

	done := b.uploader.submit(level, payload)
	go func() {
		res := <-done
		s := b.getSession(chatID)
		if s == nil {
			s = &sessionState{}
		}
		if res.Err != nil {
			s.Awaiting = "photo"
			b.setSession(chatID, s)
			_ = b.send(chatID, "❌ Ошибка загрузки в Яндекс.Диск:\n"+humanError(res.Err))
			return
		}
		// Remember successful path for quick reuse.
		b.recent.Push(chatID, RecentPath{
			Product: payload.Product,
			Color:   payload.Color,
			Section: payload.Section,
			Level:   level,
		})
		// Sticky mode: keep the chosen path and wait for new files.
		s.Awaiting = "photo"
		b.setSession(chatID, s)
		_ = b.sendWithKeyboard(chatID, "✅ Готово. Изображение сохранено:\n"+res.Target+"\n\n📤 Можете загрузить ещё — просто отправьте новые файлы.", "", nil, "", "section", 0, 0,
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("↩️ Назад", "back|section|go"),
				tgbotapi.NewInlineKeyboardButtonData("🧭 Изменить путь", "post|change|path"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🕘 Последние", "recent|open|x"),
				tgbotapi.NewInlineKeyboardButtonData("🏠 В начало", "home|go|x"),
			),
		)
	}()
	return nil
}
