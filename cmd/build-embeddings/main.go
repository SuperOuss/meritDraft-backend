package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	caseLawRefDir = "./case_law_ref"
	embeddingAPI  = "https://generativelanguage.googleapis.com/v1beta/models/gemini-embedding-001:embedContent"
	batchAPI      = "https://generativelanguage.googleapis.com/v1beta/models/gemini-embedding-001:batchEmbedContents"
)

type EmbeddingRequest struct {
	Model                string       `json:"model"`
	Content              ContentInput `json:"content"`
	TaskType             string       `json:"task_type,omitempty"`
	OutputDimensionality int          `json:"output_dimensionality,omitempty"`
}

type ContentInput struct {
	Parts []PartInput `json:"parts"`
}

type PartInput struct {
	Text string `json:"text"`
}

type EmbeddingResponse struct {
	Embedding EmbeddingData `json:"embedding"`
}

type EmbeddingData struct {
	Values []float64 `json:"values"`
}

// BatchEmbeddingItem is the structure returned by batch API (no nested "embedding" key)
type BatchEmbeddingItem struct {
	Values []float64 `json:"values"`
}

type BatchEmbeddingRequest struct {
	Requests []EmbeddingRequest `json:"requests"`
}

type BatchEmbeddingResponse struct {
	Embeddings []BatchEmbeddingItem `json:"embeddings"`
}

type Chunk struct {
	ID                 uuid.UUID
	SourceType         string
	SourceDocument     string
	ChunkIndex         int
	ChunkText          string
	RegulatoryCitation []string
	CaseCitation       string
	AppealCitation     string
	CriterionTag       string
	LegalStandard      string
	LegalTest          string
	Metadata           map[string]interface{}
	IsWinningArgument  bool
	SectionLevel       *int
	ParentSectionID    *uuid.UUID
	IsHolding          bool
	Embedding          []float64
}

func main() {
	// Load environment variables
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("GEMINI_API_KEY environment variable is required")
	}

	connString := os.Getenv("DATABASE_URL")
	if connString == "" {
		connString = "postgres://user:password@localhost:5432/meritdraft?sslmode=disable"
	}

	// Connect to database
	pool, err := pgxpool.New(context.Background(), connString)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()

	// Verify table exists
	var tableExists bool
	err = pool.QueryRow(ctx, "SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'legal_chunks')").Scan(&tableExists)
	if err != nil {
		log.Fatalf("Failed to check table existence: %v", err)
	}
	if !tableExists {
		log.Fatal("legal_chunks table does not exist. Please run: go run cmd/create-schema/main.go")
	}

	// Read all files in case_law_ref directory
	files, err := os.ReadDir(caseLawRefDir)
	if err != nil {
		log.Fatalf("Failed to read directory: %v", err)
	}

	// Process each document
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filename := file.Name()

		// Skip chunking strategy files
		if strings.HasSuffix(filename, ".chunking_strategy.txt") {
			continue
		}

		filePath := filepath.Join(caseLawRefDir, filename)
		log.Printf("\n📄 Processing: %s", filename)

		// Read file content
		content, err := os.ReadFile(filePath)
		if err != nil {
			log.Printf("❌ Error reading %s: %v", filename, err)
			continue
		}

		// Determine document type
		docType := determineDocumentType(filename, string(content))
		log.Printf("   Type: %s", docType)

		// Check if already processed
		var count int
		err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM legal_chunks WHERE source_document = $1", filename).Scan(&count)
		if err != nil {
			log.Printf("   ⚠️  Error checking existing chunks: %v", err)
		} else if count > 0 {
			log.Printf("   ⏭️  Skipping (already processed: %d chunks)", count)
			continue
		}

		// Chunk and extract metadata using Gemini
		chunks, err := chunkAndExtractMetadata(apiKey, filename, docType, string(content))
		if err != nil {
			log.Printf("   ❌ Error chunking document: %v", err)
			continue
		}

		log.Printf("   ✓ Generated %d chunks", len(chunks))

		// Generate embeddings for all chunks
		log.Printf("   🔄 Generating embeddings...")
		err = generateEmbeddings(apiKey, chunks)
		if err != nil {
			log.Printf("   ❌ Error generating embeddings: %v", err)
			continue
		}

		// Store chunks in database
		log.Printf("   💾 Storing chunks in database...")
		err = storeChunks(ctx, pool, chunks)
		if err != nil {
			log.Printf("   ❌ Error storing chunks: %v", err)
			continue
		}

		log.Printf("   ✅ Successfully processed %s (%d chunks)", filename, len(chunks))

		// Rate limiting
		time.Sleep(2 * time.Second)
	}

	log.Println("\n✅ Embedding build complete!")
}

