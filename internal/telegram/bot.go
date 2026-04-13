package telegram

import (
	"context"
	"log"
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
	listPageSize            = 8
	maxButtonLabelRunes     = 26
	albumFlushDelay         = 1200 * time.Millisecond
	telegramSendRetries     = 3
)

type flowAPI interface {
	ParseCaption(caption string) parser.ParsedInput
	RootDisplayName() string
	ListProducts() ([]string, error)
	ListColors(product string) ([]string, error)
	ListSections(product, color string) ([]string, error)
	UploadImage(payload service.UploadPayload) (string, error)
	CreateFolderAtLevel(level, product, color, section, newFolder string) (string, error)
}

type sessionState struct {
	Product     string
	Color       string
	Section     string
	AddLevel    string
	SearchQuery string
	SearchField string
	PageProduct int
	PageColor   int
	PageSection int
	FileID      string
	FileName    string
	FileMIME    string
	FileBytes   []byte
	ValueMap    map[string]string
	Awaiting    string
}

type Bot struct {
	api      *tgbotapi.BotAPI
	flow     flowAPI
	rootName string
	sessions map[int64]*sessionState
	albums   map[string]*albumBuffer
	mu       sync.RWMutex
	albumsMu sync.Mutex
}

type albumItem struct {
	Filename string
	MimeType string
	Content  []byte
}

type albumBuffer struct {
	ChatID       int64
	MediaGroupID string
	Product      string
	Color        string
	Section      string
	Items        []albumItem
	Timer        *time.Timer
	Notified     bool
}

func NewBot(token string, flow flowAPI) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(strings.TrimSpace(token))
	if err != nil {
		return nil, err
	}
	_, _ = api.Request(tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "upload", Description: "Начать пошаговую загрузку"},
		tgbotapi.BotCommand{Command: "search", Description: "Быстрый поиск товаров/цветов"},
		tgbotapi.BotCommand{Command: "help", Description: "Показать справку"},
		tgbotapi.BotCommand{Command: "cancel", Description: "Отменить текущее действие"},
	))
	return &Bot{
		api:      api,
		flow:     flow,
		rootName: flow.RootDisplayName(),
		sessions: make(map[int64]*sessionState),
		albums:   make(map[string]*albumBuffer),
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
