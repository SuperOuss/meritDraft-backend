package handlers

import (
	"context"
	"io"
	"log"
	"net/http"

	"meritdraft-backend/models"
	"meritdraft-backend/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PetitionHandler handles HTTP requests for petitions
type PetitionHandler struct {
	petitionService *service.PetitionService
	draftService    *service.DraftService
}

// NewPetitionHandler creates a new petition handler
func NewPetitionHandler(petitionService *service.PetitionService, draftService *service.DraftService) *PetitionHandler {
	return &PetitionHandler{
		petitionService: petitionService,
		draftService:    draftService,
	}
}

// CreatePetitionRequest represents the request body for creating a petition
type CreatePetitionRequest struct {
	UserID           string                 `json:"user_id" binding:"required"`
	Status           string                 `json:"status"`
	ClientName       string                 `json:"client_name"`
	VisaType         string                 `json:"visa_type"`
	PetitionerName   string                 `json:"petitioner_name"`
	FieldOfExpertise string                 `json:"field_of_expertise"`
	SelectedCriteria []string               `json:"selected_criteria"`
	CriteriaDetails  map[string]interface{} `json:"criteria_details"`
}

// CreatePetition handles POST /api/petitions
func (h *PetitionHandler) CreatePetition(c *gin.Context) {
	var req CreatePetitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": err.Error(),
			},
		})
		return
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_USER_ID",
				"message": "Invalid user_id format",
			},
		})
		return
	}

	var status models.PetitionStatus
	if req.Status != "" {
		status = models.PetitionStatus(req.Status)
	} else {
		status = models.StatusDraft
	}

	serviceReq := service.CreatePetitionRequest{
		UserID: userID,
		Status: status,
	}

	result, err := h.petitionService.CreatePetition(c.Request.Context(), serviceReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "CREATE_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	// Update with additional fields if provided
	if req.ClientName != "" || req.VisaType != "" || req.PetitionerName != "" || req.FieldOfExpertise != "" {
		result.Petition.ClientName = req.ClientName
		if req.VisaType != "" {
			result.Petition.VisaType = models.VisaType(req.VisaType)
		}
		result.Petition.PetitionerName = req.PetitionerName
		result.Petition.FieldOfExpertise = req.FieldOfExpertise

		if len(req.SelectedCriteria) > 0 {
			result.Petition.SelectedCriteria = req.SelectedCriteria
		}

		if req.CriteriaDetails != nil {
			criteriaDetails := make(models.CriteriaDetails)
			for k, v := range req.CriteriaDetails {
				if detailMap, ok := v.(map[string]interface{}); ok {
					criteriaDetails[k] = models.CriteriaDetail(detailMap)
				}
			}
			result.Petition.CriteriaDetails = criteriaDetails
		}

		updateReq := service.UpdatePetitionRequest{
			Petition: result.Petition,
		}
		_, err = h.petitionService.UpdatePetition(c.Request.Context(), updateReq)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "UPDATE_FAILED",
					"message": err.Error(),
				},
			})
			return
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    result.Petition,
	})
}

// GetPetition handles GET /api/petitions/:id
func (h *PetitionHandler) GetPetition(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_ID",
				"message": "Invalid petition ID format",
			},
		})
		return
	}

	serviceReq := service.GetPetitionRequest{
		ID: id,
	}

	result, err := h.petitionService.GetPetition(c.Request.Context(), serviceReq)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "Petition not found",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result.Petition,
	})
}

// UpdatePetitionRequest represents the request body for updating a petition
type UpdatePetitionRequest struct {
	Status            string                 `json:"status"`
	ClientName        string                 `json:"client_name"`
	VisaType          string                 `json:"visa_type"`
	PetitionerName    string                 `json:"petitioner_name"`
	FieldOfExpertise  string                 `json:"field_of_expertise"`
	CVFileID          *string                `json:"cv_file_id"`
	JobOfferFileID    *string                `json:"job_offer_file_id"`
	ScholarLink       *string                `json:"scholar_link"`
	ParsedDocuments   map[string]interface{} `json:"parsed_documents"`
	SelectedCriteria  []string               `json:"selected_criteria"`
	CriteriaDetails   map[string]interface{} `json:"criteria_details"`
	RefineInstructions *string               `json:"refine_instructions"`
}

