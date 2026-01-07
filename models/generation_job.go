package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// GenerationJobStatus represents the status of a generation job
type GenerationJobStatus string

const (
	JobStatusPending    GenerationJobStatus = "pending"
	JobStatusInProgress GenerationJobStatus = "in_progress"
	JobStatusCompleted  GenerationJobStatus = "completed"
	JobStatusFailed     GenerationJobStatus = "failed"
)

// GenerationStep represents a step in the generation process
type GenerationStep struct {
	Name        string `json:"name"`
	Status      string `json:"status"` // "pending", "in_progress", "completed", "failed"
	Description string `json:"description,omitempty"`
}

// GenerationSteps represents a list of generation steps
type GenerationSteps []GenerationStep

// Value implements driver.Valuer for JSONB
func (g GenerationSteps) Value() (driver.Value, error) {
	return json.Marshal(g)
}

// Scan implements sql.Scanner for JSONB
func (g *GenerationSteps) Scan(value interface{}) error {
	if value == nil {
		*g = make(GenerationSteps, 0)
		return nil
	}
	
	// Handle different types that pgx might return for JSONB
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		// If we can't convert, return empty slice
		*g = make(GenerationSteps, 0)
		return nil
	}
	
	// Handle empty bytes as empty slice
	if len(bytes) == 0 {
		*g = make(GenerationSteps, 0)
		return nil
	}
	
	return json.Unmarshal(bytes, g)
}

// GenerationJob represents a generation job entity
type GenerationJob struct {
	ID           uuid.UUID          `json:"id"`
	PetitionID   uuid.UUID          `json:"petition_id"`
	Status       GenerationJobStatus `json:"status"`
	CurrentStep  *string            `json:"current_step,omitempty"`
	Steps        GenerationSteps    `json:"steps"`
	ErrorMessage *string            `json:"error_message,omitempty"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
	CompletedAt  *time.Time         `json:"completed_at,omitempty"`
}

