package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"go_payment_bot/config"
	"go_payment_bot/database"
	"go_payment_bot/messages"
	"go_payment_bot/moderation"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/jackc/pgx/v5"
)

type Handler struct {
	bot             *bot.Bot
	cfg             *config.Config
	db              *database.DB
	botUsername     string
	allowedDomains  []string
	mediaGroupCache map[string]bool
	mediaGroupMu    sync.Mutex
}

func New(b *bot.Bot, cfg *config.Config, db *database.DB, username string) *Handler {
	return &Handler{
		bot:             b,
		cfg:             cfg,
		db:              db,
		botUsername:     username,
		mediaGroupCache: make(map[string]bool),
	}
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

	// === МОДЕРАЦИЯ СПАМА ВО ВСЕХ ТОПИКАХ ===
	if msg.Chat.Type == "supergroup" && msg.From != nil && !msg.From.IsBot {
		text := msg.Text
		if msg.Caption != "" {
			text = msg.Caption
		}

		if text != "" {
			if violation := moderation.Check(text, h.allowedDomains); violation != nil {
				h.handleSpamViolation(ctx, msg, violation)
				return
			}
		}
	}

	// Проверяем, это сообщение в отслеживаемой теме (платные объявления)?
	if msg.Chat.Type == "supergroup" && msg.MessageThreadID != 0 {
		topic, err := h.db.GetTopicByGroupAndTopicID(ctx, msg.Chat.ID, msg.MessageThreadID)
		if err == nil && topic.IsActive {
			if msg.From != nil && !msg.From.IsBot {
				h.onServicesTopicMessage(ctx, msg, topic)
			}
			return
		}
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

	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})

	// Формат: skip_email_<topic_id>
	if strings.HasPrefix(cb.Data, "skip_email_") {
		topicIDStr := strings.TrimPrefix(cb.Data, "skip_email_")
		topicID, err := strconv.Atoi(topicIDStr)
		if err != nil {
			return
		}

		// Отмечаем что отказался от email
		_ = h.db.SetUserEmailDeclined(ctx, cb.From.ID)
		_ = h.db.UpdateUserState(ctx, cb.From.ID, database.StateNone, &topicID)

		topic, err := h.db.GetTopicByID(ctx, topicID)
		if err != nil {
			return
		}

		// Показываем кнопку оплаты
		_, _ = h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: cb.From.ID,
			Text:   messages.FormatWelcome(topic.Price, topic.DurationDays),
			ReplyMarkup: &models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{{
					{Text: "💳 Оплатить", CallbackData: fmt.Sprintf("pay_%d", topic.ID)},
				}},
			},
		})
		return
	}

	// Формат: pay_<topic_id>
	if strings.HasPrefix(cb.Data, "pay_") {
		topicIDStr := strings.TrimPrefix(cb.Data, "pay_")
		topicID, err := strconv.Atoi(topicIDStr)
		if err != nil {
			return
		}
		h.sendInvoice(ctx, cb.From.ID, topicID)
	}
}

func (h *Handler) OnPreCheckout(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.PreCheckoutQuery == nil {
		return
	}
	_, _ = b.AnswerPreCheckoutQuery(ctx, &bot.AnswerPreCheckoutQueryParams{
		PreCheckoutQueryID: update.PreCheckoutQuery.ID,
		OK:                 true,
	})
}

