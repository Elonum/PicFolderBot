package telegram

import (
	"fmt"
	"strings"

	"PicFolderBot/internal/logging"
)

type telegramAPILogger struct{}

func (telegramAPILogger) Println(v ...interface{}) {
	logTelegramSDKMessage(strings.TrimSpace(fmt.Sprint(v...)))
}

func (telegramAPILogger) Printf(format string, v ...interface{}) {
	logTelegramSDKMessage(strings.TrimSpace(fmt.Sprintf(format, v...)))
}

func logTelegramSDKMessage(msg string) {
	if msg == "" {
		return
	}
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "failed to get updates") || strings.Contains(lower, "getupdates") {
		logging.Warn("telegram long-poll transient issue", "component", "telegram", "error", msg)
		return
	}
	logging.Info("telegram sdk", "component", "telegram", "message", msg)
}
