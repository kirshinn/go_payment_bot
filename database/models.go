package database

import "time"

type UserState string

const (
	StateNone              UserState = "none"
	StateWaitingEmail      UserState = "waiting_email"
	StateWaitingPayment    UserState = "waiting_payment"
	StateWaitingContent    UserState = "waiting_content"
	StateWaitingConfirm    UserState = "waiting_confirm"
	StateWaitingModeration UserState = "waiting_moderation"
	StateBanned            UserState = "banned"
)

type Group struct {
	ID        int64
	Title     string
	IsActive  bool
	CreatedAt time.Time
}

type Topic struct {
	ID                int
	GroupID           int64
	TopicID           int
	Title             string
	Price             int
	DurationDays      int
	MaxPhotos         int
	MaxTextLength     int
	ModerationEnabled bool
	IsActive          bool
	CreatedAt         time.Time
}

type User struct {
	ID             int64
	Username       *string
	FirstName      *string
	LastName       *string
	Email          *string
	EmailDeclined  bool
	State          UserState
	CurrentTopicID *int
	PaidAt         *time.Time
	ReceiptSentAt  *time.Time
	BannedAt       *time.Time
	BanReason      *string
	CreatedAt      time.Time
}

type PendingPost struct {
	ID           int
	UserID       int64
	TopicID      int
	ContentText  *string
	PhotoFileIDs []string
	RejectReason *string
	CreatedAt    time.Time
}

type Post struct {
	ID           int
	MessageID    int
	TopicID      int
	UserID       int64
	ContentText  *string
	PhotoFileIDs []string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	IsDeleted    bool
	DeletedAt    *time.Time
}

type Payment struct {
	ID                int
	UserID            int64
	TopicID           int
	TelegramPaymentID string
	Amount            int
	Currency          string
	CreatedAt         time.Time
}

// ExpiredPost — для удаления просроченных
type ExpiredPost struct {
	ID        int
	MessageID int
	ChatID    int64
	TopicID   int
	UserID    int64
	ExpiresAt time.Time
}

type SpamViolation struct {
	ID            int
	UserID        int64
	GroupID       int64
	TopicID       *int
	MessageText   *string
	ViolationType string
	MatchFound    *string
	CreatedAt     time.Time
}

type AllowedDomain struct {
	ID          int
	Domain      string
	Description *string
	IsActive    bool
	CreatedAt   time.Time
}
