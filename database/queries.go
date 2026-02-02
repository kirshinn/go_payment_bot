package database

import (
	"context"
	"time"
)

// ============================================
// Groups
// ============================================

func (db *DB) GetOrCreateGroup(ctx context.Context, id int64, title string) (*Group, error) {
	query := `
		INSERT INTO groups (id, title)
		VALUES ($1, $2)
		ON CONFLICT (id) DO UPDATE SET title = EXCLUDED.title
		RETURNING id, title, is_active, created_at`

	var g Group
	err := db.Pool.QueryRow(ctx, query, id, title).Scan(
		&g.ID, &g.Title, &g.IsActive, &g.CreatedAt,
	)
	return &g, err
}

// ============================================
// Topics
// ============================================

func (db *DB) GetTopicByGroupAndTopicID(ctx context.Context, groupID int64, topicID int) (*Topic, error) {
	query := `
		SELECT id, group_id, topic_id, title, price, duration_days, 
		       max_photos, max_text_length, moderation_enabled, is_active, created_at
		FROM topics
		WHERE group_id = $1 AND topic_id = $2 AND is_active = true`

	var t Topic
	err := db.Pool.QueryRow(ctx, query, groupID, topicID).Scan(
		&t.ID, &t.GroupID, &t.TopicID, &t.Title, &t.Price, &t.DurationDays,
		&t.MaxPhotos, &t.MaxTextLength, &t.ModerationEnabled, &t.IsActive, &t.CreatedAt,
	)
	return &t, err
}

func (db *DB) CreateTopic(ctx context.Context, groupID int64, topicID int, title string, price, durationDays, maxPhotos, maxTextLen int) (*Topic, error) {
	query := `
		INSERT INTO topics (group_id, topic_id, title, price, duration_days, max_photos, max_text_length)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (group_id, topic_id) DO UPDATE SET title = EXCLUDED.title
		RETURNING id, group_id, topic_id, title, price, duration_days, 
		          max_photos, max_text_length, moderation_enabled, is_active, created_at`

	var t Topic
	err := db.Pool.QueryRow(ctx, query, groupID, topicID, title, price, durationDays, maxPhotos, maxTextLen).Scan(
		&t.ID, &t.GroupID, &t.TopicID, &t.Title, &t.Price, &t.DurationDays,
		&t.MaxPhotos, &t.MaxTextLength, &t.ModerationEnabled, &t.IsActive, &t.CreatedAt,
	)
	return &t, err
}

func (db *DB) GetTopicByID(ctx context.Context, id int) (*Topic, error) {
	query := `
		SELECT id, group_id, topic_id, title, price, duration_days, 
		       max_photos, max_text_length, moderation_enabled, is_active, created_at
		FROM topics
		WHERE id = $1`

	var t Topic
	err := db.Pool.QueryRow(ctx, query, id).Scan(
		&t.ID, &t.GroupID, &t.TopicID, &t.Title, &t.Price, &t.DurationDays,
		&t.MaxPhotos, &t.MaxTextLength, &t.ModerationEnabled, &t.IsActive, &t.CreatedAt,
	)
	return &t, err
}

// ============================================
// Users
// ============================================

func (db *DB) GetOrCreateUser(ctx context.Context, id int64, username, firstName, lastName *string) (*User, error) {
	query := `
		INSERT INTO users (id, username, first_name, last_name)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET 
			username = COALESCE(EXCLUDED.username, users.username),
			first_name = COALESCE(EXCLUDED.first_name, users.first_name),
			last_name = COALESCE(EXCLUDED.last_name, users.last_name)
		RETURNING id, username, first_name, last_name, state, current_topic_id, 
		          paid_at, banned_at, ban_reason, created_at`

	var u User
	err := db.Pool.QueryRow(ctx, query, id, username, firstName, lastName).Scan(
		&u.ID, &u.Username, &u.FirstName, &u.LastName, &u.State, &u.CurrentTopicID,
		&u.PaidAt, &u.BannedAt, &u.BanReason, &u.CreatedAt,
	)
	return &u, err
}

func (db *DB) GetUser(ctx context.Context, id int64) (*User, error) {
	query := `
		SELECT id, username, first_name, last_name, state, current_topic_id, 
		       paid_at, banned_at, ban_reason, created_at
		FROM users
		WHERE id = $1`

	var u User
	err := db.Pool.QueryRow(ctx, query, id).Scan(
		&u.ID, &u.Username, &u.FirstName, &u.LastName, &u.State, &u.CurrentTopicID,
		&u.PaidAt, &u.BannedAt, &u.BanReason, &u.CreatedAt,
	)
	return &u, err
}

func (db *DB) UpdateUserState(ctx context.Context, userID int64, state UserState, topicID *int) error {
	query := `UPDATE users SET state = $1, current_topic_id = $2 WHERE id = $3`
	_, err := db.Pool.Exec(ctx, query, state, topicID, userID)
	return err
}

