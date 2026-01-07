package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"meritdraft-backend/models"
	"meritdraft-backend/repository"

	"github.com/google/generative-ai-go/genai"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DraftService handles draft generation logic
type DraftService struct {
	petitionRepo   *repository.PetitionRepository
	jobRepo        *repository.GenerationJobRepository
	legalChunkRepo *repository.LegalChunkRepository
	db             *pgxpool.Pool
	geminiClient   *genai.Client
}

// DraftServiceOption is a functional option for DraftService
type DraftServiceOption func(*DraftService)

// DraftWithPetitionRepository sets the petition repository
func DraftWithPetitionRepository(repo *repository.PetitionRepository) DraftServiceOption {
	return func(s *DraftService) {
		s.petitionRepo = repo
	}
}

// DraftWithGenerationJobRepository sets the generation job repository
func DraftWithGenerationJobRepository(repo *repository.GenerationJobRepository) DraftServiceOption {
	return func(s *DraftService) {
		s.jobRepo = repo
	}
}

// DraftWithLegalChunkRepository sets the legal chunk repository
func DraftWithLegalChunkRepository(repo *repository.LegalChunkRepository) DraftServiceOption {
	return func(s *DraftService) {
		s.legalChunkRepo = repo
	}
}

// DraftWithDatabase sets the database pool
func DraftWithDatabase(db *pgxpool.Pool) DraftServiceOption {
	return func(s *DraftService) {
		s.db = db
	}
}

// DraftWithGeminiClient sets the Gemini client
func DraftWithGeminiClient(client *genai.Client) DraftServiceOption {
	return func(s *DraftService) {
		s.geminiClient = client
	}
}

