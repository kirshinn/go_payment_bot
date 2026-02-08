package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
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

// PendingContent хранит контент объявления до подтверждения
type PendingContent struct {
	Text              string
	PhotoIDs          []string
	ReceivedAt        time.Time
	PreviewMessageIDs []int // ID сообщений media group для удаления при confirm/reload
}

// mediaGroupPhoto хранит фото с ID сообщения для сортировки
type mediaGroupPhoto struct {
	MessageID int
	PhotoID   string
}

// MediaGroupData хранит данные о фото из media group
type MediaGroupData struct {
	Photos    []mediaGroupPhoto
	Text      string
	UserID    int64
	Timer     *time.Timer
	Processed bool
}

type Handler struct {
	bot             *bot.Bot
	cfg             *config.Config
	db              *database.DB
	botUsername     string
	allowedDomains  []string
	mediaGroupCache map[string]*MediaGroupData // MediaGroupID -> данные группы
	mediaGroupMu    sync.Mutex
	pendingContent  map[int64]*PendingContent // UserID -> контент для предпросмотра
	pendingMu       sync.Mutex
}

func New(b *bot.Bot, cfg *config.Config, db *database.DB, username string) *Handler {
	return &Handler{
		bot:             b,
		cfg:             cfg,
		db:              db,
		botUsername:     username,
		mediaGroupCache: make(map[string]*MediaGroupData),
		pendingContent:  make(map[int64]*PendingContent),
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

	// Подтверждение публикации
	if cb.Data == "confirm_publish" {
		h.handleConfirmPublish(ctx, cb)
		return
	}

	// Загрузить заново
	if cb.Data == "reload_content" {
		h.handleReloadContent(ctx, cb)
		return
	}

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

// deleteCallbackMessage безопасно удаляет сообщение из callback
func (h *Handler) deleteCallbackMessage(ctx context.Context, cb *models.CallbackQuery) {
	// MaybeInaccessibleMessage — обращаемся к полю Message
	if cb.Message.Message != nil {
		_, _ = h.bot.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    cb.From.ID,
			MessageID: cb.Message.Message.ID,
		})
	}
}

// handleConfirmPublish обрабатывает подтверждение публикации
func (h *Handler) handleConfirmPublish(ctx context.Context, cb *models.CallbackQuery) {
	userID := cb.From.ID

	// Удаляем сообщение с кнопками и фото превью
	h.deleteCallbackMessage(ctx, cb)
	h.deletePreviewMessages(ctx, userID)

	// Получаем пользователя
	user, err := h.db.GetUser(ctx, userID)
	if err != nil || user.CurrentTopicID == nil {
		h.send(ctx, userID, messages.MsgError)
		return
	}

	// Проверяем состояние
	if user.State != database.StateWaitingConfirm {
		h.send(ctx, userID, "❌ Объявление уже опубликовано или отменено.")
		return
	}

	// Получаем сохранённый контент
	content := h.getPendingContent(userID)
	if content == nil {
		h.send(ctx, userID, "❌ Контент не найден. Отправьте объявление заново.")
		_ = h.db.UpdateUserState(ctx, userID, database.StateWaitingContent, user.CurrentTopicID)
		return
	}

	topic, err := h.db.GetTopicByID(ctx, *user.CurrentTopicID)
	if err != nil {
		h.send(ctx, userID, messages.MsgError)
		return
	}

	// Если модерация включена
	if topic.ModerationEnabled {
		_, err := h.db.CreatePendingPost(ctx, userID, topic.ID, &content.Text, content.PhotoIDs)
		if err != nil {
			h.send(ctx, userID, messages.MsgError)
			return
		}
		_ = h.db.UpdateUserState(ctx, userID, database.StateWaitingModeration, user.CurrentTopicID)
		h.clearPendingContent(userID)
		h.send(ctx, userID, "⏳ Ваше объявление отправлено на модерацию.")
		return
	}

	// Публикуем
	h.send(ctx, userID, messages.MsgContentAccepted)
	h.publishPost(ctx, userID, user, topic, content)
}

