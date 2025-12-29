# Technical Specification: Legal Document Chunking and pgvector Storage

## Overview

This specification defines the architecture for chunking legal documents (regulations, precedent cases, and appeal decisions) and storing them in PostgreSQL with pgvector for semantic search. The system uses Gemini embeddings model for vector generation.

## 1. Database Schema Design

### 1.1 Core Table: `legal_chunks`

```sql
CREATE TABLE legal_chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Document identification
    source_type VARCHAR(50) NOT NULL,  -- 'regulation', 'precedent_case', 'appeal_decision'
    source_document VARCHAR(255) NOT NULL,  -- e.g., 'uscis_regulations.txt', 'kazarian_case.txt', 'appeal_quintanilla'
    source_section VARCHAR(255),  -- e.g., '8 CFR § 204.5(h)(3)(v)', 'Kazarian Final Merits', 'Section A: Judging'
    
    -- Chunk content
    chunk_text TEXT NOT NULL,
    chunk_index INTEGER NOT NULL,  -- Order within source document
    
    -- Metadata for filtering and context
    criterion_tag VARCHAR(100),  -- e.g., 'authorship', 'judging', 'awards' (for appeal chunks)
    legal_standard VARCHAR(255),  -- e.g., 'Preponderance of Evidence', 'Final Merits Determination'
    is_winning_argument BOOLEAN DEFAULT false,  -- true for appeal decision chunks (AAO reasoning)
    
    -- Hierarchical context (for regulations)
    section_level INTEGER,  -- 1 = Section, 2 = Subsection, 3 = Paragraph
    parent_section_id UUID REFERENCES legal_chunks(id),
    
    -- Vector embedding
    embedding vector(768),  -- Gemini embedding dimension (verify actual dimension)
    
    -- Timestamps
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Indexes for efficient querying
CREATE INDEX idx_source_type ON legal_chunks(source_type);
CREATE INDEX idx_source_document ON legal_chunks(source_document);
CREATE INDEX idx_criterion_tag ON legal_chunks(criterion_tag);
CREATE INDEX idx_legal_standard ON legal_chunks(legal_standard);
CREATE INDEX idx_is_winning_argument ON legal_chunks(is_winning_argument);

-- Vector similarity search index (HNSW for fast approximate nearest neighbor)
CREATE INDEX idx_embedding_hnsw ON legal_chunks 
USING hnsw (embedding vector_cosine_ops)
WITH (m = 16, ef_construction = 64);

-- Composite index for common query patterns
CREATE INDEX idx_source_type_criterion ON legal_chunks(source_type, criterion_tag);
```

### 1.2 Metadata Table: `source_documents`

```sql
CREATE TABLE source_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    filename VARCHAR(255) UNIQUE NOT NULL,
    source_type VARCHAR(50) NOT NULL,
    file_path TEXT,
    total_chunks INTEGER DEFAULT 0,
    processed_at TIMESTAMP,
    processing_status VARCHAR(50) DEFAULT 'pending',  -- 'pending', 'processing', 'completed', 'failed'
    metadata JSONB,  -- Store document-specific metadata
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_source_documents_type ON source_documents(source_type);
CREATE INDEX idx_source_documents_status ON source_documents(processing_status);
```

## 2. LLM-First Chunking Strategy

**Principle**: Use Gemini 3 (with thinking capabilities) to analyze document structure, infer chunking rules, then apply them. This replaces brittle regex parsing with semantic understanding.

### 2.0 Why LLM-First Approach?

**Advantages**:
1. **Semantic Understanding**: LLM understands document structure and meaning, not just patterns
2. **Adaptability**: Handles variations in formatting, structure, and style automatically
3. **Context Awareness**: Recognizes legal concepts, citations, and relationships semantically
4. **Quality Assurance**: LLM can verify chunk completeness and semantic boundaries
5. **No Regex Maintenance**: Eliminates brittle pattern matching that breaks with format changes

**Process**:
1. **Analysis Phase**: Gemini 3 analyzes entire document, understands structure, infers chunking strategy
2. **Strategy Output**: LLM outputs structured chunk plan (JSON) with boundaries, metadata, and text
3. **Application Phase**: System applies the plan to extract chunks and metadata
4. **Verification Phase** (optional): Second LLM pass to validate chunk quality

### 2.1 Phase 1: Document Analysis & Strategy Inference

**For All Document Types**: First pass with Gemini 3 to understand structure and infer chunking strategy.

