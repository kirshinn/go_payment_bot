package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"time"

	"go_payment_bot/config"
	"go_payment_bot/handlers"
	"go_payment_bot/storage"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		return
	}
	cfg := config.Load()

	if cfg.BotToken == "" {
		log.Fatal("BOT_TOKEN не установлен")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	store := storage.New()

	opts := []bot.Option{
		bot.WithDefaultHandler(func(ctx context.Context, b *bot.Bot, update *models.Update) {}),
	}

	b, err := bot.New(cfg.BotToken, opts...)
	if err != nil {
		log.Fatal(err)
	}

	// Получаем username бота
	me, err := b.GetMe(ctx)
	if err != nil {
		log.Fatal(err)
	}
	botUsername := me.Username
	log.Printf("Бот @%s запущен", botUsername)

	h := handlers.New(b, cfg, store, botUsername)

	b.RegisterHandler(bot.HandlerTypeMessageText, "", bot.MatchTypePrefix, h.OnMessage)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "", bot.MatchTypePrefix, h.OnCallback)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.Message != nil && len(update.Message.Photo) > 0
	}, h.OnMessage)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.Message != nil && update.Message.SuccessfulPayment != nil
	}, h.OnMessage)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.PreCheckoutQuery != nil
	}, h.OnPreCheckout)

	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.DeleteExpiredPosts(ctx)
			}
		}
	}()

	log.Println("Бот запущен")
	b.Start(ctx)
}
