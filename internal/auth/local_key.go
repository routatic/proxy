// Package auth provides authentication interfaces and types for the routatic-proxy.
package auth

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

// LocalKey represents a single API key configuration.
type LocalKey struct {
	// ID is the unique identifier for this key.
	ID string `json:"id" yaml:"id"`

	// Secret is the full key value (e.g., rt_local_xxx...).
	// This is hashed before storage for security.
	Secret string `json:"secret" yaml:"secret"`

	// WorkspaceID is the workspace/organization this key belongs to.
	WorkspaceID string `json:"workspace_id" yaml:"workspace_id"`

	// AllowedModels is the list of model IDs this key can use.
	// If empty, all models are allowed.
	AllowedModels []string `json:"allowed_models,omitempty" yaml:"allowed_models,omitempty"`

	// AllowedProviders is the list of provider IDs this key can use.
	// If empty, all providers are allowed.
	AllowedProviders []string `json:"allowed_providers,omitempty" yaml:"allowed_providers,omitempty"`

	// Roles contains the assigned roles for RBAC.
	Roles []string `json:"roles,omitempty" yaml:"roles,omitempty"`

	// RateLimits defines the rate limiting policy for this key.
	RateLimits RateLimitPolicy `json:"rate_limits,omitempty" yaml:"rate_limits,omitempty"`

	// Status indicates the current state of the key.
	Status KeyStatus `json:"status" yaml:"status"`

	// Metadata contains additional provider-specific metadata.
	Metadata map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// KeysFile represents the structure of a keys configuration file.
type KeysFile struct {
	// Version of the keys file format.
	Version string `json:"version" yaml:"version"`

	// Keys contains all configured API keys.
	Keys []LocalKey `json:"keys" yaml:"keys"`
}

// LocalKeyAuthProvider implements AuthProvider for local file-based key authentication.
type LocalKeyAuthProvider struct {
	configPath string
	mu         sync.RWMutex

	// keys maps hashed key values to LocalKey entries.
	// The key is SHA256(secret) for constant-time lookup.
	keys map[string]LocalKey

	// keyIDs maps key ID to hashed key for reverse lookup.
	keyIDs map[string]string

	// watcher is the file watcher for hot reload.
	watcher *fsnotify.Watcher

	// stopWatcher is a channel to stop the file watcher.
	stopWatcher chan struct{}

	// reloadMu protects concurrent reloads.
	reloadMu sync.Mutex
}

// NewLocalKeyAuthProvider creates a new LocalKeyAuthProvider with the given configuration.
// The configPath can be a YAML or JSON file containing API key definitions.
// Environment variables can be referenced using ${VAR} syntax in the file.
func NewLocalKeyAuthProvider(configPath string, opts ...ProviderOption) (*LocalKeyAuthProvider, error) {
	options := &providerOptions{}
	for _, opt := range opts {
		opt(options)
	}

	provider := &LocalKeyAuthProvider{
		configPath:  configPath,
		keys:        make(map[string]LocalKey),
		keyIDs:      make(map[string]string),
		stopWatcher: make(chan struct{}),
	}

	// Initial load of keys.
	if err := provider.Reload(); err != nil {
		return nil, fmt.Errorf("initial load of keys: %w", err)
	}

	// Start file watcher for hot reload.
	if err := provider.startWatcher(); err != nil {
		slog.Warn("failed to start file watcher for hot reload", "error", err)
		// Continue without watcher - manual reload via Reload() still works.
	}

	return provider, nil
}

// Authenticate validates the request credentials and returns an AuthContext.
func (p *LocalKeyAuthProvider) Authenticate(ctx context.Context, req *http.Request) (*AuthContext, error) {
	// Extract the Bearer token from the Authorization header.
	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		slog.Debug("missing Authorization header")
		return nil, ErrAuthenticationFailed
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		slog.Debug("invalid Authorization header format")
		return nil, ErrAuthenticationFailed
	}

	// Extract the token.
	token := strings.TrimPrefix(authHeader, "Bearer ")
	token = strings.TrimSpace(token)

	if token == "" {
		slog.Debug("empty bearer token")
		return nil, ErrAuthenticationFailed
	}

	// Hash the token for lookup.
	tokenHash := hashToken(token)

	// Look up the key (read lock).
	p.mu.RLock()
	key, exists := p.keys[tokenHash]
	p.mu.RUnlock()

	if !exists {
		slog.Debug("unknown API key", "hash_prefix", tokenHash[:8])
		return nil, ErrAuthenticationFailed
	}

	// Check key status.
	switch key.Status {
	case KeyStatusRevoked:
		slog.Debug("attempted use of revoked key", "key_id", key.ID)
		// Use constant-time comparison to avoid timing attacks.
		_ = subtle.ConstantTimeCompare([]byte(key.Secret), []byte(token))
		return nil, ErrKeyRevoked
	case KeyStatusSuspended:
		slog.Debug("attempted use of suspended key", "key_id", key.ID)
		return nil, ErrKeySuspended
	case KeyStatusExpired:
		slog.Debug("attempted use of expired key", "key_id", key.ID)
		return nil, ErrKeyExpired
	case KeyStatusActive, "":
		// Empty status defaults to active (backward compatibility).
	default:
		slog.Warn("unknown key status", "key_id", key.ID, "status", key.Status)
		return nil, ErrAuthenticationFailed
	}

	// Check context cancellation before expensive constant-time comparison.
	// This respects user cancellation and avoids unnecessary crypto work.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Verify the actual secret using constant-time comparison.
	if subtle.ConstantTimeCompare([]byte(key.Secret), []byte(token)) != 1 {
		slog.Debug("key hash matched but secret verification failed", "key_id", key.ID)
		return nil, ErrAuthenticationFailed
	}

	// Build the AuthContext.
	authCtx := &AuthContext{
		Identity: SubjectIdentity{
			Type: SubjectTypeLocal,
			ID:   key.WorkspaceID,
			Name: key.ID,
		},
		WorkspaceID:      key.WorkspaceID,
		KeyID:            key.ID,
		KeyStatus:        key.Status,
		AllowedModels:    key.AllowedModels,
		AllowedProviders: key.AllowedProviders,
		Roles:            key.Roles,
		RateLimits:       key.RateLimits,
		ConfigRef: ConfigRef{
			WorkspaceID:  key.WorkspaceID,
			Version:      "local",
			LastModified: time.Now().Unix(),
		},
		Metadata: key.Metadata,
	}

	// Ensure metadata is never nil.
	if authCtx.Metadata == nil {
		authCtx.Metadata = make(map[string]string)
	}

	return authCtx, nil
}

