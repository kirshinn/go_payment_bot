-- Схема БД: go_payment_bot
-- PostgreSQL 17

CREATE TYPE user_state AS ENUM (
    'none',
    'waiting_email',
    'waiting_payment',
    'waiting_content',
    'waiting_moderation',
    'banned'
);

CREATE TABLE groups (
    id BIGINT PRIMARY KEY,
    title VARCHAR(255),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE topics (
    id SERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    topic_id INTEGER NOT NULL,
    title VARCHAR(255),
    price INTEGER NOT NULL DEFAULT 50000,
    duration_days INTEGER NOT NULL DEFAULT 7,
    max_photos INTEGER NOT NULL DEFAULT 5,
    max_text_length INTEGER NOT NULL DEFAULT 1000,
    moderation_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (group_id, topic_id)
);

CREATE TABLE users (
    id BIGINT PRIMARY KEY,
    username VARCHAR(255),
    first_name VARCHAR(255),
    last_name VARCHAR(255),
    email VARCHAR(255),
    email_declined BOOLEAN NOT NULL DEFAULT FALSE,
    state user_state NOT NULL DEFAULT 'none',
    current_topic_id INTEGER REFERENCES topics(id),
    paid_at TIMESTAMPTZ,
    receipt_sent_at TIMESTAMPTZ,
    banned_at TIMESTAMPTZ,
    ban_reason TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE pending_posts (
    id SERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    topic_id INTEGER NOT NULL REFERENCES topics(id),
    content_text TEXT,
    photo_file_ids TEXT[],
    reject_reason TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE posts (
    id SERIAL PRIMARY KEY,
    message_id INTEGER NOT NULL,
    topic_id INTEGER NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id),
    content_text TEXT,
    photo_file_ids TEXT[],
    created_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    deleted_at TIMESTAMPTZ
);

CREATE TABLE payments (
    id SERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    topic_id INTEGER NOT NULL REFERENCES topics(id),
    telegram_payment_id VARCHAR(255) NOT NULL,
    amount INTEGER NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'RUB',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_posts_expires ON posts(expires_at) WHERE is_deleted = FALSE;
CREATE INDEX idx_pending_posts_topic ON pending_posts(topic_id);
