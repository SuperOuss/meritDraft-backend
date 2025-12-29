# Unified Metadata and Vector Schema for Legal Documents

## Overview

This document defines a unified metadata and vector storage schema that works across three document types:
1. **Regulations** - Getting the rules right and referencing them
2. **Precedent Cases** - Using precedent to argue the rules and test them
3. **Appeal Decisions** - Getting reasoning for why initial evidence was sufficient (real-world winning decisions)

The schema enables semantic search across all three types while supporting type-specific filtering and use-case-specific queries.

## Use Cases

### Use Case 1: Getting the Rules Right (Regulations)
**Goal**: Find the exact regulatory text, definitions, and requirements for a specific criterion or legal standard.

**Query Pattern**: 
- Semantic search for regulatory definitions
- Filter by `source_type = 'regulation'` and `criterion_tag`
- Retrieve chunks with `legal_standard` or `regulatory_citation`

### Use Case 2: Using Precedent Cases (Cases)
**Goal**: Find how courts have interpreted regulations, what legal tests they've established, and how to apply rules.

**Query Pattern**:
- Semantic search for legal test definitions
- Filter by `source_type = 'precedent_case'` and `legal_standard`
- Retrieve chunks that establish binding legal principles

### Use Case 3: Appeal Decisions (Appeals)
**Goal**: Find real-world examples of sufficient evidence, winning arguments, and AAO reasoning for specific criteria.

**Query Pattern**:
- Semantic search for winning arguments
- Filter by `source_type = 'appeal_decision'`, `criterion_tag`, and `is_winning_argument = true`
- Retrieve chunks showing why evidence was sufficient

## Unified Database Schema

### Core Table: `legal_chunks`

```sql
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE legal_chunks (
    -- Primary identification
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Document identification (unified across all types)
    source_type VARCHAR(50) NOT NULL CHECK (source_type IN ('regulation', 'precedent_case', 'appeal_decision')),
    source_document VARCHAR(255) NOT NULL,  -- e.g., 'uscis_regulations.txt', 'kazarian_case.txt', 'appeal_fm-co'
    chunk_index INTEGER NOT NULL,  -- Order within source document
    
    -- Content
    chunk_text TEXT NOT NULL,
    
    -- === UNIFIED METADATA FIELDS (work across all types) ===
    
    -- Legal framework identification
    regulatory_citation TEXT[],  -- e.g., ['8 CFR § 204.5(h)(3)(vi)', 'INA § 203(b)(1)(A)']
    case_citation TEXT,  -- e.g., 'Kazarian v. USCIS, 596 F.3d 1115 (9th Cir. 2010)'
    appeal_citation TEXT,  -- e.g., 'Matter of F-M- Co., Adopted Decision 2020-01'
    
    -- Criterion/Issue identification (unified across types)
    criterion_tag VARCHAR(100),  -- e.g., 'authorship', 'judging', 'awards', 'original_contributions'
    -- For regulations: maps to specific criterion (i-x)
    -- For cases: NULL (global standards) or specific criterion if case analyzes it
    -- For appeals: the specific criterion the appeal addresses
    
    -- Legal standards and tests (unified across types)
    legal_standard VARCHAR(255),  -- e.g., 'Kazarian Two-Step', 'Final Merits Determination', 'Preponderance of Evidence'
    legal_test TEXT,  -- Full name of legal test, e.g., 'Two-Step Evidentiary Review'
    
    -- Document-specific metadata (stored as JSONB for flexibility)
    metadata JSONB DEFAULT '{}'::jsonb,
    /*
    Structure varies by source_type:
    
    For REGULATIONS:
    {
      "section": "B. Evidence of Extraordinary Ability",
      "subsection": "Criterion 6: Authorship",
      "section_level": 3,
      "definition_type": "evidentiary_criterion",
      "examples": ["peer-reviewed journals", "conference proceedings"],
      "considerations": ["scholarly article definition", "publication qualification"]
    }
    
    For PRECEDENT CASES:
    {
      "case_name": "Kazarian v. USCIS",
      "court": "9th Circuit Court of Appeals",
      "date_filed": "2010-03-04",
      "judge": "D.W. Nelson",
      "opinion_type": "Majority",
      "section": "Discussion > Application to Kazarian > Authorship",
      "holding_type": "Rule Definition",
      "outcome": "Affirmed"
    }
    
    For APPEAL DECISIONS:
    {
      "appeal_name": "Matter of F-M- Co.",
      "decision_type": "Adopted Decision",
      "decision_date": "2020-05-05",
      "authority_level": "Binding",
      "section": "III. ANALYSIS - B. Successor-in-Interest",
      "is_holding": true,
      "decision_result": "Sustained",
      "parties": {
        "petitioner": "Automobile Manufacturer",
        "beneficiary": "Steering Component Expert"
      }
    }
    */
    
    -- === TYPE-SPECIFIC FLAGS ===
    
    -- For appeal decisions: distinguish winning arguments from denials
    is_winning_argument BOOLEAN DEFAULT false,  -- true only for appeal decision chunks with AAO reasoning
    
    -- For regulations: hierarchical structure
    section_level INTEGER,  -- 1 = Section, 2 = Subsection, 3 = Paragraph
    parent_section_id UUID REFERENCES legal_chunks(id),
    
    -- For cases: identify holdings vs. dicta
    is_holding BOOLEAN DEFAULT false,  -- true if chunk contains binding legal rule
    
    -- === VECTOR EMBEDDING ===
    embedding vector(768),  -- Gemini embedding dimension (verify actual dimension)
    
    -- === TIMESTAMPS ===
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    -- === CONSTRAINTS ===
    CONSTRAINT chunk_order_unique UNIQUE (source_document, chunk_index)
);
```

