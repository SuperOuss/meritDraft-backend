# Unified Schema: Use Case Examples

This document demonstrates how the unified metadata and vector schema supports the three use cases for crafting immigration letters, using concrete examples from the analyzed documents.

## Use Case 1: Getting the Rules Right (Regulations)

**Goal**: Find exact regulatory text, definitions, and requirements for specific criteria.

### Example Query: "What are the requirements for authorship of scholarly articles?"

**Schema Fields Used**:
- `source_type = 'regulation'`
- `criterion_tag = 'authorship'`
- `regulatory_citation = ['8 CFR § 204.5(h)(3)(vi)']`
- `metadata.subsection = 'Criterion 6: Authorship of Scholarly Articles'`

**Example Chunk from `uscis_regulations.txt`**:
```json
{
  "source_type": "regulation",
  "source_document": "uscis_regulations.txt",
  "criterion_tag": "authorship",
  "regulatory_citation": ["8 CFR § 204.5(h)(3)(vi)"],
  "chunk_text": "Criterion 6: The person's authorship of scholarly articles in the field, in professional or major trade publications or other major media. First, USCIS determines whether the person has authored scholarly articles in the field. As defined in the academic arena, a scholarly article reports on original research, experimentation, or philosophical discourse...",
  "metadata": {
    "section": "B. Evidence of Extraordinary Ability",
    "subsection": "Criterion 6: Authorship of Scholarly Articles",
    "section_level": 3,
    "definition_type": "evidentiary_criterion"
  }
}
```

**Why This Works**:
- Semantic search finds the chunk by meaning
- `criterion_tag` filter ensures we get the right criterion
- `regulatory_citation` provides exact legal reference for the letter
- `metadata.subsection` gives context for citation

## Use Case 2: Using Precedent Cases (Cases)

**Goal**: Find how courts have interpreted regulations and what legal tests they've established.

### Example Query: "How does the Kazarian case define the two-step analysis?"

**Schema Fields Used**:
- `source_type = 'precedent_case'`
- `legal_standard = 'Kazarian Two-Step'`
- `case_citation = 'Kazarian v. USCIS, 596 F.3d 1115 (9th Cir. 2010)'`
- `is_holding = true`

**Example Chunk from `kazarian_case.txt`**:
```json
{
  "source_type": "precedent_case",
  "source_document": "kazarian_case.txt",
  "case_citation": "Kazarian v. USCIS, 596 F.3d 1115 (9th Cir. 2010)",
  "legal_standard": "Kazarian Two-Step",
  "is_holding": true,
  "chunk_text": "Officers should use a two-step analysis to evaluate the evidence submitted with the petition to demonstrate eligibility for classification as a person of extraordinary ability. Step 1: Assess whether evidence meets regulatory criteria: Determine, by a preponderance of the evidence, which evidence submitted by the petitioner objectively meets the parameters of the regulatory description... Step 2: Final merits determination: Evaluate all the evidence together when considering the petition in its entirety...",
  "metadata": {
    "case_name": "Kazarian v. USCIS",
    "court": "9th Circuit Court of Appeals",
    "date_filed": "2010-03-04",
    "section": "Discussion > A. The Extraordinary Ability Visa",
    "holding_type": "Rule Definition"
  }
}
```

**Why This Works**:
- `legal_standard` filter finds chunks that establish the test
- `is_holding = true` ensures we get binding legal rules, not dicta
- `case_citation` provides proper case citation for the letter
- Semantic search finds related discussions of the test

### Example Query: "What did Kazarian say about requiring citations for scholarly articles?"

**Schema Fields Used**:
- `source_type = 'precedent_case'`
- `criterion_tag = 'authorship'` (Kazarian specifically analyzes this criterion)
- `regulatory_citation = ['8 CFR § 204.5(h)(3)(vi)']`

**Example Chunk from `kazarian_case.txt`**:
```json
{
  "source_type": "precedent_case",
  "source_document": "kazarian_case.txt",
  "case_citation": "Kazarian v. USCIS, 596 F.3d 1115 (9th Cir. 2010)",
  "criterion_tag": "authorship",
  "legal_standard": "Kazarian Two-Step",
  "regulatory_citation": ["8 CFR § 204.5(h)(3)(vi)"],
  "is_holding": true,
  "chunk_text": "The AAO's conclusion rests on an improper understanding of 8 C.F.R. § 204.5(h)(3)(vi). Nothing in that provision requires a petitioner to demonstrate the research community's reaction to his published articles before those articles can be considered as evidence... While other authors' citations (or a lack thereof) might be relevant to the final merits determination of whether a petitioner is at the very top of his or her field of endeavor, they are not relevant to the antecedent procedural question of whether the petitioner has provided at least three types of evidence.",
  "metadata": {
    "case_name": "Kazarian v. USCIS",
    "section": "Discussion > Application to Kazarian > 1. Authorship of Scholarly Articles",
    "holding_type": "Rule Definition"
  }
}
```

