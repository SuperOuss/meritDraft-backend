package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// Storage interface for file storage operations
type Storage interface {
	// Upload stores a file and returns the storage path
	Upload(ctx context.Context, fileID uuid.UUID, filename string, data io.Reader) (string, error)
	
	// Download retrieves a file by storage path
	Download(ctx context.Context, storagePath string) (io.ReadCloser, error)
	
	// Delete removes a file by storage path
	Delete(ctx context.Context, storagePath string) error
}

// StorageType represents the storage backend type
type StorageType string

const (
	StorageTypeLocal StorageType = "local"
	StorageTypeS3    StorageType = "s3"
)

// StorageConfig holds configuration for storage
type StorageConfig struct {
	Type         StorageType
	LocalPath    string // For local storage
	S3Bucket     string // For S3 storage
	S3Region     string // For S3 storage
	AWSAccessKey string
	AWSSecretKey string
}

// NewStorage creates a new storage instance based on configuration
func NewStorage(cfg StorageConfig) (Storage, error) {
	switch cfg.Type {
	case StorageTypeLocal:
		return NewLocalStorage(cfg.LocalPath)
	case StorageTypeS3:
		return NewS3Storage(cfg)
	default:
		return nil, fmt.Errorf("unknown storage type: %s", cfg.Type)
	}
}

// NewStorageFromEnv creates a storage instance from environment variables
func NewStorageFromEnv() (Storage, error) {
	storageType := os.Getenv("STORAGE_TYPE")
	if storageType == "" {
		storageType = "local" // Default to local for development
	}

	cfg := StorageConfig{
		Type: StorageType(storageType),
	}

	switch StorageType(storageType) {
	case StorageTypeLocal:
		localPath := os.Getenv("STORAGE_LOCAL_PATH")
		if localPath == "" {
			localPath = "./storage/files" // Default local storage path
		}
		cfg.LocalPath = localPath
		return NewLocalStorage(cfg.LocalPath)

	case StorageTypeS3:
		cfg.S3Bucket = os.Getenv("AWS_S3_BUCKET")
		cfg.S3Region = os.Getenv("AWS_REGION")
		if cfg.S3Region == "" {
			cfg.S3Region = "us-east-1" // Default region
		}
		cfg.AWSAccessKey = os.Getenv("AWS_ACCESS_KEY_ID")
		cfg.AWSSecretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")

		if cfg.S3Bucket == "" {
			return nil, errors.New("AWS_S3_BUCKET environment variable is required for S3 storage")
		}

		return NewS3Storage(cfg)

	default:
		return nil, fmt.Errorf("unknown storage type: %s", storageType)
	}
}

// generateStoragePath generates a unique storage path for a file
func generateStoragePath(fileID uuid.UUID, filename string) string {
	ext := filepath.Ext(filename)
	baseName := strings.TrimSuffix(filename, ext)
	// Sanitize filename
	baseName = strings.ReplaceAll(baseName, " ", "_")
	baseName = strings.ReplaceAll(baseName, "/", "_")
	baseName = strings.ReplaceAll(baseName, "\\", "_")
	
	// Use fileID to ensure uniqueness
	return fmt.Sprintf("%s/%s_%s%s", fileID.String()[:2], fileID.String(), baseName, ext)
}

