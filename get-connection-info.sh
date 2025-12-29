#!/bin/bash

echo "=== MeritDraft AWS Connection Info ==="
echo ""

echo "PostgreSQL RDS:"
aws rds describe-db-instances \
  --db-instance-identifier meritdraft-postgres \
  --region us-east-1 \
  --query 'DBInstances[0].[DBInstanceStatus,Endpoint.Address,Endpoint.Port,MasterUsername]' \
  --output table

echo ""
echo "pgvector Extension:"
echo "  Status: Installed in 'meritdraft' database"
echo "  Extension: vector"
echo ""
echo "To create .env file with these values, run:"
echo "  ./create-env.sh"