**Why This Works**:
- `criterion_tag` filter finds case analysis specific to authorship
- `is_holding = true` ensures this is binding precedent
- The chunk text provides the exact legal reasoning to cite in the letter
- Shows how to argue against improper agency requirements

## Use Case 3: Appeal Decisions (Appeals)

**Goal**: Find real-world examples of sufficient evidence and winning arguments.

### Example Query: "What evidence was sufficient for the judging criterion in winning appeals?"

**Schema Fields Used**:
- `source_type = 'appeal_decision'`
- `criterion_tag = 'judging'`
- `is_winning_argument = true`
- `metadata.decision_result = 'Sustained'`

**Example Chunk from `appeal_quintanilla`** (hypothetical based on structure):
```json
{
  "source_type": "appeal_decision",
  "source_document": "appeal_quintanilla",
  "appeal_citation": "Matter of Quintanilla, Adopted Decision 2023-XX (AAO Date)",
  "criterion_tag": "judging",
  "is_winning_argument": true,
  "regulatory_citation": ["8 CFR § 204.5(h)(3)(iv)"],
  "chunk_text": "The record contains evidence that the beneficiary served as a judge for the International Hackathon Competition in 2023. The competition attracted over 500 participants from 15 countries. The beneficiary evaluated technical submissions based on innovation, code quality, and problem-solving approach. This judging activity demonstrates the beneficiary's recognition as a person of distinguished judgment in the field of software engineering. The evidence is sufficient to meet the regulatory criteria for judging the work of others.",
  "metadata": {
    "appeal_name": "Matter of Quintanilla",
    "decision_type": "Adopted Decision",
    "authority_level": "Binding",
    "section": "Section A: Judging",
    "decision_result": "Sustained"
  }
}
```

**Why This Works**:
- `is_winning_argument = true` ensures we only get successful reasoning
- `criterion_tag` filter finds examples for the specific criterion
- The chunk text shows exactly what evidence was sufficient
- Provides a template for arguing similar evidence in a new case

### Example Query: "How did Matter of F-M- Co. establish successor-in-interest principles?"

**Schema Fields Used**:
- `source_type = 'appeal_decision'`
- `appeal_citation = 'Matter of F-M- Co., Adopted Decision 2020-01'`
- `is_winning_argument = true`
- `metadata.is_holding = true` (contains the official headnote)

**Example Chunk from `appeal_fm-co`**:
```json
{
  "source_type": "appeal_decision",
  "source_document": "appeal_fm-co",
  "appeal_citation": "Matter of F-M- Co., Adopted Decision 2020-01 (AAO May 5, 2020)",
  "is_winning_argument": true,
  "regulatory_citation": ["8 C.F.R. § 204.5(j)(2)", "INA § 203(b)(1)(C)"],
  "chunk_text": "In the event a corporate restructuring affecting the foreign entity occurs prior to the filing of a first preference multinational executive or manager petition, a petitioner may establish that the beneficiary's qualifying foreign employer continues to exist and do business through a valid successor entity. If these conditions are met, USCIS will consider the successor-in-interest to be the same entity that employed the beneficiary abroad.",
  "metadata": {
    "appeal_name": "Matter of F-M- Co.",
    "decision_type": "Adopted Decision",
    "authority_level": "Binding",
    "section": "Headnotes",
    "is_holding": true,
    "decision_result": "Sustained"
  }
}
```

**Why This Works**:
- `metadata.is_holding = true` finds the official binding rule
- `is_winning_argument = true` ensures we get the correct reasoning
- Provides the exact legal principle to cite in similar situations

## Cross-Type Query Examples

### Query: "Find all documents discussing authorship criterion"

This query spans all three document types:

```sql
SELECT 
    source_type,
    chunk_text,
    COALESCE(regulatory_citation[1], case_citation, appeal_citation) as citation,
    criterion_tag,
    legal_standard
FROM legal_chunks
WHERE criterion_tag = 'authorship'
ORDER BY source_type, chunk_index;
```

**Results**:
1. **Regulation**: Definition of authorship criterion (8 CFR § 204.5(h)(3)(vi))
2. **Precedent Case**: Kazarian's interpretation of authorship requirements
3. **Appeal Decision**: Real-world examples of sufficient authorship evidence

