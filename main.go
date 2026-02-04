package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"time"

	"go_payment_bot/config"
	"go_payment_bot/database"
	"go_payment_bot/handlers"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Файл .env не найден, используем переменные окружения")
	}

	cfg := config.Load()

	if cfg.BotToken == "" {
		log.Fatal("BOT_TOKEN не установлен")
	}
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL не установлен")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Подключение к БД
	db, err := database.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Ошибка подключения к БД: %v", err)
	}
	defer db.Close()
	log.Println("Подключено к БД")

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

	h := handlers.New(b, cfg, db, botUsername)

	// Загружаем разрешённые домены
	h.LoadAllowedDomains(ctx)

	// Перезагрузка каждый час (опционально)
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.LoadAllowedDomains(context.Background())
			}
		}
	}()

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

	// Проверка просроченных постов каждые 5 минут
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
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
