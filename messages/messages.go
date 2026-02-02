package messages

import "fmt"

const (
	MsgDeleted = `🚫 Ваше сообщение удалено.

Размещение услуг — платное. Для офомления нажмите кнопку далее`

	MsgPaymentSuccess = `✅ Оплата прошла!

Пришлите объявление:
• Текст с описанием
• Фото (до %d шт.)
• Контакты

⚠️ Одним сообщением.`

	MsgContentAccepted = `✅ Принято! Публикую...`

	MsgPostPublished = `🎉 Опубликовано на %d дней!`

	MsgPaymentRequired = `💳 Сначала оплатите размещение.`

	MsgPaymentExpired = `⏰ Время истекло (24ч). Оплатите снова.`

	MsgSendTextOrPhoto = `❌ Отправьте текст или фото.`

	MsgError = `❌ Ошибка. Попробуйте позже.`

	MsgWelcome = `👋 Бот для платных объявлений.

💰 Стоимость: %d ₽ за %d дней`
)

func FormatDeleted(price, days int) string {
	return fmt.Sprintf(MsgDeleted, price/100, days)
}

func FormatPaymentSuccess(maxPhotos int) string {
	return fmt.Sprintf(MsgPaymentSuccess, maxPhotos)
}

func FormatPublished(days int) string {
	return fmt.Sprintf(MsgPostPublished, days)
}

func FormatWelcome(price, days int) string {
	return fmt.Sprintf(MsgWelcome, price/100, days)
}