**Prompt Template**:
```
You are analyzing a legal document to determine the optimal chunking strategy for semantic search.

Document Type: [regulation|precedent_case|appeal_decision]
Document Content:
{document_content}

Task: Analyze this document and infer the chunking strategy. Think through:

1. Document Structure:
   - What is the hierarchical organization? (sections, subsections, paragraphs)
   - How are legal concepts organized?
   - What are the natural semantic boundaries?

2. Chunking Rules:
   - Where should chunk boundaries be placed?
   - What constitutes an "atomic legal rule" or "complete legal concept"?
   - What metadata should be extracted for each chunk?
   - What are the optimal chunk sizes for this document type?

3. Metadata Extraction:
   - What citations, section numbers, or identifiers exist?
   - What legal standards or tests are mentioned?
   - What criteria or topics are discussed?

Output a JSON strategy object:
{
  "chunking_strategy": {
    "boundary_rules": ["rule1", "rule2", ...],
    "target_size_range": {"min": 200, "max": 800},
    "semantic_units": ["atomic legal rule", "complete definition", ...]
  },
  "structure_analysis": {
    "hierarchy_levels": [...],
    "section_markers": [...],
    "citation_patterns": [...]
  },
  "metadata_extraction": {
    "citations": [...],
    "legal_standards": [...],
    "criteria": [...]
  },
  "chunk_plan": [
    {
      "chunk_id": 1,
      "start_line": 10,
      "end_line": 25,
      "source_section": "8 CFR § 204.5(h)(3)(v)",
      "metadata": {...}
    },
    ...
  ]
}
```

### 2.2 Phase 2: Strategy Application

Once the LLM has inferred the strategy, apply it to create chunks. The LLM's `chunk_plan` provides the blueprint.

### 2.3 Regulations & Policy Manual (`source_type = 'regulation'`)

**Principle**: Chunk by Atomic Legal Rule - one chunk = one specific criterion definition.

**LLM Analysis Phase**:
- Gemini 3 analyzes the regulation document
- Identifies hierarchical structure (sections, subsections, paragraphs)
- Recognizes CFR citations and section markers semantically
- Determines natural boundaries for atomic legal rules
- Outputs chunk plan with metadata

**Chunking Rules (Inferred by LLM)**:
1. **Semantic boundaries**: Chunk at complete legal definitions, not arbitrary line breaks
2. **Hierarchical awareness**: Respect section → subsection → paragraph hierarchy
3. **Citation preservation**: Include CFR citations in chunk metadata
4. **Size guidance**: 200-800 words, prioritizing semantic completeness

**Metadata Extraction (LLM-Derived)**:
- `source_section`: CFR citation or section heading (extracted semantically)
- `section_level`: Inferred from document structure
- `legal_standard`: Extracted if mentions specific legal tests
- `criterion_tag`: Extracted if discusses specific EB-1A criteria

**Example LLM Output**:
```json
{
  "chunk_plan": [
    {
      "chunk_id": 1,
      "start_line": 50,
      "end_line": 65,
      "source_section": "8 CFR § 204.5(h)(3)(v) - Original Contributions",
      "section_level": 3,
      "legal_standard": null,
      "criterion_tag": "original_contributions",
      "chunk_text": "Original Contributions. Evidence of the alien's original scientific, scholarly, artistic, athletic, or business-related contributions of major significance in the field..."
    }
  ]
}
```

### 2.4 Precedent Cases (`source_type = 'precedent_case'`)

**Principle**: Chunk by Legal Standard / Test - isolate paragraphs that define specific legal tests.

**LLM Analysis Phase**:
- Gemini 3 analyzes the precedent case document
- Identifies case structure: Background, Legal Framework, Analysis, Conclusion
- Recognizes legal standard definitions semantically (not via keyword matching)
- Identifies where legal tests are defined (e.g., "Two-Step Analysis", "Final Merits Determination")
- Determines which paragraphs contain complete legal test definitions

**Chunking Rules (Inferred by LLM)**:
1. **Legal standard identification**: LLM identifies paragraphs that define legal tests, not just keyword matching
2. **Completeness**: One chunk = one complete legal test definition
3. **Global scope**: These are precedent-setting, apply to all petitions
4. **Size guidance**: 300-1000 words for complete legal test definitions

**Metadata Extraction (LLM-Derived)**:
- `source_section`: Case name + test name (e.g., "Kazarian - Final Merits Determination")
- `legal_standard`: Name of the test extracted semantically
- `criterion_tag`: NULL (global standards, not criterion-specific)
- `is_winning_argument`: N/A for precedent cases

