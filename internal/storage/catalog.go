package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// CatalogRepo provides methods for persisting and loading catalog provider and model records.
type CatalogRepo struct {
	db *Database
}

// NewCatalogRepo creates a new CatalogRepo backed by the given database.
func NewCatalogRepo(db *Database) *CatalogRepo {
	return &CatalogRepo{db: db}
}

// ProviderRecord represents a provider entry persisted in the catalog.
type ProviderRecord struct {
	Name                   string
	BaseURL                string
	APIKey                 string
	Enabled                *bool
	AnthropicToolsDisabled bool
}

// ModelRecord represents a model entry persisted in the catalog.
type ModelRecord struct {
	ID            string
	Name          string
	Reasoning     bool
	ToolCall      bool
	Vision        bool
	ContextWindow int64
	CostInput     float64
	CostOutput    float64
}

// Provider holds a provider's configuration as loaded from the database.
type Provider struct {
	Name                   string
	BaseURL                string
	APIKey                 string
	Enabled                *bool
	AnthropicToolsDisabled bool
}

// Modalities describes the input and output data types a model supports.
type Modalities struct {
	Input  []string
	Output []string
}

// Limit describes the context window and output token limits for a model.
type Limit struct {
	Context int64
	Output  int64
}

// Rates holds per-million-token pricing for a model.
type Rates struct {
	Input  float64
	Output float64
}

// Model holds a full model definition with nested limit and pricing details.
type Model struct {
	ID         string
	Name       string
	Reasoning  bool
	ToolCall   bool
	Vision     bool
	Modalities Modalities
	Limit      *Limit
	Rates      *Rates
}

// Catalog holds the parsed provider and model maps as loaded from the database.
type Catalog struct {
	Providers map[string]Provider
	Models    map[string]Model
}

// IndexedCatalog extends Catalog with an index from provider name to its models.
type IndexedCatalog struct {
	Catalog
	ProviderModels map[string][]Model
}

// ContextWindow returns the model's context window limit, or 0 if unknown.
func (m Model) ContextWindow() int64 {
	if m.Limit != nil {
		return m.Limit.Context
	}
	return 0
}

// CostInputPerM returns the input cost per million tokens, or 0 if unknown.
func (m Model) CostInputPerM() float64 {
	if m.Rates != nil {
		return m.Rates.Input
	}
	return 0
}

// CostOutputPerM returns the output cost per million tokens, or 0 if unknown.
func (m Model) CostOutputPerM() float64 {
	if m.Rates != nil {
		return m.Rates.Output
	}
	return 0
}

// UpsertBatch atomically inserts or replaces provider and model records in a single transaction.
func (r *CatalogRepo) UpsertBatch(ctx context.Context, providers []ProviderRecord, models []ModelRecord) error {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)

	for _, p := range providers {
		enabled := 1
		if p.Enabled != nil && !*p.Enabled {
			enabled = 0
		}
		anthropicToolsDisabled := 0
		if p.AnthropicToolsDisabled {
			anthropicToolsDisabled = 1
		}

		_, err := tx.ExecContext(ctx, `
			INSERT OR REPLACE INTO providers (name, base_url, api_key, enabled, anthropic_tools_disabled, created_at)
			VALUES (?, ?, ?, ?, ?, COALESCE((SELECT created_at FROM providers WHERE name = ?), ?))
		`,
			p.Name, p.BaseURL, p.APIKey, enabled, anthropicToolsDisabled, p.Name, now)
		if err != nil {
			return err
		}
	}

	for _, m := range models {
		provider := providerFromModelKey(m.ID)
		modelName := modelNameFromKey(m.ID)

		supportsTools := 1
		if !m.ToolCall {
			supportsTools = 0
		}
		supportsVision := 0
		if m.Vision {
			supportsVision = 1
		}
		supportsReasoning := 0
		if m.Reasoning {
			supportsReasoning = 1
		}

		_, err := tx.ExecContext(ctx, `
			INSERT OR REPLACE INTO models (id, provider, name, display_name, context_window, cost_input_per_m, cost_output_per_m, supports_tools, supports_vision, supports_reasoning, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, COALESCE((SELECT created_at FROM models WHERE id = ?), ?))
		`,
			m.ID, provider, modelName, m.Name, m.ContextWindow, m.CostInput, m.CostOutput,
			supportsTools, supportsVision, supportsReasoning, m.ID, now)
		if err != nil {
			return err
		}
	}

	_, err = tx.ExecContext(ctx, `
		INSERT OR REPLACE INTO schema_info (key, value) VALUES ('catalog_last_sync', ?)
	`, now)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// Load reads all providers and models from the database and returns an indexed catalog.