func determineDocumentType(filename, content string) string {
	filenameLower := strings.ToLower(filename)
	contentLower := strings.ToLower(content)

	if strings.Contains(filenameLower, "regulation") || strings.Contains(filenameLower, "federal") {
		return "regulation"
	}
	if strings.Contains(filenameLower, "appeal") {
		return "appeal_decision"
	}
	if strings.Contains(filenameLower, "case") || strings.Contains(filenameLower, "kazarian") ||
		strings.Contains(filenameLower, "dhanasar") || strings.Contains(filenameLower, "chawath") {
		return "precedent_case"
	}

	// Fallback: analyze content
	if strings.Contains(contentLower, "administrative appeals office") ||
		strings.Contains(contentLower, "aao") ||
		strings.Contains(contentLower, "director's denial") {
		return "appeal_decision"
	}
	if strings.Contains(contentLower, "cfr") || strings.Contains(contentLower, "regulation") {
		return "regulation"
	}
	if strings.Contains(contentLower, "matter of") || strings.Contains(contentLower, "court of appeals") {
		return "precedent_case"
	}

	return "unknown"
}

func chunkAndExtractMetadata(apiKey, filename, docType, content string) ([]Chunk, error) {
	prompt := createChunkingPrompt(filename, docType, content)

	// Call Gemini API for chunking and metadata extraction
	chunkingResponse, err := callGeminiAPI(apiKey, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to call Gemini API: %w", err)
	}

	// Parse the response to extract chunks
	chunks, err := parseChunkingResponse(chunkingResponse, filename, docType)
	if err != nil {
		return nil, fmt.Errorf("failed to parse chunking response: %w", err)
	}

	return chunks, nil
}

func createChunkingPrompt(filename, docType, content string) string {
	return fmt.Sprintf(`You are a legal document processor. Your task is to chunk this document and extract metadata according to the unified schema.

Document Information:
- Filename: %s
- Document Type: %s
- Content Length: %d characters

Document Content:
%s

Task: Chunk this document and extract metadata for each chunk according to the unified metadata schema.

For each chunk, extract:
1. chunk_text: The actual text content (200-1000 words depending on document type)
2. regulatory_citation: Array of CFR citations (e.g., ["8 CFR § 204.5(h)(3)(vi)"])
3. case_citation: Full case citation if applicable
4. appeal_citation: Full appeal citation if applicable
5. criterion_tag: One of: awards, membership, media_coverage, judging, original_contributions, authorship, exhibitions, critical_role, high_salary, commercial_success (or null)
6. legal_standard: Name of legal test (e.g., "Kazarian Two-Step", "Final Merits Determination")
7. legal_test: Full name of legal test
8. metadata: JSON object with type-specific fields (see schema)
9. is_winning_argument: true for appeal decisions (only winning arguments), false otherwise
10. section_level: 1-3 for regulations, null otherwise
11. is_holding: true if chunk contains binding legal rule (for cases)

Chunking Rules:
- Regulations: Atomic legal rules (200-800 words), no overlap
- Precedent Cases: Complete legal test definitions (300-1000 words), 10-15%% overlap
- Appeal Decisions: Complete winning arguments (500-1000 words), 10-15%% overlap, exclude Director denials

Return your response as a JSON array of chunk objects. Each chunk object should have:
{
  "chunk_index": 0,
  "chunk_text": "...",
  "regulatory_citation": ["8 CFR § 204.5(h)(3)(vi)"],
  "case_citation": null,
  "appeal_citation": null,
  "criterion_tag": "authorship",
  "legal_standard": "Kazarian Two-Step",
  "legal_test": null,
  "metadata": {...},
  "is_winning_argument": false,
  "section_level": 3,
  "is_holding": false
}

Return ONLY valid JSON, no markdown, no explanations.`, filename, docType, len(content), content)
}

func callGeminiAPI(apiKey, prompt string) (string, error) {
	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature": 0.1, // Lower temperature for more consistent extraction
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := "https://generativelanguage.googleapis.com/v1beta/models/gemini-3-pro-preview:generateContent"
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error: %d - %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	var responseText strings.Builder
	for _, candidate := range apiResp.Candidates {
		for _, part := range candidate.Content.Parts {
			responseText.WriteString(part.Text)
		}
	}

	return responseText.String(), nil
}

