package telegram

import "fmt"

const (
	msgUnknownCommand = "❓ Неизвестная команда. Доступно: /upload, /search, /recent, /help, /cancel"
	msgCanceled       = "🛑 Действие отменено. Нажмите /upload, чтобы начать заново."
	msgWaitPhoto      = "🖼️ Ожидаю изображение.\nРазрешенные форматы: " + allowedFormatsText
	msgDefaultHint    = "ℹ️ Отправьте фото с подписью или используйте /upload для пошаговой загрузки."

	msgPendingAlbumHasNoKey = "⚠️ Не найден ожидающий пакет. Отправьте файлы ещё раз."
	msgUploadInProgress     = "⏳ Загружаю в Яндекс.Диск…"
	msgBatchInProgress      = "⏳ Загружаю пакет…"
	msgAskNewFolderName     = "📁 Введите название новой папки:"
	msgSearchPromptProduct  = "🔎 Введите часть названия товара для поиска:"
	msgSearchPromptColor    = "🔎 Введите часть названия цвета для поиска:"
	msgSearchColorNeedProduct = "⚠️ Сначала выберите товар, затем выполните поиск по цветам."
	msgSelectProductFirst   = "⚠️ Сначала выберите товар."
	msgSelectProductColorFirst = "⚠️ Сначала выберите товар и цвет."
	msgSelectProductColorSectionFirst = "⚠️ Сначала выберите товар, цвет и раздел."
	msgSendPhotoToCurrentFolder = "📥 Отправьте фото — сохраню в текущую папку."
	msgRecentPathApplyFailed = "⚠️ Не удалось применить недавний путь. Выберите путь вручную."

	btnBack       = "↩️ Назад"
	btnChangePath = "🧭 Изменить путь"
	btnRecent     = "🕘 Последние"
	btnHome       = "🏠 В начало"
	btnSkip       = "⏭️ Без изменений"
	btnSearchProduct = "🔎 Поиск товара"
	btnSearchColor   = "🔎 Поиск цвета"
	btnShowList      = "📋 Показать список"
	btnEditQuery     = "✏️ Изменить запрос"
	btnSaveHere      = "📥 Сохранить в эту папку"
	btnRefresh       = "🔄 Обновить список"
	btnAddFolder     = "➕ Добавить папку"
	btnNavPrev       = "⬅️ Назад"
	btnNavNext       = "➡️ Далее"

	msgRecentEmpty = "🕘 Пока нет последних папок. Загрузите хотя бы одно изображение — я запомню путь."
	msgRecentTitle = "🕘 Последние папки (выберите, и я продолжу):"
	msgBatchReceived = "📦 Получено несколько файлов. Загружаю одним пакетом..."
	msgPendingAlbumExists = "📦 Уже есть ожидающий пакет файлов. Выберите путь — загружу автоматически."
	msgNewFolderEmptyName = "⚠️ Введите непустое имя новой папки."
	msgRenameSingleEmpty  = "⚠️ Введите новое имя файла."
	msgRenameAlbumEmpty   = "⚠️ Введите новое базовое имя (применю ко всем файлам)."
	msgRenameAlbumApplied = "⏳ Применил переименование. Загружаю пакет…"
	msgResolveNoExactMatch = "🔎 Не нашел точное совпадение. Возможно, вы имели в виду:"
	msgResolveProductAmbiguous = "⚠️ Не удалось однозначно определить папку товара."
	msgResolveColorAmbiguous   = "⚠️ Не удалось однозначно определить папку цвета."
	msgResolveSectionAmbiguous = "⚠️ Не удалось однозначно определить папку раздела."
	msgResolvePathProductAmbiguous = "⚠️ Товар из полного пути не найден или неоднозначен."
	msgResolvePathColorAmbiguous   = "⚠️ Товар найден. Цвет из полного пути не найден или неоднозначен."
	msgResolvePathSectionAmbiguous = "⚠️ Товар и цвет найдены. Раздел из полного пути не найден или неоднозначен."
	msgResolveHintNoSuggestions = "\nВведите точнее или выберите кнопку из списка."
	msgResolveHintHasSuggestions = "\nВарианты ниже помогут выбрать быстрее."
	msgRecentPathUndefined = "Путь не определен"
	msgAlbumPathSelectedFlushing = "⏳ Путь выбран. Загружаю ранее отправленный пакет..."
	msgAlbumChoosePath = "⚠️ Для пакетной загрузки выберите путь — и я автоматически загружу уже отправленные файлы."
	msgPhotoReadFailed = "⚠️ Не удалось прочитать изображение. Отправьте фото еще раз."
	msgTelegramGetFileFailed = "❌ Не удалось получить файл из Telegram."
	msgTelegramDownloadFailed = "❌ Не удалось скачать изображение."
	msgUnsupportedFormat = "⚠️ Неподдерживаемый формат файла.\nРазрешенные форматы: " + allowedFormatsText
	msgUploaderNotInitialized = "❌ Ошибка: uploader не инициализирован."
	msgDiskUploadErrorPrefix  = "❌ Ошибка загрузки в Яндекс.Диск:\n"
	msgSendPhotoNow = "📸 Теперь отправьте фото в выбранную папку.\n"
	msgRenameSinglePromptPrefix = "✍️ Переименование файла (титульники)\n\nТекущее имя:\n"
	msgRenameSinglePromptSuffix = "\n\nОтправьте новое имя файла.\nМожно без расширения — я сохраню текущее."
	msgRenameAlbumPrompt = "✍️ Переименование файлов (титульники)\n\nВведите новое базовое имя.\nЯ применю его ко всем файлам как: Имя_01.jpg, Имя_02.jpg …\n\nИли нажмите «Без изменений»."
	msgFolderCreatedPlain = "Папка создана."
	msgPathPrefix = "📁 Путь: "
	msgProductsEmptySuffix = "\nСоздайте первую папку товара кнопкой ниже."
	msgColorsEmptySuffix   = "\nСоздайте первую кнопкой ниже."
	msgSectionsEmptySuffix = "\nСоздайте нужный раздел кнопкой ниже."
	msgChooseProductPrefix = "📦 Выберите товар:\n"
	msgChooseColorPrefix   = "🎨 Выберите цвет:\n"
	msgChooseSectionPrefix = "🗂️ Выберите раздел:\n"
	msgSearchAppliedPrefix = "\n🔎 Поиск: "
	msgTextInputHint = "\n✍️ Можно ввести название текстом или полный путь."
	msgProductsEmptyPrefix = "📦 Список товаров пуст.\n"
	msgColorsEmptyPrefix   = "🎨 Для выбранного товара пока нет папок цветов.\n"
	msgSectionsEmptyPrefix = "🗂️ В этой папке цвета пока нет разделов.\n"
	msgSearchMenuTitle = "🔎 Выберите режим поиска:"
	btnSearchColorInProduct = "🔎 Поиск цвета в выбранном товаре"
	cmdDescUpload = "Начать пошаговую загрузку"
	cmdDescSearch = "Быстрый поиск товаров/цветов"
	cmdDescRecent = "Последние папки для загрузки"
	cmdDescHelp   = "Показать справку"
	cmdDescCancel = "Отменить текущее действие"
)