### Indexes for Efficient Querying

```sql
-- Vector similarity search (HNSW for fast approximate nearest neighbor)
CREATE INDEX idx_embedding_hnsw ON legal_chunks 
USING hnsw (embedding vector_cosine_ops)
WITH (m = 16, ef_construction = 64);

-- Type-based filtering
CREATE INDEX idx_source_type ON legal_chunks(source_type);
CREATE INDEX idx_source_document ON legal_chunks(source_document);

-- Criterion-based filtering (critical for use case 3)
CREATE INDEX idx_criterion_tag ON legal_chunks(criterion_tag) WHERE criterion_tag IS NOT NULL;

-- Legal standard filtering (critical for use case 2)
CREATE INDEX idx_legal_standard ON legal_chunks(legal_standard) WHERE legal_standard IS NOT NULL;

-- Winning argument filtering (critical for use case 3)
CREATE INDEX idx_is_winning_argument ON legal_chunks(is_winning_argument) WHERE is_winning_argument = true;

-- Citation-based filtering
CREATE INDEX idx_regulatory_citation ON legal_chunks USING gin (regulatory_citation);
CREATE INDEX idx_case_citation ON legal_chunks(case_citation) WHERE case_citation IS NOT NULL;
CREATE INDEX idx_appeal_citation ON legal_chunks(appeal_citation) WHERE appeal_citation IS NOT NULL;

-- Metadata JSONB filtering (for flexible queries)
CREATE INDEX idx_metadata_gin ON legal_chunks USING gin (metadata);

-- Composite indexes for common query patterns
CREATE INDEX idx_type_criterion ON legal_chunks(source_type, criterion_tag) WHERE criterion_tag IS NOT NULL;
CREATE INDEX idx_type_standard ON legal_chunks(source_type, legal_standard) WHERE legal_standard IS NOT NULL;
CREATE INDEX idx_appeal_winning_criterion ON legal_chunks(source_type, criterion_tag, is_winning_argument) 
    WHERE source_type = 'appeal_decision' AND is_winning_argument = true;
```

## Metadata Extraction Rules

### For Regulations (`source_type = 'regulation'`)

**Required Fields**:
- `regulatory_citation`: Extract all CFR citations (e.g., `['8 CFR § 204.5(h)(3)(vi)']`)
- `criterion_tag`: Map to one of the 10 criteria:
  - `awards` (Criterion 1)
  - `membership` (Criterion 2)
  - `media_coverage` (Criterion 3)
  - `judging` (Criterion 4)
  - `original_contributions` (Criterion 5)
  - `authorship` (Criterion 6)
  - `exhibitions` (Criterion 7)
  - `critical_role` (Criterion 8)
  - `high_salary` (Criterion 9)
  - `commercial_success` (Criterion 10)
- `legal_standard`: Extract if mentions specific tests (e.g., `'Kazarian Two-Step'`, `'Final Merits Determination'`)
- `metadata.section`: Section heading (e.g., `"B. Evidence of Extraordinary Ability"`)
- `metadata.subsection`: Subsection heading (e.g., `"Criterion 6: Authorship"`)
- `metadata.section_level`: Hierarchical level (1-3)

