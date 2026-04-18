package telegram

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"PicFolderBot/internal/logging"
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
	flushMu      sync.Mutex
	flushing     map[string]struct{}
	prefetchMu   sync.Mutex
	prefetchLast map[string]time.Time
	uploader     *uploader
	stopTimeout  time.Duration
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
	_ = tgbotapi.SetLogger(telegramAPILogger{})

	httpClient := &http.Client{
		Timeout: 70 * time.Second,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	api, err := tgbotapi.NewBotAPIWithClient(strings.TrimSpace(token), tgbotapi.APIEndpoint, httpClient)
	if err != nil {
		return nil, err
	}
	_, _ = api.Request(tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "upload", Description: cmdDescUpload},
		tgbotapi.BotCommand{Command: "search", Description: cmdDescSearch},
		tgbotapi.BotCommand{Command: "recent", Description: cmdDescRecent},
		tgbotapi.BotCommand{Command: "help", Description: cmdDescHelp},
		tgbotapi.BotCommand{Command: "cancel", Description: cmdDescCancel},
	))
	return &Bot{
		api:          api,
		flow:         flow,
		rootName:     flow.RootDisplayName(),
		sessionStore: sessionStore,
		albumStore:   albumStore,
		albums:       make(map[string]*albumBuffer),
		flushing:     make(map[string]struct{}),
		prefetchLast: make(map[string]time.Time),
		uploader:     newUploader(flow, uploadWorkers, 256),
		stopTimeout:  20 * time.Second,
		recent:       NewMemoryRecentStore(8),
	}, nil
}

func (b *Bot) SetShutdownTimeout(timeout time.Duration) {
	if timeout > 0 {
		b.stopTimeout = timeout
	}
}

func (b *Bot) Run(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	updates := b.api.GetUpdatesChan(u)
	logging.Info("telegram bot started", "component", "telegram", "bot_username", b.api.Self.UserName)
	for {
		select {
		case <-ctx.Done():
			b.api.StopReceivingUpdates()
			if b.uploader != nil {
				if err := b.uploader.stopWithTimeout(context.Background(), b.stopTimeout); err != nil {
					logging.Warn("uploader shutdown timed out", "component", "telegram", "error", err)
				}
			}
			return nil
		case upd := <-updates:
			func() {
				requestID := logging.NewRequestID(upd.UpdateID)
				chatID := extractChatID(upd)
				defer func() {
					if r := recover(); r != nil {
						if chatID != 0 {
							logging.Critical(logging.MsgUpdatePanic(), "component", "telegram", "request_id", requestID, "update_id", upd.UpdateID, "user_id", chatID, "error", fmt.Errorf("panic: %v", r))
							return
						}
						logging.Critical(logging.MsgUpdatePanic(), "component", "telegram", "request_id", requestID, "update_id", upd.UpdateID, "error", fmt.Errorf("panic: %v", r))
					}
				}()
				if err := b.handleUpdate(upd); err != nil {
					if chatID != 0 {
						logging.Error("update processing failed", "component", "telegram", "request_id", requestID, "update_id", upd.UpdateID, "user_id", chatID, "error", err)
					} else {
						logging.Error("update processing failed", "component", "telegram", "request_id", requestID, "update_id", upd.UpdateID, "error", err)
					}
					if strings.Contains(strings.ToLower(err.Error()), "unauthorized") || strings.Contains(strings.ToLower(err.Error()), "(401)") {
						if chatID != 0 {
							logging.Alert(logging.MsgUserFlowAuthFailed(), "component", "telegram", "request_id", requestID, "update_id", upd.UpdateID, "user_id", chatID, "error", err)
						} else {
							logging.Alert(logging.MsgUserFlowAuthFailed(), "component", "telegram", "request_id", requestID, "update_id", upd.UpdateID, "error", err)
						}
					}
				}
			}()
		}
	}
}

func extractChatID(upd tgbotapi.Update) int64 {
	if upd.Message != nil && upd.Message.Chat != nil {
		return upd.Message.Chat.ID
	}
	if upd.CallbackQuery != nil && upd.CallbackQuery.Message != nil && upd.CallbackQuery.Message.Chat != nil {
		return upd.CallbackQuery.Message.Chat.ID
	}
	return 0
}