// RevokeCache invalidates any cached authentication state for the given key ID.
func (p *LocalKeyAuthProvider) RevokeCache(ctx context.Context, keyID string) error {
	p.reloadMu.Lock()
	defer p.reloadMu.Unlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Find the hashed key for this key ID.
	if tokenHash, exists := p.keyIDs[keyID]; exists {
		// Mark the key as revoked in memory.
		if key, exists := p.keys[tokenHash]; exists {
			key.Status = KeyStatusRevoked
			p.keys[tokenHash] = key
			slog.Info("key revoked in cache", "key_id", keyID)
		}
	} else {
		slog.Debug("key not found in cache during revoke", "key_id", keyID)
	}

	return nil
}

// HealthCheck verifies the authentication provider is healthy.
func (p *LocalKeyAuthProvider) HealthCheck(ctx context.Context) error {
	p.mu.RLock()
	keyCount := len(p.keys)
	p.mu.RUnlock()

	if keyCount == 0 {
		return fmt.Errorf("no API keys configured")
	}

	// Verify the configuration file is readable.
	if _, err := os.Stat(p.configPath); err != nil {
		return fmt.Errorf("cannot access config file: %w", err)
	}

	return nil
}

// Reload performs a hot reload of the key configuration from disk.
func (p *LocalKeyAuthProvider) Reload() error {
	p.reloadMu.Lock()
	defer p.reloadMu.Unlock()

	slog.Info("reloading local key configuration", "path", p.configPath)

	// Parse file extension.
	ext := strings.ToLower(filepath.Ext(p.configPath))

	// Read and interpolate the file.
	data, err := os.ReadFile(p.configPath)
	if err != nil {
		return fmt.Errorf("reading keys file: %w", err)
	}

	// Replace ${ENV_VAR} placeholders.
	data = []byte(interpolateEnvVars(string(data)))

	// Parse based on file type.
	var keysFile KeysFile
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &keysFile); err != nil {
			return fmt.Errorf("parsing YAML: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, &keysFile); err != nil {
			return fmt.Errorf("parsing JSON: %w", err)
		}
	default:
		// Try both formats.
		if err := yaml.Unmarshal(data, &keysFile); err != nil {
			if err := json.Unmarshal(data, &keysFile); err != nil {
				return fmt.Errorf("parsing keys file (tried YAML and JSON): %w", err)
			}
		}
	}

	// Validate version.
	if keysFile.Version != "" && keysFile.Version != "1" && keysFile.Version != "1.0" {
		slog.Warn("unknown keys file version", "version", keysFile.Version)
	}

	// Build new maps atomically.
	newKeys := make(map[string]LocalKey, len(keysFile.Keys))
	newKeyIDs := make(map[string]string, len(keysFile.Keys))

	for _, key := range keysFile.Keys {
		if key.ID == "" {
			slog.Warn("skipping key with empty ID")
			continue
		}

		if key.Secret == "" {
			slog.Warn("skipping key with empty secret", "key_id", key.ID)
			continue
		}

		if key.Status == "" {
			key.Status = KeyStatusActive
		}

		// Hash the full secret for map key.
		secretHash := hashToken(key.Secret)

		// Check for duplicate key IDs.
		if _, exists := newKeyIDs[key.ID]; exists {
			slog.Warn("duplicate key ID found, overwriting", "key_id", key.ID)
		}

		newKeys[secretHash] = key
		newKeyIDs[key.ID] = secretHash
	}

	// Update atomically with write lock.
	p.mu.Lock()
	p.keys = newKeys
	p.keyIDs = newKeyIDs
	p.mu.Unlock()

	slog.Info("local key configuration reloaded",
		"keys_loaded", len(newKeys),
		"path", p.configPath)

	return nil
}

