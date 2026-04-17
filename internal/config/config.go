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
	TelegramToken  string
	YandexToken    string
	YandexRootPath string
	YandexTimeout  time.Duration
	RedisAddr      string
	RedisPassword  string
	RedisDB        int
	StateTTL       time.Duration
	CacheTTL       time.Duration
}

func Load() (Config, error) {
	// Best-effort .env loading for local/dev runs.
	// Real environment variables still have priority.
	_ = godotenv.Load()

	cfg := Config{
		TelegramToken:  strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		YandexToken:    strings.TrimSpace(os.Getenv("YANDEX_OAUTH_TOKEN")),
		YandexRootPath: strings.TrimSpace(os.Getenv("YANDEX_ROOT_PATH")),
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
