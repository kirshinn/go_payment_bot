package tglog

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

var (
	b         *bot.Bot
	channelID int64
	enabled   bool
)

// Init инициализирует логгер в TG-канал
func Init(tgBot *bot.Bot, chID int64) {
	if chID == 0 {
		log.Println("LOG_CHANNEL_ID не задан, логирование в канал отключено")
		return
	}
	b = tgBot
	channelID = chID
	enabled = true
	log.Printf("Логирование в канал %d включено", chID)
}

// Send отправляет сообщение в лог-канал (неблокирующий)
func Send(format string, args ...any) {
	if !enabled {
		return
	}
	text := fmt.Sprintf(format, args...)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    channelID,
			Text:      text,
			ParseMode: models.ParseModeHTML,
		})
		if err != nil {
			log.Printf("Ошибка отправки лога в канал: %v", err)
		}
	}()
}