func (r *CatalogRepo) Load(ctx context.Context) (*IndexedCatalog, error) {
	providers := make(map[string]Provider)
	models := make(map[string]Model)

	rows, err := r.db.DB().QueryContext(ctx, `
		SELECT name, base_url, api_key, enabled, anthropic_tools_disabled
		FROM providers
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var p Provider
		var enabled sql.NullBool
		var anthropicToolsDisabled int

		if err := rows.Scan(&p.Name, &p.BaseURL, &p.APIKey, &enabled, &anthropicToolsDisabled); err != nil {
			return nil, err
		}
		if enabled.Valid {
			p.Enabled = &enabled.Bool
		}
		p.AnthropicToolsDisabled = anthropicToolsDisabled == 1
		providers[p.Name] = p
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = r.db.DB().QueryContext(ctx, `
		SELECT id, provider, name, context_window, cost_input_per_m, cost_output_per_m,
		       supports_tools, supports_vision, supports_reasoning
		FROM models
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var m Model
		var provider string
		var displayName string
		var contextWindow sql.NullInt64
		var costInput, costOutput sql.NullFloat64
		var supportsTools, supportsVision, supportsReasoning int

		if err := rows.Scan(&m.ID, &provider, &displayName, &contextWindow, &costInput, &costOutput,
			&supportsTools, &supportsVision, &supportsReasoning); err != nil {
			return nil, err
		}
		m.Name = displayName
		m.ToolCall = supportsTools == 1
		m.Reasoning = supportsReasoning == 1
		m.Vision = supportsVision == 1

		if contextWindow.Valid {
			m.Limit = &Limit{Context: contextWindow.Int64}
		}
		if costInput.Valid || costOutput.Valid {
			m.Rates = &Rates{}
			if costInput.Valid {
				m.Rates.Input = costInput.Float64
			}
			if costOutput.Valid {
				m.Rates.Output = costOutput.Float64
			}
		}

		if m.Vision {
			m.Modalities.Input = []string{"text", "image"}
		} else {
			m.Modalities.Input = []string{"text"}
		}
		m.Modalities.Output = []string{"text"}

		models[m.ID] = m
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(providers) == 0 {
		return nil, errors.New("catalog providers map is empty")
	}
	if len(models) == 0 {
		return nil, errors.New("catalog models map is empty")
	}

	for key := range models {
		prov := providerFromModelKey(key)
		if prov == "" {
			return nil, errors.New("model key missing provider prefix")
		}
		if _, ok := providers[prov]; !ok {
			return nil, errors.New("model references unknown provider")
		}
	}

	cat := &Catalog{
		Providers: providers,
		Models:    models,
	}

	idx := &IndexedCatalog{
		Catalog:        *cat,
		ProviderModels: make(map[string][]Model, len(providers)),
	}

	for key, model := range models {
		prov := providerFromModelKey(key)
		if prov != "" {
			idx.ProviderModels[prov] = append(idx.ProviderModels[prov], model)
		}
	}

	return idx, nil
}

// GetModel retrieves a single model by its ID. Returns nil, nil if not found.
func (r *CatalogRepo) GetModel(ctx context.Context, id string) (*Model, error) {
	var m Model
	var displayName string
	var contextWindow sql.NullInt64
	var costInput, costOutput sql.NullFloat64
	var supportsTools, supportsVision, supportsReasoning int

	err := r.db.DB().QueryRowContext(ctx, `
		SELECT id, name, context_window, cost_input_per_m, cost_output_per_m,
		       supports_tools, supports_vision, supports_reasoning
		FROM models
		WHERE id = ?
	`, id).Scan(&m.ID, &displayName, &contextWindow, &costInput, &costOutput,
		&supportsTools, &supportsVision, &supportsReasoning)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	m.Name = displayName
	m.ToolCall = supportsTools == 1
	m.Reasoning = supportsReasoning == 1
	m.Vision = supportsVision == 1

	if contextWindow.Valid {
		m.Limit = &Limit{Context: contextWindow.Int64}
	}
	if costInput.Valid || costOutput.Valid {
		m.Rates = &Rates{}
		if costInput.Valid {
			m.Rates.Input = costInput.Float64
		}
		if costOutput.Valid {
			m.Rates.Output = costOutput.Float64
		}
	}

	if m.Vision {
		m.Modalities.Input = []string{"text", "image"}
	} else {
		m.Modalities.Input = []string{"text"}
	}
	m.Modalities.Output = []string{"text"}

	return &m, nil
}

// ListModelsByProvider returns all models that belong to the given provider.
func (r *CatalogRepo) ListModelsByProvider(ctx context.Context, provider string) ([]Model, error) {
	rows, err := r.db.DB().QueryContext(ctx, `
		SELECT id, name, context_window, cost_input_per_m, cost_output_per_m,
		       supports_tools, supports_vision, supports_reasoning
		FROM models
		WHERE provider = ?
	`, provider)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []Model
	for rows.Next() {
		var m Model
		var displayName string
		var contextWindow sql.NullInt64
		var costInput, costOutput sql.NullFloat64
		var supportsTools, supportsVision, supportsReasoning int

		if err := rows.Scan(&m.ID, &displayName, &contextWindow, &costInput, &costOutput,
			&supportsTools, &supportsVision, &supportsReasoning); err != nil {
			return nil, err
		}

		m.Name = displayName
		m.ToolCall = supportsTools == 1
		m.Reasoning = supportsReasoning == 1
		m.Vision = supportsVision == 1

		if contextWindow.Valid {
			m.Limit = &Limit{Context: contextWindow.Int64}
		}
		if costInput.Valid || costOutput.Valid {
			m.Rates = &Rates{}
			if costInput.Valid {
				m.Rates.Input = costInput.Float64
			}
			if costOutput.Valid {
				m.Rates.Output = costOutput.Float64
			}
		}

		if m.Vision {
			m.Modalities.Input = []string{"text", "image"}
		} else {
			m.Modalities.Input = []string{"text"}
		}
		m.Modalities.Output = []string{"text"}

		result = append(result, m)
	}

	return result, rows.Err()
}

// LastSync returns the timestamp of the last catalog sync, or zero time if never synced.
func (r *CatalogRepo) LastSync(ctx context.Context) (time.Time, error) {
	var syncedAt sql.NullString
	err := r.db.DB().QueryRowContext(ctx, `
		SELECT value FROM schema_info WHERE key = 'catalog_last_sync'
	`).Scan(&syncedAt)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}

	if !syncedAt.Valid {
		return time.Time{}, nil
	}

	return time.Parse(time.RFC3339, syncedAt.String)
}

