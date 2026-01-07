package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// PetitionStatus represents the status of a petition
type PetitionStatus string

const (
	StatusDraft       PetitionStatus = "draft"
	StatusInProgress  PetitionStatus = "in_progress"
	StatusCompleted   PetitionStatus = "completed"
	StatusArchived    PetitionStatus = "archived"
)

// VisaType represents the type of visa
type VisaType string

const (
	VisaTypeO1A   VisaType = "O-1A"
	VisaTypeEB1A  VisaType = "EB-1A"
	VisaTypeEB2NIW VisaType = "EB-2 NIW"
)

// ParsedDocuments represents parsed document data
type ParsedDocuments struct {
	PublicationsCount int `json:"publicationsCount"`
	CitationsCount    int `json:"citationsCount"`
}

// Value implements driver.Valuer for JSONB
func (p ParsedDocuments) Value() (driver.Value, error) {
	return json.Marshal(p)
}

// Scan implements sql.Scanner for JSONB
func (p *ParsedDocuments) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, p)
}

// CriteriaDetail represents details for a specific criterion
type CriteriaDetail map[string]interface{}

// CriteriaDetails represents a map of criterion IDs to their details
type CriteriaDetails map[string]CriteriaDetail

// Value implements driver.Valuer for JSONB
func (c CriteriaDetails) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan implements sql.Scanner for JSONB
func (c *CriteriaDetails) Scan(value interface{}) error {
	if value == nil {
		*c = make(CriteriaDetails)
		return nil
	}
	
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return nil
	}
	
	if len(bytes) == 0 {
		*c = make(CriteriaDetails)
		return nil
	}
	
	return json.Unmarshal(bytes, c)
}

// Petition represents a petition entity
type Petition struct {
	ID                uuid.UUID       `json:"id"`
	UserID            uuid.UUID       `json:"user_id"`
	Status            PetitionStatus  `json:"status"`
	
	// Step 1: Intake
	ClientName        string          `json:"client_name"`
	VisaType          VisaType        `json:"visa_type"`
	PetitionerName    string          `json:"petitioner_name"`
	FieldOfExpertise  string          `json:"field_of_expertise"`
	
	// Step 2: Documents
	CVFileID          *uuid.UUID      `json:"cv_file_id"`
	JobOfferFileID    *uuid.UUID      `json:"job_offer_file_id"`
	ScholarLink       *string         `json:"scholar_link"`
	ParsedDocuments   *ParsedDocuments `json:"parsed_documents"`
	
	// Step 3: Strategy
	SelectedCriteria  []string        `json:"selected_criteria"`
	
	// Step 4: Deep Dive
	CriteriaDetails   CriteriaDetails `json:"criteria_details"`
	
	// Step 5/6: Generation
	GeneratedContent  *string         `json:"generated_content"`
	RefineInstructions *string        `json:"refine_instructions"`
	
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	CompletedAt       *time.Time      `json:"completed_at,omitempty"`
}

