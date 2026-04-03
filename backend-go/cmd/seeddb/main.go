package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	dbHost := envOr("DB_HOST", "127.0.0.1")
	dbPort := envOr("DB_PORT", "3306")
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASSWORD")
	dbName := envOr("DB_NAME", "mandiri_gapuraai")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&loc=UTC&multiStatements=true",
		dbUser, dbPass, dbHost, dbPort, dbName)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "ping: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Connected to database.")

	seedFile := "seed.sql"
	if len(os.Args) > 1 {
		seedFile = os.Args[1]
	}

	data, err := os.ReadFile(seedFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", seedFile, err)
		os.Exit(1)
	}

	// Split by semicolons and execute each statement
	statements := strings.Split(string(data), ";")
	executed := 0
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" || strings.HasPrefix(stmt, "--") {
			continue
		}
		// Skip comment-only blocks
		lines := strings.Split(stmt, "\n")
		hasCode := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.HasPrefix(trimmed, "--") {
				hasCode = true
				break
			}
		}
		if !hasCode {
			continue
		}

		_, err := db.Exec(stmt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR executing statement:\n%s\n=> %v\n\n", truncate(stmt, 200), err)
			continue
		}
		executed++
	}

	fmt.Printf("Done. %d statements executed successfully.\n", executed)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
