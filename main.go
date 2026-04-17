package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"PicFolderBot/internal/alerts"
	"PicFolderBot/internal/cache"
	"PicFolderBot/internal/config"
	"PicFolderBot/internal/health"
	"PicFolderBot/internal/logging"
	"PicFolderBot/internal/parser"
	"PicFolderBot/internal/service"
	"PicFolderBot/internal/telegram"
	"PicFolderBot/internal/yadisk"

	"github.com/redis/go-redis/v9"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		logging.Critical("config load failed", "component", "main", "error", err)
		os.Exit(1)
	}
	logging.Init(cfg.LogLevel, cfg.LogFormat)

	var startupAlertNotifier *alerts.TelegramNotifier
	if cfg.AlertChannelID != 0 {
		notifier, alertErr := alerts.NewTelegramNotifier(cfg.AlertBotToken, cfg.AlertChannelID, cfg.YandexTimeout)
		if alertErr != nil {
			logging.Error("failed to initialize telegram alert notifier", "component", "main", "error", alertErr)
		} else {
			logging.SetCriticalNotifier(notifier, cfg.AlertCooldown)
			startupAlertNotifier = notifier
			logging.Info("critical alerts enabled", "component", "main", "channel_id", cfg.AlertChannelID)
		}
	}

	diskClient := yadisk.NewClient(cfg.YandexToken, cfg.YandexTimeout)
	var (
		treeCache    cache.TreeCache
		sessionStore telegram.SessionStore
		albumStore   telegram.AlbumStore
	)
	if cfg.RedisAddr != "" {
		redisClient := redis.NewClient(&redis.Options{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		})
		treeCache = cache.NewRedisTreeCache(redisClient, cfg.CacheTTL)
		sessionStore = telegram.NewRedisSessionStore(redisClient, cfg.StateTTL)
		albumStore = telegram.NewRedisAlbumStore(redisClient, cfg.StateTTL)
	} else {
		treeCache = cache.NewMemoryTreeCache(cfg.CacheTTL)
		sessionStore = telegram.NewMemorySessionStore(cfg.StateTTL)
		albumStore = telegram.NewMemoryAlbumStore(cfg.StateTTL)
	}
	flow := service.NewFlow(diskClient, cfg.YandexRootPath, parser.ParseCaption, service.WithTreeCache(treeCache))

	bot, err := telegram.NewBot(cfg.TelegramToken, flow, sessionStore, albumStore)
	if err != nil {
		logging.Critical("telegram init failed", "component", "main", "error", err)
		os.Exit(1)
	}
	bot.SetShutdownTimeout(cfg.ShutdownTimeout)
	if cfg.AlertNotifyOnStartup && startupAlertNotifier != nil {
		host, _ := os.Hostname()
		alertText := fmt.Sprintf(
			"✅ Bot startup\nservice=PicFolderBot\nhost=%s\nroot=%s\ntime=%s",
			host,
			cfg.YandexRootPath,
			time.Now().Format(time.RFC3339),
		)
		if alertErr := startupAlertNotifier.Notify(context.Background(), alertText); alertErr != nil {
			logging.Warn("failed to send startup alert", "component", "main", "error", alertErr)
		} else {
			logging.Info("startup alert sent", "component", "main")
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	healthSrv := health.NewServer(cfg.HealthAddr)
	go func() {
		if serveErr := healthSrv.Start(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logging.Warn("health server stopped with error", "component", "main", "error", serveErr)
		}
	}()
	healthSrv.SetReady(true)

	if err = bot.Run(ctx); err != nil {
		logging.Critical("bot stopped with error", "component", "main", "error", err)
		os.Exit(1)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := healthSrv.Shutdown(shutdownCtx); err != nil {
		logging.Warn("health server shutdown error", "component", "main", "error", err)
	}
}