// handleReloadContent обрабатывает запрос на повторную загрузку
func (h *Handler) handleReloadContent(ctx context.Context, cb *models.CallbackQuery) {
	userID := cb.From.ID

	// Удаляем сообщение с кнопками и фото превью
	h.deleteCallbackMessage(ctx, cb)
	h.deletePreviewMessages(ctx, userID)

	// Получаем пользователя
	user, err := h.db.GetUser(ctx, userID)
	if err != nil || user.CurrentTopicID == nil {
		h.send(ctx, userID, messages.MsgError)
		return
	}

	// Очищаем сохранённый контент
	h.clearPendingContent(userID)

	// Возвращаем в состояние ожидания контента
	_ = h.db.UpdateUserState(ctx, userID, database.StateWaitingContent, user.CurrentTopicID)

	topic, err := h.db.GetTopicByID(ctx, *user.CurrentTopicID)
	if err != nil {
		h.send(ctx, userID, messages.MsgError)
		return
	}

	h.send(ctx, userID, messages.FormatReloadContent(topic.MaxPhotos))
}

// publishPost публикует объявление в группу
func (h *Handler) publishPost(ctx context.Context, userID int64, user *database.User, topic *database.Topic, content *PendingContent) {
	formattedText := h.formatPostFromContent(userID, content)
	var sentMsg *models.Message
	var allMessageIDs []int
	var err error

	if len(content.PhotoIDs) == 0 {
		// Только текст
		sentMsg, err = h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          topic.GroupID,
			MessageThreadID: topic.TopicID,
			Text:            formattedText,
			ParseMode:       models.ParseModeHTML,
		})
		if sentMsg != nil {
			allMessageIDs = []int{sentMsg.ID}
		}
	} else if len(content.PhotoIDs) == 1 {
		// Одно фото
		sentMsg, err = h.bot.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID:          topic.GroupID,
			MessageThreadID: topic.TopicID,
			Photo:           &models.InputFileString{Data: content.PhotoIDs[0]},
			Caption:         formattedText,
			ParseMode:       models.ParseModeHTML,
		})
		if sentMsg != nil {
			allMessageIDs = []int{sentMsg.ID}
		}
	} else {
		// Несколько фото - используем SendMediaGroup
		media := make([]models.InputMedia, len(content.PhotoIDs))
		for i, photoID := range content.PhotoIDs {
			inputPhoto := &models.InputMediaPhoto{
				Media: photoID,
			}
			// Подпись только к первому фото
			if i == 0 {
				inputPhoto.Caption = formattedText
				inputPhoto.ParseMode = models.ParseModeHTML
			}
			media[i] = inputPhoto
		}

		sentMsgs, mediaErr := h.bot.SendMediaGroup(ctx, &bot.SendMediaGroupParams{
			ChatID:          topic.GroupID,
			MessageThreadID: topic.TopicID,
			Media:           media,
		})
		err = mediaErr
		if len(sentMsgs) > 0 {
			sentMsg = sentMsgs[0]
			for _, m := range sentMsgs {
				allMessageIDs = append(allMessageIDs, m.ID)
			}
		}
	}

	if err != nil {
		log.Printf("Ошибка публикации: %v", err)
		h.send(ctx, userID, messages.MsgError)
		// Возвращаем в состояние ожидания контента
		_ = h.db.UpdateUserState(ctx, userID, database.StateWaitingContent, user.CurrentTopicID)
		return
	}

	// Сохраняем пост (проверяем что sentMsg не nil)
	if sentMsg != nil {
		expires := time.Now().Add(time.Duration(topic.DurationDays) * 24 * time.Hour)
		_, _ = h.db.CreatePost(ctx, sentMsg.ID, allMessageIDs, topic.ID, userID, &content.Text, content.PhotoIDs, expires)
	}

	// Очищаем контент и сбрасываем состояние
	h.clearPendingContent(userID)
	_ = h.db.ResetUser(ctx, userID)

	h.send(ctx, userID, messages.FormatPublished(topic.DurationDays))
}

