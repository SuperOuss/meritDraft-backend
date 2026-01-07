package models

import (
	"time"

	"github.com/google/uuid"
)

// User represents a user entity
type User struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"` // Never serialize password hash
	Name         string    `json:"name"`
	FirmName     *string   `json:"firm_name,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// UserPreferences represents user preferences
type UserPreferences struct {
	UserID             uuid.UUID `json:"user_id"`
	EmailNotifications bool      `json:"email_notifications"`
	AutoSaveDrafts     bool      `json:"auto_save_drafts"`
	UpdatedAt          time.Time `json:"updated_at"`
}

