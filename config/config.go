package config

import (
	"os"
	"strconv"
)

type Config struct {
	BotToken             string
	PaymentProviderToken string
	DatabaseURL          string
	LogChannelID         int64

	// Дефолтные значения для новых тем
	DefaultPrice        int
	DefaultDurationDays int
	DefaultMaxTextLen   int
	DefaultMaxPhotos    int

	TestMode bool
}

func Load() *Config {
	price, _ := strconv.Atoi(getEnv("DEFAULT_PRICE", "50000"))
	duration, _ := strconv.Atoi(getEnv("DEFAULT_DURATION_DAYS", "7"))
	maxText, _ := strconv.Atoi(getEnv("DEFAULT_MAX_TEXT_LENGTH", "1000"))
	maxPhotos, _ := strconv.Atoi(getEnv("DEFAULT_MAX_PHOTOS", "5"))
	logChannel, _ := strconv.ParseInt(getEnv("LOG_CHANNEL_ID", "0"), 10, 64)

	return &Config{
		BotToken:             getEnv("BOT_TOKEN", ""),
		PaymentProviderToken: getEnv("PAYMENT_PROVIDER_TOKEN", ""),
		DatabaseURL:          getEnv("DATABASE_URL", ""),
		DefaultPrice:         price,
		DefaultDurationDays:  duration,
		DefaultMaxTextLen:    maxText,
		DefaultMaxPhotos:     maxPhotos,
		LogChannelID:         logChannel,
		TestMode:             getEnv("TEST_MODE", "false") == "true",
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
