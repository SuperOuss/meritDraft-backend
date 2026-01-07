package repository

import (
	"context"
	"fmt"
	"strings"

	"meritdraft-backend/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

// LegalChunkRepository handles database operations for legal chunks
type LegalChunkRepository struct {
	db *pgxpool.Pool
}

// NewLegalChunkRepository creates a new legal chunk repository
func NewLegalChunkRepository(db *pgxpool.Pool) *LegalChunkRepository {
	return &LegalChunkRepository{db: db}
}

// formatVector formats an embedding vector as a string for pgx
func formatVector(embedding []float64) string {
	if len(embedding) == 0 {
		return "[]"
	}
	var parts []string
	for _, v := range embedding {
		parts = append(parts, fmt.Sprintf("%.6f", v))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// SearchByCriterion performs a hybrid vector search for legal chunks
// embedding: Query embedding vector (768 dimensions)
// criterion: Criterion tag (e.g., "awards", "judging")
// sourceType: Source type filter ("regulation", "appeal_decision", "precedent_case")
// limit: Maximum number of chunks to return
func (r *LegalChunkRepository) SearchByCriterion(
	ctx context.Context,
	embedding []float64,
	criterion string,
	sourceType string,
	limit int,
) ([]models.LegalChunk, error) {
	if len(embedding) != 768 {
		return nil, fmt.Errorf("embedding must be 768 dimensions, got %d", len(embedding))
	}

	vectorStr := formatVector(embedding)

	// Handle empty criterion as NULL for Prong 2 searches
	var criterionFilter string
	var args []interface{}
	if criterion == "" {
		criterionFilter = "criterion_tag IS NULL"
		args = []interface{}{vectorStr, sourceType, limit}
	} else {
		criterionFilter = "criterion_tag = $2"
		args = []interface{}{vectorStr, criterion, sourceType, limit}
	}

	query := fmt.Sprintf(`
		SELECT 
			id,
			chunk_text,
			source_type,
			source_document,
			regulatory_citation,
			case_citation,
			appeal_citation,
			criterion_tag,
			legal_standard,
			legal_test,
			is_winning_argument,
			is_holding,
			metadata,
			embedding <=> $1::vector AS distance
		FROM legal_chunks
		WHERE 
			%s
			AND source_type = $%d
			AND visa_type = 'O-1'
			AND (
				source_type != 'appeal_decision' 
				OR is_winning_argument = true
			)
			AND (
				source_type != 'precedent_case' 
				OR is_holding = true
			)
		ORDER BY 
			embedding <=> $1::vector
		LIMIT $%d`, criterionFilter, len(args)-1, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query legal chunks: %w", err)
	}
	defer rows.Close()

	var chunks []models.LegalChunk
	for rows.Next() {
		var chunk models.LegalChunk
		err := rows.Scan(
			&chunk.ID,
			&chunk.Text,
			&chunk.SourceType,
			&chunk.SourceDocument,
			&chunk.RegulatoryCitation,
			&chunk.CaseCitation,
			&chunk.AppealCitation,
			&chunk.CriterionTag,
			&chunk.LegalStandard,
			&chunk.LegalTest,
			&chunk.IsWinningArgument,
			&chunk.IsHolding,
			&chunk.Metadata,
			&chunk.Distance,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan legal chunk: %w", err)
		}
		chunks = append(chunks, chunk)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating legal chunks: %w", err)
	}

	return chunks, nil
}