func parseChunkingResponse(response, filename, docType string) ([]Chunk, error) {
	// Extract JSON from response (may be wrapped in markdown code blocks)
	response = strings.TrimSpace(response)
	if strings.HasPrefix(response, "```") {
		lines := strings.Split(response, "\n")
		var jsonLines []string
		inCodeBlock := false
		for _, line := range lines {
			if strings.HasPrefix(line, "```") {
				inCodeBlock = !inCodeBlock
				continue
			}
			if inCodeBlock {
				jsonLines = append(jsonLines, line)
			}
		}
		response = strings.Join(jsonLines, "\n")
	}

	// Try to find JSON array in response
	startIdx := strings.Index(response, "[")
	endIdx := strings.LastIndex(response, "]")
	if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
		return nil, fmt.Errorf("could not find JSON array in response")
	}

	jsonStr := response[startIdx : endIdx+1]

	var chunkData []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &chunkData); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	chunks := make([]Chunk, 0, len(chunkData))
	for i, data := range chunkData {
		chunk := Chunk{
			ID:             uuid.New(),
			SourceType:     docType,
			SourceDocument: filename,
		}

		if idx, ok := data["chunk_index"].(float64); ok {
			chunk.ChunkIndex = int(idx)
		} else {
			chunk.ChunkIndex = i
		}

		if text, ok := data["chunk_text"].(string); ok {
			chunk.ChunkText = text
		}

		if citations, ok := data["regulatory_citation"].([]interface{}); ok {
			chunk.RegulatoryCitation = make([]string, 0, len(citations))
			for _, cit := range citations {
				if str, ok := cit.(string); ok {
					chunk.RegulatoryCitation = append(chunk.RegulatoryCitation, str)
				}
			}
		}

		if cit, ok := data["case_citation"].(string); ok && cit != "" {
			chunk.CaseCitation = cit
		}

		if cit, ok := data["appeal_citation"].(string); ok && cit != "" {
			chunk.AppealCitation = cit
		}

		if tag, ok := data["criterion_tag"].(string); ok && tag != "" {
			chunk.CriterionTag = tag
		}

		if std, ok := data["legal_standard"].(string); ok && std != "" {
			chunk.LegalStandard = std
		}

		if test, ok := data["legal_test"].(string); ok && test != "" {
			chunk.LegalTest = test
		}

		if meta, ok := data["metadata"].(map[string]interface{}); ok {
			chunk.Metadata = meta
		} else {
			chunk.Metadata = make(map[string]interface{})
		}

		if winning, ok := data["is_winning_argument"].(bool); ok {
			chunk.IsWinningArgument = winning
		}

		if level, ok := data["section_level"].(float64); ok {
			levelInt := int(level)
			chunk.SectionLevel = &levelInt
		}

		if holding, ok := data["is_holding"].(bool); ok {
			chunk.IsHolding = holding
		}

		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

func generateEmbeddings(apiKey string, chunks []Chunk) error {
	// Prepare embedding inputs with context (as per schema)
	embeddingInputs := make([]string, len(chunks))
	for i, chunk := range chunks {
		embeddingInputs[i] = buildEmbeddingInput(chunk)
	}

	// Use batch API for efficiency
	if len(chunks) > 1 {
		return generateBatchEmbeddings(apiKey, embeddingInputs, chunks)
	}

	// Single embedding for small batches
	return generateSingleEmbeddings(apiKey, embeddingInputs, chunks)
}

func buildEmbeddingInput(chunk Chunk) string {
	var builder strings.Builder

	switch chunk.SourceType {
	case "regulation":
		builder.WriteString(fmt.Sprintf("[REGULATION: %s]\n", strings.Join(chunk.RegulatoryCitation, ", ")))
		if chunk.CriterionTag != "" {
			builder.WriteString(fmt.Sprintf("[CRITERION: %s]\n", chunk.CriterionTag))
		}
		if chunk.LegalStandard != "" {
			builder.WriteString(fmt.Sprintf("[LEGAL_STANDARD: %s]\n", chunk.LegalStandard))
		}
		builder.WriteString("\n")
		builder.WriteString(chunk.ChunkText)

	case "precedent_case":
		if chunk.CaseCitation != "" {
			builder.WriteString(fmt.Sprintf("[PRECEDENT_CASE: %s]\n", chunk.CaseCitation))
		}
		if chunk.LegalStandard != "" {
			builder.WriteString(fmt.Sprintf("[LEGAL_STANDARD: %s]\n", chunk.LegalStandard))
		}
		builder.WriteString(fmt.Sprintf("[HOLDING: %v]\n", chunk.IsHolding))
		builder.WriteString("\n")
		builder.WriteString(chunk.ChunkText)

	case "appeal_decision":
		if chunk.AppealCitation != "" {
			builder.WriteString(fmt.Sprintf("[APPEAL_DECISION: %s]\n", chunk.AppealCitation))
		}
		if chunk.CriterionTag != "" {
			builder.WriteString(fmt.Sprintf("[CRITERION: %s]\n", chunk.CriterionTag))
		}
		builder.WriteString(fmt.Sprintf("[WINNING_ARGUMENT: %v]\n", chunk.IsWinningArgument))
		if result, ok := chunk.Metadata["decision_result"].(string); ok {
			builder.WriteString(fmt.Sprintf("[OUTCOME: %s]\n", result))
		}
		builder.WriteString("\n")
		builder.WriteString(chunk.ChunkText)

	default:
		builder.WriteString(chunk.ChunkText)
	}

	return builder.String()
}

func generateBatchEmbeddings(apiKey string, inputs []string, chunks []Chunk) error {
	requests := make([]EmbeddingRequest, len(inputs))
	for i, input := range inputs {
		requests[i] = EmbeddingRequest{
			Model: "models/gemini-embedding-001",
			Content: ContentInput{
				Parts: []PartInput{{Text: input}},
			},
			TaskType:             "FACT_VERIFICATION",
			OutputDimensionality: 768,
		}
	}

	reqBody := BatchEmbeddingRequest{Requests: requests}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal batch request: %w", err)
	}

	req, err := http.NewRequest("POST", batchAPI, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %d - %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	var apiResp BatchEmbeddingResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if len(apiResp.Embeddings) != len(chunks) {
		return fmt.Errorf("mismatch: got %d embeddings for %d chunks", len(apiResp.Embeddings), len(chunks))
	}

	for i := range chunks {
		if len(apiResp.Embeddings[i].Values) == 0 {
			return fmt.Errorf("chunk %d has empty embedding", i)
		}
		chunks[i].Embedding = apiResp.Embeddings[i].Values
	}

	return nil
}

func generateSingleEmbeddings(apiKey string, inputs []string, chunks []Chunk) error {
	for i, input := range inputs {
		reqBody := EmbeddingRequest{
			Model: "models/gemini-embedding-001",
			Content: ContentInput{
				Parts: []PartInput{{Text: input}},
			},
			TaskType:             "FACT_VERIFICATION",
			OutputDimensionality: 768,
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}

		req, err := http.NewRequest("POST", embeddingAPI, bytes.NewBuffer(jsonData))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-goog-api-key", apiKey)

		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to send request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("API error: %d - %s", resp.StatusCode, string(body))
		}

		var apiResp EmbeddingResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}

		chunks[i].Embedding = apiResp.Embedding.Values

		// Rate limiting
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

func storeChunks(ctx context.Context, pool *pgxpool.Pool, chunks []Chunk) error {
	// Normalize embeddings (required for dimensions < 3072)
	for i := range chunks {
		if len(chunks[i].Embedding) > 0 {
			normalizeEmbedding(chunks[i].Embedding)
		}
	}

	// Format vector as string for pgx
	formatVector := func(embedding []float64) interface{} {
		if len(embedding) == 0 {
			return nil
		}
		var parts []string
		for _, v := range embedding {
			parts = append(parts, fmt.Sprintf("%.6f", v))
		}
		return "[" + strings.Join(parts, ",") + "]"
	}

	// Insert chunks in a transaction
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, chunk := range chunks {
		metadataJSON, err := json.Marshal(chunk.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}

		vectorValue := formatVector(chunk.Embedding)

		query := `
		INSERT INTO legal_chunks (
			id, source_type, source_document, chunk_index, chunk_text,
			regulatory_citation, case_citation, appeal_citation,
			criterion_tag, legal_standard, legal_test, metadata,
			is_winning_argument, section_level, parent_section_id, is_holding, embedding
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17::vector
		)`

		_, err = tx.Exec(ctx, query,
			chunk.ID, chunk.SourceType, chunk.SourceDocument, chunk.ChunkIndex, chunk.ChunkText,
			chunk.RegulatoryCitation, chunk.CaseCitation, chunk.AppealCitation,
			chunk.CriterionTag, chunk.LegalStandard, chunk.LegalTest, string(metadataJSON),
			chunk.IsWinningArgument, chunk.SectionLevel, chunk.ParentSectionID, chunk.IsHolding, vectorValue,
		)

		if err != nil {
			return fmt.Errorf("failed to insert chunk %d: %w", chunk.ChunkIndex, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func normalizeEmbedding(embedding []float64) {
	if len(embedding) == 0 {
		return
	}

	// Calculate L2 norm
	var sumSq float64
	for _, v := range embedding {
		sumSq += v * v
	}

	if sumSq == 0 {
		return
	}

	// Calculate L2 norm (sqrt of sum of squares)
	norm := math.Sqrt(sumSq)
	if norm == 0 {
		return
	}

	// Normalize by dividing by norm
	for i := range embedding {
		embedding[i] /= norm
	}
}