// NewDraftService creates a new draft service
func NewDraftService(opts ...DraftServiceOption) *DraftService {
	s := &DraftService{}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// GenerateDraftRequest represents a request to generate a draft
type GenerateDraftRequest struct {
	PetitionID         uuid.UUID
	RefineInstructions *string // Optional, for regeneration
}

// GenerateDraftResult represents the result of creating a generation job
type GenerateDraftResult struct {
	JobID uuid.UUID
}

// GetJobStatusRequest represents a request to get job status
type GetJobStatusRequest struct {
	JobID uuid.UUID
}

// GetJobStatusResult represents the result of getting job status
type GetJobStatusResult struct {
	Job *models.GenerationJob
}

var (
	ErrPetitionNotFound    = errors.New("petition not found")
	ErrMissingRequiredData = errors.New("petition missing required data for generation")
	ErrJobCreationFailed   = errors.New("failed to create generation job")
	ErrRetrievalFailed     = errors.New("failed to retrieve legal context")
	ErrGenerationFailed    = errors.New("failed to generate content")
	ErrEmbeddingFailed     = errors.New("failed to generate embedding")
	ErrJobNotFound         = errors.New("generation job not found")
)

const (
	embeddingAPI   = "https://generativelanguage.googleapis.com/v1beta/models/gemini-embedding-001:embedContent"
	generationAPI  = "https://generativelanguage.googleapis.com/v1beta/models/gemini-3-pro-preview:generateContent"
	maxRetries     = 3
	initialBackoff = time.Second
)

// GenerateDraft creates a generation job and returns immediately
// This method must complete in <100ms to avoid HTTP timeouts
func (s *DraftService) GenerateDraft(
	ctx context.Context,
	req GenerateDraftRequest,
) (*GenerateDraftResult, error) {
	if s.petitionRepo == nil {
		return nil, errors.New("petition repository not set")
	}
	if s.jobRepo == nil {
		return nil, errors.New("generation job repository not set")
	}

	// 1. Validate petition exists and has required data
	petition, err := s.petitionRepo.GetByID(ctx, req.PetitionID)
	if err != nil {
		return nil, ErrPetitionNotFound
	}

	// 2. Validate required fields
	if petition.ClientName == "" {
		return nil, ErrMissingRequiredData
	}
	if petition.FieldOfExpertise == "" {
		return nil, ErrMissingRequiredData
	}
	if len(petition.SelectedCriteria) == 0 {
		return nil, ErrMissingRequiredData
	}
	if len(petition.CriteriaDetails) == 0 {
		return nil, ErrMissingRequiredData
	}

	// 3. Create generation job with initial steps
	job := &models.GenerationJob{
		ID:         uuid.New(),
		PetitionID: req.PetitionID,
		Status:     models.JobStatusPending,
		Steps:      s.initializeSteps(petition.SelectedCriteria),
	}

	// Store refine instructions if provided
	if req.RefineInstructions != nil {
		// Note: RefineInstructions is stored in petition, not job
		// We'll handle this in ProcessDraft
	}

	err = s.jobRepo.Create(ctx, job)
	if err != nil {
		return nil, ErrJobCreationFailed
	}

	return &GenerateDraftResult{
		JobID: job.ID,
	}, nil
}

// GetJobStatus retrieves the status of a generation job
func (s *DraftService) GetJobStatus(
	ctx context.Context,
	req GetJobStatusRequest,
) (*GetJobStatusResult, error) {
	if s.jobRepo == nil {
		return nil, errors.New("generation job repository not set")
	}

	job, err := s.jobRepo.GetByID(ctx, req.JobID)
	if err != nil {
		return nil, ErrJobNotFound
	}

	return &GetJobStatusResult{
		Job: job,
	}, nil
}

// initializeSteps creates the initial generation steps based on selected criteria
func (s *DraftService) initializeSteps(criteria []string) models.GenerationSteps {
	steps := make(models.GenerationSteps, 0)

	// Add step for each criterion
	for _, criterion := range criteria {
		steps = append(steps, models.GenerationStep{
			Name:   getCriterionStepName(criterion),
			Status: "pending",
		})
	}

	// Add final steps
	steps = append(steps, models.GenerationStep{
		Name:   "Final Merits Determination",
		Status: "pending",
	})
	steps = append(steps, models.GenerationStep{
		Name:   "Assembling Document",
		Status: "pending",
	})

	return steps
}

// getCriterionStepName returns a human-readable step name for a criterion
func getCriterionStepName(criterion string) string {
	titles := map[string]string{
		"awards":                 "Drafting Awards Criterion",
		"membership":             "Drafting Membership Criterion",
		"media_coverage":         "Drafting Media Coverage Criterion",
		"judging":                "Drafting Judging Criterion",
		"original_contributions": "Drafting Original Contributions Criterion",
		"authorship":             "Drafting Authorship Criterion",
		"exhibitions":            "Drafting Exhibitions Criterion",
		"critical_role":          "Drafting Critical Role Criterion",
		"high_salary":            "Drafting High Salary Criterion",
		"commercial_success":     "Drafting Commercial Success Criterion",
	}
	if title, ok := titles[criterion]; ok {
		return title
	}
	return "Drafting " + criterion + " Criterion"
}

// ProcessDraft performs the actual generation work in the background
// This method runs in a goroutine and can take 45-90 seconds
func (s *DraftService) ProcessDraft(
	ctx context.Context,
	jobID uuid.UUID,
) error {
	if s.jobRepo == nil {
		return errors.New("generation job repository not set")
	}
	if s.petitionRepo == nil {
		return errors.New("petition repository not set")
	}

	// 1. Load job and petition
	job, err := s.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to load generation job: %w", err)
	}

	petition, err := s.petitionRepo.GetByID(ctx, job.PetitionID)
	if err != nil {
		s.markJobFailed(ctx, jobID, "failed to load petition: "+err.Error())
		return err
	}

	// 2. Update job status to in_progress
	err = s.jobRepo.UpdateStatus(ctx, jobID, models.JobStatusInProgress)
	if err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}

	// 3. Process each criterion (Prong 1)
	sections := make([]DraftSection, 0)

	for _, criterion := range petition.SelectedCriteria {
		stepName := getCriterionStepName(criterion)

		// Update step to in_progress
		err = s.updateStepStatus(ctx, jobID, stepName, "in_progress")
		if err != nil {
			s.markJobFailed(ctx, jobID, "failed to update step: "+err.Error())
			return err
		}

		// Retrieve context and generate section
		details, ok := petition.CriteriaDetails[criterion]
		if !ok {
			s.markJobFailed(ctx, jobID, fmt.Sprintf("missing details for criterion: %s", criterion))
			return fmt.Errorf("missing details for criterion: %s", criterion)
		}

		context, err := s.retrieveContext(ctx, criterion, petition.FieldOfExpertise, details)
		if err != nil {
			log.Printf("Warning: Failed to retrieve context for %s: %v. Continuing with empty context.", criterion, err)
			context = &RetrievedContext{}
		}

		content, err := s.generateProng1Section(ctx, criterion, details, context, petition.ClientName, petition.FieldOfExpertise)
		if err != nil {
			s.markJobFailed(ctx, jobID, fmt.Sprintf("failed to generate section for %s: %v", criterion, err))
			return fmt.Errorf("failed to generate section for %s: %w", criterion, err)
		}

		citations := s.extractCitations(context, criterion)
		sections = append(sections, DraftSection{
			Title:     getCriterionTitle(criterion),
			Content:   content,
			Citations: citations,
		})

		// Update step to completed
		err = s.updateStepStatus(ctx, jobID, stepName, "completed")
		if err != nil {
			s.markJobFailed(ctx, jobID, "failed to update step: "+err.Error())
			return err
		}
	}

	// 4. Generate Final Merits (Prong 2)
	err = s.updateStepStatus(ctx, jobID, "Final Merits Determination", "in_progress")
	if err != nil {
		s.markJobFailed(ctx, jobID, "failed to update step: "+err.Error())
		return err
	}

	finalMeritsContent, err := s.generateProng2(ctx, sections, petition)
	if err != nil {
		s.markJobFailed(ctx, jobID, fmt.Sprintf("failed to generate final merits: %v", err))
		return fmt.Errorf("failed to generate final merits: %w", err)
	}

	finalMerits := DraftSection{
		Title:   "Final Merits Determination",
		Content: finalMeritsContent,
	}
	sections = append(sections, finalMerits)

	err = s.updateStepStatus(ctx, jobID, "Final Merits Determination", "completed")
	if err != nil {
		s.markJobFailed(ctx, jobID, "failed to update step: "+err.Error())
		return err
	}

	// 5. Assemble document
	err = s.updateStepStatus(ctx, jobID, "Assembling Document", "in_progress")
	if err != nil {
		s.markJobFailed(ctx, jobID, "failed to update step: "+err.Error())
		return err
	}

	assembledContent := s.assembleDocument(petition, sections)

	err = s.updateStepStatus(ctx, jobID, "Assembling Document", "completed")
	if err != nil {
		s.markJobFailed(ctx, jobID, "failed to update step: "+err.Error())
		return err
	}

	// 6. Store result
	err = s.petitionRepo.UpdateGeneratedContent(ctx, job.PetitionID, assembledContent)
	if err != nil {
		s.markJobFailed(ctx, jobID, "failed to store generated content: "+err.Error())
		return err
	}

	// 7. Mark job as completed
	err = s.jobRepo.Complete(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to complete job: %w", err)
	}

	return nil
}

// DraftSection represents a section of the generated document
type DraftSection struct {
	Title     string
	Content   string
	Citations []string
}

