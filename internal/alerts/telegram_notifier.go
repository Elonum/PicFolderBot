package alerts

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type TelegramNotifier struct {
	api       *tgbotapi.BotAPI
	channelID int64
	timeout   time.Duration
}

func NewTelegramNotifier(botToken string, channelID int64, timeout time.Duration) (*TelegramNotifier, error) {
	token := strings.TrimSpace(botToken)
	if token == "" {
		return nil, fmt.Errorf("alert bot token is empty")
	}
	if channelID == 0 {
		return nil, fmt.Errorf("alert channel id is empty")
	}
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &TelegramNotifier{
		api:       api,
		channelID: channelID,
		timeout:   timeout,
	}, nil
}

func (n *TelegramNotifier) Notify(ctx context.Context, text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, n.timeout)
	defer cancel()

	msg := tgbotapi.NewMessage(n.channelID, text)
	msg.DisableWebPagePreview = true

	done := make(chan error, 1)
	go func() {
		_, err := n.api.Send(msg)
		done <- err
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}
