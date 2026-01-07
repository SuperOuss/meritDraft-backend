package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"meritdraft-backend/models"
	"meritdraft-backend/repository"
	"meritdraft-backend/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// FileHandler handles HTTP requests for file operations
type FileHandler struct {
	fileRepo        *repository.FileRepository
	petitionRepo    *repository.PetitionRepository
	storage         storage.Storage
	maxFileSize     int64
	allowedMimeTypes map[string]bool
}

// NewFileHandler creates a new file handler
func NewFileHandler(fileRepo *repository.FileRepository, petitionRepo *repository.PetitionRepository, storage storage.Storage) *FileHandler {
	return &FileHandler{
		fileRepo:        fileRepo,
		petitionRepo:    petitionRepo,
		storage:         storage,
		maxFileSize:     10 * 1024 * 1024, // 10MB
		allowedMimeTypes: map[string]bool{
			"application/pdf":      true,
			"text/plain":           true,
			"application/msword":   true, // .doc
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true, // .docx
		},
	}
}

// UploadFile handles POST /api/files/upload
func (h *FileHandler) UploadFile(c *gin.Context) {
	// Get user_id from query or form (for now, we'll get it from petition)
	petitionIDStr := c.PostForm("petition_id")
	var petitionID *uuid.UUID
	var userID uuid.UUID

	if petitionIDStr != "" {
		pid, err := uuid.Parse(petitionIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "INVALID_PETITION_ID",
					"message": "Invalid petition_id format",
				},
			})
			return
		}
		petitionID = &pid

		// Get petition to retrieve user_id
		petition, err := h.petitionRepo.GetByID(c.Request.Context(), pid)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "PETITION_NOT_FOUND",
					"message": "Petition not found",
				},
			})
			return
		}
		userID = petition.UserID
	} else {
		// If no petition_id, require user_id in form
		userIDStr := c.PostForm("user_id")
		if userIDStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "MISSING_USER_ID",
					"message": "Either petition_id or user_id is required",
				},
			})
			return
		}
		uid, err := uuid.Parse(userIDStr)
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
		userID = uid
	}

	// Get file from form
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "MISSING_FILE",
				"message": "File is required",
			},
		})
		return
	}

	// Validate file size
	if fileHeader.Size > h.maxFileSize {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "FILE_TOO_LARGE",
				"message": fmt.Sprintf("File size exceeds maximum of %d bytes", h.maxFileSize),
			},
		})
		return
	}

	// Open file
	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "FILE_OPEN_ERROR",
				"message": err.Error(),
			},
		})
		return
	}
	defer file.Close()

	// Determine MIME type
	mimeType := fileHeader.Header.Get("Content-Type")
	if mimeType == "" {
		// Try to infer from extension
		filename := fileHeader.Filename
		if strings.HasSuffix(strings.ToLower(filename), ".pdf") {
			mimeType = "application/pdf"
		} else if strings.HasSuffix(strings.ToLower(filename), ".txt") {
			mimeType = "text/plain"
		} else if strings.HasSuffix(strings.ToLower(filename), ".doc") {
			mimeType = "application/msword"
		} else if strings.HasSuffix(strings.ToLower(filename), ".docx") {
			mimeType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		} else {
			mimeType = "application/octet-stream"
		}
	}

	// Validate MIME type
	if !h.allowedMimeTypes[mimeType] && !strings.HasPrefix(mimeType, "text/") {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_FILE_TYPE",
				"message": "File type not allowed. Allowed types: PDF, TXT, DOC, DOCX",
			},
		})
		return
	}

	// Generate file ID
	fileID := uuid.New()

	// Upload to storage
	storagePath, err := h.storage.Upload(c.Request.Context(), fileID, fileHeader.Filename, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "UPLOAD_FAILED",
				"message": fmt.Sprintf("Failed to upload file: %v", err),
			},
		})
		return
	}

	// Create file record in database
	fileRecord := &models.File{
		ID:          fileID,
		UserID:      userID,
		PetitionID:  petitionID,
		Filename:    fileHeader.Filename,
		MimeType:    mimeType,
		Size:        fileHeader.Size,
		StoragePath: storagePath,
	}

	err = h.fileRepo.Create(c.Request.Context(), fileRecord)
	if err != nil {
		// Try to clean up uploaded file
		h.storage.Delete(c.Request.Context(), storagePath)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "DATABASE_ERROR",
				"message": fmt.Sprintf("Failed to save file record: %v", err),
			},
		})
		return
	}

	// If file is linked to a petition, update the petition's cv_file_id
	// (We'll set it as CV file if petition doesn't have one yet)
	if petitionID != nil {
		petition, err := h.petitionRepo.GetByID(c.Request.Context(), *petitionID)
		if err == nil && petition != nil {
			// If petition doesn't have a CV file yet, set it
			if petition.CVFileID == nil {
				petition.CVFileID = &fileRecord.ID
				// Use the Update method to save the cv_file_id
				err = h.petitionRepo.Update(c.Request.Context(), petition)
				if err != nil {
					// Log error but don't fail the upload
					fmt.Printf("Warning: Failed to update petition cv_file_id: %v\n", err)
				}
			}
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data": gin.H{
			"id":        fileRecord.ID,
			"filename":  fileRecord.Filename,
			"mime_type": fileRecord.MimeType,
			"size":      fileRecord.Size,
			"created_at": fileRecord.CreatedAt,
		},
	})
}

// GetFile handles GET /api/files/:id
func (h *FileHandler) GetFile(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_ID",
				"message": "Invalid file ID format",
			},
		})
		return
	}

	file, err := h.fileRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "File not found",
			},
		})
		return
	}

	// Download from storage
	reader, err := h.storage.Download(c.Request.Context(), file.StoragePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "DOWNLOAD_FAILED",
				"message": fmt.Sprintf("Failed to download file: %v", err),
			},
		})
		return
	}
	defer reader.Close()

	// Set headers
	c.Header("Content-Type", file.MimeType)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", file.Filename))
	c.DataFromReader(http.StatusOK, file.Size, file.MimeType, reader, nil)
}

