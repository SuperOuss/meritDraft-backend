package service

import (
	"context"
	"errors"

	"meritdraft-backend/models"
	"meritdraft-backend/repository"

	"github.com/google/uuid"
)

// PetitionService handles business logic for petitions
type PetitionService struct {
	petitionRepo *repository.PetitionRepository
	jobRepo      *repository.GenerationJobRepository
}

// PetitionServiceOption is a functional option for PetitionService
type PetitionServiceOption func(*PetitionService)

// WithPetitionRepository sets the petition repository
func WithPetitionRepository(repo *repository.PetitionRepository) PetitionServiceOption {
	return func(s *PetitionService) {
		s.petitionRepo = repo
	}
}

// WithGenerationJobRepository sets the generation job repository
func WithGenerationJobRepository(repo *repository.GenerationJobRepository) PetitionServiceOption {
	return func(s *PetitionService) {
		s.jobRepo = repo
	}
}

// NewPetitionService creates a new petition service
func NewPetitionService(opts ...PetitionServiceOption) *PetitionService {
	s := &PetitionService{}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// CreatePetitionRequest represents a request to create a petition
type CreatePetitionRequest struct {
	UserID uuid.UUID
	Status models.PetitionStatus
}

// CreatePetitionResult represents the result of creating a petition
type CreatePetitionResult struct {
	Petition *models.Petition
}

// CreatePetition creates a new petition with default values
func (s *PetitionService) CreatePetition(ctx context.Context, req CreatePetitionRequest) (*CreatePetitionResult, error) {
	if s.petitionRepo == nil {
		return nil, errors.New("petition repository not set")
	}

	petition := &models.Petition{
		UserID:           req.UserID,
		Status:           req.Status,
		SelectedCriteria: []string{},
		CriteriaDetails:  make(models.CriteriaDetails),
	}

	if petition.Status == "" {
		petition.Status = models.StatusDraft
	}

	err := s.petitionRepo.Create(ctx, petition)
	if err != nil {
		return nil, err
	}

	return &CreatePetitionResult{Petition: petition}, nil
}

// GetPetitionRequest represents a request to get a petition
type GetPetitionRequest struct {
	ID uuid.UUID
}

// GetPetitionResult represents the result of getting a petition
type GetPetitionResult struct {
	Petition *models.Petition
}

// GetPetition retrieves a petition by ID
func (s *PetitionService) GetPetition(ctx context.Context, req GetPetitionRequest) (*GetPetitionResult, error) {
	if s.petitionRepo == nil {
		return nil, errors.New("petition repository not set")
	}

	petition, err := s.petitionRepo.GetByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}

	return &GetPetitionResult{Petition: petition}, nil
}

// UpdatePetitionRequest represents a request to update a petition
type UpdatePetitionRequest struct {
	Petition *models.Petition
}

// UpdatePetitionResult represents the result of updating a petition
type UpdatePetitionResult struct {
	Petition *models.Petition
}

// UpdatePetition updates a petition
func (s *PetitionService) UpdatePetition(ctx context.Context, req UpdatePetitionRequest) (*UpdatePetitionResult, error) {
	if s.petitionRepo == nil {
		return nil, errors.New("petition repository not set")
	}

	err := s.petitionRepo.Update(ctx, req.Petition)
	if err != nil {
		return nil, err
	}

	return &UpdatePetitionResult{Petition: req.Petition}, nil
}

// UpdateGeneratedContentRequest represents a request to update generated content
type UpdateGeneratedContentRequest struct {
	PetitionID uuid.UUID
	Content    string
}

// UpdateGeneratedContentResult represents the result of updating generated content
type UpdateGeneratedContentResult struct{}

// UpdateGeneratedContent updates the generated content for a petition
func (s *PetitionService) UpdateGeneratedContent(ctx context.Context, req UpdateGeneratedContentRequest) (*UpdateGeneratedContentResult, error) {
	if s.petitionRepo == nil {
		return nil, errors.New("petition repository not set")
	}

	err := s.petitionRepo.UpdateGeneratedContent(ctx, req.PetitionID, req.Content)
	if err != nil {
		return nil, err
	}

	return &UpdateGeneratedContentResult{}, nil
}

// ListPetitionsRequest represents a request to list petitions
type ListPetitionsRequest struct {
	UserID uuid.UUID
	Status *models.PetitionStatus
	Limit  int
	Offset int
}

// ListPetitionsResult represents the result of listing petitions
type ListPetitionsResult struct {
	Petitions []*models.Petition
}

// ListPetitions lists petitions for a user
func (s *PetitionService) ListPetitions(ctx context.Context, req ListPetitionsRequest) (*ListPetitionsResult, error) {
	if s.petitionRepo == nil {
		return nil, errors.New("petition repository not set")
	}

	petitions, err := s.petitionRepo.ListByUserID(ctx, req.UserID, req.Status, req.Limit, req.Offset)
	if err != nil {
		return nil, err
	}

	return &ListPetitionsResult{Petitions: petitions}, nil
}