**Optional Fields**:
- `metadata.definition_type`: `'evidentiary_criterion'`, `'eligibility_rule'`, `'adjudication_process'`
- `metadata.examples`: Array of example types mentioned
- `metadata.considerations`: Array of key considerations

### For Precedent Cases (`source_type = 'precedent_case'`)

**Required Fields**:
- `case_citation`: Full case citation (e.g., `'Kazarian v. USCIS, 596 F.3d 1115 (9th Cir. 2010)'`)
- `legal_standard`: Name of legal test established (e.g., `'Kazarian Two-Step'`, `'Final Merits Determination'`)
- `is_holding`: `true` if chunk contains binding legal rule
- `metadata.case_name`: Short case name (e.g., `"Kazarian v. USCIS"`)
- `metadata.court`: Court name (e.g., `"9th Circuit Court of Appeals"`)
- `metadata.date_filed`: Decision date
- `metadata.section`: Section within case (e.g., `"Discussion > Application to Kazarian > Authorship"`)

**Optional Fields**:
- `criterion_tag`: Only if case specifically analyzes a criterion (e.g., Kazarian analyzes authorship)
- `regulatory_citation`: CFR citations discussed in the chunk
- `metadata.holding_type`: `'Rule Definition'`, `'Application to Facts'`, `'Procedural Ruling'`
- `metadata.outcome`: `'Affirmed'`, `'Reversed'`, etc.

### For Appeal Decisions (`source_type = 'appeal_decision'`)

**Required Fields**:
- `appeal_citation`: Full citation (e.g., `'Matter of F-M- Co., Adopted Decision 2020-01 (AAO May 5, 2020)'`)
- `criterion_tag`: The specific criterion the appeal addresses
- `is_winning_argument`: Always `true` (only winning arguments are chunked)
- `metadata.appeal_name`: Short name (e.g., `"Matter of F-M- Co."`)
- `metadata.decision_type`: `'Adopted Decision'`, `'Non-Precedent Decision'`
- `metadata.decision_date`: Decision date
- `metadata.authority_level`: `'Binding'` for Adopted Decisions, `'Non-Binding'` for others
- `metadata.section`: Section within appeal (e.g., `"III. ANALYSIS - B. Successor-in-Interest"`)
- `metadata.decision_result`: `'Sustained'`, `'Dismissed'`

**Optional Fields**:
- `regulatory_citation`: CFR citations discussed
- `legal_standard`: If appeal references specific legal tests
- `metadata.parties`: Object with petitioner/beneficiary info
- `metadata.is_holding`: `true` if chunk contains the official holding/headnote

## Query Patterns by Use Case

### Use Case 1: Getting the Rules Right (Regulations)

**Query**: Find the regulatory definition for "authorship of scholarly articles"

```sql
SELECT 
    id,
    chunk_text,
    regulatory_citation,
    criterion_tag,
    metadata->>'subsection' as subsection,
    1 - (embedding <=> $1::vector) AS similarity
FROM legal_chunks
WHERE source_type = 'regulation'
  AND criterion_tag = 'authorship'
ORDER BY embedding <=> $1::vector
LIMIT 5;
```

**Query**: Find all regulatory criteria related to "original contributions"

```sql
SELECT 
    id,
    chunk_text,
    regulatory_citation,
    criterion_tag,
    metadata->>'subsection' as subsection
FROM legal_chunks
WHERE source_type = 'regulation'
  AND (criterion_tag = 'original_contributions' 
       OR chunk_text ILIKE '%original contribution%')
ORDER BY chunk_index;
```

### Use Case 2: Using Precedent Cases (Cases)

**Query**: Find how courts have defined the "Final Merits Determination" test

```sql
SELECT 
    id,
    chunk_text,
    case_citation,
    legal_standard,
    metadata->>'case_name' as case_name,
    metadata->>'section' as section,
    1 - (embedding <=> $1::vector) AS similarity
FROM legal_chunks
WHERE source_type = 'precedent_case'
  AND legal_standard = 'Final Merits Determination'
  AND is_holding = true
ORDER BY embedding <=> $1::vector
LIMIT 5;
```

**Query**: Find all legal tests established by Kazarian case