// updateStepStatus updates the status of a specific step in the generation job
func (s *DraftService) updateStepStatus(ctx context.Context, jobID uuid.UUID, stepName, status string) error {
	job, err := s.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return err
	}

	steps := job.Steps
	var currentStep string
	if job.CurrentStep != nil {
		currentStep = *job.CurrentStep
	}

	for i := range steps {
		if steps[i].Name == stepName {
			steps[i].Status = status
			if status == "in_progress" {
				currentStep = stepName
			}
			break
		}
	}

	return s.jobRepo.UpdateProgress(ctx, jobID, currentStep, steps)
}

// markJobFailed marks a job as failed with an error message
func (s *DraftService) markJobFailed(ctx context.Context, jobID uuid.UUID, errorMessage string) {
	err := s.jobRepo.Fail(ctx, jobID, errorMessage)
	if err != nil {
		// Log error but don't return - we're already in error handling
		// In production, use proper logging
		_ = err
	}
}

// getCriterionTitle returns the human-readable title for a criterion
func getCriterionTitle(criterion string) string {
	titles := map[string]string{
		"awards":                 "Criterion 1: Receipt of Nationally or Internationally Recognized Prizes or Awards",
		"membership":             "Criterion 2: Membership in Associations",
		"media_coverage":         "Criterion 3: Published Material About the Person",
		"judging":                "Criterion 4: Participation as a Judge",
		"original_contributions": "Criterion 5: Original Scientific Contributions",
		"authorship":             "Criterion 6: Scholarly Articles",
		"exhibitions":            "Criterion 7: Display of Work",
		"critical_role":          "Criterion 8: Critical or Essential Capacity",
		"high_salary":            "Criterion 9: High Salary",
		"commercial_success":     "Criterion 10: Commercial Success",
	}
	if title, ok := titles[criterion]; ok {
		return title
	}
	return "Criterion: " + criterion
}

// RetrievedContext holds retrieved legal context for generation
type RetrievedContext struct {
	Regulations []models.LegalChunk // Legal standards (2-3 chunks)
	Appeals     []models.LegalChunk // Winning arguments (2-3 chunks)
	Cases       []models.LegalChunk // Precedent cases (1-2 chunks)
}

// EmbeddingRequest represents an embedding API request
type EmbeddingRequest struct {
	Model                string       `json:"model"`
	Content              ContentInput `json:"content"`
	TaskType             string       `json:"task_type,omitempty"`
	OutputDimensionality int          `json:"output_dimensionality,omitempty"`
}

// ContentInput represents content for embedding
type ContentInput struct {
	Parts []PartInput `json:"parts"`
}

// PartInput represents a part of content
type PartInput struct {
	Text string `json:"text"`
}

// EmbeddingResponse represents an embedding API response
type EmbeddingResponse struct {
	Embedding EmbeddingData `json:"embedding"`
}

// EmbeddingData contains the embedding values
type EmbeddingData struct {
	Values []float64 `json:"values"`
}

// sanitizeFieldOfExpertise extracts a canonical field name from user input
// Used only for embedding queries (not final document output)
// Preserves compound field names like "Artificial Intelligence and Machine Learning"
func (s *DraftService) sanitizeFieldOfExpertise(field string) string {
	field = strings.TrimSpace(field)

	// Remove common prefixes
	prefixes := []string{"I am a", "I am an", "I specialize in", "Field of", "Area of"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(strings.ToLower(field), strings.ToLower(prefix)) {
			field = strings.TrimSpace(field[len(prefix):])
		}
	}

	// Take first 5 words max to preserve compound field names
	// Examples: "Artificial Intelligence and Machine Learning" (5 words) stays intact
	// This ensures important terms aren't truncated in search queries
	words := strings.Fields(field)
	if len(words) > 5 {
		words = words[:5]
	}

	return strings.Join(words, " ")
}

// generateQueryEmbedding generates an embedding for a retrieval query
func (s *DraftService) generateQueryEmbedding(
	ctx context.Context,
	criterion string,
	fieldOfExpertise string,
	factSummary string,
) ([]float64, error) {
	// Sanitize field of expertise
	sanitizedField := s.sanitizeFieldOfExpertise(fieldOfExpertise)

	// Build query text
	queryText := fmt.Sprintf("[CRITERION: %s] [FIELD: %s] %s", criterion, sanitizedField, factSummary)

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not set")
	}

	reqBody := EmbeddingRequest{
		Model: "models/gemini-embedding-001",
		Content: ContentInput{
			Parts: []PartInput{{Text: queryText}},
		},
		TaskType:             "RETRIEVAL_QUERY",
		OutputDimensionality: 768,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var embedding []float64
	backoff := initialBackoff
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(backoff)
			backoff *= 2
		}

		req, err := http.NewRequestWithContext(ctx, "POST", embeddingAPI, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-goog-api-key", apiKey)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			if attempt == maxRetries-1 {
				return nil, fmt.Errorf("failed to send request after %d attempts: %w", maxRetries, err)
			}
			continue
		}

		if resp.StatusCode == http.StatusOK {
			var apiResp EmbeddingResponse
			if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
				resp.Body.Close()
				if attempt == maxRetries-1 {
					return nil, fmt.Errorf("failed to decode response: %w", err)
				}
				continue
			}
			resp.Body.Close()

			embedding = apiResp.Embedding.Values
			// Normalize embedding
			norm := 0.0
			for _, v := range embedding {
				norm += v * v
			}
			norm = math.Sqrt(norm)
			if norm > 0 {
				for i := range embedding {
					embedding[i] /= norm
				}
			}

			return embedding, nil
		}

		resp.Body.Close()

		// Don't retry on 400 or 401 errors
		if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("API error: %d", resp.StatusCode)
		}

		if attempt == maxRetries-1 {
			return nil, fmt.Errorf("API error after %d attempts: %d", maxRetries, resp.StatusCode)
		}
	}

	return nil, ErrEmbeddingFailed
}

