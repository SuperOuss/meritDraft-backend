package main

import (
	"context"
	"log"
	"os"

	"meritdraft-backend/handlers"
	"meritdraft-backend/repository"
	"meritdraft-backend/service"
	"meritdraft-backend/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/generative-ai-go/genai"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"google.golang.org/api/option"
)

func main() {
	// Load .env file from project root (relative to cmd/server/)
	// Try current directory first, then project root
	if err := godotenv.Load(); err != nil {
		if err := godotenv.Load("../../.env"); err != nil {
			log.Printf("Warning: No .env file found, using environment variables")
		}
	}

	// Initialize database connections
	db, err := initPostgres()
	if err != nil {
		log.Fatal("Failed to initialize Postgres:", err)
	}
	defer db.Close()

	// Initialize storage
	fileStorage, err := storage.NewStorageFromEnv()
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	log.Println("Storage initialized")

	// Initialize repositories
	petitionRepo := repository.NewPetitionRepository(db)
	jobRepo := repository.NewGenerationJobRepository(db)
	fileRepo := repository.NewFileRepository(db)
	legalChunkRepo := repository.NewLegalChunkRepository(db)

	// Initialize Gemini client
	geminiClient, err := initGemini()
	if err != nil {
		log.Fatal("Failed to initialize Gemini:", err)
	}

	// Initialize services
	petitionService := service.NewPetitionService(
		service.WithPetitionRepository(petitionRepo),
		service.WithGenerationJobRepository(jobRepo),
	)

	draftService := service.NewDraftService(
		service.DraftWithPetitionRepository(petitionRepo),
		service.DraftWithGenerationJobRepository(jobRepo),
		service.DraftWithLegalChunkRepository(legalChunkRepo),
		service.DraftWithDatabase(db),
		service.DraftWithGeminiClient(geminiClient),
	)

	// Initialize handlers
	petitionHandler := handlers.NewPetitionHandler(petitionService, draftService)
	fileHandler := handlers.NewFileHandler(fileRepo, petitionRepo, fileStorage)

	// Setup Gin router
	r := gin.Default()

	// Health check endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
		})
	})

	// API routes
	api := r.Group("/api")
	{
		// Petition endpoints
		api.POST("/petitions", petitionHandler.CreatePetition)
		api.GET("/petitions/:id", petitionHandler.GetPetition)
		api.PUT("/petitions/:id", petitionHandler.UpdatePetition)
		api.POST("/petitions/:id/generate", petitionHandler.GenerateDraft)

		// Job endpoints
		api.GET("/jobs/:id", petitionHandler.GetJobStatus)

		// File endpoints
		api.POST("/files/upload", fileHandler.UploadFile)
		api.GET("/files/:id", fileHandler.GetFile)
	}

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}

func initPostgres() (*pgxpool.Pool, error) {
	connString := os.Getenv("DATABASE_URL")
	if connString == "" {
		connString = "postgres://user:password@localhost:5432/meritdraft?sslmode=disable"
	}

	pool, err := pgxpool.New(context.Background(), connString)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(context.Background()); err != nil {
		return nil, err
	}

	// Enable pgvector extension
	ctx := context.Background()
	_, err = pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector")
	if err != nil {
		log.Printf("Warning: Failed to create pgvector extension: %v", err)
		log.Println("This may be normal if extension is already installed or requires superuser privileges")
	} else {
		log.Println("pgvector extension enabled")
	}

	log.Println("Postgres connection established with pgvector support")
	return pool, nil
}

func initGemini() (*genai.Client, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Println("Warning: GEMINI_API_KEY not set")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, err
	}

	log.Println("Gemini client initialized")
	return client, nil
}