func (db *DB) MarkUserPaid(ctx context.Context, userID int64, topicID int) error {
	query := `UPDATE users SET state = $1, current_topic_id = $2, paid_at = NOW() WHERE id = $3`
	_, err := db.Pool.Exec(ctx, query, StateWaitingContent, topicID, userID)
	return err
}

func (db *DB) ResetUser(ctx context.Context, userID int64) error {
	query := `UPDATE users SET state = 'none', current_topic_id = NULL, paid_at = NULL WHERE id = $1`
	_, err := db.Pool.Exec(ctx, query, userID)
	return err
}

func (db *DB) BanUser(ctx context.Context, userID int64, reason string) error {
	query := `UPDATE users SET state = 'banned', banned_at = NOW(), ban_reason = $1 WHERE id = $2`
	_, err := db.Pool.Exec(ctx, query, reason, userID)
	return err
}

// ============================================
// Pending Posts (модерация)
// ============================================

func (db *DB) CreatePendingPost(ctx context.Context, userID int64, topicID int, text *string, photoIDs []string) (*PendingPost, error) {
	query := `
		INSERT INTO pending_posts (user_id, topic_id, content_text, photo_file_ids)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, topic_id, content_text, photo_file_ids, reject_reason, created_at`

	var p PendingPost
	err := db.Pool.QueryRow(ctx, query, userID, topicID, text, photoIDs).Scan(
		&p.ID, &p.UserID, &p.TopicID, &p.ContentText, &p.PhotoFileIDs, &p.RejectReason, &p.CreatedAt,
	)
	return &p, err
}

func (db *DB) GetPendingPost(ctx context.Context, userID int64) (*PendingPost, error) {
	query := `
		SELECT id, user_id, topic_id, content_text, photo_file_ids, reject_reason, created_at
		FROM pending_posts
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 1`

	var p PendingPost
	err := db.Pool.QueryRow(ctx, query, userID).Scan(
		&p.ID, &p.UserID, &p.TopicID, &p.ContentText, &p.PhotoFileIDs, &p.RejectReason, &p.CreatedAt,
	)
	return &p, err
}

func (db *DB) DeletePendingPost(ctx context.Context, id int) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM pending_posts WHERE id = $1`, id)
	return err
}

// ============================================
// Posts
// ============================================

func (db *DB) CreatePost(ctx context.Context, messageID int, topicID int, userID int64, text *string, photoIDs []string, expiresAt time.Time) (*Post, error) {
	query := `
		INSERT INTO posts (message_id, topic_id, user_id, content_text, photo_file_ids, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, message_id, topic_id, user_id, content_text, photo_file_ids, 
		          created_at, expires_at, is_deleted, deleted_at`

	var p Post
	err := db.Pool.QueryRow(ctx, query, messageID, topicID, userID, text, photoIDs, expiresAt).Scan(
		&p.ID, &p.MessageID, &p.TopicID, &p.UserID, &p.ContentText, &p.PhotoFileIDs,
		&p.CreatedAt, &p.ExpiresAt, &p.IsDeleted, &p.DeletedAt,
	)
	return &p, err
}

func (db *DB) GetExpiredPosts(ctx context.Context) ([]ExpiredPost, error) {
	query := `
		SELECT p.id, p.message_id, t.group_id, t.topic_id, p.user_id, p.expires_at
		FROM posts p
		JOIN topics t ON t.id = p.topic_id
		WHERE p.is_deleted = FALSE AND p.expires_at < NOW()`

	rows, err := db.Pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []ExpiredPost
	for rows.Next() {
		var p ExpiredPost
		if err := rows.Scan(&p.ID, &p.MessageID, &p.ChatID, &p.TopicID, &p.UserID, &p.ExpiresAt); err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}
	return posts, rows.Err()
}

func (db *DB) MarkPostDeleted(ctx context.Context, id int) error {
	query := `UPDATE posts SET is_deleted = TRUE, deleted_at = NOW() WHERE id = $1`
	_, err := db.Pool.Exec(ctx, query, id)
	return err
}

// ============================================
// Payments
// ============================================

func (db *DB) CreatePayment(ctx context.Context, userID int64, topicID int, telegramPaymentID string, amount int, currency string) (*Payment, error) {
	query := `
		INSERT INTO payments (user_id, topic_id, telegram_payment_id, amount, currency)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, user_id, topic_id, telegram_payment_id, amount, currency, created_at`

	var p Payment
	err := db.Pool.QueryRow(ctx, query, userID, topicID, telegramPaymentID, amount, currency).Scan(
		&p.ID, &p.UserID, &p.TopicID, &p.TelegramPaymentID, &p.Amount, &p.Currency, &p.CreatedAt,
	)
	return &p, err
}
