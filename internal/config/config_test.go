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
}

func TestLoadFailsOnMissingVars(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	t.Setenv("YANDEX_OAUTH_TOKEN", "")
	_, err := Load()
	if err == nil {
		t.Fatalf("expected error for missing vars")
	}
}