// SetLastSync records the timestamp of a catalog sync.
func (r *CatalogRepo) SetLastSync(ctx context.Context, t time.Time) error {
	_, err := r.db.DB().ExecContext(ctx, `
		INSERT OR REPLACE INTO schema_info (key, value) VALUES ('catalog_last_sync', ?)
	`, t.Format(time.RFC3339))
	return err
}

func providerFromModelKey(key string) string {
	idx := strings.IndexByte(key, '/')
	if idx < 0 {
		return ""
	}
	return key[:idx]
}

func modelNameFromKey(key string) string {
	idx := strings.IndexByte(key, '/')
	if idx < 0 {
		return key
	}
	return key[idx+1:]
}

// ProviderModel is a flattened model entry used for display in provider listings.
type ProviderModel struct {
	ModelID     string
	DisplayName string
	ToolCall    bool
	Reasoning   bool
	Vision      bool
	Context     int64
	CostInput   float64
	CostOutput  float64
}

// ModelsForProvider returns the models that support the named provider.
func (ic *IndexedCatalog) ModelsForProvider(provider string) []Model {
	return ic.ProviderModels[provider]
}

// ListProviderModels returns a flattened ProviderModel slice for the given provider.
func (ic *IndexedCatalog) ListProviderModels(provider string) []ProviderModel {
	models := ic.ProviderModels[provider]
	if models == nil {
		return nil
	}
	result := make([]ProviderModel, len(models))
	for i, m := range models {
		result[i] = ProviderModel{
			ModelID:     ModelNameFromKey(m.ID),
			DisplayName: m.Name,
			ToolCall:    m.ToolCall,
			Reasoning:   m.Reasoning,
			Vision:      m.Vision,
		}
		if m.Limit != nil {
			result[i].Context = m.Limit.Context
		}
		if m.Rates != nil {
			result[i].CostInput = m.Rates.Input
			result[i].CostOutput = m.Rates.Output
		}
	}
	return result
}

// ModelNameFromKey extracts the model name portion from a model key of the form "provider/model-name".
func ModelNameFromKey(key string) string {
	idx := strings.IndexByte(key, '/')
	if idx < 0 {
		return key
	}
	return key[idx+1:]
}
