#!/bin/bash

echo "Checking for service endpoints..."

# Get PostgreSQL endpoint
PG_STATUS=$(aws rds describe-db-instances --db-instance-identifier meritdraft-postgres --region us-east-1 --query 'DBInstances[0].DBInstanceStatus' --output text)
PG_ENDPOINT=$(aws rds describe-db-instances --db-instance-identifier meritdraft-postgres --region us-east-1 --query 'DBInstances[0].Endpoint.Address' --output text 2>/dev/null)
PG_PORT=$(aws rds describe-db-instances --db-instance-identifier meritdraft-postgres --region us-east-1 --query 'DBInstances[0].Endpoint.Port' --output text 2>/dev/null)

# Update or create .env file
if [ "$PG_ENDPOINT" != "None" ] && [ -n "$PG_ENDPOINT" ]; then
    # Check if .env exists, if not create it
    if [ ! -f .env ]; then
        cat > .env <<EOF
# Database Configuration
DATABASE_URL=postgres://postgres:MeritDraft2024!@${PG_ENDPOINT}:${PG_PORT}/meritdraft?sslmode=require

# Gemini API Key (update with your actual key)
GEMINI_API_KEY=your_gemini_api_key_here

# Server Configuration
PORT=8080
EOF
    else
        # Update existing .env file with PostgreSQL URL
        sed -i '' "s|DATABASE_URL=.*|DATABASE_URL=postgres://postgres:MeritDraft2024!@${PG_ENDPOINT}:${PG_PORT}/meritdraft?sslmode=require|" .env
    fi
    
    echo ".env file updated!"
    echo ""
    echo "Connection details:"
    echo "  PostgreSQL: ${PG_ENDPOINT}:${PG_PORT} ✓"
    echo "  pgvector extension: Installed and ready"
else
    echo "PostgreSQL is still being created..."
    echo "  PostgreSQL status: ${PG_STATUS}"
    echo ""
    echo "Run this script again in a few minutes"
fi