// extractFactSummary extracts a fact summary from criterion details
func (s *DraftService) extractFactSummary(criterion string, details models.CriteriaDetail) string {
	var facts []string

	switch criterion {
	case "awards":
		if awards, ok := details["awards"].([]interface{}); ok {
			for _, award := range awards {
				if a, ok := award.(map[string]interface{}); ok {
					if name, ok := a["name"].(string); ok {
						facts = append(facts, name)
					}
					if desc, ok := a["description"].(string); ok && desc != "" {
						facts = append(facts, desc)
					}
				}
			}
		}
	case "judging":
		if venue, ok := details["venue"].(string); ok {
			facts = append(facts, venue)
		}
		if role, ok := details["role"].(string); ok {
			facts = append(facts, role)
		}
		if count, ok := details["papers_reviewed"].(float64); ok {
			facts = append(facts, fmt.Sprintf("%.0f papers reviewed", count))
		}
	case "authorship":
		if publications, ok := details["publications"].([]interface{}); ok {
			for _, pub := range publications {
				if p, ok := pub.(map[string]interface{}); ok {
					if title, ok := p["title"].(string); ok {
						facts = append(facts, title)
					}
					if journal, ok := p["journal"].(string); ok {
						facts = append(facts, journal)
					}
					// Extract citations - handle both int and float64 types
					// Use exact numbers from JSON to prevent hallucination
					if citationsFloat, ok := p["citations"].(float64); ok {
						facts = append(facts, fmt.Sprintf("%.0f citations", citationsFloat))
					} else if citationsInt, ok := p["citations"].(int); ok {
						facts = append(facts, fmt.Sprintf("%d citations", citationsInt))
					} else if citationsInt64, ok := p["citations"].(int64); ok {
						facts = append(facts, fmt.Sprintf("%d citations", citationsInt64))
					}
				}
			}
		}
	case "original_contributions":
		if contributions, ok := details["contributions"].([]interface{}); ok {
			for _, contrib := range contributions {
				if c, ok := contrib.(map[string]interface{}); ok {
					if title, ok := c["title"].(string); ok {
						facts = append(facts, title)
					}
					if impact, ok := c["impact"].(string); ok {
						facts = append(facts, impact)
					}
				}
			}
		}
	default:
		// Generic extraction for other criteria
		if desc, ok := details["description"].(string); ok {
			facts = append(facts, desc)
		}
	}

	return strings.Join(facts, " ")
}

// retrieveContext retrieves legal context for a criterion
func (s *DraftService) retrieveContext(
	ctx context.Context,
	criterion string,
	fieldOfExpertise string,
	details models.CriteriaDetail,
) (*RetrievedContext, error) {
	if s.legalChunkRepo == nil {
		return nil, errors.New("legal chunk repository not set")
	}

	factSummary := s.extractFactSummary(criterion, details)

	// Generate query embedding
	embedding, err := s.generateQueryEmbedding(ctx, criterion, fieldOfExpertise, factSummary)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	context := &RetrievedContext{}

	// Retrieve regulations
	regs, err := s.legalChunkRepo.SearchByCriterion(ctx, embedding, criterion, "regulation", 3)
	if err != nil {
		log.Printf("Warning: Failed to retrieve regulations: %v", err)
	} else {
		context.Regulations = regs
	}

	// Retrieve appeals
	appeals, err := s.legalChunkRepo.SearchByCriterion(ctx, embedding, criterion, "appeal_decision", 3)
	if err != nil {
		log.Printf("Warning: Failed to retrieve appeals: %v", err)
	} else {
		context.Appeals = appeals
	}

	// Retrieve cases
	cases, err := s.legalChunkRepo.SearchByCriterion(ctx, embedding, criterion, "precedent_case", 2)
	if err != nil {
		log.Printf("Warning: Failed to retrieve cases: %v", err)
	} else {
		context.Cases = cases
	}

	return context, nil
}

// getCriterionCitation returns the regulatory citation for a criterion
// Format matches IMPLEMENTATION_SPEC.md Appendix B: (8 C.F.R. § 214.2(o)(3)(iii)(X))
// All 10 O-1A criteria are mapped A-J as specified
func getCriterionCitation(criterion string) string {
	citations := map[string]string{
		"awards":                 "(8 C.F.R. § 214.2(o)(3)(iii)(A))",
		"membership":             "(8 C.F.R. § 214.2(o)(3)(iii)(B))",
		"media_coverage":         "(8 C.F.R. § 214.2(o)(3)(iii)(C))",
		"judging":                "(8 C.F.R. § 214.2(o)(3)(iii)(D))",
		"original_contributions": "(8 C.F.R. § 214.2(o)(3)(iii)(E))",
		"authorship":             "(8 C.F.R. § 214.2(o)(3)(iii)(F))",
		"exhibitions":            "(8 C.F.R. § 214.2(o)(3)(iii)(G))",
		"critical_role":          "(8 C.F.R. § 214.2(o)(3)(iii)(H))",
		"high_salary":            "(8 C.F.R. § 214.2(o)(3)(iii)(I))",
		"commercial_success":     "(8 C.F.R. § 214.2(o)(3)(iii)(J))",
	}
	if citation, ok := citations[criterion]; ok {
		return citation
	}
	return ""
}