// formatPostFromContent форматирует пост из сохранённого контента
func (h *Handler) formatPostFromContent(userID int64, content *PendingContent) string {
	// Получаем данные пользователя
	ctx := context.Background()
	user, err := h.db.GetUser(ctx, userID)
	if err != nil {
		return content.Text
	}

	name := ""
	if user.FirstName != nil {
		name = *user.FirstName
	}
	if user.LastName != nil && *user.LastName != "" {
		name += " " + *user.LastName
	}

	result := fmt.Sprintf("🛠 <b>Услуга</b>\n\n%s\n\n👤 %s", content.Text, name)
	if user.Username != nil && *user.Username != "" {
		result += fmt.Sprintf(" (@%s)", *user.Username)
	}
	return result
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
				Text:   "📧 Укажите email для получения чеков и информационных сообщений.\n",
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

	// Ожидаем подтверждения публикации
	if user.State == database.StateWaitingConfirm {
		h.send(ctx, userID, "⚠️ У вас есть неопубликованное объявление.\n\nИспользуйте кнопки выше для подтверждения или отмены.")
		return
	}

	// По умолчанию
	h.send(ctx, userID, messages.MsgPaymentRequired)
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
		Title:         "размещение объявления",
		Description:   fmt.Sprintf("Публикация на %d дней в теме «%s»", topic.DurationDays, topic.Title),
		Payload:       fmt.Sprintf("topic_%d_user_%d_%d", topicID, userID, time.Now().Unix()),
		ProviderToken: h.cfg.PaymentProviderToken,
		Currency:      "RUB",
		Prices: []models.LabeledPrice{{
			Label:  "размещение",
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

	if user.CurrentTopicID == nil {
		h.send(ctx, userID, messages.MsgError)
		return
	}

	topic, err := h.db.GetTopicByID(ctx, *user.CurrentTopicID)
	if err != nil {
		h.send(ctx, userID, messages.MsgError)
		return
	}

	// Получаем текст
	text := msg.Text
	if msg.Caption != "" {
		text = msg.Caption
	}

	// Получаем фото (берём максимальный размер)
	var photoID string
	if len(msg.Photo) > 0 {
		photoID = msg.Photo[len(msg.Photo)-1].FileID
	}

	// Если это media group (несколько фото отправленных вместе)
	if msg.MediaGroupID != "" {
		h.handleMediaGroup(ctx, msg, user, topic, text, photoID)
		return
	}

	// Одиночное сообщение (текст или одно фото)
	hasContent := text != "" || photoID != ""
	if !hasContent {
		h.send(ctx, userID, messages.MsgSendTextOrPhoto)
		return
	}

	// Проверка длины текста
	if len(text) > topic.MaxTextLength {
		h.send(ctx, userID, fmt.Sprintf("❌ Текст слишком длинный. Максимум %d символов.", topic.MaxTextLength))
		return
	}

	// Сохраняем контент для предпросмотра
	var photoIDs []string
	if photoID != "" {
		photoIDs = []string{photoID}
	}
	h.savePendingContent(userID, text, photoIDs)

	// Показываем предпросмотр
	h.showPreview(ctx, userID, user, topic)
}

// handleMediaGroup собирает все фото из media group
func (h *Handler) handleMediaGroup(ctx context.Context, msg *models.Message, user *database.User, topic *database.Topic, text, photoID string) {
	userID := msg.From.ID
	groupID := msg.MediaGroupID

	h.mediaGroupMu.Lock()
	defer h.mediaGroupMu.Unlock()

	// Проверяем, есть ли уже данные для этой группы
	data, exists := h.mediaGroupCache[groupID]
	if !exists {
		// Создаём новую запись
		data = &MediaGroupData{
			Photos:    []mediaGroupPhoto{},
			UserID:    userID,
			Processed: false,
		}
		h.mediaGroupCache[groupID] = data
	}

	// Добавляем фото с ID сообщения для сортировки
	if photoID != "" {
		data.Photos = append(data.Photos, mediaGroupPhoto{
			MessageID: msg.ID,
			PhotoID:   photoID,
		})
	}

	// Сохраняем текст (обычно он приходит только с первым фото)
	if text != "" && data.Text == "" {
		data.Text = text
	}

	// Отменяем предыдущий таймер если был
	if data.Timer != nil {
		data.Timer.Stop()
	}

	// Создаём новый таймер - ждём 1.5 секунды после последнего фото
	data.Timer = time.AfterFunc(1500*time.Millisecond, func() {
		h.processMediaGroup(groupID, user, topic)
	})
}

// processMediaGroup обрабатывает собранную media group
func (h *Handler) processMediaGroup(groupID string, user *database.User, topic *database.Topic) {
	h.mediaGroupMu.Lock()
	data, exists := h.mediaGroupCache[groupID]
	if !exists || data.Processed {
		h.mediaGroupMu.Unlock()
		return
	}
	data.Processed = true

	// Копируем данные и сортируем фото по message_id
	userID := data.UserID
	text := data.Text
	photos := make([]mediaGroupPhoto, len(data.Photos))
	copy(photos, data.Photos)

	// Сортируем по message_id для правильного порядка
	sort.Slice(photos, func(i, j int) bool {
		return photos[i].MessageID < photos[j].MessageID
	})

	// Извлекаем отсортированные photoIDs
	photoIDs := make([]string, len(photos))
	for i, p := range photos {
		photoIDs[i] = p.PhotoID
	}

	// Удаляем из кэша через минуту (для защиты от повторов)
	go func() {
		time.Sleep(1 * time.Minute)
		h.mediaGroupMu.Lock()
		delete(h.mediaGroupCache, groupID)
		h.mediaGroupMu.Unlock()
	}()

	h.mediaGroupMu.Unlock()

	ctx := context.Background()

	// Проверка количества фото
	if len(photoIDs) > topic.MaxPhotos {
		h.send(ctx, userID, fmt.Sprintf("⚠️ Вы отправили %d фото, максимум %d. Будут использованы первые %d.",
			len(photoIDs), topic.MaxPhotos, topic.MaxPhotos))
		photoIDs = photoIDs[:topic.MaxPhotos]
	}

	// Проверка длины текста
	if len(text) > topic.MaxTextLength {
		h.send(ctx, userID, fmt.Sprintf("❌ Текст слишком длинный. Максимум %d символов.", topic.MaxTextLength))
		return
	}

	// Сохраняем контент для предпросмотра
	h.savePendingContent(userID, text, photoIDs)

	// Показываем предпросмотр
	h.showPreview(ctx, userID, user, topic)
}

// savePendingContent сохраняет контент для предпросмотра
func (h *Handler) savePendingContent(userID int64, text string, photoIDs []string) {
	h.pendingMu.Lock()
	defer h.pendingMu.Unlock()
	h.pendingContent[userID] = &PendingContent{
		Text:       text,
		PhotoIDs:   photoIDs,
		ReceivedAt: time.Now(),
	}
}

// getPendingContent получает сохранённый контент
func (h *Handler) getPendingContent(userID int64) *PendingContent {
	h.pendingMu.Lock()
	defer h.pendingMu.Unlock()
	return h.pendingContent[userID]
}

// clearPendingContent удаляет сохранённый контент
func (h *Handler) clearPendingContent(userID int64) {
	h.pendingMu.Lock()
	defer h.pendingMu.Unlock()
	delete(h.pendingContent, userID)
}

// setPreviewMessageIDs сохраняет ID сообщений превью для последующего удаления
func (h *Handler) setPreviewMessageIDs(userID int64, msgIDs []int) {
	h.pendingMu.Lock()
	defer h.pendingMu.Unlock()
	if content, ok := h.pendingContent[userID]; ok {
		content.PreviewMessageIDs = msgIDs
	}
}

// deletePreviewMessages удаляет сообщения media group из превью
func (h *Handler) deletePreviewMessages(ctx context.Context, userID int64) {
	content := h.getPendingContent(userID)
	if content == nil {
		return
	}
	for _, msgID := range content.PreviewMessageIDs {
		_, _ = h.bot.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    userID,
			MessageID: msgID,
		})
	}
}