**Example LLM Output**:
```json
{
  "chunk_plan": [
    {
      "chunk_id": 1,
      "start_line": 120,
      "end_line": 145,
      "source_section": "Kazarian - Final Merits Determination",
      "legal_standard": "Final Merits Determination",
      "criterion_tag": null,
      "chunk_text": "In the final merits determination, the officer must evaluate all the evidence together when considering the petition in its entirety, in the context of the high level of expertise required for this immigrant classification..."
    }
  ]
}
```

### 2.5 Appeal Decisions (`source_type = 'appeal_decision'`)

**Principle**: Chunk by Winning Argument per Criterion - extract only AAO's successful reasoning, exclude Director's denial arguments.

**LLM Analysis Phase (Gemini 3 with Thinking)**:

**Step 1: Document Structure Analysis & Strategy Inference**
```
You are analyzing an AAO appeal decision to determine chunking strategy.

Document contains:
1. Background facts
2. Director's denial arguments (REJECT - these are wrong)
3. AAO's analysis and reasoning (KEEP - these are correct)
4. Conclusion

Task: Think through the document structure and infer the chunking strategy:

1. Identify document sections semantically (not by line numbers alone)
2. Distinguish between Director's denial arguments and AAO's reasoning
3. Identify which EB-1A criteria are discussed:
   - Authorship, Judging, Awards, Original Contributions, Critical Role
   - High Salary, Commercial Success, Media Coverage, Membership, Exhibitions
4. Determine optimal chunk boundaries for winning arguments
5. Plan how to extract only AAO's successful reasoning

Output a JSON strategy:
{
  "document_structure": {
    "sections": [
      {"type": "background", "start": ..., "end": ...},
      {"type": "director_denial", "start": ..., "end": ..., "exclude": true},
      {"type": "aao_analysis", "start": ..., "end": ..., "keep": true},
      ...
    ]
  },
  "criteria_discussed": ["judging", "awards", ...],
  "chunking_strategy": {
    "boundary_rules": ["complete winning argument per criterion", ...],
    "purification_rules": ["exclude director denial", "keep only AAO reasoning", ...]
  },
  "chunk_plan": [
    {
      "chunk_id": 1,
      "criterion": "judging",
      "source_section": "Quintanilla - Section A: Judging",
      "start_line": 150,
      "end_line": 200,
      "is_winning_argument": true,
      "contains_director_denial": false,
      "chunk_text": "[purified AAO reasoning only]"
    },
    ...
  ]
}
```

**Step 2: Purification & Chunk Creation**

The LLM's chunk plan includes purified text (Director's denial removed). For each planned chunk:

1. **Verify purification**: LLM confirms no Director denial content
2. **Complete reasoning chain**: Ensure chunk contains full argument, not fragments
3. **Metadata assignment**: Use LLM-extracted metadata

**Chunking Rules (Inferred by LLM)**:
1. **Semantic separation**: LLM distinguishes denial vs. winning arguments semantically
2. **Criterion-based chunking**: One chunk = one complete winning argument per criterion
3. **Size guidance**: 500-1000 words for complete reasoning chains
4. **Quality assurance**: LLM verifies chunks don't contain denial arguments

**Metadata Extraction (LLM-Derived)**:
- `source_section`: Appeal name + criterion (e.g., "Quintanilla - Section A: Judging")
- `criterion_tag`: Specific criterion extracted semantically
- `is_winning_argument`: Always `true` (LLM ensures only winning arguments are chunked)
- `legal_standard`: Extracted if mentions specific tests (e.g., "Kazarian two-step")

**Example LLM Output**:
```json
{
  "chunk_plan": [
    {
      "chunk_id": 1,
      "criterion": "judging",
      "source_section": "Quintanilla - Section A: Judging",
      "start_line": 150,
      "end_line": 200,
      "is_winning_argument": true,
      "legal_standard": null,
      "chunk_text": "The record contains evidence that the beneficiary served as a judge for the International Hackathon Competition in 2023. The competition attracted over 500 participants from 15 countries. The beneficiary evaluated technical submissions based on innovation, code quality, and problem-solving approach. This judging activity demonstrates the beneficiary's recognition as a person of distinguished judgment in the field of software engineering. The evidence is sufficient to meet the regulatory criteria for judging the work of others."
    }
  ]
}
```

## 3. Embedding Generation

### 3.1 Gemini Embedding Model Configuration

- **Model**: `text-embedding-004` (or latest Gemini embedding model)
- **Dimension**: Verify actual dimension (typically 768 for Gemini)
- **API Endpoint**: Google Generative AI Go SDK

### 3.2 Embedding Generation Workflow

1. **Input preparation**:
   - Use `chunk_text` as the input
   - Optionally prepend metadata for context:
     ```
     [REGULATION: 8 CFR § 204.5(h)(3)(v)] 
     [CRITERION: Original Contributions]
     
     {chunk_text}
     ```

