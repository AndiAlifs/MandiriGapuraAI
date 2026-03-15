package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

type Repository struct {
	db *sql.DB
}

type AppAuth struct {
	AppID           int
	ProjectName     string
	Username        string
	PasswordHash    string
	DailyTokenLimit int
}

type AuditLogInput struct {
	AppID          int
	ModelUsed      string
	OriginalPrompt string
	ScrubbedPrompt string
	ResponseText   string
	InputTokens    int
	OutputTokens   int
	CalculatedCost float64
	LatencyMS      int
}

func NewRepository(dsn string) (*Repository, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Repository{db: db}, nil
}

func (r *Repository) Close() error {
	return r.db.Close()
}

func (r *Repository) AuthenticateApp(ctx context.Context, username, password string) (*AppAuth, error) {
	const query = `
		SELECT AppID, ProjectName, Username, PasswordHash, DailyTokenLimit
		FROM Apps_Auth
		WHERE Username = ?
		LIMIT 1`

	var app AppAuth
	if err := r.db.QueryRowContext(ctx, query, username).Scan(
		&app.AppID,
		&app.ProjectName,
		&app.Username,
		&app.PasswordHash,
		&app.DailyTokenLimit,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if !passwordMatches(password, app.PasswordHash) {
		return nil, nil
	}
	return &app, nil
}

func (r *Repository) DailyTokenUsage(ctx context.Context, appID int) (int, error) {
	const query = `
		SELECT COALESCE(SUM(InputTokens + OutputTokens), 0)
		FROM Audit_Logs
		WHERE AppID = ? AND DATE(Timestamp) = CURDATE()`

	var usage int
	if err := r.db.QueryRowContext(ctx, query, appID).Scan(&usage); err != nil {
		return 0, err
	}
	return usage, nil
}

func (r *Repository) ModelProvider(ctx context.Context, modelName string) (string, error) {
	const query = `
		SELECT Provider
		FROM Model_Registry
		WHERE ModelName = ?
		LIMIT 1`

	var provider string
	if err := r.db.QueryRowContext(ctx, query, modelName).Scan(&provider); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "OpenAI", nil // default provider when model not registered
		}
		return "", err
	}
	return provider, nil
}

func (r *Repository) LocalFallbackModel(ctx context.Context) (string, error) {
	const query = `
		SELECT ModelName
		FROM Model_Registry
		WHERE IsLocalFallback = TRUE
		ORDER BY ModelID ASC
		LIMIT 1`

	var model string
	if err := r.db.QueryRowContext(ctx, query).Scan(&model); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "llama3-8b-local", nil
		}
		return "", err
	}
	return model, nil
}

func (r *Repository) InsertAuditLog(ctx context.Context, in AuditLogInput) error {
	const query = `
		INSERT INTO Audit_Logs (
			AppID,
			ModelUsed,
			OriginalPrompt,
			ScrubbedPrompt,
			ResponseText,
			InputTokens,
			OutputTokens,
			CalculatedCost,
			LatencyMS
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(
		ctx,
		query,
		in.AppID,
		in.ModelUsed,
		in.OriginalPrompt,
		in.ScrubbedPrompt,
		in.ResponseText,
		in.InputTokens,
		in.OutputTokens,
		in.CalculatedCost,
		in.LatencyMS,
	)
	return err
}

func passwordMatches(rawPassword, stored string) bool {
	if strings.HasPrefix(stored, "$2a$") || strings.HasPrefix(stored, "$2b$") || strings.HasPrefix(stored, "$2y$") {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(rawPassword)) == nil
	}
	return rawPassword == stored
}

func FormatDSN(user, pass, host string, port int, dbName string) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4&loc=UTC", user, pass, host, port, dbName)
}
