package handlers

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"go_payment_bot/config"
	"go_payment_bot/messages"
	"go_payment_bot/storage"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type Handler struct {
	bot         *bot.Bot
	cfg         *config.Config
	storage     *storage.Storage
	botUsername string
}

func New(b *bot.Bot, cfg *config.Config, store *storage.Storage, username string) *Handler {
	return &Handler{bot: b, cfg: cfg, storage: store, botUsername: username}
}

func (h *Handler) OnMessage(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}
	msg := update.Message

	if msg.SuccessfulPayment != nil {
		h.onPaymentSuccess(ctx, msg)
		return
	}

	if h.isServicesTopic(msg) {
		h.onServicesTopicMessage(ctx, msg)
		return
	}

	if msg.Chat.Type == "private" {
		h.onPrivateMessage(ctx, msg)
	}
}

func (h *Handler) OnCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}
	cb := update.CallbackQuery

	_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
	if err != nil {
		return
	}

	if cb.Data == "pay" {
		h.sendInvoice(ctx, cb.From.ID)
	}
}

func (h *Handler) OnPreCheckout(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.PreCheckoutQuery == nil {
		return
	}
	_, err := b.AnswerPreCheckoutQuery(ctx, &bot.AnswerPreCheckoutQueryParams{
		PreCheckoutQueryID: update.PreCheckoutQuery.ID,
		OK:                 true,
	})
	if err != nil {
		return
	}
}

func (h *Handler) isServicesTopic(msg *models.Message) bool {
	return msg.Chat.ID == h.cfg.GroupID &&
		msg.MessageThreadID == h.cfg.ServicesTopicID &&
		msg.From != nil &&
		!msg.From.IsBot
}

