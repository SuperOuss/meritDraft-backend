package models

import (
	"github.com/google/uuid"
)

// LegalChunk represents a chunk of legal text from the knowledge base
type LegalChunk struct {
	ID                 uuid.UUID              `json:"id"`
	Text               string                  `json:"text"`
	SourceType         string                  `json:"source_type"` // "regulation", "appeal_decision", "precedent_case"
	SourceDocument     string                  `json:"source_document"`
	RegulatoryCitation []string                `json:"regulatory_citation"`
	CaseCitation       *string                 `json:"case_citation,omitempty"`
	AppealCitation     *string                 `json:"appeal_citation,omitempty"`
	CriterionTag       string                  `json:"criterion_tag"`
	LegalStandard      *string                 `json:"legal_standard,omitempty"`
	LegalTest          *string                 `json:"legal_test,omitempty"`
	IsWinningArgument  bool                    `json:"is_winning_argument"`
	IsHolding          bool                    `json:"is_holding"`
	Metadata           map[string]interface{}  `json:"metadata,omitempty"`
	Distance           float64                 `json:"distance,omitempty"` // Vector similarity distance
}

