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
	maxButtonLabelRunes     = 34
	albumFlushDelay         = 1200 * time.Millisecond
	telegramSendRetries     = 3
	prefetchCooldown        = 12 * time.Second
	uploadWorkers           = 3
)

type flowAPI interface {
	ParseCaption(caption string) parser.ParsedInput
	RootDisplayName() string
	ListProducts() ([]string, error)
	ListColors(product string) ([]string, error)
	ListSections(product, color string) ([]string, error)
	InvalidateProducts()
	InvalidateColors(product string)
	InvalidateSections(product, color string)
	UploadImage(payload service.UploadPayload) (string, error)
	UploadImageAtLevel(level string, payload service.UploadPayload) (string, error)
	CreateFolderAtLevel(level, product, color, section, newFolder string) (string, error)
}

type sessionState struct {
	Product         string
	Color           string
	Section         string
	UploadLevel     string
	AddLevel        string
	SearchQuery     string
	SearchField     string
	PageProduct     int
	PageColor       int
	PageSection     int
	FileID          string
	FileName        string
	FileMIME        string
	FileBytes       []byte
	ValueMap        map[string]string
	PendingAlbumKey string
	Awaiting        string
}

type Bot struct {
	api          *tgbotapi.BotAPI
	flow         flowAPI
	rootName     string
	sessionStore SessionStore
	albumStore   AlbumStore
	albums       map[string]*albumBuffer
	albumsMu     sync.Mutex
	prefetchMu   sync.Mutex
	prefetchLast map[string]time.Time
	uploader     *uploader
	recent       RecentStore
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
	UploadLevel  string
	Items        []albumItem
	Timer        *time.Timer
	Notified     bool
}

func NewBot(token string, flow flowAPI, sessionStore SessionStore, albumStore AlbumStore) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(strings.TrimSpace(token))
	if err != nil {
		return nil, err
	}
	_, _ = api.Request(tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "upload", Description: "Начать пошаговую загрузку"},
		tgbotapi.BotCommand{Command: "search", Description: "Быстрый поиск товаров/цветов"},
		tgbotapi.BotCommand{Command: "recent", Description: "Последние папки для загрузки"},
		tgbotapi.BotCommand{Command: "help", Description: "Показать справку"},
		tgbotapi.BotCommand{Command: "cancel", Description: "Отменить текущее действие"},
	))
	return &Bot{
		api:          api,
		flow:         flow,
		rootName:     flow.RootDisplayName(),
		sessionStore: sessionStore,
		albumStore:   albumStore,
		albums:       make(map[string]*albumBuffer),
		prefetchLast: make(map[string]time.Time),
		uploader:     newUploader(flow, uploadWorkers, 256),
		recent:       NewMemoryRecentStore(8),
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
			if b.uploader != nil {
				b.uploader.stop()
			}
			return nil
		case upd := <-updates:
			if err := b.handleUpdate(upd); err != nil {
				log.Printf("update error: %v", err)
			}
		}
	}
}