// showPreview показывает предпросмотр объявления
func (h *Handler) showPreview(ctx context.Context, userID int64, user *database.User, topic *database.Topic) {
	content := h.getPendingContent(userID)
	if content == nil {
		h.send(ctx, userID, messages.MsgError)
		return
	}

	// Обновляем состояние пользователя
	_ = h.db.UpdateUserState(ctx, userID, database.StateWaitingConfirm, user.CurrentTopicID)

	// Формируем текст предпросмотра
	previewText := "📋 <b>Предпросмотр объявления:</b>\n\n"
	previewText += "━━━━━━━━━━━━━━━\n"
	if content.Text != "" {
		previewText += content.Text + "\n"
	}
	previewText += "━━━━━━━━━━━━━━━\n\n"

	if len(content.PhotoIDs) > 0 {
		previewText += fmt.Sprintf("📷 Фото: %d шт.\n\n", len(content.PhotoIDs))
	}

	previewText += "Подтвердите публикацию или загрузите заново."

	// Отправляем предпросмотр с кнопками
	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "✅ Опубликовать", CallbackData: "confirm_publish"},
				{Text: "🔄 Загрузить заново", CallbackData: "reload_content"},
			},
		},
	}

	if len(content.PhotoIDs) > 1 {
		// Несколько фото — отправляем media group, затем текст с кнопками
		media := make([]models.InputMedia, len(content.PhotoIDs))
		for i, photoID := range content.PhotoIDs {
			media[i] = &models.InputMediaPhoto{Media: photoID}
		}

		sentMsgs, err := h.bot.SendMediaGroup(ctx, &bot.SendMediaGroupParams{
			ChatID: userID,
			Media:  media,
		})
		if err != nil {
			log.Printf("Ошибка отправки фото предпросмотра: %v", err)
			h.send(ctx, userID, messages.MsgError)
			return
		}

		// Сохраняем ID сообщений media group для удаления при confirm/reload
		var msgIDs []int
		for _, m := range sentMsgs {
			msgIDs = append(msgIDs, m.ID)
		}
		h.setPreviewMessageIDs(userID, msgIDs)

		// Отдельное сообщение с текстом и кнопками
		_, err = h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      userID,
			Text:        previewText,
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: keyboard,
		})
		if err != nil {
			log.Printf("Ошибка отправки предпросмотра: %v", err)
			h.send(ctx, userID, messages.MsgError)
		}
	} else if len(content.PhotoIDs) == 1 {
		// Одно фото — отправляем с подписью и кнопками
		_, err := h.bot.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID:      userID,
			Photo:       &models.InputFileString{Data: content.PhotoIDs[0]},
			Caption:     previewText,
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: keyboard,
		})
		if err != nil {
			log.Printf("Ошибка отправки предпросмотра: %v", err)
			h.send(ctx, userID, messages.MsgError)
		}
	} else {
		// Без фото — только текст с кнопками
		_, err := h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      userID,
			Text:        previewText,
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: keyboard,
		})
		if err != nil {
			log.Printf("Ошибка отправки предпросмотра: %v", err)
			h.send(ctx, userID, messages.MsgError)
		}
	}
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
		// Удаляем все сообщения поста (media group или одиночное)
		msgIDs := p.AllMessageIDs
		if len(msgIDs) == 0 {
			// Для старых постов без all_message_ids
			msgIDs = []int{p.MessageID}
		}
		for _, msgID := range msgIDs {
			_, err := h.bot.DeleteMessage(ctx, &bot.DeleteMessageParams{
				ChatID:    p.ChatID,
				MessageID: msgID,
			})
			if err != nil {
				log.Printf("Ошибка удаления сообщения %d: %v", msgID, err)
			}
		}

		_ = h.db.MarkPostDeleted(ctx, p.ID)
		log.Printf("Удалён пост %d (chat=%d, сообщений: %d)", p.MessageID, p.ChatID, len(msgIDs))

		// Напоминание пользователю о переопубликации
		topic, err := h.db.GetTopicByID(ctx, p.InternalTopicID)
		if err != nil {
			log.Printf("Ошибка получения темы %d: %v", p.InternalTopicID, err)
			continue
		}

		_, _ = h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: p.UserID,
			Text:   messages.FormatExpiredReminder(topic.Title, topic.Price, topic.DurationDays),
			ReplyMarkup: &models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{{
					{Text: "🔄 Разместить заново", URL: fmt.Sprintf("https://t.me/%s?start=pay_%d", h.botUsername, topic.ID)},
				}},
			},
		})
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
		Text:            messages.FormatSpamWarning(msg.From.ID, msg.From.FirstName),
		ParseMode:       models.ParseModeHTML,
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