// getHardcodedRegulation returns fallback regulation text for a criterion
// This prevents LLM hallucination when retrieval fails or returns empty results
func getHardcodedRegulation(criterion string) string {
	regulations := map[string]string{
		"awards":                 `Documentation of the alien's receipt of lesser nationally or internationally recognized prizes or awards for excellence in the field of endeavor (8 C.F.R. § 214.2(o)(3)(iii)(A)).`,
		"membership":             `Documentation of the alien's membership in associations in the field for which classification is sought, which require outstanding achievements of their members, as judged by recognized national or international experts in their disciplines or fields (8 C.F.R. § 214.2(o)(3)(iii)(B)).`,
		"media_coverage":         `Published material about the alien in professional or major trade publications or other major media, relating to the alien's work in the field for which classification is sought. Such evidence shall include the title, date, and author of the material, and any necessary translation (8 C.F.R. § 214.2(o)(3)(iii)(C)).`,
		"judging":                `Evidence of the alien's participation, either individually or on a panel, as a judge of the work of others in the same or an allied field of specification for which classification is sought (8 C.F.R. § 214.2(o)(3)(iii)(D)).`,
		"original_contributions": `Evidence of the alien's original scientific, scholarly, artistic, athletic, or business-related contributions of major significance in the field (8 C.F.R. § 214.2(o)(3)(iii)(E)).`,
		"authorship":             `Evidence of the alien's authorship of scholarly articles in the field, in professional or major trade publications or other major media (8 C.F.R. § 214.2(o)(3)(iii)(F)).`,
		"exhibitions":            `Evidence of the display of the alien's work in the field at artistic exhibitions or showcases (8 C.F.R. § 214.2(o)(3)(iii)(G)).`,
		"critical_role":          `Evidence that the alien has performed in a leading or critical role for organizations or establishments that have a distinguished reputation (8 C.F.R. § 214.2(o)(3)(iii)(H)).`,
		"high_salary":            `Evidence that the alien has commanded a high salary or other significantly high remuneration for services, in relation to others in the field (8 C.F.R. § 214.2(o)(3)(iii)(I)).`,
		"commercial_success":     `Evidence of commercial successes in the performing arts, as shown by box office receipts or record, cassette, compact disk, or video sales (8 C.F.R. § 214.2(o)(3)(iii)(J)).`,
	}
	if regulation, ok := regulations[criterion]; ok {
		return regulation
	}
	return "Evidence that the alien meets the regulatory criteria for extraordinary ability (8 C.F.R. § 214.2(o)(3)(iii))."
}

// formatClientFacts formats criterion details as a readable string
func (s *DraftService) formatClientFacts(criterion string, details models.CriteriaDetail) string {
	var builder strings.Builder

	switch criterion {
	case "awards":
		if awards, ok := details["awards"].([]interface{}); ok {
			for i, award := range awards {
				if i > 0 {
					builder.WriteString("\n")
				}
				if a, ok := award.(map[string]interface{}); ok {
					if name, ok := a["name"].(string); ok {
						builder.WriteString(fmt.Sprintf("Award: %s", name))
					}
					if date, ok := a["date"].(string); ok {
						builder.WriteString(fmt.Sprintf(" (Date: %s)", date))
					}
					if desc, ok := a["description"].(string); ok && desc != "" {
						builder.WriteString(fmt.Sprintf("\nDescription: %s", desc))
					}
					// Add impact hints if available
					if importance, ok := a["importance"].(string); ok && importance != "" {
						builder.WriteString(fmt.Sprintf("\nSignificance: %s", importance))
					}
					if impact, ok := a["impact"].(string); ok && impact != "" {
						builder.WriteString(fmt.Sprintf("\nImpact: %s", impact))
					}
				}
			}
		}
	case "judging":
		if venue, ok := details["venue"].(string); ok {
			builder.WriteString(fmt.Sprintf("Venue: %s\n", venue))
		}
		if role, ok := details["role"].(string); ok {
			builder.WriteString(fmt.Sprintf("Role: %s\n", role))
		}
		if count, ok := details["papers_reviewed"].(float64); ok {
			builder.WriteString(fmt.Sprintf("Papers Reviewed: %.0f", count))
		}
		// Add impact hints if available
		if importance, ok := details["importance"].(string); ok && importance != "" {
			builder.WriteString(fmt.Sprintf("\nSignificance: %s", importance))
		}
		if impact, ok := details["impact"].(string); ok && impact != "" {
			builder.WriteString(fmt.Sprintf("\nImpact: %s", impact))
		}
	case "authorship":
		if publications, ok := details["publications"].([]interface{}); ok {
			for i, pub := range publications {
				if i > 0 {
					builder.WriteString("\n\n")
				}
				builder.WriteString(fmt.Sprintf("Publication %d:", i+1))
				if p, ok := pub.(map[string]interface{}); ok {
					if title, ok := p["title"].(string); ok {
						builder.WriteString(fmt.Sprintf("\nTitle: %s", title))
					}
					if journal, ok := p["journal"].(string); ok {
						builder.WriteString(fmt.Sprintf("\nJournal: %s", journal))
					}
					// Add journal impact factor if available
					if impactFactor, ok := p["impact_factor"].(float64); ok && impactFactor > 0 {
						builder.WriteString(fmt.Sprintf("\nImpact Factor: %.2f", impactFactor))
					}
					// Extract citations - handle both int and float64 types
					// CRITICAL: Use exact numbers to prevent LLM hallucination
					if citationsFloat, ok := p["citations"].(float64); ok {
						builder.WriteString(fmt.Sprintf("\nCitations: %.0f", citationsFloat))
					} else if citationsInt, ok := p["citations"].(int); ok {
						builder.WriteString(fmt.Sprintf("\nCitations: %d", citationsInt))
					} else if citationsInt64, ok := p["citations"].(int64); ok {
						builder.WriteString(fmt.Sprintf("\nCitations: %d", citationsInt64))
					}
					// Add impact hints if available
					if importance, ok := p["importance"].(string); ok && importance != "" {
						builder.WriteString(fmt.Sprintf("\nSignificance: %s", importance))
					}
					if impact, ok := p["impact"].(string); ok && impact != "" {
						builder.WriteString(fmt.Sprintf("\nImpact: %s", impact))
					}
				}
			}
		}
	default:
		if desc, ok := details["description"].(string); ok {
			builder.WriteString(desc)
		} else {
			builder.WriteString(fmt.Sprintf("%v", details))
		}
		// Add impact hints if available for generic criteria
		if importance, ok := details["importance"].(string); ok && importance != "" {
			builder.WriteString(fmt.Sprintf("\nSignificance: %s", importance))
		}
		if impact, ok := details["impact"].(string); ok && impact != "" {
			builder.WriteString(fmt.Sprintf("\nImpact: %s", impact))
		}
	}

	return builder.String()
}

