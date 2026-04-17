package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

type CriticalNotifier interface {
	Notify(ctx context.Context, text string) error
}

var (
	mu            sync.RWMutex
	base          = newDefaultLogger()
	criticalSink  CriticalNotifier
	alertCooldown = 2 * time.Minute
	lastAlertAt   = map[string]time.Time{}
)

func Init(levelText string, format string) {
	mu.Lock()
	defer mu.Unlock()
	base = newLogger(levelFromText(levelText), format)
}

func SetCriticalNotifier(sink CriticalNotifier, cooldown time.Duration) {
	mu.Lock()
	defer mu.Unlock()
	criticalSink = sink
	if cooldown > 0 {
		alertCooldown = cooldown
	}
}

func Info(msg string, args ...any)  { logger().Info(msg, args...) }
func Warn(msg string, args ...any)  { logger().Warn(msg, args...) }
func Error(msg string, args ...any) { logger().Error(msg, args...) }

func Alert(msg string, args ...any) {
	l := logger()
	l.Error(msg, append(args, "severity", "alert")...)
	notifyWithCooldown("alert", msg, args...)
}

func Critical(msg string, args ...any) {
	l := logger()
	l.Error(msg, append(args, "severity", "critical")...)
	notifyWithCooldown("critical", msg, args...)
}

func notifyWithCooldown(severity string, msg string, args ...any) {
	mu.Lock()
	sink := criticalSink
	key := alertKey(severity, msg, args...)
	last, ok := lastAlertAt[key]
	if ok && time.Since(last) < alertCooldown {
		mu.Unlock()
		return
	}
	lastAlertAt[key] = time.Now()
	mu.Unlock()

	if sink == nil {
		return
	}
	text := formatAlert(severity, msg, args...)
	go func() {
		if err := sink.Notify(context.Background(), text); err != nil {
			logger().Warn("failed to send critical alert", "error", err)
		}
	}()
}

func logger() *slog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return base
}

func newDefaultLogger() *slog.Logger {
	return newLogger(slog.LevelInfo, "text")
}

func newLogger(level slog.Level, format string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	default:
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
}

func levelFromText(v string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func formatAlert(severity string, msg string, args ...any) string {
	sb := strings.Builder{}
	sb.WriteString("🚨 PicFolderBot alert\n")
	sb.WriteString(fmt.Sprintf("severity: %s\n", strings.ToUpper(strings.TrimSpace(severity))))
	sb.WriteString(fmt.Sprintf("time: %s\n", time.Now().Format(time.RFC3339)))
	sb.WriteString("service: PicFolderBot\n")
	sb.WriteString(msg)
	if len(args) == 0 {
		return sb.String()
	}
	sb.WriteString("\n")
	allowed := map[string]bool{
		"component": true,
		"op":        true,
		"user_id":   true,
		"chat_id":   true,
		"status":    true,
		"attempts":  true,
		"path":      true,
		"error":     true,
	}
	for i := 0; i+1 < len(args); i += 2 {
		k, ok := args[i].(string)
		if !ok || !allowed[k] {
			continue
		}
		sb.WriteString(fmt.Sprintf("- %s: %v\n", k, args[i+1]))
	}
	return strings.TrimSpace(sb.String())
}

func alertKey(severity string, msg string, args ...any) string {
	var b strings.Builder
	b.WriteString(severity)
	b.WriteString("|")
	b.WriteString(msg)
	for i := 0; i+1 < len(args); i += 2 {
		key, _ := args[i].(string)
		if key == "error" || key == "component" || key == "op" {
			b.WriteString("|")
			b.WriteString(fmt.Sprintf("%v=%v", args[i], args[i+1]))
		}
	}
	return b.String()
}
