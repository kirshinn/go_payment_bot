package messages

import "fmt"

const (
	MsgDeleted = `🚫 Ваше сообщение удалено.

Размещение услуг — платное. Для продолжения нажмите кнопку далее`

	MsgPaymentSuccess = `✅ Оплата прошла!

Пришлите объявление:
• Текст с описанием
• Фото (до %d шт.)
• Контакты

⚠️ Одним сообщением.`

	MsgContentAccepted = `✅ Принято! Публикую...`

	MsgPostPublished = `🎉 Опубликовано на %d дней!`

	MsgPaymentRequired = `💳 Для размещения объявления напишите в тему группы и оплатите размещение.`

	MsgPaymentExpired = `⏰ Время истекло (24ч). Оплатите снова.`

	MsgSendTextOrPhoto = `❌ Отправьте текст или фото.`

	MsgError = `❌ Ошибка. Попробуйте позже.`

	MsgWelcome = `👋 Бот для платных объявлений.

💰 Стоимость: %d ₽ за %d дней`

	MsgReloadContent = `🔄 Отправьте объявление заново:
• Текст с описанием
• Фото (до %d шт.)
• Контакты

⚠️ Одним сообщением.`

	MsgExpiredReminder = `⏰ Срок вашего объявления в теме «%s» истёк и оно удалено.

Хотите разместить заново? 💰 %d ₽ за %d дней.`
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

func FormatSpamWarning(userID int64, firstName string) string {
	return fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>, ваше сообщение удалено.

⚠️ Публикация номеров телефонов, личных контактов и коротких ссылок запрещена.

Платные объявления — только в разделе «Услуги».`, userID, firstName)
}

func FormatReloadContent(maxPhotos int) string {
	return fmt.Sprintf(MsgReloadContent, maxPhotos)
}

func FormatExpiredReminder(topicTitle string, price, days int) string {
	return fmt.Sprintf(MsgExpiredReminder, topicTitle, price/100, days)
}