func (h *Handler) onServicesTopicMessage(ctx context.Context, msg *models.Message) {
	_, err := h.bot.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    msg.Chat.ID,
		MessageID: msg.ID,
	})
	if err != nil {
		log.Printf("Ошибка удаления: %v", err)
	}

	// HTML вместо Markdown — не нужно экранировать
	text := fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>, для оформления нажмите кнопку ниже.`,
		msg.From.ID, msg.From.FirstName)

	warning, err := h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		MessageThreadID: h.cfg.ServicesTopicID,
		Text:            text,
		ParseMode:       models.ParseModeHTML, // <-- HTML
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{{
				{Text: "💳 Оплатить размещение", URL: "https://t.me/" + h.botUsername + "?start=pay"},
			}},
		},
	})
	if err != nil {
		log.Printf("Ошибка отправки: %v", err)
		return
	}

	go func() {
		time.Sleep(60 * time.Second)
		_, err := h.bot.DeleteMessage(context.Background(), &bot.DeleteMessageParams{
			ChatID:    msg.Chat.ID,
			MessageID: warning.ID,
		})
		if err != nil {
			return
		}
	}()
}

func (h *Handler) onPrivateMessage(ctx context.Context, msg *models.Message) {
	userID := msg.From.ID

	if strings.HasPrefix(msg.Text, "/start") {
		text := messages.FormatWelcome(h.cfg.ServicePrice, h.cfg.PostDurationDays)
		_, err := h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: userID,
			Text:   text,
			ReplyMarkup: &models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{{
					{Text: "💳 Оплатить", CallbackData: "pay"},
				}},
			},
		})
		if err != nil {
			return
		}
		return
	}

	// Тестовая оплата
	if strings.HasPrefix(msg.Text, "/testpay") && h.cfg.TestMode {
		h.storage.MarkPaid(userID, "test_payment")
		h.send(ctx, userID, messages.FormatPaymentSuccess(h.cfg.MaxPhotos))
		return
	}

	user := h.storage.GetUser(userID)

	if user.State == storage.StateWaitingContent {
		if time.Since(user.PaidAt) > 24*time.Hour {
			h.storage.ResetUser(userID)
			h.send(ctx, userID, messages.MsgPaymentExpired)
			return
		}
		h.onContentSubmit(ctx, msg)
		return
	}

	_, err := h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: userID,
		Text:   messages.MsgPaymentRequired,
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{{
				{Text: "💳 Оплатить", CallbackData: "pay"},
			}},
		},
	})
	if err != nil {
		return
	}
}

func (h *Handler) sendInvoice(ctx context.Context, userID int64) {
	_, err := h.bot.SendInvoice(ctx, &bot.SendInvoiceParams{
		ChatID:        userID,
		Title:         "Размещение объявления",
		Description:   fmt.Sprintf("Публикация на %d дней", h.cfg.PostDurationDays),
		Payload:       fmt.Sprintf("svc_%d_%d", userID, time.Now().Unix()),
		ProviderToken: h.cfg.PaymentProviderToken,
		Currency:      "RUB",
		Prices: []models.LabeledPrice{{
			Label:  "Размещение",
			Amount: h.cfg.ServicePrice,
		}},
	})
	if err != nil {
		return
	}
}

func (h *Handler) onPaymentSuccess(ctx context.Context, msg *models.Message) {
	userID := msg.From.ID
	p := msg.SuccessfulPayment

	log.Printf("Оплата: user=%d amount=%d %s", userID, p.TotalAmount, p.Currency)

	h.storage.MarkPaid(userID, p.TelegramPaymentChargeID)
	h.send(ctx, userID, messages.FormatPaymentSuccess(h.cfg.MaxPhotos))
}

func (h *Handler) onContentSubmit(ctx context.Context, msg *models.Message) {
	userID := msg.From.ID

	hasContent := msg.Text != "" || msg.Caption != "" || len(msg.Photo) > 0
	if !hasContent {
		h.send(ctx, userID, messages.MsgSendTextOrPhoto)
		return
	}

	h.send(ctx, userID, messages.MsgContentAccepted)

	text := h.formatPost(msg)
	var sentMsg *models.Message
	var err error

	if len(msg.Photo) > 0 {
		photo := msg.Photo[len(msg.Photo)-1]
		sentMsg, err = h.bot.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID:          h.cfg.GroupID,
			MessageThreadID: h.cfg.ServicesTopicID,
			Photo:           &models.InputFileString{Data: photo.FileID},
			Caption:         text,
			ParseMode:       models.ParseModeHTML,
		})
	} else {
		sentMsg, err = h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          h.cfg.GroupID,
			MessageThreadID: h.cfg.ServicesTopicID,
			Text:            text,
			ParseMode:       models.ParseModeHTML,
		})
	}

	if err != nil {
		log.Printf("Ошибка публикации: %v", err)
		h.send(ctx, userID, messages.MsgError)
		return
	}

	expires := time.Now().Add(time.Duration(h.cfg.PostDurationDays) * 24 * time.Hour)
	h.storage.AddPost(sentMsg.ID, userID, expires)
	h.storage.ResetUser(userID)

	h.send(ctx, userID, messages.FormatPublished(h.cfg.PostDurationDays))
}

func (h *Handler) formatPost(msg *models.Message) string {
	text := msg.Text
	if msg.Caption != "" {
		text = msg.Caption
	}

	name := msg.From.FirstName
	if msg.From.LastName != "" {
		name += " " + msg.From.LastName
	}

	result := fmt.Sprintf("🛠 <b>Услуга</b>\n\n%s\n\n👤 %s", text, name)
	if msg.From.Username != "" {
		result += fmt.Sprintf(" (@%s)", msg.From.Username)
	}
	return result
}

func (h *Handler) send(ctx context.Context, chatID int64, text string) {
	_, err := h.bot.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text})
	if err != nil {
		return
	}
}

func (h *Handler) DeleteExpiredPosts(ctx context.Context) {
	for _, p := range h.storage.GetExpiredPosts() {
		_, err := h.bot.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    h.cfg.GroupID,
			MessageID: p.MessageID,
		})
		if err != nil {
			return
		}
		h.storage.RemovePost(p.MessageID)
		log.Printf("Удалён пост %d", p.MessageID)
	}
}
