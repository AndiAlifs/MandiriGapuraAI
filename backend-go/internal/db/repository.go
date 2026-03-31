package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	mysql "github.com/go-sql-driver/mysql"
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

type ModelInfo struct {
	ModelID         int     `json:"modelID"`
	ModelName       string  `json:"modelName"`
	Provider        string  `json:"provider"`
	CostPer1kInput  float64 `json:"costPer1kInput"`
	CostPer1kOutput float64 `json:"costPer1kOutput"`
	IsLocalFallback bool    `json:"isLocalFallback"`
}

type StudioScorecards struct {
	TotalPIIEntitiesScrubbed int     `json:"total_pii_entities_scrubbed"`
	TotalAPICostSaved        float64 `json:"total_api_cost_saved"`
}

type AuditLogRecord struct {
	LogID          int64     `json:"log_id"`
	AppID          int       `json:"app_id"`
	ProjectName    string    `json:"project_name"`
	ModelUsed      string    `json:"model_used"`
	OriginalPrompt string    `json:"original_prompt"`
	ScrubbedPrompt string    `json:"scrubbed_prompt"`
	ResponseText   string    `json:"response_text"`
	InputTokens    int       `json:"input_tokens"`
	OutputTokens   int       `json:"output_tokens"`
	CalculatedCost float64   `json:"calculated_cost"`
	LatencyMS      int       `json:"latency_ms"`
	Timestamp      time.Time `json:"timestamp"`
}

type AuditLogFilter struct {
	ProjectName string
	ModelUsed   string
	Limit       int
	Offset      int
}

type CreateAppAuthInput struct {
	ProjectName     string
	Username        string
	Password        string
	DailyTokenLimit int
}

var ErrAppAuthUsernameExists = errors.New("app auth username already exists")

func NewRepository(dsn string) (*Repository, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Printf("db: failed to open connection: %v", err)
		return nil, err
	}
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Printf("db: ping failed: %v", err)
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
			log.Printf("db: AuthenticateApp no row found for user=%q", username)
			return nil, nil
		}
		log.Printf("db: AuthenticateApp query error for user=%q: %v", username, err)
		return nil, err
	}

	if !passwordMatches(password, app.PasswordHash) {
		return nil, nil
	}
	return &app, nil
}

func (r *Repository) CreateAppAuth(ctx context.Context, in CreateAppAuthInput) (*AppAuth, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("db: CreateAppAuth password hashing failed for user=%q: %v", in.Username, err)
		return nil, err
	}

	const query = `
		INSERT INTO Apps_Auth (ProjectName, Username, PasswordHash, DailyTokenLimit)
		VALUES (?, ?, ?, ?)`

	result, err := r.db.ExecContext(
		ctx,
		query,
		in.ProjectName,
		in.Username,
		string(hashedPassword),
		in.DailyTokenLimit,
	)
	if err != nil {
		if isDuplicateEntryError(err) {
			return nil, ErrAppAuthUsernameExists
		}
		log.Printf("db: CreateAppAuth insert failed for user=%q: %v", in.Username, err)
		return nil, err
	}

	appID, err := result.LastInsertId()
	if err != nil {
		log.Printf("db: CreateAppAuth failed to get inserted id for user=%q: %v", in.Username, err)
		return nil, err
	}

	return &AppAuth{
		AppID:           int(appID),
		ProjectName:     in.ProjectName,
		Username:        in.Username,
		PasswordHash:    string(hashedPassword),
		DailyTokenLimit: in.DailyTokenLimit,
	}, nil
}

func (r *Repository) DailyTokenUsage(ctx context.Context, appID int) (int, error) {
	const query = `
		SELECT COALESCE(SUM(InputTokens + OutputTokens), 0)
		FROM Audit_Logs
		WHERE AppID = ? AND DATE(Timestamp) = CURDATE()`

	var usage int
	if err := r.db.QueryRowContext(ctx, query, appID).Scan(&usage); err != nil {
		log.Printf("db: DailyTokenUsage query error for app_id=%d: %v", appID, err)
		return 0, err
	}
	return usage, nil
}

func (r *Repository) GetModelInfo(ctx context.Context, modelName string) (*ModelInfo, error) {
	const query = `
		SELECT ModelID, ModelName, Provider, CostPer1kInput, CostPer1kOutput, IsLocalFallback
		FROM Model_Registry
		WHERE ModelName = ?
		LIMIT 1`

	var info ModelInfo
	if err := r.db.QueryRowContext(ctx, query, modelName).Scan(
		&info.ModelID,
		&info.ModelName,
		&info.Provider,
		&info.CostPer1kInput,
		&info.CostPer1kOutput,
		&info.IsLocalFallback,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("db: GetModelInfo no registry entry for model=%q, defaulting to OpenAI", modelName)
			return &ModelInfo{
				ModelName:       modelName,
				Provider:        "OpenAI",
				CostPer1kInput:  0,
				CostPer1kOutput: 0,
				IsLocalFallback: false,
			}, nil
		}
		log.Printf("db: GetModelInfo query error for model=%q: %v", modelName, err)
		return nil, err
	}
	return &info, nil
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
			log.Printf("db: LocalFallbackModel no fallback configured, using default llama3-8b-local")
			return "llama3-8b-local", nil
		}
		log.Printf("db: LocalFallbackModel query error: %v", err)
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
	if err != nil {
		log.Printf("db: InsertAuditLog failed for app_id=%d: %v", in.AppID, err)
	}
	return err
}