// getMostCompellingFact extracts the most compelling fact from details
func (s *DraftService) getMostCompellingFact(criterion string, details models.CriteriaDetail) string {
	switch criterion {
	case "awards":
		if awards, ok := details["awards"].([]interface{}); ok && len(awards) > 0 {
			if a, ok := awards[0].(map[string]interface{}); ok {
				if name, ok := a["name"].(string); ok {
					return name
				}
			}
		}
	case "judging":
		if venue, ok := details["venue"].(string); ok {
			return fmt.Sprintf("serving as %s at %s", details["role"], venue)
		}
	case "authorship":
		if publications, ok := details["publications"].([]interface{}); ok && len(publications) > 0 {
			if p, ok := publications[0].(map[string]interface{}); ok {
				if title, ok := p["title"].(string); ok {
					return title
				}
			}
		}
	}
	return "their achievements in the field"
}

// generateProng1Section generates a Prong 1 section using IRAC format
func (s *DraftService) generateProng1Section(
	ctx context.Context,
	criterion string,
	details models.CriteriaDetail,
	context *RetrievedContext,
	clientName string,
	fieldOfExpertise string,
) (string, error) {
	if s.geminiClient == nil {
		return "", errors.New("gemini client not set")
	}

	// Build prompt
	var regulationText strings.Builder
	for _, reg := range context.Regulations {
		regulationText.WriteString(reg.Text)
		regulationText.WriteString("\n\n")
	}

	// Guard clause: Use fallback if no regulation context retrieved
	// This prevents LLM hallucination when retrieval fails
	if regulationText.Len() == 0 {
		log.Printf("Warning: No regulation context found for %s. Using fallback.", criterion)
		regulationText.WriteString(getHardcodedRegulation(criterion))
		regulationText.WriteString("\n\n")
	}

	var appealText strings.Builder
	for _, appeal := range context.Appeals {
		appealText.WriteString(appeal.Text)
		appealText.WriteString("\n\n")
	}

	clientFacts := s.formatClientFacts(criterion, details)
	specificFact := s.getMostCompellingFact(criterion, details)
	criterionTitle := getCriterionTitle(criterion)
	citation := getCriterionCitation(criterion)

	prompt := fmt.Sprintf(`You are an expert O-1A immigration attorney drafting a support letter section.

LEGAL STANDARD:
%s

PRECEDENT CASES:
%s

CLIENT FACTS:
%s

FIELD OF EXPERTISE: %s

TASK:
Write the "%s" section using IRAC format:

1. Issue: State the legal requirement in plain language (1 paragraph)
2. Rule: Cite the regulation above %s (1 paragraph)
3. Analysis: 
   - Present the client's specific achievement: %s
   - Argue by analogy to the precedent case(s) above
   - Preempt common denial reasons (e.g., "This is not a student award but a professional recognition")
   - Link to field of expertise (3-4 paragraphs)
4. Conclusion: State that the client satisfies this criterion (1 paragraph)

OUTPUT REQUIREMENTS:
- Use formal legal language
- Include proper citations: %s
- 5-7 paragraphs total
- No markdown formatting (plain text)
- Write in third person about the client
- When referencing specific evidence (awards, publications, etc.), append [Exhibit __] placeholders at the end of the sentence (e.g., "as documented in the exhibits attached hereto [Exhibit A]")
- Do NOT include a section header/title - the content will be inserted under an existing header
- CRITICAL: Use EXACT numbers from CLIENT FACTS above. Do NOT estimate, round, or aggregate numbers. If CLIENT FACTS shows "Citations: 89", use "89 citations" exactly, not "350 citations" or any other number.

TONE CONSTRAINTS (CRITICAL):
- Do NOT use flowery adjectives (e.g., "game-changing", "revolutionary", "esteemed", "world-renowned")
- Use objective descriptors (e.g., "significant", "highly cited", "nationally recognized", "peer-reviewed")
- Avoid hyperbole and marketing language
- Maintain professional, factual tone throughout

Write the section now:`,
		regulationText.String(),
		appealText.String(),
		clientFacts,
		fieldOfExpertise,
		criterionTitle,
		citation,
		specificFact,
		citation,
	)

	// Generate content with retry using HTTP API
	systemInstruction := "You are an expert O-1A immigration attorney. Use formal legal language. Avoid flowery adjectives. Use objective descriptors only."
	fullPrompt := systemInstruction + "\n\n" + prompt

	var content string
	var err error
	backoff := initialBackoff
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(backoff)
			backoff *= 2
		}

		// Truncate prompt if too long to avoid context limits
		truncatedPrompt := fullPrompt
		// Gemini models have context limits - if prompt is very long, truncate
		if len(fullPrompt) > 30000 { // Rough estimate: truncate if over ~30k chars
			truncatedPrompt = fullPrompt[:30000] + "\n\n[Content truncated due to length...]"
			log.Printf("Warning: Prompt too long (%d chars), truncating to 30000 chars", len(fullPrompt))
		}
		content, err = s.callGenerationAPI(ctx, truncatedPrompt, 0.2)
		if err != nil {
			if attempt == maxRetries-1 {
				return "", fmt.Errorf("failed to generate content after %d attempts: %w", maxRetries, err)
			}
			continue
		}

		if content != "" {
			break
		}

		if attempt == maxRetries-1 {
			return "", ErrGenerationFailed
		}
	}

	if content == "" {
		return "", ErrGenerationFailed
	}

	return content, nil
}

