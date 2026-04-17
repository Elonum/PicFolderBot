package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"PicFolderBot/internal/cache"
	"PicFolderBot/internal/config"
	"PicFolderBot/internal/parser"
	"PicFolderBot/internal/service"
	"PicFolderBot/internal/telegram"
	"PicFolderBot/internal/yadisk"

	"github.com/redis/go-redis/v9"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
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
		log.Fatalf("telegram init error: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err = bot.Run(ctx); err != nil {
		log.Fatalf("bot stopped with error: %v", err)
	}
}
