package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
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

	// Create a test user
	email := "test@example.com"
	password := "testpassword123"
	name := "Test User"

	// Check if user already exists
	var existingID uuid.UUID
	err = pool.QueryRow(ctx, "SELECT id FROM users WHERE email = $1", email).Scan(&existingID)
	if err == nil {
		log.Printf("User with email %s already exists (ID: %s)", email, existingID)
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	// Insert user
	var userID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, name)
		VALUES ($1, $2, $3)
		RETURNING id
	`, email, string(hashedPassword), name).Scan(&userID)

	if err != nil {
		log.Fatalf("Failed to create user: %v", err)
	}

	fmt.Printf("✅ Test user created successfully!\n")
	fmt.Printf("   ID: %s\n", userID)
	fmt.Printf("   Email: %s\n", email)
	fmt.Printf("   Password: %s\n", password)
	fmt.Printf("   Name: %s\n", name)
}
