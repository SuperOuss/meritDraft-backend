package repository

import (
	"context"

	"meritdraft-backend/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FileRepository handles database operations for files
type FileRepository struct {
	db *pgxpool.Pool
}

// NewFileRepository creates a new file repository
func NewFileRepository(db *pgxpool.Pool) *FileRepository {
	return &FileRepository{db: db}
}

// Create creates a new file record
func (r *FileRepository) Create(ctx context.Context, file *models.File) error {
	query := `
		INSERT INTO files (
			user_id, petition_id, filename, mime_type, size, storage_path
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`

	err := r.db.QueryRow(
		ctx, query,
		file.UserID,
		file.PetitionID,
		file.Filename,
		file.MimeType,
		file.Size,
		file.StoragePath,
	).Scan(&file.ID, &file.CreatedAt)

	return err
}

// GetByID retrieves a file by ID
func (r *FileRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.File, error) {
	file := &models.File{}
	query := `
		SELECT id, user_id, petition_id, filename, mime_type, size, storage_path, created_at
		FROM files
		WHERE id = $1`

	err := r.db.QueryRow(ctx, query, id).Scan(
		&file.ID,
		&file.UserID,
		&file.PetitionID,
		&file.Filename,
		&file.MimeType,
		&file.Size,
		&file.StoragePath,
		&file.CreatedAt,
	)

	if err != nil {
		return nil, err
	}

	return file, nil
}

// ListByUserID retrieves all files for a user
func (r *FileRepository) ListByUserID(ctx context.Context, userID uuid.UUID) ([]*models.File, error) {
	query := `
		SELECT id, user_id, petition_id, filename, mime_type, size, storage_path, created_at
		FROM files
		WHERE user_id = $1
		ORDER BY created_at DESC`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*models.File
	for rows.Next() {
		file := &models.File{}
		err := rows.Scan(
			&file.ID,
			&file.UserID,
			&file.PetitionID,
			&file.Filename,
			&file.MimeType,
			&file.Size,
			&file.StoragePath,
			&file.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, rows.Err()
}

// ListByPetitionID retrieves all files for a petition
func (r *FileRepository) ListByPetitionID(ctx context.Context, petitionID uuid.UUID) ([]*models.File, error) {
	query := `
		SELECT id, user_id, petition_id, filename, mime_type, size, storage_path, created_at
		FROM files
		WHERE petition_id = $1
		ORDER BY created_at DESC`

	rows, err := r.db.Query(ctx, query, petitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*models.File
	for rows.Next() {
		file := &models.File{}
		err := rows.Scan(
			&file.ID,
			&file.UserID,
			&file.PetitionID,
			&file.Filename,
			&file.MimeType,
			&file.Size,
			&file.StoragePath,
			&file.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, rows.Err()
}

// Delete deletes a file record
func (r *FileRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM files WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	return err
}