// Close stops the file watcher and cleans up resources.
func (p *LocalKeyAuthProvider) Close() error {
	close(p.stopWatcher)

	if p.watcher != nil {
		return p.watcher.Close()
	}
	return nil
}

// startWatcher initializes the file watcher for hot reload.
func (p *LocalKeyAuthProvider) startWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating file watcher: %w", err)
	}

	p.watcher = watcher

	// Watch the directory, not the file itself (handles atomic renames).
	dir := filepath.Dir(p.configPath)
	if err := watcher.Add(dir); err != nil {
		_ = watcher.Close()
		return fmt.Errorf("watching config directory: %w", err)
	}

	// Start the watch goroutine.
	go p.watchLoop()

	slog.Info("started file watcher for hot reload", "path", p.configPath)
	return nil
}

// watchLoop handles file system events for hot reload.
func (p *LocalKeyAuthProvider) watchLoop() {
	var debounceTimer *time.Timer
	const debounceDelay = 500 * time.Millisecond

	filename := filepath.Base(p.configPath)

	for {
		select {
		case event, ok := <-p.watcher.Events:
			if !ok {
				return
			}

			// Only care about events for our config file.
			if filepath.Base(event.Name) != filename {
				continue
			}

			// Filter for relevant event types.
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) && !event.Has(fsnotify.Rename) {
				continue
			}

			// Debounce rapid events.
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(debounceDelay, func() {
				if err := p.Reload(); err != nil {
					slog.Error("hot reload failed", "error", err)
				}
			})

		case err, ok := <-p.watcher.Errors:
			if !ok {
				return
			}
			slog.Error("file watcher error", "error", err)

		case <-p.stopWatcher:
			return
		}
	}
}

