.PHONY: migrate-up migrate-down migrate-create run

# Загружаем .env
include .env
export

# Миграции
migrate-up:
	migrate -path migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path migrations -database "$(DATABASE_URL)" down 1

migrate-create:
	@read -p "Migration name: " name; \
	migrate create -ext sql -dir migrations -seq $$name

# Запуск
run:
	go run .

# Сборка
build:
	go build -o bin/bot .