```sql
SELECT 
    id,
    chunk_text,
    legal_standard,
    metadata->>'section' as section,
    is_holding
FROM legal_chunks
WHERE source_type = 'precedent_case'
  AND case_citation LIKE '%Kazarian%'
ORDER BY chunk_index;
```

### Use Case 3: Appeal Decisions (Appeals)

**Query**: Find winning arguments for "judging" criterion

```sql
SELECT 
    id,
    chunk_text,
    appeal_citation,
    criterion_tag,
    metadata->>'appeal_name' as appeal_name,
    metadata->>'section' as section,
    1 - (embedding <=> $1::vector) AS similarity
FROM legal_chunks
WHERE source_type = 'appeal_decision'
  AND criterion_tag = 'judging'
  AND is_winning_argument = true
ORDER BY embedding <=> $1::vector
LIMIT 5;
```

**Query**: Find all winning arguments for "authorship" across all appeals

```sql
SELECT 
    id,
    chunk_text,
    appeal_citation,
    metadata->>'appeal_name' as appeal_name,
    metadata->>'decision_date' as decision_date,
    metadata->>'section' as section
FROM legal_chunks
WHERE source_type = 'appeal_decision'
  AND criterion_tag = 'authorship'
  AND is_winning_argument = true
ORDER BY metadata->>'decision_date' DESC;
```

### Cross-Type Queries

**Query**: Find all documents (regulations, cases, appeals) discussing "authorship" criterion

```sql
SELECT 
    id,
    source_type,
    chunk_text,
    COALESCE(regulatory_citation[1], case_citation, appeal_citation) as citation,
    criterion_tag,
    legal_standard,
    1 - (embedding <=> $1::vector) AS similarity
FROM legal_chunks
WHERE criterion_tag = 'authorship'
ORDER BY source_type, embedding <=> $1::vector
LIMIT 20;
```

**Query**: Find regulations and cases that establish the "Kazarian Two-Step" test

```sql
SELECT 
    id,
    source_type,
    chunk_text,
    legal_standard,
    COALESCE(regulatory_citation[1], case_citation) as citation,
    1 - (embedding <=> $1::vector) AS similarity
FROM legal_chunks
WHERE source_type IN ('regulation', 'precedent_case')
  AND legal_standard = 'Kazarian Two-Step'
ORDER BY source_type, embedding <=> $1::vector;
```

## Chunking Strategy by Document Type

### Regulations: Atomic Legal Rules
- **Principle**: One chunk = one complete legal definition or criterion
- **Size**: 200-800 words
- **Boundaries**: Chunk at complete criterion definitions, not arbitrary breaks
- **Overlap**: None (atomic rules should be self-contained)

### Precedent Cases: Legal Test Definitions
- **Principle**: One chunk = one complete legal test definition
- **Size**: 300-1000 words
- **Boundaries**: Chunk at complete legal standard definitions (e.g., "Two-Step Analysis")
- **Overlap**: Minimal (10-15% if test spans multiple paragraphs)

### Appeal Decisions: Winning Arguments
- **Principle**: One chunk = one complete winning argument per criterion
- **Size**: 500-1000 words
- **Boundaries**: Chunk at complete reasoning chains, exclude Director's denial arguments
- **Overlap**: Minimal (10-15% if argument spans multiple paragraphs)

## Vector Embedding Strategy

### Embedding Model
- **Model**: Gemini `text-embedding-004` (or latest)
- **Dimension**: 768 (verify actual dimension)
- **Distance Metric**: Cosine similarity

### Embedding Input Format

For better semantic search, prepend metadata context to chunk text:

```python
# For regulations
embedding_input = f"""
[REGULATION: {regulatory_citation}]
[CRITERION: {criterion_tag}]
[LEGAL_STANDARD: {legal_standard or 'N/A'}]

{chunk_text}
"""

# For precedent cases
embedding_input = f"""
[PRECEDENT_CASE: {case_citation}]
[LEGAL_STANDARD: {legal_standard}]
[HOLDING: {is_holding}]

{chunk_text}
"""

# For appeal decisions
embedding_input = f"""
[APPEAL_DECISION: {appeal_citation}]
[CRITERION: {criterion_tag}]
[WINNING_ARGUMENT: {is_winning_argument}]
[OUTCOME: {metadata->>'decision_result'}]

{chunk_text}
"""
```

This context-enriched input helps the embedding model understand the semantic role of each chunk, improving retrieval quality.