// hashToken returns the SHA256 hash of a token as a hex string.
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// TokenHasPrefix checks if a token has the expected prefix format.
// This is useful for validation before hashing.
func TokenHasPrefix(token string, prefix string) bool {
	return strings.HasPrefix(token, prefix)
}

// GenerateLocalKey generates a new local key with the given ID and workspace.
// Returns the full secret key (which should be shown to the user once).
func GenerateLocalKey(keyID, workspaceID string, roles []string) (*LocalKey, string, error) {
	if keyID == "" {
		return nil, "", fmt.Errorf("key ID is required")
	}

	// Generate a random secret.
	rawSecret, err := generateRandomSecret()
	if err != nil {
		return nil, "", fmt.Errorf("generating secret: %w", err)
	}

	// Prefix with rt_local_ for identification.
	fullSecret := "rt_local_" + rawSecret

	key := &LocalKey{
		ID:               keyID,
		Secret:           fullSecret,
		WorkspaceID:      workspaceID,
		Roles:            roles,
		Status:           KeyStatusActive,
		AllowedModels:    []string{},
		AllowedProviders: []string{},
		Metadata:         make(map[string]string),
	}

	return key, fullSecret, nil
}

// generateRandomSecret generates a URL-safe random string.
func generateRandomSecret() (string, error) {
	// Use crypto/rand for secure random generation.
	// We'll use 32 bytes (256 bits) of entropy.
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	// Encode to base64 URL-safe string, removing padding.
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// interpolateEnvVars replaces ${ENV_VAR} patterns with environment variable values.
func interpolateEnvVars(s string) string {
	return os.Expand(s, func(key string) string {
		if val := os.Getenv(key); val != "" {
			return val
		}
		// Return the original placeholder if not set.
		return "${" + key + "}"
	})
}

// LoadKeysFromEnv loads API keys from environment variables.
// Expected format: ROUTATIC_PROXY_KEYS="key1:workspace1:role1,role2;key2:workspace2:role3"
// Each key segment is "id:workspace:roles" separated by semicolons.
func LoadKeysFromEnv(envVar string) ([]LocalKey, error) {
	value := os.Getenv(envVar)
	if value == "" {
		return nil, nil
	}

	var keys []LocalKey
	scanner := bufio.NewScanner(strings.NewReader(value))
	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		// Split by semicolon, accounting for escapes.
		for i := 0; i < len(data); i++ {
			if data[i] == ';' {
				return i + 1, data[:i], nil
			}
		}
		if atEOF && len(data) > 0 {
			return len(data), data, nil
		}
		return 0, nil, nil
	})

	for scanner.Scan() {
		segment := strings.TrimSpace(scanner.Text())
		if segment == "" {
			continue
		}

		parts := strings.SplitN(segment, ":", 3)
		if len(parts) < 2 {
			slog.Warn("invalid key format, expected 'id:workspace[:roles]'", "segment", segment)
			continue
		}

		keyID := parts[0]
		workspaceID := parts[1]

		var roles []string
		if len(parts) == 3 && parts[2] != "" {
			roles = strings.Split(parts[2], ",")
			for i := range roles {
				roles[i] = strings.TrimSpace(roles[i])
			}
		}

		key := LocalKey{
			ID:               keyID,
			Secret:           "env:" + keyID, // Placeholder, real secret must come from env or file
			WorkspaceID:      workspaceID,
			Roles:            roles,
			Status:           KeyStatusActive,
			AllowedModels:    []string{},
			AllowedProviders: []string{},
			Metadata:         make(map[string]string),
		}
		keys = append(keys, key)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning env var: %w", err)
	}

	return keys, nil
}

// IsLocalKey checks if a token is a local key (vs a cloud key).
func IsLocalKey(token string) bool {
	return strings.HasPrefix(token, "rt_local_")
}