2. **Batch processing**:
   - Process chunks in batches of 100 to optimize API calls
   - Handle rate limits and retries

3. **Storage**:
   - Store embedding vector in `legal_chunks.embedding` column
   - Verify dimension matches table definition

### 3.3 Embedding Quality Considerations

- **Context preservation**: Include enough context in chunk to make embedding meaningful
- **Consistency**: Use same model and parameters for all embeddings
- **Normalization**: pgvector cosine similarity handles normalization, but verify model output

## 4. Processing Pipeline

### 4.1 Overall Flow (LLM-First Approach)

```
1. Document Ingestion
   ↓
2. Document Type Detection (filename or LLM classification)
   ↓
3. LLM Analysis Phase (Gemini 3 with Thinking)
   ├─→ Analyze document structure semantically
   ├─→ Infer chunking strategy
   ├─→ Identify chunk boundaries
   ├─→ Extract metadata
   └─→ Generate chunk plan (JSON)
   ↓
4. Strategy Application
   ├─→ Apply LLM's chunk plan
   ├─→ Extract chunk text based on plan
   └─→ Assign metadata from plan
   ↓
5. Quality Verification (Optional LLM pass)
   ├─→ Verify chunk completeness
   ├─→ Check for denial arguments (appeals)
   └─→ Validate metadata accuracy
   ↓
6. Embedding Generation (Gemini Embedding Model)
   ↓
7. Database Storage (PostgreSQL + pgvector)
   ↓
8. Index Creation/Update
```

### 4.2 LLM Processing Details

**Models**:
- **Gemini 3** (with thinking capabilities): Document analysis, strategy inference, chunk planning
- **Gemini Embedding Model**: Vector generation for semantic search

**Processing Strategy**:
- **Single-pass analysis**: LLM analyzes entire document and outputs complete chunk plan
- **Structured output**: Use JSON mode or structured output schema for reliable parsing
- **Thinking mode**: Leverage Gemini 3's thinking capabilities for complex reasoning about document structure
- **Error handling**: 
  - If LLM output is malformed, retry with more explicit instructions
  - Validate JSON structure before applying chunk plan
  - Fallback: Request LLM to output in more structured format
- **Batch processing**: Process multiple documents, but analyze each individually for accuracy
- **Rate limiting**: Respect API rate limits, implement exponential backoff

**Structured Output Schema** (for reliable parsing):
```json
{
  "chunking_strategy": {
    "document_type": "regulation|precedent_case|appeal_decision",
    "boundary_rules": ["string"],
    "target_size_range": {"min": number, "max": number}
  },
  "chunk_plan": [
    {
      "chunk_id": number,
      "start_line": number,
      "end_line": number,
      "source_section": "string",
      "section_level": number,
      "criterion_tag": "string|null",
      "legal_standard": "string|null",
      "is_winning_argument": boolean,
      "chunk_text": "string"
    }
  ]
}
```

### 4.2 Error Handling

- **Chunking failures**: Log error, mark document as 'failed', continue with other documents
- **LLM API failures**: Retry with exponential backoff, max 3 retries
- **Embedding failures**: Retry individual chunks, don't fail entire document
- **Database failures**: Transaction rollback, preserve processing state

### 4.3 Idempotency

- Check if document already processed (query `source_documents` table)
- Skip if `processing_status = 'completed'`
- Allow re-processing with `force_reprocess` flag (deletes old chunks first)

## 5. Query Patterns

### 5.1 Semantic Search Query

```sql
-- Find chunks semantically similar to query
SELECT 
    id,
    source_type,
    source_document,
    source_section,
    chunk_text,
    criterion_tag,
    legal_standard,
    1 - (embedding <=> $1::vector) AS similarity
FROM legal_chunks
WHERE source_type = $2  -- Optional filter
ORDER BY embedding <=> $1::vector
LIMIT 10;
```

### 5.2 Criterion-Specific Search

```sql
-- Find winning arguments for a specific criterion
SELECT 
    id,
    source_section,
    chunk_text,
    1 - (embedding <=> $1::vector) AS similarity
FROM legal_chunks
WHERE criterion_tag = $2
  AND is_winning_argument = true
  AND source_type = 'appeal_decision'
ORDER BY embedding <=> $1::vector
LIMIT 5;
```

### 5.3 Legal Standard Search

```sql
-- Find chunks that define a specific legal standard
SELECT 
    id,
    source_section,
    chunk_text,
    legal_standard
FROM legal_chunks
WHERE legal_standard = $1
  AND source_type IN ('regulation', 'precedent_case')
ORDER BY source_type, chunk_index;
```

