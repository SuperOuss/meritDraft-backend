package repository

import (
	"context"
	"fmt"

	"meritdraft-backend/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PetitionRepository handles database operations for petitions
type PetitionRepository struct {
	db *pgxpool.Pool
}

// NewPetitionRepository creates a new petition repository
func NewPetitionRepository(db *pgxpool.Pool) *PetitionRepository {
	return &PetitionRepository{db: db}
}

// Create creates a new petition
func (r *PetitionRepository) Create(ctx context.Context, petition *models.Petition) error {
	query := `
		INSERT INTO petitions (
			user_id, status, client_name, visa_type, petitioner_name, 
			field_of_expertise, cv_file_id, job_offer_file_id, scholar_link,
			parsed_documents, selected_criteria, criteria_details,
			generated_content, refine_instructions
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
		) RETURNING id, created_at, updated_at`

	err := r.db.QueryRow(
		ctx, query,
		petition.UserID,
		petition.Status,
		petition.ClientName,
		petition.VisaType,
		petition.PetitionerName,
		petition.FieldOfExpertise,
		petition.CVFileID,
		petition.JobOfferFileID,
		petition.ScholarLink,
		petition.ParsedDocuments,
		petition.SelectedCriteria,
		petition.CriteriaDetails,
		petition.GeneratedContent,
		petition.RefineInstructions,
	).Scan(&petition.ID, &petition.CreatedAt, &petition.UpdatedAt)

	return err
}

// GetByID retrieves a petition by ID
func (r *PetitionRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Petition, error) {
	petition := &models.Petition{}
	query := `
		SELECT id, user_id, status, client_name, visa_type, petitioner_name,
			field_of_expertise, cv_file_id, job_offer_file_id, scholar_link,
			parsed_documents, selected_criteria, criteria_details,
			generated_content, refine_instructions,
			created_at, updated_at, completed_at
		FROM petitions
		WHERE id = $1`

	err := r.db.QueryRow(ctx, query, id).Scan(
		&petition.ID,
		&petition.UserID,
		&petition.Status,
		&petition.ClientName,
		&petition.VisaType,
		&petition.PetitionerName,
		&petition.FieldOfExpertise,
		&petition.CVFileID,
		&petition.JobOfferFileID,
		&petition.ScholarLink,
		&petition.ParsedDocuments,
		&petition.SelectedCriteria,
		&petition.CriteriaDetails,
		&petition.GeneratedContent,
		&petition.RefineInstructions,
		&petition.CreatedAt,
		&petition.UpdatedAt,
		&petition.CompletedAt,
	)

	if err != nil {
		return nil, err
	}

	return petition, nil
}

// Update updates a petition
func (r *PetitionRepository) Update(ctx context.Context, petition *models.Petition) error {
	query := `
		UPDATE petitions SET
			status = $2,
			client_name = $3,
			visa_type = $4,
			petitioner_name = $5,
			field_of_expertise = $6,
			cv_file_id = $7,
			job_offer_file_id = $8,
			scholar_link = $9,
			parsed_documents = $10,
			selected_criteria = $11,
			criteria_details = $12,
			generated_content = $13,
			refine_instructions = $14,
			updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at`

	err := r.db.QueryRow(
		ctx, query,
		petition.ID,
		petition.Status,
		petition.ClientName,
		petition.VisaType,
		petition.PetitionerName,
		petition.FieldOfExpertise,
		petition.CVFileID,
		petition.JobOfferFileID,
		petition.ScholarLink,
		petition.ParsedDocuments,
		petition.SelectedCriteria,
		petition.CriteriaDetails,
		petition.GeneratedContent,
		petition.RefineInstructions,
	).Scan(&petition.UpdatedAt)

	return err
}

// UpdateGeneratedContent updates only the generated content
func (r *PetitionRepository) UpdateGeneratedContent(ctx context.Context, id uuid.UUID, content string) error {
	query := `
		UPDATE petitions SET
			generated_content = $2,
			updated_at = NOW()
		WHERE id = $1`

	_, err := r.db.Exec(ctx, query, id, content)
	return err
}

// ListByUserID retrieves all petitions for a user
func (r *PetitionRepository) ListByUserID(ctx context.Context, userID uuid.UUID, status *models.PetitionStatus, limit, offset int) ([]*models.Petition, error) {
	query := `
		SELECT id, user_id, status, client_name, visa_type, petitioner_name,
			field_of_expertise, cv_file_id, job_offer_file_id, scholar_link,
			parsed_documents, selected_criteria, criteria_details,
			generated_content, refine_instructions,
			created_at, updated_at, completed_at
		FROM petitions
		WHERE user_id = $1`

	args := []interface{}{userID}
	argIndex := 2

	if status != nil {
		query += fmt.Sprintf(" AND status = $%d", argIndex)
		args = append(args, *status)
		argIndex++
	}

	query += " ORDER BY created_at DESC"

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIndex)
		args = append(args, limit)
		argIndex++
		if offset > 0 {
			query += fmt.Sprintf(" OFFSET $%d", argIndex)
			args = append(args, offset)
		}
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var petitions []*models.Petition
	for rows.Next() {
		petition := &models.Petition{}
		err := rows.Scan(
			&petition.ID,
			&petition.UserID,
			&petition.Status,
			&petition.ClientName,
			&petition.VisaType,
			&petition.PetitionerName,
			&petition.FieldOfExpertise,
			&petition.CVFileID,
			&petition.JobOfferFileID,
			&petition.ScholarLink,
			&petition.ParsedDocuments,
			&petition.SelectedCriteria,
			&petition.CriteriaDetails,
			&petition.GeneratedContent,
			&petition.RefineInstructions,
			&petition.CreatedAt,
			&petition.UpdatedAt,
			&petition.CompletedAt,
		)
		if err != nil {
			return nil, err
		}
		petitions = append(petitions, petition)
	}

	return petitions, rows.Err()
}

// Delete deletes a petition
func (r *PetitionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM petitions WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	return err
}