func (r *Repository) GetStudioScorecards(ctx context.Context) (StudioScorecards, error) {
	const piiQuery = `
		SELECT CAST(COALESCE(SUM(
			(LENGTH(ScrubbedPrompt) - LENGTH(REPLACE(ScrubbedPrompt, '[NIK_MASKED]', ''))) / LENGTH('[NIK_MASKED]') +
			(LENGTH(ScrubbedPrompt) - LENGTH(REPLACE(ScrubbedPrompt, '[ACCOUNT_MASKED]', ''))) / LENGTH('[ACCOUNT_MASKED]')
		), 0) AS INTEGER)
		FROM Audit_Logs`

	const savingsQuery = `
		SELECT COALESCE(SUM(
			((a.InputTokens / 1000.0) * base.CostPer1kInput) +
			((a.OutputTokens / 1000.0) * base.CostPer1kOutput)
		), 0)
		FROM Audit_Logs a
		JOIN Model_Registry used ON used.ModelName = a.ModelUsed
		CROSS JOIN (
			SELECT CostPer1kInput, CostPer1kOutput
			FROM Model_Registry
			WHERE IsLocalFallback = FALSE
			ORDER BY (CostPer1kInput + CostPer1kOutput) ASC
			LIMIT 1
		) base
		WHERE used.IsLocalFallback = TRUE`

	var cards StudioScorecards
	if err := r.db.QueryRowContext(ctx, piiQuery).Scan(&cards.TotalPIIEntitiesScrubbed); err != nil {
		log.Printf("db: GetStudioScorecards pii query failed: %v", err)
		return StudioScorecards{}, err
	}

	if err := r.db.QueryRowContext(ctx, savingsQuery).Scan(&cards.TotalAPICostSaved); err != nil {
		log.Printf("db: GetStudioScorecards savings query failed: %v", err)
		return StudioScorecards{}, err
	}

	return cards, nil
}

func (r *Repository) ListAuditLogs(ctx context.Context, filter AuditLogFilter) ([]AuditLogRecord, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 200 {
		filter.Limit = 200
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	query := `
		SELECT
			a.LogID,
			a.AppID,
			app.ProjectName,
			a.ModelUsed,
			a.OriginalPrompt,
			a.ScrubbedPrompt,
			a.ResponseText,
			a.InputTokens,
			a.OutputTokens,
			a.CalculatedCost,
			a.LatencyMS,
			a.Timestamp
		FROM Audit_Logs a
		JOIN Apps_Auth app ON app.AppID = a.AppID
		WHERE 1 = 1`

	args := make([]any, 0, 4)
	if strings.TrimSpace(filter.ProjectName) != "" {
		query += ` AND app.ProjectName = ?`
		args = append(args, strings.TrimSpace(filter.ProjectName))
	}
	if strings.TrimSpace(filter.ModelUsed) != "" {
		query += ` AND a.ModelUsed = ?`
		args = append(args, strings.TrimSpace(filter.ModelUsed))
	}

	query += ` ORDER BY a.Timestamp DESC LIMIT ? OFFSET ?`
	args = append(args, filter.Limit, filter.Offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		log.Printf("db: ListAuditLogs query failed: %v", err)
		return nil, err
	}
	defer rows.Close()

	logs := make([]AuditLogRecord, 0, filter.Limit)
	for rows.Next() {
		var rec AuditLogRecord
		if err := rows.Scan(
			&rec.LogID,
			&rec.AppID,
			&rec.ProjectName,
			&rec.ModelUsed,
			&rec.OriginalPrompt,
			&rec.ScrubbedPrompt,
			&rec.ResponseText,
			&rec.InputTokens,
			&rec.OutputTokens,
			&rec.CalculatedCost,
			&rec.LatencyMS,
			&rec.Timestamp,
		); err != nil {
			log.Printf("db: ListAuditLogs scan failed: %v", err)
			return nil, err
		}
		logs = append(logs, rec)
	}

	if err := rows.Err(); err != nil {
		log.Printf("db: ListAuditLogs row iteration failed: %v", err)
		return nil, err
	}

	return logs, nil
}

func (r *Repository) ListModelRegistry(ctx context.Context) ([]ModelInfo, error) {
	const query = `
		SELECT ModelID, ModelName, Provider, CostPer1kInput, CostPer1kOutput, IsLocalFallback
		FROM Model_Registry
		ORDER BY IsLocalFallback ASC, ModelName ASC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		log.Printf("db: ListModelRegistry query failed: %v", err)
		return nil, err
	}
	defer rows.Close()

	models := make([]ModelInfo, 0)
	for rows.Next() {
		var m ModelInfo
		if err := rows.Scan(
			&m.ModelID,
			&m.ModelName,
			&m.Provider,
			&m.CostPer1kInput,
			&m.CostPer1kOutput,
			&m.IsLocalFallback,
		); err != nil {
			log.Printf("db: ListModelRegistry scan failed: %v", err)
			return nil, err
		}
		models = append(models, m)
	}

	if err := rows.Err(); err != nil {
		log.Printf("db: ListModelRegistry row iteration failed: %v", err)
		return nil, err
	}

	return models, nil
}

func passwordMatches(rawPassword, stored string) bool {
	if strings.HasPrefix(stored, "$2a$") || strings.HasPrefix(stored, "$2b$") || strings.HasPrefix(stored, "$2y$") {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(rawPassword)) == nil
	}
	return rawPassword == stored
}

func isDuplicateEntryError(err error) bool {
	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) {
		return false
	}
	return mysqlErr.Number == 1062
}

func FormatDSN(user, pass, host string, port int, dbName string) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4&loc=UTC", user, pass, host, port, dbName)
}