### 5.4 Hybrid Search (Semantic + Metadata)

```sql
-- Combine semantic similarity with metadata filters
SELECT 
    id,
    source_type,
    source_section,
    chunk_text,
    criterion_tag,
    (1 - (embedding <=> $1::vector)) * 0.7 + 
    CASE WHEN criterion_tag = $2 THEN 0.3 ELSE 0 END AS combined_score
FROM legal_chunks
WHERE source_type IN ('appeal_decision', 'regulation')
ORDER BY combined_score DESC
LIMIT 10;
```

## 6. Implementation Considerations

### 6.1 Chunk Size Guidelines

- **Regulations**: 200-800 words (prioritize semantic completeness)
- **Precedent Cases**: 300-1000 words (complete legal test definition)
- **Appeal Decisions**: 500-1000 words (complete winning argument)

### 6.2 Overlap Strategy

- **No overlap** for regulations and precedent cases (atomic rules)
- **Minimal overlap** (50-100 words) for appeal decisions if argument spans multiple paragraphs
- Track overlap in `chunk_index` to maintain order

### 6.3 Metadata Enrichment

- **LLM-based extraction**: Gemini 3 extracts citations, case names, and criteria semantically
- **No regex needed**: LLM understands context and extracts metadata accurately
- **Validation**: Optional second LLM pass to verify extracted metadata matches content

### 6.4 Performance Optimization

- **Batch embedding generation**: Process 100 chunks at a time
- **Parallel processing**: Process multiple documents concurrently (with rate limit awareness)
- **Incremental updates**: Only re-process changed documents
- **Index maintenance**: Update HNSW index after bulk inserts

## 7. Validation & Quality Assurance

### 7.1 Chunk Quality Checks

**LLM-Based Validation** (Optional second pass with Gemini 3):
1. **Completeness check**: LLM verifies chunk contains complete thought/rule/argument
2. **Boundary verification**: LLM confirms no mid-sentence or mid-paragraph breaks
3. **Denial argument detection**: For appeals, LLM verifies no Director denial content semantically
4. **Metadata accuracy**: LLM validates tags and sections match chunk content
5. **Semantic coherence**: LLM confirms chunk maintains semantic integrity

**Automated Validation**:
- Check chunk text length is within target range
- Verify required metadata fields are present
- Validate JSON structure of chunk plan

### 7.2 Sample Validation Queries

```sql
-- Check for chunks that might contain denial arguments
-- Note: This is a basic check; LLM validation is more reliable
SELECT id, source_section, chunk_text
FROM legal_chunks
WHERE source_type = 'appeal_decision'
  AND is_winning_argument = true
  AND (chunk_text ILIKE '%director denied%' 
       OR chunk_text ILIKE '%director found insufficient%');

-- Verify chunk sizes are reasonable
SELECT 
    source_type,
    AVG(LENGTH(chunk_text)) AS avg_length,
    MIN(LENGTH(chunk_text)) AS min_length,
    MAX(LENGTH(chunk_text)) AS max_length
FROM legal_chunks
GROUP BY source_type;

-- Check for missing embeddings
SELECT COUNT(*) 
FROM legal_chunks 
WHERE embedding IS NULL;
```

## 8. Future Enhancements

1. **Versioning**: Track document versions and chunk updates
2. **Confidence scores**: Add LLM-generated confidence scores for chunk quality
3. **Cross-references**: Link related chunks across documents
4. **Citation extraction**: Automatically extract and link legal citations
5. **Multi-language support**: Handle documents in other languages if needed

## 9. Dependencies

- **PostgreSQL 15+** with pgvector extension
- **Google Generative AI Go SDK** (`github.com/google/generative-ai-go/genai`)
- **pgx/v5** for PostgreSQL connectivity
- **Gemini API Key** for embeddings and LLM pre-processing

## 10. Configuration

### Environment Variables

```env
DATABASE_URL=postgres://user:password@host:5432/meritdraft
GEMINI_API_KEY=your_api_key
GEMINI_EMBEDDING_MODEL=text-embedding-004  # Verify actual model name
GEMINI_LLM_MODEL=gemini-3  # For document analysis and chunking strategy inference (with thinking)
CHUNK_BATCH_SIZE=100
MAX_RETRIES=3
```

### Processing Parameters

- **Regulation chunk size**: 200-800 words
- **Precedent chunk size**: 300-1000 words  
- **Appeal chunk size**: 500-1000 words
- **HNSW index parameters**: m=16, ef_construction=64 (tune based on data size)
- **Similarity threshold**: 0.7 (for filtering low-quality matches)