// generateProng2 generates the Final Merits Determination section
func (s *DraftService) generateProng2(
	ctx context.Context,
	sections []DraftSection,
	petition *models.Petition,
) (string, error) {
	if s.geminiClient == nil {
		return "", errors.New("gemini client not set")
	}
	if s.legalChunkRepo == nil {
		return "", errors.New("legal chunk repository not set")
	}

	// Retrieve Kazarian and Chawathe context
	factSummary := fmt.Sprintf("Final Merits Kazarian Chawathe %s", petition.FieldOfExpertise)
	embedding, err := s.generateQueryEmbedding(ctx, "", petition.FieldOfExpertise, factSummary)
	if err != nil {
		log.Printf("Warning: Failed to generate embedding for Prong 2: %v", err)
		embedding = make([]float64, 768) // Use zero vector as fallback
	}

	var kazarianText strings.Builder
	var chawatheText strings.Builder

	// Search for Kazarian chunks
	kazarianChunks, err := s.legalChunkRepo.SearchByCriterion(ctx, embedding, "", "regulation", 5)
	if err == nil {
		for _, chunk := range kazarianChunks {
			if chunk.LegalStandard != nil && strings.Contains(*chunk.LegalStandard, "Kazarian") {
				kazarianText.WriteString(chunk.Text)
				kazarianText.WriteString("\n\n")
			}
		}
	}

	// Search for Chawathe chunks
	chawatheChunks, err := s.legalChunkRepo.SearchByCriterion(ctx, embedding, "", "appeal_decision", 5)
	if err == nil {
		for _, chunk := range chawatheChunks {
			if chunk.AppealCitation != nil && strings.Contains(*chunk.AppealCitation, "Chawathe") {
				chawatheText.WriteString(chunk.Text)
				chawatheText.WriteString("\n\n")
			}
		}
	}

	// Build criteria summary
	var criteriaSummary strings.Builder
	for i, criterion := range petition.SelectedCriteria {
		if i > 0 {
			criteriaSummary.WriteString(", ")
		}
		criteriaSummary.WriteString(getCriterionTitle(criterion))
	}

	// Build prompt
	prompt := fmt.Sprintf(`You are an expert O-1A immigration attorney drafting the Final Merits Determination section.

LEGAL STANDARD (Kazarian):
%s

STANDARD OF PROOF (Chawathe):
%s

CRITERIA SATISFIED:
The client has satisfied the following criteria:
%s

TASK:
Write the "Final Merits Determination" section that:

1. Opens by stating the "Preponderance of the Evidence" standard (Matter of Chawathe) to frame the legal standard immediately
2. States the legal standard (Kazarian two-part test)
3. Summarizes the evidence presented (do not repeat verbatim)
4. Argues that the totality of evidence demonstrates the client has risen to the very top of the field
5. Links the criteria together (e.g., "The client's awards (Criterion 1) are supported by their peer recognition as a Senior Area Chair (Criterion 3), which together with their highly cited publications (Criterion 2) demonstrate sustained impact")
6. Concludes by reinforcing the preponderance of evidence standard

OUTPUT REQUIREMENTS:
- Use formal legal language
- Include proper citations: (8 C.F.R. § 214.2(o)(3)(iii)) and (Matter of Chawathe, 25 I&N Dec. 369 (AAO 2010))
- 6-8 paragraphs
- No markdown formatting
- Write in third person
- Do NOT include a section header/title - the content will be inserted under an existing header
- CRITICAL: When referencing specific numbers (citation counts, award dates, etc.), use the EXACT numbers from the evidence presented in the criteria sections above. Do NOT estimate, round, or aggregate numbers.

TONE CONSTRAINTS (CRITICAL):
- Do NOT use flowery adjectives (e.g., "game-changing", "revolutionary", "esteemed", "world-renowned")
- Use objective descriptors (e.g., "significant", "highly cited", "nationally recognized", "peer-reviewed")
- Avoid hyperbole and marketing language
- Maintain professional, factual tone throughout

Write the section now:`,
		kazarianText.String(),
		chawatheText.String(),
		criteriaSummary.String(),
	)

	// Generate content with retry using HTTP API
	systemInstruction := "You are an expert O-1A immigration attorney. Use formal legal language. Avoid flowery adjectives. Use objective descriptors only."
	fullPrompt := systemInstruction + "\n\n" + prompt

	var content string
	backoff := initialBackoff
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(backoff)
			backoff *= 2
		}

		// Truncate prompt if too long to avoid context limits
		truncatedPrompt := fullPrompt
		if len(fullPrompt) > 30000 {
			truncatedPrompt = fullPrompt[:30000] + "\n\n[Content truncated due to length...]"
			log.Printf("Warning: Prompt too long (%d chars), truncating to 30000 chars", len(fullPrompt))
		}
		content, err = s.callGenerationAPI(ctx, truncatedPrompt, 0.3)
		if err != nil {
			if attempt == maxRetries-1 {
				return "", fmt.Errorf("failed to generate final merits after %d attempts: %w", maxRetries, err)
			}
			continue
		}

		if content != "" {
			break
		}

		if attempt == maxRetries-1 {
			return "", ErrGenerationFailed
		}
	}

	if content == "" {
		return "", ErrGenerationFailed
	}

	return content, nil
}

