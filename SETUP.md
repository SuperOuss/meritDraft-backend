# MeritDraft Backend Setup

## AWS Services Created

### PostgreSQL RDS Instance
- **Instance ID**: `meritdraft-postgres`
- **Engine**: PostgreSQL 15.15
- **Instance Class**: db.t3.micro
- **Username**: `postgres`
- **Password**: `MeritDraft2024!`
- **Database**: `meritdraft`
- **Extensions**: `pgvector` (for vector similarity search)

## Getting Connection Details

To get the connection endpoint, run:

```bash
./get-connection-info.sh
```

Or use the automated script to update your `.env` file:

```bash
./create-env.sh
```

## Environment Variables

The `.env` file in the backend directory should contain:

```env
# Database Configuration
DATABASE_URL=postgres://postgres:MeritDraft2024!@<RDS_ENDPOINT>:5432/meritdraft?sslmode=require

# Gemini API Key
GEMINI_API_KEY=your_gemini_api_key_here

# Server Configuration
PORT=8080
```

Replace:
- `<RDS_ENDPOINT>` with the PostgreSQL endpoint from the script output
- `your_gemini_api_key_here` with your actual Gemini API key

## pgvector Extension

The pgvector extension has been installed in the PostgreSQL database to enable vector similarity search capabilities. This allows you to store and query vector embeddings directly in PostgreSQL.

Example usage:
```sql
-- Create a table with vector column
CREATE TABLE embeddings (
    id SERIAL PRIMARY KEY,
    content TEXT,
    embedding vector(1536)  -- Adjust dimension as needed
);

-- Create an index for efficient similarity search
CREATE INDEX ON embeddings USING hnsw (embedding vector_l2_ops);
```

## Running the Server

```bash
go run main.go
```

The server will start on port 8080 (or the PORT specified in .env).

## Security Notes

- The RDS instance is currently publicly accessible. Consider restricting access to specific IPs in production.
- Update the default password for production use.
- Store sensitive credentials in AWS Secrets Manager or environment variables, not in code.
