package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"PicFolderBot/internal/config"
	"PicFolderBot/internal/parser"
	"PicFolderBot/internal/service"
	"PicFolderBot/internal/telegram"
	"PicFolderBot/internal/yadisk"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	diskClient := yadisk.NewClient(cfg.YandexToken, cfg.YandexTimeout)
	flow := service.NewFlow(diskClient, cfg.YandexRootPath, parser.ParseCaption)

	bot, err := telegram.NewBot(cfg.TelegramToken, flow)
	if err != nil {
		log.Fatalf("telegram init error: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err = bot.Run(ctx); err != nil {
		log.Fatalf("bot stopped with error: %v", err)
	}
}