**Why This Works**:
- Unified `criterion_tag` field works across all document types
- Semantic search finds related discussions even if terminology varies
- Provides complete picture: rule → interpretation → application

### Query: "Find regulations and cases that establish the Kazarian Two-Step test"

```sql
SELECT 
    source_type,
    chunk_text,
    COALESCE(regulatory_citation[1], case_citation) as citation,
    legal_standard
FROM legal_chunks
WHERE source_type IN ('regulation', 'precedent_case')
  AND legal_standard = 'Kazarian Two-Step'
ORDER BY source_type, chunk_index;
```

**Results**:
1. **Regulation**: USCIS Policy Manual section explaining the two-step analysis
2. **Precedent Case**: Kazarian case establishing the test

**Why This Works**:
- Unified `legal_standard` field links related concepts across types
- Shows how regulations codify court precedent
- Provides both the regulatory guidance and the binding case law

## Letter Crafting Workflow

### Step 1: Get the Rules Right (Regulations)

**Query Pattern**:
```sql
WHERE source_type = 'regulation' 
  AND criterion_tag = '{criterion}'
```

**Output**: Exact regulatory text to cite in the letter

### Step 2: Use Precedent Cases (Cases)

**Query Pattern**:
```sql
WHERE source_type = 'precedent_case'
  AND (legal_standard = '{test_name}' OR criterion_tag = '{criterion}')
  AND is_holding = true
```

**Output**: Court interpretations and legal tests to argue the rules

### Step 3: Appeal Decisions (Appeals)

**Query Pattern**:
```sql
WHERE source_type = 'appeal_decision'
  AND criterion_tag = '{criterion}'
  AND is_winning_argument = true
```

**Output**: Real-world examples of sufficient evidence and winning reasoning

## Key Schema Design Decisions

### 1. Unified `criterion_tag` Field

**Decision**: Use same field across all three document types

**Rationale**:
- Enables cross-type queries (find all documents about "authorship")
- Consistent filtering regardless of document type
- Simplifies query logic

**Trade-off**: Some case chunks won't have `criterion_tag` (global standards), which is correct

### 2. Separate Citation Fields

**Decision**: `regulatory_citation`, `case_citation`, `appeal_citation` as separate fields

**Rationale**:
- Type-specific citations have different formats
- Enables type-specific filtering
- Clearer than a single generic citation field

**Alternative Considered**: Single `citation` field with type discriminator - rejected for clarity

### 3. `is_winning_argument` Flag

**Decision**: Boolean flag for appeal decisions only

**Rationale**:
- Critical for Use Case 3 (only want winning arguments)
- Prevents retrieving Director's denial arguments
- Simple boolean filter is fast

**Trade-off**: Requires careful chunking to ensure only winning arguments are stored

### 4. JSONB Metadata Field

**Decision**: Flexible JSONB for type-specific metadata

**Rationale**:
- Different document types have different metadata needs
- Avoids sparse columns
- Enables flexible querying with GIN index

**Trade-off**: Less structured than normalized tables, but acceptable for this use case

### 5. `legal_standard` Field

**Decision**: Unified field across all types

**Rationale**:
- Links regulations that codify tests with cases that establish them
- Enables finding all discussions of a specific test
- Critical for Use Case 2

**Example**: Both USCIS Policy Manual and Kazarian case reference "Kazarian Two-Step"

## Performance Considerations

### Index Strategy

1. **HNSW Index on `embedding`**: Fast semantic search
2. **B-tree on `source_type`**: Fast type filtering
3. **B-tree on `criterion_tag`**: Fast criterion filtering
4. **B-tree on `legal_standard`**: Fast legal test filtering
5. **B-tree on `is_winning_argument`**: Fast winning argument filtering
6. **GIN on `metadata`**: Fast JSONB queries
7. **Composite indexes**: Optimize common query patterns

### Query Optimization

- Use metadata filters BEFORE vector similarity (reduces search space)
- Composite indexes for common filter combinations
- GIN index on JSONB enables fast metadata queries

### Example Optimized Query

```sql
-- Filter first, then search (more efficient)
SELECT 
    id,
    chunk_text,
    1 - (embedding <=> $1::vector) AS similarity
FROM legal_chunks
WHERE source_type = 'appeal_decision'
  AND criterion_tag = 'authorship'
  AND is_winning_argument = true
ORDER BY embedding <=> $1::vector
LIMIT 5;
```

This query:
1. Filters by type, criterion, and winning argument (uses indexes)
2. Then performs vector similarity search on filtered subset (much faster)
3. Returns top 5 most similar chunks

