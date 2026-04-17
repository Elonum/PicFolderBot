package config

import "testing"

func TestLoadReadsEnv(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "tg")
	t.Setenv("YANDEX_OAUTH_TOKEN", "ya")
	t.Setenv("YANDEX_ROOT_PATH", "disk:/Root")
	t.Setenv("YANDEX_TIMEOUT_SEC", "10")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if cfg.TelegramToken != "tg" || cfg.YandexToken != "ya" {
		t.Fatalf("unexpected tokens: %#v", cfg)
	}
	if cfg.YandexRootPath != "disk:/Root" {
		t.Fatalf("unexpected root: %s", cfg.YandexRootPath)
	}
	if cfg.YandexTimeout.Seconds() != 10 {
		t.Fatalf("unexpected timeout: %v", cfg.YandexTimeout)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("unexpected log level: %s", cfg.LogLevel)
	}
	if cfg.LogFormat != "text" {
		t.Fatalf("unexpected log format: %s", cfg.LogFormat)
	}
	if cfg.AlertChannelID != 0 {
		t.Fatalf("unexpected alert channel id: %d", cfg.AlertChannelID)
	}
	if cfg.AlertBotToken != "tg" {
		t.Fatalf("unexpected alert bot token fallback: %s", cfg.AlertBotToken)
	}
	if cfg.AlertNotifyOnStartup {
		t.Fatalf("unexpected startup alert flag: %v", cfg.AlertNotifyOnStartup)
	}
	if cfg.HealthAddr != ":8080" {
		t.Fatalf("unexpected health addr: %s", cfg.HealthAddr)
	}
	if cfg.ShutdownTimeout.Seconds() != 20 {
		t.Fatalf("unexpected shutdown timeout: %v", cfg.ShutdownTimeout)
	}
}

func TestLoadFailsOnMissingVars(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	t.Setenv("YANDEX_OAUTH_TOKEN", "")
	_, err := Load()
	if err == nil {
		t.Fatalf("expected error for missing vars")
	}
}

func TestLoadFailsOnInvalidAlertChannelID(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "tg")
	t.Setenv("YANDEX_OAUTH_TOKEN", "ya")
	t.Setenv("TELEGRAM_ALERT_CHANNEL_ID", "@my_channel")
	_, err := Load()
	if err == nil {
		t.Fatalf("expected error for invalid TELEGRAM_ALERT_CHANNEL_ID")
	}
}
