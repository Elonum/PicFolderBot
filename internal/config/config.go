package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	TelegramToken        string
	YandexToken          string
	YandexRootPath       string
	YandexTimeout        time.Duration
	LogLevel             string
	LogFormat            string
	AlertChannelID       int64
	AlertBotToken        string
	AlertCooldown        time.Duration
	AlertNotifyOnStartup bool
	HealthAddr           string
	ShutdownTimeout      time.Duration
	RedisAddr            string
	RedisPassword        string
	RedisDB              int
	StateTTL             time.Duration
	CacheTTL             time.Duration
}

func Load() (Config, error) {
	// Best-effort .env loading for local/dev runs.
	// Real environment variables still have priority.
	_ = godotenv.Load()

	cfg := Config{
		TelegramToken:  strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		YandexToken:    strings.TrimSpace(os.Getenv("YANDEX_OAUTH_TOKEN")),
		YandexRootPath: strings.TrimSpace(os.Getenv("YANDEX_ROOT_PATH")),
		LogLevel:       strings.TrimSpace(os.Getenv("LOG_LEVEL")),
		LogFormat:      strings.TrimSpace(os.Getenv("LOG_FORMAT")),
		AlertBotToken:  strings.TrimSpace(os.Getenv("TELEGRAM_ALERT_BOT_TOKEN")),
		HealthAddr:     strings.TrimSpace(os.Getenv("HEALTH_ADDR")),
		RedisAddr:      strings.TrimSpace(os.Getenv("REDIS_ADDR")),
		RedisPassword:  strings.TrimSpace(os.Getenv("REDIS_PASSWORD")),
	}

	if cfg.YandexRootPath == "" {
		cfg.YandexRootPath = "disk:/Товары Innogods"
	}

	timeout, err := readTimeoutSec()
	if err != nil {
		return Config{}, err
	}
	cfg.YandexTimeout = timeout
	cfg.RedisDB = readIntOrDefault("REDIS_DB", 0)
	cfg.StateTTL = readDurationMinutes("STATE_TTL_MINUTES", 1440)
	cfg.CacheTTL = readDurationMinutes("CACHE_TTL_MINUTES", 10)
	alertChannelRaw := strings.TrimSpace(os.Getenv("TELEGRAM_ALERT_CHANNEL_ID"))
	if alertChannelRaw != "" {
		alertID, parseErr := strconv.ParseInt(alertChannelRaw, 10, 64)
		if parseErr != nil {
			return Config{}, fmt.Errorf("TELEGRAM_ALERT_CHANNEL_ID must be numeric (example: -1001234567890)")
		}
		cfg.AlertChannelID = alertID
	}
	cfg.AlertCooldown = readDurationMinutes("ALERT_COOLDOWN_MINUTES", 5)
	cfg.AlertNotifyOnStartup = readBool("ALERT_NOTIFY_ON_STARTUP", false)
	cfg.ShutdownTimeout = readDurationSeconds("SHUTDOWN_TIMEOUT_SEC", 20)
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.LogFormat == "" {
		cfg.LogFormat = "text"
	}
	if cfg.AlertBotToken == "" {
		cfg.AlertBotToken = cfg.TelegramToken
	}
	if cfg.HealthAddr == "" {
		cfg.HealthAddr = ":8080"
	}

	var missing []string
	if cfg.TelegramToken == "" {
		missing = append(missing, "TELEGRAM_BOT_TOKEN")
	}
	if cfg.YandexToken == "" {
		missing = append(missing, "YANDEX_OAUTH_TOKEN")
	}
	if len(missing) > 0 {
		return Config{}, errors.New("missing env vars: " + strings.Join(missing, ", "))
	}

	return cfg, nil
}

func readInt64OrDefault(key string, fallback int64) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return v
}

func readIntOrDefault(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		return fallback
	}
	return v
}

func readDurationMinutes(key string, fallback int) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return time.Duration(fallback) * time.Minute
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return time.Duration(fallback) * time.Minute
	}
	return time.Duration(v) * time.Minute
}

func readTimeoutSec() (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv("YANDEX_TIMEOUT_SEC"))
	if raw == "" {
		return 25 * time.Second, nil
	}

	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("YANDEX_TIMEOUT_SEC must be a positive integer")
	}
	return time.Duration(v) * time.Second, nil
}

func readBool(key string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func readDurationSeconds(key string, fallback int) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return time.Duration(fallback) * time.Second
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return time.Duration(fallback) * time.Second
	}
	return time.Duration(v) * time.Second
}