// UpdatePetition handles PUT /api/petitions/:id
func (h *PetitionHandler) UpdatePetition(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_ID",
				"message": "Invalid petition ID format",
			},
		})
		return
	}

	// Get existing petition
	getReq := service.GetPetitionRequest{ID: id}
	result, err := h.petitionService.GetPetition(c.Request.Context(), getReq)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "Petition not found",
			},
		})
		return
	}

	petition := result.Petition

	var req UpdatePetitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": err.Error(),
			},
		})
		return
	}

	// Update fields if provided
	if req.Status != "" {
		petition.Status = models.PetitionStatus(req.Status)
	}
	if req.ClientName != "" {
		petition.ClientName = req.ClientName
	}
	if req.VisaType != "" {
		petition.VisaType = models.VisaType(req.VisaType)
	}
	if req.PetitionerName != "" {
		petition.PetitionerName = req.PetitionerName
	}
	if req.FieldOfExpertise != "" {
		petition.FieldOfExpertise = req.FieldOfExpertise
	}
	if req.CVFileID != nil {
		cvID, err := uuid.Parse(*req.CVFileID)
		if err == nil {
			petition.CVFileID = &cvID
		}
	}
	if req.JobOfferFileID != nil {
		jobID, err := uuid.Parse(*req.JobOfferFileID)
		if err == nil {
			petition.JobOfferFileID = &jobID
		}
	}
	if req.ScholarLink != nil {
		petition.ScholarLink = req.ScholarLink
	}
	if req.ParsedDocuments != nil {
		pubCount, _ := req.ParsedDocuments["publicationsCount"].(float64)
		citCount, _ := req.ParsedDocuments["citationsCount"].(float64)
		petition.ParsedDocuments = &models.ParsedDocuments{
			PublicationsCount: int(pubCount),
			CitationsCount:    int(citCount),
		}
	}
	if req.SelectedCriteria != nil {
		petition.SelectedCriteria = req.SelectedCriteria
	}
	if req.CriteriaDetails != nil {
		criteriaDetails := make(models.CriteriaDetails)
		for k, v := range req.CriteriaDetails {
			if detailMap, ok := v.(map[string]interface{}); ok {
				criteriaDetails[k] = models.CriteriaDetail(detailMap)
			}
		}
		petition.CriteriaDetails = criteriaDetails
	}
	if req.RefineInstructions != nil {
		petition.RefineInstructions = req.RefineInstructions
	}

	updateReq := service.UpdatePetitionRequest{
		Petition: petition,
	}

	updateResult, err := h.petitionService.UpdatePetition(c.Request.Context(), updateReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "UPDATE_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    updateResult.Petition,
	})
}

// GenerateDraft handles POST /api/petitions/:id/generate
func (h *PetitionHandler) GenerateDraft(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_ID",
				"message": "Invalid petition ID format",
			},
		})
		return
	}

	var reqBody struct {
		RefineInstructions *string `json:"refine_instructions"`
	}
	if err := c.ShouldBindJSON(&reqBody); err != nil && err != io.EOF {
		// JSON is optional, ignore binding errors if body is empty
	}

	serviceReq := service.GenerateDraftRequest{
		PetitionID:         id,
		RefineInstructions: reqBody.RefineInstructions,
	}

	// Create job (synchronous, fast)
	result, err := h.draftService.GenerateDraft(c.Request.Context(), serviceReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "GENERATION_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	// Spawn background goroutine for actual processing
	// Use background context (not request context) to avoid cancellation
	go func() {
		bgCtx := context.Background()
		if err := h.draftService.ProcessDraft(bgCtx, result.JobID); err != nil {
			// Error is logged and stored in job.ErrorMessage
			// No need to return to HTTP client (they'll poll status)
			log.Printf("Generation job %s failed: %v", result.JobID, err)
		}
	}()

	// Return immediately (within 100ms)
	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"data": gin.H{
			"job_id":  result.JobID,
			"status":   "pending",
			"message": "Generation job created. Poll /api/jobs/:id for updates.",
		},
	})
}

// GetJobStatus handles GET /api/jobs/:id
func (h *PetitionHandler) GetJobStatus(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_ID",
				"message": "Invalid job ID format",
			},
		})
		return
	}

	serviceReq := service.GetJobStatusRequest{
		JobID: id,
	}

	result, err := h.draftService.GetJobStatus(c.Request.Context(), serviceReq)
	if err != nil {
		if err == service.ErrJobNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "NOT_FOUND",
					"message": "Generation job not found",
				},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "RETRIEVAL_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result.Job,
	})
}

