# Telegram Services Bot

Бот для платного размещения услуг в теме Telegram-группы.

## Как работает

```
Пользователь пишет в тему → Бот удаляет → Предлагает оплату →
→ Оплата → Пользователь шлёт контент → Бот публикует в тему
```

## Запуск

### Локально

```bash
cp .env.example .env
# заполнить .env

go mod tidy
go run .
```

### Docker

```bash
cp .env.example .env
# заполнить .env

docker-compose up -d
```

## Настройка

| Переменная | Описание |
|------------|----------|
| `BOT_TOKEN` | Токен от @BotFather |
| `GROUP_ID` | ID супергруппы (с минусом) |
| `SERVICES_TOPIC_ID` | ID темы «Услуги» |
| `PAYMENT_PROVIDER_TOKEN` | Токен платёжного провайдера |
| `SERVICE_PRICE` | Цена в копейках (50000 = 500₽) |
| `POST_DURATION_DAYS` | Срок размещения |

## Получение ID

**GROUP_ID**: добавить бота в группу → отправить сообщение → открыть `https://api.telegram.org/bot<TOKEN>/getUpdates`

**SERVICES_TOPIC_ID**: ПКМ по теме → «Скопировать ссылку» → число после последнего `/`

**PAYMENT_PROVIDER_TOKEN**: @BotFather → /mybots → бот → Payments → подключить провайдера