## Validation Rules

### Data Quality Checks

1. **Required Fields**:
   - All chunks must have `source_type`, `source_document`, `chunk_text`
   - Regulations must have `regulatory_citation` and `criterion_tag`
   - Cases must have `case_citation` and `legal_standard`
   - Appeals must have `appeal_citation`, `criterion_tag`, and `is_winning_argument = true`

2. **Consistency Checks**:
   - `criterion_tag` values must be from the standard set: `awards`, `membership`, `media_coverage`, `judging`, `original_contributions`, `authorship`, `exhibitions`, `critical_role`, `high_salary`, `commercial_success`
   - `legal_standard` values should be consistent (e.g., `'Kazarian Two-Step'` not `'Kazarian two-step'` or `'Two-Step Analysis'`)

3. **Appeal Decision Purity**:
   - All appeal chunks must have `is_winning_argument = true`
   - No chunks should contain Director's denial arguments (LLM should filter these out)

## Example Metadata Values

### Regulation Chunk Example

```json
{
  "id": "uuid-here",
  "source_type": "regulation",
  "source_document": "uscis_regulations.txt",
  "chunk_index": 45,
  "chunk_text": "Criterion 6: The person's authorship of scholarly articles...",
  "regulatory_citation": ["8 CFR § 204.5(h)(3)(vi)"],
  "criterion_tag": "authorship",
  "legal_standard": null,
  "metadata": {
    "section": "B. Evidence of Extraordinary Ability",
    "subsection": "Criterion 6: Authorship of Scholarly Articles",
    "section_level": 3,
    "definition_type": "evidentiary_criterion",
    "examples": ["peer-reviewed journals", "conference proceedings"]
  },
  "is_winning_argument": false,
  "is_holding": false
}
```

### Precedent Case Chunk Example

```json
{
  "id": "uuid-here",
  "source_type": "precedent_case",
  "source_document": "kazarian_case.txt",
  "chunk_index": 12,
  "chunk_text": "The AAO's conclusion rests on an improper understanding...",
  "case_citation": "Kazarian v. USCIS, 596 F.3d 1115 (9th Cir. 2010)",
  "criterion_tag": "authorship",
  "legal_standard": "Kazarian Two-Step",
  "regulatory_citation": ["8 CFR § 204.5(h)(3)(vi)"],
  "metadata": {
    "case_name": "Kazarian v. USCIS",
    "court": "9th Circuit Court of Appeals",
    "date_filed": "2010-03-04",
    "judge": "D.W. Nelson",
    "opinion_type": "Majority",
    "section": "Discussion > Application to Kazarian > Authorship",
    "holding_type": "Rule Definition",
    "outcome": "Affirmed"
  },
  "is_winning_argument": false,
  "is_holding": true
}
```

### Appeal Decision Chunk Example

```json
{
  "id": "uuid-here",
  "source_type": "appeal_decision",
  "source_document": "appeal_quintanilla",
  "chunk_index": 8,
  "chunk_text": "The record contains evidence that the beneficiary served as a judge...",
  "appeal_citation": "Matter of Quintanilla, Adopted Decision 2023-XX (AAO Date)",
  "criterion_tag": "judging",
  "legal_standard": null,
  "regulatory_citation": ["8 CFR § 204.5(h)(3)(iv)"],
  "metadata": {
    "appeal_name": "Matter of Quintanilla",
    "decision_type": "Adopted Decision",
    "decision_date": "2023-XX-XX",
    "authority_level": "Binding",
    "section": "Section A: Judging",
    "is_holding": false,
    "decision_result": "Sustained",
    "parties": {
      "petitioner": "Tech Company",
      "beneficiary": "Software Engineer"
    }
  },
  "is_winning_argument": true,
  "is_holding": false
}
```

## Implementation Notes

1. **LLM-Based Extraction**: Use Gemini 3 to extract metadata semantically, not regex patterns
2. **Consistent Tagging**: Maintain a controlled vocabulary for `criterion_tag` and `legal_standard`
3. **Citation Normalization**: Normalize citations to consistent format (e.g., `'8 CFR § 204.5(h)(3)(vi)'` not `'8 C.F.R. § 204.5(h)(3)(vi)'`)
4. **Appeal Purity**: Use LLM to verify appeal chunks contain only winning arguments, not Director denials
5. **Cross-References**: Consider adding a `related_chunks` field to link related chunks across document types