func msgUploadSuccess(target string) string {
	return "✅ Готово. Изображение сохранено:\n" + target + "\n\n📤 Можете загрузить ещё — просто отправьте новые файлы."
}

func msgBatchUploadSuccess(result string) string {
	return result + "\n\n📤 Можете загрузить ещё — просто отправьте новые файлы."
}

func msgFolderCreateError(err error) string {
	return "❌ Не удалось создать папку: " + humanError(err)
}

func msgFolderCreated(target string) string {
	return "✅ Папка создана:\n" + target
}

func msgRenameSinglePrompt(currentFileName string) string {
	return msgRenameSinglePromptPrefix + currentFileName + msgRenameSinglePromptSuffix
}

func msgBatchResult(success, fail int, savedFolder string) string {
	result := fmt.Sprintf("✅ Пакетная загрузка завершена.\nУспешно: %d\nС ошибками: %d", success, fail)
	if savedFolder != "" {
		result += "\nСохранено в:\n" + savedFolder
	}
	return result
}

func msgWelcomeText() string {
	return "👋 Добро пожаловать в PicFolderBot.\n\n" +
		"Что умею:\n" +
		"• Помогаю выбрать товар → цвет → раздел\n" +
		"• Загружаю изображение в выбранную папку\n" +
		"• Создаю папки кнопкой ➕ на нужном уровне\n\n" +
		"🚀 Нажмите /upload, чтобы начать.\n" +
		"🔎 Для больших каталогов используйте /search.\n" +
		"🖼️ Форматы: " + allowedFormatsText
}

func msgListProductsError(err error) string {
	return "❌ Не удалось получить список товаров:\n" + humanError(err)
}
func msgListColorsError(err error) string {
	return "❌ Не удалось получить список цветов:\n" + humanError(err)
}
func msgListSectionsError(err error) string {
	return "❌ Не удалось получить список разделов:\n" + humanError(err)
}
