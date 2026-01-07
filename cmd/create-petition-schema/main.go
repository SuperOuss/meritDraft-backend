package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: No .env file found, using environment variables: %v", err)
	}

	connString := os.Getenv("DATABASE_URL")
	if connString == "" {
		connString = "postgres://user:password@localhost:5432/meritdraft?sslmode=disable"
		log.Println("Warning: DATABASE_URL not set, using default connection string")
	}

	pool, err := pgxpool.New(context.Background(), connString)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()

	// Enable pgvector extension (if not already enabled)
	_, err = pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector")
	if err != nil {
		log.Printf("Warning: Failed to create pgvector extension: %v", err)
	} else {
		log.Println("✓ pgvector extension enabled")
	}

	// Create users table
	usersSQL := `
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    firm_name VARCHAR(255),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);`

	_, err = pool.Exec(ctx, usersSQL)
	if err != nil {
		log.Fatalf("Failed to create users table: %v", err)
	}
	log.Println("✓ Created users table")

	// Create files table (needed before petitions due to FK)
	filesSQL := `
CREATE TABLE IF NOT EXISTS files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    petition_id UUID,
    filename VARCHAR(255) NOT NULL,
    mime_type VARCHAR(100) NOT NULL,
    size BIGINT NOT NULL,
    storage_path TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);`

	_, err = pool.Exec(ctx, filesSQL)
	if err != nil {
		log.Fatalf("Failed to create files table: %v", err)
	}
	log.Println("✓ Created files table")

	// Create petitions table
	petitionsSQL := `
CREATE TABLE IF NOT EXISTS petitions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status VARCHAR(50) NOT NULL DEFAULT 'draft',
    
    -- Step 1: Intake
    client_name VARCHAR(255),
    visa_type VARCHAR(50),
    petitioner_name VARCHAR(255),
    field_of_expertise VARCHAR(255),
    
    -- Step 2: Documents
    cv_file_id UUID REFERENCES files(id),
    job_offer_file_id UUID REFERENCES files(id),
    scholar_link TEXT,
    parsed_documents JSONB,
    
    -- Step 3: Strategy
    selected_criteria TEXT[],
    
    -- Step 4: Deep Dive
    criteria_details JSONB DEFAULT '{}'::jsonb,
    
    -- Step 5/6: Generation
    generated_content TEXT,
    refine_instructions TEXT,
    
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    completed_at TIMESTAMP
);`

	_, err = pool.Exec(ctx, petitionsSQL)
	if err != nil {
		log.Fatalf("Failed to create petitions table: %v", err)
	}
	log.Println("✓ Created petitions table")

	// Add FK constraint for files.petition_id after petitions table exists
	// Check if constraint already exists first
	var constraintExists bool
	err = pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_constraint 
			WHERE conname = 'fk_files_petition_id'
		)`).Scan(&constraintExists)
	
	if err == nil && !constraintExists {
		_, err = pool.Exec(ctx, `
			ALTER TABLE files 
			ADD CONSTRAINT fk_files_petition_id 
			FOREIGN KEY (petition_id) REFERENCES petitions(id) ON DELETE SET NULL`)
		if err != nil {
			log.Printf("Warning: Failed to add FK constraint for files.petition_id: %v", err)
		} else {
			log.Println("✓ Added FK constraint for files.petition_id")
		}
	} else if constraintExists {
		log.Println("✓ FK constraint for files.petition_id already exists")
	}

	// Create user_preferences table
	preferencesSQL := `
CREATE TABLE IF NOT EXISTS user_preferences (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    email_notifications BOOLEAN DEFAULT true,
    auto_save_drafts BOOLEAN DEFAULT true,
    updated_at TIMESTAMP DEFAULT NOW()
);`

	_, err = pool.Exec(ctx, preferencesSQL)
	if err != nil {
		log.Fatalf("Failed to create user_preferences table: %v", err)
	}
	log.Println("✓ Created user_preferences table")

	// Create generation_jobs table
	generationJobsSQL := `
CREATE TABLE IF NOT EXISTS generation_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    petition_id UUID NOT NULL REFERENCES petitions(id) ON DELETE CASCADE,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    current_step VARCHAR(255),
    steps JSONB,
    error_message TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    completed_at TIMESTAMP
);`

	_, err = pool.Exec(ctx, generationJobsSQL)
	if err != nil {
		log.Fatalf("Failed to create generation_jobs table: %v", err)
	}
	log.Println("✓ Created generation_jobs table")

	// Create indexes
	indexes := []struct {
		name string
		sql  string
	}{
		{
			name: "idx_petitions_user_id",
			sql:  "CREATE INDEX IF NOT EXISTS idx_petitions_user_id ON petitions(user_id);",
		},
		{
			name: "idx_petitions_status",
			sql:  "CREATE INDEX IF NOT EXISTS idx_petitions_status ON petitions(status);",
		},
		{
			name: "idx_petitions_created_at",
			sql:  "CREATE INDEX IF NOT EXISTS idx_petitions_created_at ON petitions(created_at DESC);",
		},
		{
			name: "idx_files_user_id",
			sql:  "CREATE INDEX IF NOT EXISTS idx_files_user_id ON files(user_id);",
		},
		{
			name: "idx_files_petition_id",
			sql:  "CREATE INDEX IF NOT EXISTS idx_files_petition_id ON files(petition_id);",
		},
		{
			name: "idx_generation_jobs_petition_id",
			sql:  "CREATE INDEX IF NOT EXISTS idx_generation_jobs_petition_id ON generation_jobs(petition_id);",
		},
		{
			name: "idx_generation_jobs_status",
			sql:  "CREATE INDEX IF NOT EXISTS idx_generation_jobs_status ON generation_jobs(status);",
		},
	}

	for _, idx := range indexes {
		_, err = pool.Exec(ctx, idx.sql)
		if err != nil {
			log.Printf("Warning: Failed to create index %s: %v", idx.name, err)
		} else {
			log.Printf("✓ Created index: %s", idx.name)
		}
	}

	fmt.Println("\n✅ Core entity schema created successfully!")
	fmt.Println("   Tables: users, files, petitions, user_preferences, generation_jobs")
	fmt.Println("   Indexes: 7 indexes created")
}

