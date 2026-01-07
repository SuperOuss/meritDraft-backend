package repository

import (
	"context"
	"time"

	"meritdraft-backend/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GenerationJobRepository handles database operations for generation jobs
type GenerationJobRepository struct {
	db *pgxpool.Pool
}

// NewGenerationJobRepository creates a new generation job repository
func NewGenerationJobRepository(db *pgxpool.Pool) *GenerationJobRepository {
	return &GenerationJobRepository{db: db}
}

// Create creates a new generation job
func (r *GenerationJobRepository) Create(ctx context.Context, job *models.GenerationJob) error {
	query := `
		INSERT INTO generation_jobs (
			petition_id, status, current_step, steps, error_message
		) VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`

	err := r.db.QueryRow(
		ctx, query,
		job.PetitionID,
		job.Status,
		job.CurrentStep,
		job.Steps,
		job.ErrorMessage,
	).Scan(&job.ID, &job.CreatedAt, &job.UpdatedAt)

	return err
}

// GetByID retrieves a generation job by ID
func (r *GenerationJobRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.GenerationJob, error) {
	job := &models.GenerationJob{}
	query := `
		SELECT id, petition_id, status, current_step, steps, error_message,
			created_at, updated_at, completed_at
		FROM generation_jobs
		WHERE id = $1`

	err := r.db.QueryRow(ctx, query, id).Scan(
		&job.ID,
		&job.PetitionID,
		&job.Status,
		&job.CurrentStep,
		&job.Steps,
		&job.ErrorMessage,
		&job.CreatedAt,
		&job.UpdatedAt,
		&job.CompletedAt,
	)

	if err != nil {
		return nil, err
	}

	// Ensure Steps is never nil (safeguard in case Scan didn't handle NULL properly)
	if job.Steps == nil {
		job.Steps = make(models.GenerationSteps, 0)
	}

	return job, nil
}

// GetByPetitionID retrieves the latest generation job for a petition
func (r *GenerationJobRepository) GetByPetitionID(ctx context.Context, petitionID uuid.UUID) (*models.GenerationJob, error) {
	job := &models.GenerationJob{}
	query := `
		SELECT id, petition_id, status, current_step, steps, error_message,
			created_at, updated_at, completed_at
		FROM generation_jobs
		WHERE petition_id = $1
		ORDER BY created_at DESC
		LIMIT 1`

	err := r.db.QueryRow(ctx, query, petitionID).Scan(
		&job.ID,
		&job.PetitionID,
		&job.Status,
		&job.CurrentStep,
		&job.Steps,
		&job.ErrorMessage,
		&job.CreatedAt,
		&job.UpdatedAt,
		&job.CompletedAt,
	)

	if err != nil {
		return nil, err
	}

	// Ensure Steps is never nil (safeguard in case Scan didn't handle NULL properly)
	if job.Steps == nil {
		job.Steps = make(models.GenerationSteps, 0)
	}

	return job, nil
}

// UpdateStatus updates the status of a generation job
func (r *GenerationJobRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status models.GenerationJobStatus) error {
	query := `
		UPDATE generation_jobs SET
			status = $2,
			updated_at = NOW()
		WHERE id = $1`

	_, err := r.db.Exec(ctx, query, id, status)
	return err
}

// UpdateProgress updates the progress of a generation job
func (r *GenerationJobRepository) UpdateProgress(ctx context.Context, id uuid.UUID, currentStep string, steps models.GenerationSteps) error {
	query := `
		UPDATE generation_jobs SET
			current_step = $2,
			steps = $3,
			updated_at = NOW()
		WHERE id = $1`

	_, err := r.db.Exec(ctx, query, id, currentStep, steps)
	return err
}

// Complete marks a generation job as completed
func (r *GenerationJobRepository) Complete(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	query := `
		UPDATE generation_jobs SET
			status = $2,
			completed_at = $3,
			updated_at = $3
		WHERE id = $1`

	_, err := r.db.Exec(ctx, query, id, models.JobStatusCompleted, now)
	return err
}

// Fail marks a generation job as failed
func (r *GenerationJobRepository) Fail(ctx context.Context, id uuid.UUID, errorMessage string) error {
	query := `
		UPDATE generation_jobs SET
			status = $2,
			error_message = $3,
			updated_at = NOW()
		WHERE id = $1`

	_, err := r.db.Exec(ctx, query, id, models.JobStatusFailed, errorMessage)
	return err
}