func (h *Handler) onServicesTopicMessage(ctx context.Context, msg *models.Message, topic *database.Topic) {
	// Удаляем сообщение
	_, err := h.bot.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    msg.Chat.ID,
		MessageID: msg.ID,
	})
	if err != nil {
		log.Printf("Ошибка удаления: %v", err)
	}

	// Создаём/обновляем пользователя
	username := ptrStr(msg.From.Username)
	firstName := ptrStr(msg.From.FirstName)
	lastName := ptrStr(msg.From.LastName)
	_, _ = h.db.GetOrCreateUser(ctx, msg.From.ID, username, firstName, lastName)

	// Отправляем предупреждение с кнопкой оплаты
	text := fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>, для оформления нажмите кнопку ниже.`,
		msg.From.ID, msg.From.FirstName)

	warning, err := h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		MessageThreadID: topic.TopicID,
		Text:            text,
		ParseMode:       models.ParseModeHTML,
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{{
				{Text: "💳 Оплатить размещение", URL: fmt.Sprintf("https://t.me/%s?start=pay_%d", h.botUsername, topic.ID)},
			}},
		},
	})
	if err != nil {
		log.Printf("Ошибка отправки: %v", err)
		return
	}

	// Удаляем предупреждение через 60 сек
	go func() {
		time.Sleep(60 * time.Second)
		_, _ = h.bot.DeleteMessage(context.Background(), &bot.DeleteMessageParams{
			ChatID:    msg.Chat.ID,
			MessageID: warning.ID,
		})
	}()
}

func (h *Handler) onPrivateMessage(ctx context.Context, msg *models.Message) {
	userID := msg.From.ID

	// Создаём/получаем пользователя
	username := ptrStr(msg.From.Username)
	firstName := ptrStr(msg.From.FirstName)
	lastName := ptrStr(msg.From.LastName)
	user, err := h.db.GetOrCreateUser(ctx, userID, username, firstName, lastName)
	if err != nil {
		log.Printf("Ошибка получения пользователя: %v", err)
		return
	}

	// Проверка на бан
	if user.State == database.StateBanned {
		h.send(ctx, userID, "🚫 Вы заблокированы.")
		return
	}

	// /start pay_<topic_id>
	if strings.HasPrefix(msg.Text, "/start pay_") {
		topicIDStr := strings.TrimPrefix(msg.Text, "/start pay_")
		topicID, err := strconv.Atoi(topicIDStr)
		if err != nil {
			return
		}

		topic, err := h.db.GetTopicByID(ctx, topicID)
		if err != nil {
			h.send(ctx, userID, "❌ Тема не найдена.")
			return
		}

		// Сохраняем выбранную тему
		_ = h.db.UpdateUserState(ctx, userID, database.StateNone, &topicID)

		// Проверяем, нужно ли спрашивать email
		if user.Email == nil && !user.EmailDeclined {
			// Спрашиваем email
			_ = h.db.UpdateUserState(ctx, userID, database.StateWaitingEmail, &topicID)
			_, _ = h.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: userID,
				Text: fmt.Sprintf("📧 Укажите email для получения чеков и информационных сообщений.\n\n💰 Стоимость размещения: %d ₽ на %d дней",
					topic.Price/100, topic.DurationDays),
				ReplyMarkup: &models.InlineKeyboardMarkup{
					InlineKeyboard: [][]models.InlineKeyboardButton{
						{{Text: "❌ Пропустить", CallbackData: fmt.Sprintf("skip_email_%d", topic.ID)}},
					},
				},
			})
			return
		}

		// Email уже есть или отказались — сразу к оплате
		text := messages.FormatWelcome(topic.Price, topic.DurationDays)
		_, _ = h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: userID,
			Text:   text,
			ReplyMarkup: &models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{{
					{Text: "💳 Оплатить", CallbackData: fmt.Sprintf("pay_%d", topic.ID)},
				}},
			},
		})
		return
	}

	// Обычный /start
	if strings.HasPrefix(msg.Text, "/start") {
		h.send(ctx, userID, "👋 Для размещения объявления напишите в соответствующую тему группы.")
		return
	}

	// Тестовая оплата
	if strings.HasPrefix(msg.Text, "/testpay") && h.cfg.TestMode {
		if user.CurrentTopicID == nil {
			h.send(ctx, userID, "❌ Сначала выберите тему для размещения.")
			return
		}
		topic, err := h.db.GetTopicByID(ctx, *user.CurrentTopicID)
		if err != nil {
			return
		}
		_ = h.db.MarkUserPaid(ctx, userID, topic.ID)
		_, _ = h.db.CreatePayment(ctx, userID, topic.ID, "test_payment", topic.Price, "RUB")
		h.send(ctx, userID, messages.FormatPaymentSuccess(topic.MaxPhotos))
		return
	}

	// Ожидаем email
	if user.State == database.StateWaitingEmail {
		email := strings.TrimSpace(msg.Text)
		if !isValidEmail(email) {
			h.send(ctx, userID, "❌ Неверный формат email. Попробуйте ещё раз или нажмите «Пропустить».")
			return
		}

		// Сохраняем email
		_ = h.db.SetUserEmail(ctx, userID, email)
		h.send(ctx, userID, fmt.Sprintf("✅ Email %s сохранён!", email))

		// Показываем кнопку оплаты
		if user.CurrentTopicID != nil {
			topic, err := h.db.GetTopicByID(ctx, *user.CurrentTopicID)
			if err == nil {
				_ = h.db.UpdateUserState(ctx, userID, database.StateNone, user.CurrentTopicID)
				_, _ = h.bot.SendMessage(ctx, &bot.SendMessageParams{
					ChatID: userID,
					Text:   messages.FormatWelcome(topic.Price, topic.DurationDays),
					ReplyMarkup: &models.InlineKeyboardMarkup{
						InlineKeyboard: [][]models.InlineKeyboardButton{{
							{Text: "💳 Оплатить", CallbackData: fmt.Sprintf("pay_%d", topic.ID)},
						}},
					},
				})
			}
		}
		return
	}

	// Ожидаем контент
	if user.State == database.StateWaitingContent {
		if user.PaidAt != nil && time.Since(*user.PaidAt) > 24*time.Hour {
			_ = h.db.ResetUser(ctx, userID)
			h.send(ctx, userID, messages.MsgPaymentExpired)
			return
		}
		h.onContentSubmit(ctx, msg, user)
		return
	}

	// По умолчанию
	h.send(ctx, userID, "💳 Для размещения объявления напишите в тему группы и оплатите размещение.")
}

func (h *Handler) sendInvoice(ctx context.Context, userID int64, topicID int) {
	topic, err := h.db.GetTopicByID(ctx, topicID)
	if err != nil {
		log.Printf("Тема не найдена: %v", err)
		return
	}

	// Сохраняем выбранную тему
	_ = h.db.UpdateUserState(ctx, userID, database.StateWaitingPayment, &topicID)

	_, err = h.bot.SendInvoice(ctx, &bot.SendInvoiceParams{
		ChatID:        userID,
		Title:         "Размещение объявления",
		Description:   fmt.Sprintf("Публикация на %d дней в теме «%s»", topic.DurationDays, topic.Title),
		Payload:       fmt.Sprintf("topic_%d_user_%d_%d", topicID, userID, time.Now().Unix()),
		ProviderToken: h.cfg.PaymentProviderToken,
		Currency:      "RUB",
		Prices: []models.LabeledPrice{{
			Label:  "Размещение",
			Amount: topic.Price,
		}},
	})
	if err != nil {
		log.Printf("Ошибка отправки инвойса: %v", err)
	}
}

func (h *Handler) onPaymentSuccess(ctx context.Context, msg *models.Message) {
	userID := msg.From.ID
	p := msg.SuccessfulPayment

	log.Printf("Оплата: user=%d amount=%d %s", userID, p.TotalAmount, p.Currency)

	// Получаем пользователя
	user, err := h.db.GetUser(ctx, userID)
	if err != nil || user.CurrentTopicID == nil {
		log.Printf("Ошибка: пользователь или тема не найдены")
		return
	}

	topic, err := h.db.GetTopicByID(ctx, *user.CurrentTopicID)
	if err != nil {
		return
	}

	// Сохраняем платёж
	_, _ = h.db.CreatePayment(ctx, userID, topic.ID, p.TelegramPaymentChargeID, p.TotalAmount, p.Currency)

	// Обновляем статус
	_ = h.db.MarkUserPaid(ctx, userID, topic.ID)

	h.send(ctx, userID, messages.FormatPaymentSuccess(topic.MaxPhotos))
}

func (h *Handler) onContentSubmit(ctx context.Context, msg *models.Message, user *database.User) {
	userID := msg.From.ID

	// Проверка на media group (несколько фото)
	if msg.MediaGroupID != "" {
		h.mediaGroupMu.Lock()
		if h.mediaGroupCache[msg.MediaGroupID] {
			// Уже обработали первое фото из этой группы — игнорируем остальные
			h.mediaGroupMu.Unlock()
			return
		}
		h.mediaGroupCache[msg.MediaGroupID] = true
		h.mediaGroupMu.Unlock()

		// Очистка кэша через 1 минуту
		go func(groupID string) {
			time.Sleep(1 * time.Minute)
			h.mediaGroupMu.Lock()
			delete(h.mediaGroupCache, groupID)
			h.mediaGroupMu.Unlock()
		}(msg.MediaGroupID)

		// Предупреждаем что взяли только первое фото
		h.send(ctx, userID, "⚠️ Вы отправили несколько фото. Будет использовано только первое.")
	}

	if user.CurrentTopicID == nil {
		h.send(ctx, userID, messages.MsgError)
		return
	}

	topic, err := h.db.GetTopicByID(ctx, *user.CurrentTopicID)
	if err != nil {
		h.send(ctx, userID, messages.MsgError)
		return
	}

	hasContent := msg.Text != "" || msg.Caption != "" || len(msg.Photo) > 0
	if !hasContent {
		h.send(ctx, userID, messages.MsgSendTextOrPhoto)
		return
	}

	// Проверка длины текста
	text := msg.Text
	if msg.Caption != "" {
		text = msg.Caption
	}
	if len(text) > topic.MaxTextLength {
		h.send(ctx, userID, fmt.Sprintf("❌ Текст слишком длинный. Максимум %d символов.", topic.MaxTextLength))
		return
	}

	// Если модерация включена
	if topic.ModerationEnabled {
		var photoIDs []string
		if len(msg.Photo) > 0 {
			photoIDs = []string{msg.Photo[len(msg.Photo)-1].FileID}
		}
		_, err := h.db.CreatePendingPost(ctx, userID, topic.ID, &text, photoIDs)
		if err != nil {
			h.send(ctx, userID, messages.MsgError)
			return
		}
		_ = h.db.UpdateUserState(ctx, userID, database.StateWaitingModeration, user.CurrentTopicID)
		h.send(ctx, userID, "⏳ Ваше объявление отправлено на модерацию.")
		return
	}

	// Публикуем сразу
	h.send(ctx, userID, messages.MsgContentAccepted)

	formattedText := h.formatPost(msg)
	var sentMsg *models.Message

	if len(msg.Photo) > 0 {
		photo := msg.Photo[len(msg.Photo)-1]
		sentMsg, err = h.bot.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID:          topic.GroupID,
			MessageThreadID: topic.TopicID,
			Photo:           &models.InputFileString{Data: photo.FileID},
			Caption:         formattedText,
			ParseMode:       models.ParseModeHTML,
		})
	} else {
		sentMsg, err = h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          topic.GroupID,
			MessageThreadID: topic.TopicID,
			Text:            formattedText,
			ParseMode:       models.ParseModeHTML,
		})
	}

	if err != nil {
		log.Printf("Ошибка публикации: %v", err)
		h.send(ctx, userID, messages.MsgError)
		return
	}

	// Сохраняем пост
	expires := time.Now().Add(time.Duration(topic.DurationDays) * 24 * time.Hour)
	var photoIDs []string
	if len(msg.Photo) > 0 {
		photoIDs = []string{msg.Photo[len(msg.Photo)-1].FileID}
	}
	_, _ = h.db.CreatePost(ctx, sentMsg.ID, topic.ID, userID, &text, photoIDs, expires)

	// Сбрасываем состояние
	_ = h.db.ResetUser(ctx, userID)

	h.send(ctx, userID, messages.FormatPublished(topic.DurationDays))
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
	_, _ = h.bot.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text})
}

func (h *Handler) DeleteExpiredPosts(ctx context.Context) {
	posts, err := h.db.GetExpiredPosts(ctx)
	if err != nil {
		log.Printf("Ошибка получения просроченных постов: %v", err)
		return
	}

	for _, p := range posts {
		_, err := h.bot.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    p.ChatID,
			MessageID: p.MessageID,
		})
		if err != nil {
			log.Printf("Ошибка удаления поста %d: %v", p.MessageID, err)
		}

		_ = h.db.MarkPostDeleted(ctx, p.ID)
		log.Printf("Удалён пост %d (chat=%d)", p.MessageID, p.ChatID)
	}
}

func (h *Handler) handleSpamViolation(ctx context.Context, msg *models.Message, violation *moderation.Violation) {
	// Удаляем сообщение
	_, err := h.bot.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    msg.Chat.ID,
		MessageID: msg.ID,
	})
	if err != nil {
		log.Printf("Ошибка удаления спам-сообщения: %v", err)
	}

	// Сохраняем нарушение в БД
	text := msg.Text
	if msg.Caption != "" {
		text = msg.Caption
	}
	var topicID *int
	if msg.MessageThreadID != 0 {
		topicID = &msg.MessageThreadID
	}
	_ = h.db.CreateSpamViolation(ctx, msg.From.ID, msg.Chat.ID, topicID, text, string(violation.Type), violation.Match)

	// Отправляем предупреждение в топик
	warning, err := h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		MessageThreadID: msg.MessageThreadID,
		Text: fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>, ваше сообщение удалено.

⚠️ Публикация номеров телефонов, личных контактов и коротких ссылок запрещена.

Коммерческие объявления — только в разделе «Услуги».`,
			msg.From.ID, msg.From.FirstName),
		ParseMode: models.ParseModeHTML,
	})
	if err != nil {
		log.Printf("Ошибка отправки предупреждения: %v", err)
		return
	}

	// Удаляем предупреждение через 30 сек
	go func() {
		time.Sleep(30 * time.Second)
		_, _ = h.bot.DeleteMessage(context.Background(), &bot.DeleteMessageParams{
			ChatID:    msg.Chat.ID,
			MessageID: warning.ID,
		})
	}()

	log.Printf("Спам от user=%d: type=%s match=%s", msg.From.ID, violation.Type, violation.Match)
}

func (h *Handler) LoadAllowedDomains(ctx context.Context) {
	domains, err := h.db.GetAllowedDomains(ctx)
	if err != nil {
		log.Printf("Ошибка загрузки разрешённых доменов: %v", err)
		return
	}
	h.allowedDomains = domains
	log.Printf("Загружено %d разрешённых доменов", len(domains))
}

// Хелпер для указателя на строку
func ptrStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Валидация email
func isValidEmail(email string) bool {
	if len(email) < 5 || len(email) > 254 {
		return false
	}
	at := strings.Index(email, "@")
	dot := strings.LastIndex(email, ".")
	return at > 0 && dot > at+1 && dot < len(email)-1
}

// Для проверки ошибки "not found"
func isNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