// extractCitations extracts citations from retrieved context
func (s *DraftService) extractCitations(context *RetrievedContext, criterion string) []string {
	citations := make([]string, 0)

	// Add regulatory citation
	citation := getCriterionCitation(criterion)
	if citation != "" {
		citations = append(citations, citation)
	}

	// Add appeal citations
	for _, appeal := range context.Appeals {
		if appeal.AppealCitation != nil {
			citations = append(citations, *appeal.AppealCitation)
		}
	}

	// Add case citations
	for _, caseChunk := range context.Cases {
		if caseChunk.CaseCitation != nil {
			citations = append(citations, *caseChunk.CaseCitation)
		}
	}

	return citations
}

// callGenerationAPI calls the Gemini generation API directly via HTTP
func (s *DraftService) callGenerationAPI(ctx context.Context, prompt string, temperature float64) (string, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY not set")
	}

	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature": temperature,
			// No maxOutputTokens limit - let the model use its default maximum
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", generationAPI, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Gemini API error: Status %d, Body: %s", resp.StatusCode, string(bodyBytes))
		return "", fmt.Errorf("API error: %d - %s", resp.StatusCode, string(bodyBytes))
	}

	var apiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason,omitempty"`
		} `json:"candidates"`
		PromptFeedback struct {
			BlockReason string `json:"blockReason,omitempty"`
		} `json:"promptFeedback,omitempty"`
		Error struct {
			Code    int    `json:"code,omitempty"`
			Message string `json:"message,omitempty"`
		} `json:"error,omitempty"`
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		log.Printf("Failed to decode response. Body: %s", string(bodyBytes))
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	// Check for API errors in response
	if apiResp.Error.Message != "" {
		return "", fmt.Errorf("API error: %s (code: %d)", apiResp.Error.Message, apiResp.Error.Code)
	}

	if apiResp.PromptFeedback.BlockReason != "" {
		return "", fmt.Errorf("API blocked prompt: %s", apiResp.PromptFeedback.BlockReason)
	}

	if len(apiResp.Candidates) == 0 {
		log.Printf("API returned no candidates. Full response: %s", string(bodyBytes))
		return "", fmt.Errorf("API returned no candidates")
	}

	var responseText strings.Builder
	for i, candidate := range apiResp.Candidates {
		// Log finish reason if present (e.g., SAFETY, RECITATION)
		if candidate.FinishReason != "" && candidate.FinishReason != "STOP" {
			log.Printf("Warning: Candidate %d finished with reason: %s", i, candidate.FinishReason)
		}

		// Check if content exists and has parts
		if len(candidate.Content.Parts) == 0 {
			// Log the actual candidate structure for debugging
			candidateJSON, _ := json.Marshal(candidate)
			log.Printf("Error: Candidate %d has no parts. Candidate structure: %s", i, string(candidateJSON))
			log.Printf("Full API response (first 1000 chars): %s", string(bodyBytes[:min(1000, len(bodyBytes))]))
			return "", fmt.Errorf("API candidate has no parts (finish reason: %s)", candidate.FinishReason)
		}

		for j, part := range candidate.Content.Parts {
			if part.Text == "" {
				log.Printf("Warning: Candidate %d, part %d has empty text", i, j)
				continue // Skip empty parts but don't fail
			}
			responseText.WriteString(part.Text)
		}
	}

	result := responseText.String()
	if result == "" {
		return "", fmt.Errorf("API returned empty content")
	}

	return result, nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// assembleDocument combines all sections into a complete document
func (s *DraftService) assembleDocument(petition *models.Petition, sections []DraftSection) string {
	var builder strings.Builder

	builder.WriteString("PETITION FOR O-1A VISA\n\n")
	builder.WriteString("I. INTRODUCTION\n")
	builder.WriteString(fmt.Sprintf("%s, in the field of %s\n\n",
		petition.ClientName, petition.FieldOfExpertise))

	builder.WriteString("II. QUALIFICATIONS SUMMARY\n")
	builder.WriteString(fmt.Sprintf("The client has satisfied the following criteria: %s\n\n",
		strings.Join(petition.SelectedCriteria, ", ")))

	builder.WriteString("III. REGULATORY CRITERIA\n\n")
	for _, section := range sections {
		if section.Title != "Final Merits Determination" {
			// Check if content already starts with a header (common patterns)
			content := section.Content
			contentLower := strings.ToLower(strings.TrimSpace(content))

			// Remove duplicate header if content already starts with the section title
			titleLower := strings.ToLower(section.Title)
			if strings.HasPrefix(contentLower, titleLower) {
				// Content already has header, skip adding it
				builder.WriteString(content + "\n\n")
			} else {
				// Add header if not present
				builder.WriteString(section.Title + "\n")
				builder.WriteString(content + "\n\n")
			}
		}
	}

	builder.WriteString("IV. FINAL MERITS DETERMINATION\n")
	for _, section := range sections {
		if section.Title == "Final Merits Determination" {
			// Check if content already starts with "Final Merits Determination" header
			content := section.Content
			contentLower := strings.ToLower(strings.TrimSpace(content))

			// Remove duplicate header if present
			if strings.HasPrefix(contentLower, "final merits determination") {
				// Find where the header ends (after colon or newline)
				idx := strings.Index(content, ":")
				if idx > 0 && idx < 100 {
					content = strings.TrimSpace(content[idx+1:])
				} else {
					// Try to find newline after header
					lines := strings.SplitN(content, "\n", 2)
					if len(lines) > 1 && strings.Contains(strings.ToLower(lines[0]), "final merits") {
						content = strings.TrimSpace(lines[1])
					}
				}
			}
			builder.WriteString(content + "\n\n")
		}
	}

	builder.WriteString("V. CONCLUSION\n")
	builder.WriteString("Based on the evidence presented, the client satisfies the requirements for O-1A classification.\n")

	return builder.String()
}
