package models

import (
	"time"

	"github.com/google/uuid"
)

// File represents a file entity
type File struct {
	ID          uuid.UUID  `json:"id"`
	UserID      uuid.UUID  `json:"user_id"`
	PetitionID  *uuid.UUID `json:"petition_id,omitempty"`
	Filename    string     `json:"filename"`
	MimeType    string     `json:"mime_type"`
	Size        int64      `json:"size"`
	StoragePath string     `json:"storage_path"`
	CreatedAt   time.Time  `json:"created_at"`
}

