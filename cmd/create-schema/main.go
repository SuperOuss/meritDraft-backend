package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	connString := os.Getenv("DATABASE_URL")
	if connString == "" {
		connString = "postgres://user:password@localhost:5432/meritdraft?sslmode=disable"
	}

	pool, err := pgxpool.New(context.Background(), connString)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()

	// Enable pgvector extension
	_, err = pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector")
	if err != nil {
		log.Printf("Warning: Failed to create pgvector extension: %v", err)
	} else {
		log.Println("✓ pgvector extension enabled")
	}

	// Drop table if exists (for development - remove in production)
	_, err = pool.Exec(ctx, "DROP TABLE IF EXISTS legal_chunks CASCADE")
	if err != nil {
		log.Fatalf("Failed to drop table: %v", err)
	}
	log.Println("✓ Dropped existing legal_chunks table (if any)")

	// Create the legal_chunks table
	schemaSQL := `
CREATE TABLE legal_chunks (
    -- Primary identification
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Document identification (unified across all types)
    source_type VARCHAR(50) NOT NULL CHECK (source_type IN ('regulation', 'precedent_case', 'appeal_decision')),
    source_document VARCHAR(255) NOT NULL,
    chunk_index INTEGER NOT NULL,
    
    -- Content
    chunk_text TEXT NOT NULL,
    
    -- === UNIFIED METADATA FIELDS (work across all types) ===
    
    -- Legal framework identification
    regulatory_citation TEXT[],
    case_citation TEXT,
    appeal_citation TEXT,
    
    -- Criterion/Issue identification (unified across types)
    criterion_tag VARCHAR(100),
    
    -- Legal standards and tests (unified across types)
    legal_standard VARCHAR(255),
    legal_test TEXT,
    
    -- Document-specific metadata (stored as JSONB for flexibility)
    metadata JSONB DEFAULT '{}'::jsonb,
    
    -- === TYPE-SPECIFIC FLAGS ===
    
    -- For appeal decisions: distinguish winning arguments from denials
    is_winning_argument BOOLEAN DEFAULT false,
    
    -- For regulations: hierarchical structure
    section_level INTEGER,
    parent_section_id UUID REFERENCES legal_chunks(id),
    
    -- For cases: identify holdings vs. dicta
    is_holding BOOLEAN DEFAULT false,
    
    -- === VECTOR EMBEDDING ===
    embedding vector(768),
    
    -- === TIMESTAMPS ===
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    -- === CONSTRAINTS ===
    CONSTRAINT chunk_order_unique UNIQUE (source_document, chunk_index)
);`

	_, err = pool.Exec(ctx, schemaSQL)
	if err != nil {
		log.Fatalf("Failed to create legal_chunks table: %v", err)
	}
	log.Println("✓ Created legal_chunks table")

	// Create indexes
	indexes := []struct {
		name string
		sql  string
	}{
		{
			name: "Vector similarity search (HNSW)",
			sql: `CREATE INDEX idx_embedding_hnsw ON legal_chunks 
USING hnsw (embedding vector_cosine_ops)
WITH (m = 16, ef_construction = 64);`,
		},
		{
			name: "Type-based filtering",
			sql:  "CREATE INDEX idx_source_type ON legal_chunks(source_type);",
		},
		{
			name: "Source document filtering",
			sql:  "CREATE INDEX idx_source_document ON legal_chunks(source_document);",
		},
		{
			name: "Criterion-based filtering",
			sql:  "CREATE INDEX idx_criterion_tag ON legal_chunks(criterion_tag) WHERE criterion_tag IS NOT NULL;",
		},
		{
			name: "Legal standard filtering",
			sql:  "CREATE INDEX idx_legal_standard ON legal_chunks(legal_standard) WHERE legal_standard IS NOT NULL;",
		},
		{
			name: "Winning argument filtering",
			sql:  "CREATE INDEX idx_is_winning_argument ON legal_chunks(is_winning_argument) WHERE is_winning_argument = true;",
		},
		{
			name: "Regulatory citation filtering",
			sql:  "CREATE INDEX idx_regulatory_citation ON legal_chunks USING gin (regulatory_citation);",
		},
		{
			name: "Case citation filtering",
			sql:  "CREATE INDEX idx_case_citation ON legal_chunks(case_citation) WHERE case_citation IS NOT NULL;",
		},
		{
			name: "Appeal citation filtering",
			sql:  "CREATE INDEX idx_appeal_citation ON legal_chunks(appeal_citation) WHERE appeal_citation IS NOT NULL;",
		},
		{
			name: "Metadata JSONB filtering",
			sql:  "CREATE INDEX idx_metadata_gin ON legal_chunks USING gin (metadata);",
		},
		{
			name: "Composite: type and criterion",
			sql:  "CREATE INDEX idx_type_criterion ON legal_chunks(source_type, criterion_tag) WHERE criterion_tag IS NOT NULL;",
		},
		{
			name: "Composite: type and legal standard",
			sql:  "CREATE INDEX idx_type_standard ON legal_chunks(source_type, legal_standard) WHERE legal_standard IS NOT NULL;",
		},
		{
			name: "Composite: appeal winning arguments by criterion",
			sql: `CREATE INDEX idx_appeal_winning_criterion ON legal_chunks(source_type, criterion_tag, is_winning_argument) 
    WHERE source_type = 'appeal_decision' AND is_winning_argument = true;`,
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

	fmt.Println("\n✅ Database schema created successfully!")
	fmt.Println("   Table: legal_chunks")
	fmt.Println("   Indexes: 13 indexes created")
}

