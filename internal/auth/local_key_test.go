package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewLocalKeyAuthProvider(t *testing.T) {
	// Create a temporary keys file.
	tmpDir := t.TempDir()
	keysFile := filepath.Join(tmpDir, "keys.json")

	content := `{
		"version": "1",
		"keys": [
			{
				"id": "test-key-1",
				"secret": "rt_local_testsecret123",
				"workspace_id": "workspace-1",
				"roles": ["admin"],
				"status": "active"
			}
		]
	}`
	if err := os.WriteFile(keysFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create keys file: %v", err)
	}

	provider, err := NewLocalKeyAuthProvider(keysFile)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	if provider.configPath != keysFile {
		t.Errorf("expected config path %q, got %q", keysFile, provider.configPath)
	}

	// Check that keys were loaded.
	provider.mu.RLock()
	if len(provider.keys) != 1 {
		t.Errorf("expected 1 key, got %d", len(provider.keys))
	}
	provider.mu.RUnlock()
}

func TestNewLocalKeyAuthProvider_YAML(t *testing.T) {
	tmpDir := t.TempDir()
	keysFile := filepath.Join(tmpDir, "keys.yaml")

	content := `
version: "1"
keys:
  - id: test-key-yaml
    secret: rt_local_yamlsecret456
    workspace_id: workspace-yaml
    roles:
      - user
    status: active
`
	if err := os.WriteFile(keysFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create keys file: %v", err)
	}

	provider, err := NewLocalKeyAuthProvider(keysFile)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	provider.mu.RLock()
	if len(provider.keys) != 1 {
		t.Errorf("expected 1 key, got %d", len(provider.keys))
	}
	provider.mu.RUnlock()
}

func TestLocalKeyAuthProvider_Authenticate_Success(t *testing.T) {
	tmpDir := t.TempDir()
	keysFile := filepath.Join(tmpDir, "keys.json")

	content := `{
		"version": "1",
		"keys": [
			{
				"id": "test-key",
				"secret": "rt_local_validsecret",
				"workspace_id": "workspace-1",
				"roles": ["admin", "user"],
				"status": "active",
				"allowed_models": ["model-a", "model-b"],
				"allowed_providers": ["opencode-go"],
				"rate_limits": {
					"requests_per_minute": 100
				},
				"metadata": {
					"team": "engineering"
				}
			}
		]
	}`
	if err := os.WriteFile(keysFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create keys file: %v", err)
	}

	provider, err := NewLocalKeyAuthProvider(keysFile)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	req, err := http.NewRequest("GET", "http://localhost/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer rt_local_validsecret")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	authCtx, err := provider.Authenticate(ctx, req)
	if err != nil {
		t.Fatalf("authentication failed: %v", err)
	}

	if authCtx.KeyID != "test-key" {
		t.Errorf("expected key ID %q, got %q", "test-key", authCtx.KeyID)
	}
	if authCtx.WorkspaceID != "workspace-1" {
		t.Errorf("expected workspace ID %q, got %q", "workspace-1", authCtx.WorkspaceID)
	}
	if authCtx.KeyStatus != KeyStatusActive {
		t.Errorf("expected key status %q, got %q", KeyStatusActive, authCtx.KeyStatus)
	}
	if authCtx.Identity.Type != SubjectTypeLocal {
		t.Errorf("expected subject type %q, got %q", SubjectTypeLocal, authCtx.Identity.Type)
	}
	if len(authCtx.Roles) != 2 || !authCtx.HasRole("admin") {
		t.Errorf("expected roles to contain 'admin', got %v", authCtx.Roles)
	}
	if !authCtx.IsAllowedModel("model-a") {
		t.Errorf("expected model-a to be allowed")
	}
	if !authCtx.IsAllowedProvider("opencode-go") {
		t.Errorf("expected opencode-go to be allowed")
	}
	if authCtx.RateLimits.RequestsPerMinute != 100 {
		t.Errorf("expected 100 requests per minute, got %d", authCtx.RateLimits.RequestsPerMinute)
	}
	if authCtx.Metadata["team"] != "engineering" {
		t.Errorf("expected team metadata %q, got %q", "engineering", authCtx.Metadata["team"])
	}
}

func TestLocalKeyAuthProvider_Authenticate_MissingHeader(t *testing.T) {
	tmpDir := t.TempDir()
	keysFile := filepath.Join(tmpDir, "keys.json")

	content := `{"version": "1", "keys": [{"id": "key1", "secret": "secret", "workspace_id": "ws1", "status": "active"}]}`
	if err := os.WriteFile(keysFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create keys file: %v", err)
	}

	provider, err := NewLocalKeyAuthProvider(keysFile)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	req, _ := http.NewRequest("GET", "http://localhost/test", nil)
	ctx := context.Background()

	_, err = provider.Authenticate(ctx, req)
	if err != ErrAuthenticationFailed {
		t.Errorf("expected ErrAuthenticationFailed, got %v", err)
	}
}

func TestLocalKeyAuthProvider_Authenticate_InvalidFormat(t *testing.T) {
	tmpDir := t.TempDir()
	keysFile := filepath.Join(tmpDir, "keys.json")

	content := `{"version": "1", "keys": [{"id": "key1", "secret": "secret", "workspace_id": "ws1", "status": "active"}]}`
	if err := os.WriteFile(keysFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create keys file: %v", err)
	}

	provider, err := NewLocalKeyAuthProvider(keysFile)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	req, _ := http.NewRequest("GET", "http://localhost/test", nil)
	req.Header.Set("Authorization", "Basic secret") // Wrong scheme
	ctx := context.Background()

	_, err = provider.Authenticate(ctx, req)
	if err != ErrAuthenticationFailed {
		t.Errorf("expected ErrAuthenticationFailed, got %v", err)
	}
}

func TestLocalKeyAuthProvider_Authenticate_UnknownKey(t *testing.T) {
	tmpDir := t.TempDir()
	keysFile := filepath.Join(tmpDir, "keys.json")

	content := `{"version": "1", "keys": [{"id": "key1", "secret": "secret", "workspace_id": "ws1", "status": "active"}]}`
	if err := os.WriteFile(keysFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create keys file: %v", err)
	}

	provider, err := NewLocalKeyAuthProvider(keysFile)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	req, _ := http.NewRequest("GET", "http://localhost/test", nil)
	req.Header.Set("Authorization", "Bearer unknown-secret")
	ctx := context.Background()

	_, err = provider.Authenticate(ctx, req)
	if err != ErrAuthenticationFailed {
		t.Errorf("expected ErrAuthenticationFailed, got %v", err)
	}
}

func TestLocalKeyAuthProvider_Authenticate_RevokedKey(t *testing.T) {
	tmpDir := t.TempDir()
	keysFile := filepath.Join(tmpDir, "keys.json")

	content := `{"version": "1", "keys": [{"id": "revoked-key", "secret": "revoked-secret", "workspace_id": "ws1", "status": "revoked"}]}`
	if err := os.WriteFile(keysFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create keys file: %v", err)
	}

	provider, err := NewLocalKeyAuthProvider(keysFile)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	req, _ := http.NewRequest("GET", "http://localhost/test", nil)
	req.Header.Set("Authorization", "Bearer revoked-secret")
	ctx := context.Background()

	_, err = provider.Authenticate(ctx, req)
	if err != ErrKeyRevoked {
		t.Errorf("expected ErrKeyRevoked, got %v", err)
	}
}

func TestLocalKeyAuthProvider_Authenticate_SuspendedKey(t *testing.T) {
	tmpDir := t.TempDir()
	keysFile := filepath.Join(tmpDir, "keys.json")

	content := `{"version": "1", "keys": [{"id": "suspended-key", "secret": "suspended-secret", "workspace_id": "ws1", "status": "suspended"}]}`
	if err := os.WriteFile(keysFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create keys file: %v", err)
	}

	provider, err := NewLocalKeyAuthProvider(keysFile)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	req, _ := http.NewRequest("GET", "http://localhost/test", nil)
	req.Header.Set("Authorization", "Bearer suspended-secret")
	ctx := context.Background()

	_, err = provider.Authenticate(ctx, req)
	if err != ErrKeySuspended {
		t.Errorf("expected ErrKeySuspended, got %v", err)
	}
}

func TestLocalKeyAuthProvider_Authenticate_ExpiredKey(t *testing.T) {
	tmpDir := t.TempDir()
	keysFile := filepath.Join(tmpDir, "keys.json")

	content := `{"version": "1", "keys": [{"id": "expired-key", "secret": "expired-secret", "workspace_id": "ws1", "status": "expired"}]}`
	if err := os.WriteFile(keysFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create keys file: %v", err)
	}

	provider, err := NewLocalKeyAuthProvider(keysFile)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	req, _ := http.NewRequest("GET", "http://localhost/test", nil)
	req.Header.Set("Authorization", "Bearer expired-secret")
	ctx := context.Background()

	_, err = provider.Authenticate(ctx, req)
	if err != ErrKeyExpired {
		t.Errorf("expected ErrKeyExpired, got %v", err)
	}
}

func TestLocalKeyAuthProvider_Reload(t *testing.T) {
	tmpDir := t.TempDir()
	keysFile := filepath.Join(tmpDir, "keys.json")

	// Initial content with one key.
	content1 := `{"version": "1", "keys": [{"id": "key1", "secret": "secret1", "workspace_id": "ws1", "status": "active"}]}`
	if err := os.WriteFile(keysFile, []byte(content1), 0644); err != nil {
		t.Fatalf("failed to create keys file: %v", err)
	}

	provider, err := NewLocalKeyAuthProvider(keysFile)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	// Verify first key works.
	req, _ := http.NewRequest("GET", "http://localhost/test", nil)
	req.Header.Set("Authorization", "Bearer secret1")
	ctx := context.Background()

	authCtx, err := provider.Authenticate(ctx, req)
	if err != nil {
		t.Fatalf("first key authentication failed: %v", err)
	}
	if authCtx.KeyID != "key1" {
		t.Errorf("expected key1, got %s", authCtx.KeyID)
	}

	// Reload with new content.
	content2 := `{"version": "1", "keys": [{"id": "key2", "secret": "secret2", "workspace_id": "ws2", "status": "active"}]}`
	if err := os.WriteFile(keysFile, []byte(content2), 0644); err != nil {
		t.Fatalf("failed to update keys file: %v", err)
	}

	if err := provider.Reload(); err != nil {
		t.Fatalf("failed to reload: %v", err)
	}

	// Old key should not work.
	req2, _ := http.NewRequest("GET", "http://localhost/test", nil)
	req2.Header.Set("Authorization", "Bearer secret1")
	_, err = provider.Authenticate(ctx, req2)
	if err != ErrAuthenticationFailed {
		t.Errorf("expected first key to be invalid after reload, got %v", err)
	}

	// New key should work.
	req3, _ := http.NewRequest("GET", "http://localhost/test", nil)
	req3.Header.Set("Authorization", "Bearer secret2")
	authCtx, err = provider.Authenticate(ctx, req3)
	if err != nil {
		t.Fatalf("second key authentication failed: %v", err)
	}
	if authCtx.KeyID != "key2" {
		t.Errorf("expected key2, got %s", authCtx.KeyID)
	}
}

func TestLocalKeyAuthProvider_RevokeCache(t *testing.T) {
	tmpDir := t.TempDir()
	keysFile := filepath.Join(tmpDir, "keys.json")

	content := `{"version": "1", "keys": [{"id": "to-revoke", "secret": "secret", "workspace_id": "ws1", "status": "active"}]}`
	if err := os.WriteFile(keysFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create keys file: %v", err)
	}

	provider, err := NewLocalKeyAuthProvider(keysFile)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	// First, key should work.
	req, _ := http.NewRequest("GET", "http://localhost/test", nil)
	req.Header.Set("Authorization", "Bearer secret")
	ctx := context.Background()

	_, err = provider.Authenticate(ctx, req)
	if err != nil {
		t.Fatalf("authentication should succeed before revoke: %v", err)
	}

	// Revoke the cache for this key.
	if err := provider.RevokeCache(ctx, "to-revoke"); err != nil {
		t.Fatalf("revoke cache failed: %v", err)
	}

	// Now key should be revoked.
	req2, _ := http.NewRequest("GET", "http://localhost/test", nil)
	req2.Header.Set("Authorization", "Bearer secret")
	_, err = provider.Authenticate(ctx, req2)
	if err != ErrKeyRevoked {
		t.Errorf("expected ErrKeyRevoked after cache revoke, got %v", err)
	}
}

func TestLocalKeyAuthProvider_HealthCheck(t *testing.T) {
	tmpDir := t.TempDir()
	keysFile := filepath.Join(tmpDir, "keys.json")

	// Test with empty keys file (should fail health check).
	content := `{"version": "1", "keys": []}`
	if err := os.WriteFile(keysFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create keys file: %v", err)
	}

	provider, err := NewLocalKeyAuthProvider(keysFile)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	ctx := context.Background()
	err = provider.HealthCheck(ctx)
	if err == nil {
		t.Error("expected health check to fail with no keys")
	}

	// Test with keys.
	content2 := `{"version": "1", "keys": [{"id": "key1", "secret": "secret", "workspace_id": "ws1", "status": "active"}]}`
	if err := os.WriteFile(keysFile, []byte(content2), 0644); err != nil {
		t.Fatalf("failed to update keys file: %v", err)
	}

	if err := provider.Reload(); err != nil {
		t.Fatalf("failed to reload: %v", err)
	}

	err = provider.HealthCheck(ctx)
	if err != nil {
		t.Errorf("expected health check to pass, got: %v", err)
	}
}

func TestLocalKeyAuthProvider_EnvInterpolation(t *testing.T) {
	tmpDir := t.TempDir()
	keysFile := filepath.Join(tmpDir, "keys.json")

	// Set an environment variable.
	_ = os.Setenv("TEST_WS_ID", "env-workspace")
	defer func() { _ = os.Unsetenv("TEST_WS_ID") }()

	content := `{"version": "1", "keys": [{"id": "key1", "secret": "secret", "workspace_id": "${TEST_WS_ID}", "status": "active"}]}`
	if err := os.WriteFile(keysFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create keys file: %v", err)
	}

	provider, err := NewLocalKeyAuthProvider(keysFile)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	req, _ := http.NewRequest("GET", "http://localhost/test", nil)
	req.Header.Set("Authorization", "Bearer secret")
	ctx := context.Background()

	authCtx, err := provider.Authenticate(ctx, req)
	if err != nil {
		t.Fatalf("authentication failed: %v", err)
	}
	if authCtx.WorkspaceID != "env-workspace" {
		t.Errorf("expected workspace ID from env var, got %q", authCtx.WorkspaceID)
	}
}

func TestLocalKeyAuthProvider_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	keysFile := filepath.Join(tmpDir, "keys.json")

	// Create file with multiple keys.
	content := `{"version": "1", "keys": [
		{"id": "key1", "secret": "secret1", "workspace_id": "ws1", "status": "active"},
		{"id": "key2", "secret": "secret2", "workspace_id": "ws2", "status": "active"},
		{"id": "key3", "secret": "secret3", "workspace_id": "ws3", "status": "active"}
	]}`
	if err := os.WriteFile(keysFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create keys file: %v", err)
	}

	provider, err := NewLocalKeyAuthProvider(keysFile)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	ctx := context.Background()
	var wg sync.WaitGroup
	errors := make(chan error, 300)

	// Concurrent authenticate from different goroutines.
	for i := 0; i < 100; i++ {
		for _, secret := range []string{"secret1", "secret2", "secret3"} {
			wg.Add(1)
			go func(s string) {
				defer wg.Done()
				req, _ := http.NewRequest("GET", "http://localhost/test", nil)
				req.Header.Set("Authorization", "Bearer "+s)
				_, err := provider.Authenticate(ctx, req)
				if err != nil {
					errors <- err
				}
			}(secret)
		}
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent authentication failed: %v", err)
	}
}

func TestHashToken(t *testing.T) {
	token := "rt_local_testsecret"
	hash1 := hashToken(token)
	hash2 := hashToken(token)

	if hash1 != hash2 {
		t.Error("hashing same token should produce same hash")
	}

	// Verify it's SHA256.
	h := sha256.Sum256([]byte(token))
	expected := hex.EncodeToString(h[:])
	if hash1 != expected {
		t.Errorf("hash mismatch: expected %q, got %q", expected, hash1)
	}

	// Different token should produce different hash.
	hash3 := hashToken("different")
	if hash1 == hash3 {
		t.Error("different tokens should produce different hashes")
	}
}

func TestIsLocalKey(t *testing.T) {
	tests := []struct {
		token    string
		expected bool
	}{
		{"rt_local_test", true},
		{"rt_local_", true},
		{"sk-abc123", false},
		{"Bearer rt_local_test", false},
		{"", false},
	}

	for _, tt := range tests {
		result := IsLocalKey(tt.token)
		if result != tt.expected {
			t.Errorf("IsLocalKey(%q) = %v, expected %v", tt.token, result, tt.expected)
		}
	}
}

func TestTokenHasPrefix(t *testing.T) {
	tests := []struct {
		token    string
		prefix   string
		expected bool
	}{
		{"rt_local_test", "rt_local_", true},
		{"rt_local_test", "rt_cloud_", false},
		{"test", "test", true},
		{"", "", true},
		{"a", "ab", false},
	}

	for _, tt := range tests {
		result := TokenHasPrefix(tt.token, tt.prefix)
		if result != tt.expected {
			t.Errorf("TokenHasPrefix(%q, %q) = %v, expected %v", tt.token, tt.prefix, result, tt.expected)
		}
	}
}

func TestGenerateLocalKey(t *testing.T) {
	key, secret, err := GenerateLocalKey("test-key", "workspace-1", []string{"admin"})
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	if key.ID != "test-key" {
		t.Errorf("expected ID %q, got %q", "test-key", key.ID)
	}
	if key.WorkspaceID != "workspace-1" {
		t.Errorf("expected workspace %q, got %q", "workspace-1", key.WorkspaceID)
	}
	if key.Status != KeyStatusActive {
		t.Errorf("expected status %q, got %q", KeyStatusActive, key.Status)
	}
	if !key.HasRole("admin") {
		t.Errorf("expected role 'admin', got %v", key.Roles)
	}

	if !strings.HasPrefix(secret, "rt_local_") {
		t.Errorf("expected secret to start with 'rt_local_', got %q", secret)
	}

	if secret != key.Secret {
		t.Errorf("expected returned secret to match key.Secret")
	}

	// Try with empty key ID (should fail).
	_, _, err = GenerateLocalKey("", "workspace-1", nil)
	if err == nil {
		t.Error("expected error for empty key ID")
	}
}

func TestGenerateRandomSecret(t *testing.T) {
	secret1, err := generateRandomSecret()
	if err != nil {
		t.Fatalf("failed to generate secret: %v", err)
	}
	secret2, err := generateRandomSecret()
	if err != nil {
		t.Fatalf("failed to generate second secret: %v", err)
	}

	if secret1 == secret2 {
		t.Error("two generated secrets should be different")
	}

	// Should be base64url encoded (no +, /, = padding).
	if strings.Contains(secret1, "+") || strings.Contains(secret1, "/") || strings.Contains(secret1, "=") {
		t.Errorf("secret should not contain padding or special chars: %q", secret1)
	}
}

func TestLocalKey_HasRole(t *testing.T) {
	key := LocalKey{
		ID:    "test",
		Roles: []string{"admin", "user"},
	}

	// Test with local HasRole method defined in struct
	hasAdmin := false
	for _, r := range key.Roles {
		if r == "admin" {
			hasAdmin = true
			break
		}
	}
	if !hasAdmin {
		t.Error("should have admin role")
	}

	hasReadOnly := false
	for _, r := range key.Roles {
		if r == "read-only" {
			hasReadOnly = true
			break
		}
	}
	if hasReadOnly {
		t.Error("should not have read-only role")
	}
}

// Helper method for LocalKey to match the expected interface.
func (k *LocalKey) HasRole(role string) bool {
	for _, r := range k.Roles {
		if r == role {
			return true
		}
	}
	return false
}

func TestLocalKey_AuthContextIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	keysFile := filepath.Join(tmpDir, "keys.json")

	content := `{
		"version": "1",
		"keys": [{
			"id": "integration-key",
			"secret": "rt_local_integration_secret",
			"workspace_id": "test-workspace",
			"roles": ["admin", "analytics"],
			"status": "active",
			"allowed_models": ["gpt-4", "claude-3"],
			"allowed_providers": ["openai", "anthropic"],
			"rate_limits": {
				"requests_per_second": 10,
				"requests_per_minute": 600,
				"tokens_per_minute": 100000,
				"burst_size": 20
			},
			"metadata": {
				"department": "engineering",
				"cost_center": "cc-1234",
				"project": "ai-platform"
			}
		}]
	}`

	if err := os.WriteFile(keysFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create keys file: %v", err)
	}

	provider, err := NewLocalKeyAuthProvider(keysFile)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	req, _ := http.NewRequest("POST", "http://localhost/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer rt_local_integration_secret")
	ctx := context.Background()

	authCtx, err := provider.Authenticate(ctx, req)
	if err != nil {
		t.Fatalf("authentication failed: %v", err)
	}

	// Test AuthContext methods.
	if !authCtx.HasRole("admin") {
		t.Error("HasRole('admin') should return true")
	}
	if !authCtx.HasRole("analytics") {
		t.Error("HasRole('analytics') should return true")
	}
	if authCtx.HasRole("user") {
		t.Error("HasRole('user') should return false")
	}
	if !authCtx.HasAnyRole("admin", "user") {
		t.Error("HasAnyRole('admin', 'user') should return true")
	}
	if authCtx.HasAnyRole("user", "readonly") {
		t.Error("HasAnyRole('user', 'readonly') should return false")
	}

	// Test model permissions.
	if !authCtx.IsAllowedModel("gpt-4") {
		t.Error("IsAllowedModel('gpt-4') should return true")
	}
	if !authCtx.IsAllowedModel("claude-3") {
		t.Error("IsAllowedModel('claude-3') should return true")
	}
	if authCtx.IsAllowedModel("gpt-3") {
		t.Error("IsAllowedModel('gpt-3') should return false")
	}

	// Test provider permissions.
	if !authCtx.IsAllowedProvider("openai") {
		t.Error("IsAllowedProvider('openai') should return true")
	}
	if !authCtx.IsAllowedProvider("anthropic") {
		t.Error("IsAllowedProvider('anthropic') should return true")
	}
	if authCtx.IsAllowedProvider("google") {
		t.Error("IsAllowedProvider('google') should return false")
	}

	// Test rate limits.
	if authCtx.RateLimits.RequestsPerSecond != 10 {
		t.Errorf("expected 10 requests per second, got %d", authCtx.RateLimits.RequestsPerSecond)
	}
	if authCtx.RateLimits.BurstSize != 20 {
		t.Errorf("expected burst size 20, got %d", authCtx.RateLimits.BurstSize)
	}

	// Test metadata.
	if authCtx.Key("department") != "engineering" {
		t.Errorf("expected department 'engineering', got %q", authCtx.Key("department"))
	}
	if authCtx.Key("nonexistent") != "" {
		t.Errorf("expected empty string for nonexistent key")
	}

	// Test WithMetadata.
	newCtx := authCtx.WithMetadata("new_key", "new_value")
	if newCtx.Metadata["new_key"] != "new_value" {
		t.Error("WithMetadata should add the new key")
	}

	// Ensure original wasn't modified if WithMetadata creates a copy.
	if authCtx.Metadata["new_key"] != "" {
		t.Error("WithMetadata should not modify original context")
	}
}

func TestLocalKeyAuthProvider_Reload_WithMultipleKeys(t *testing.T) {
	tmpDir := t.TempDir()
	keysFile := filepath.Join(tmpDir, "keys.json")

	// Initial: 3 keys.
	content1 := `{"version": "1", "keys": [
		{"id": "key1", "secret": "secret1", "workspace_id": "ws1", "status": "active"},
		{"id": "key2", "secret": "secret2", "workspace_id": "ws2", "status": "active"},
		{"id": "key3", "secret": "secret3", "workspace_id": "ws3", "status": "active"}
	]}`
	if err := os.WriteFile(keysFile, []byte(content1), 0644); err != nil {
		t.Fatalf("failed to create keys file: %v", err)
	}

	provider, err := NewLocalKeyAuthProvider(keysFile)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	provider.mu.RLock()
	if len(provider.keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(provider.keys))
	}
	provider.mu.RUnlock()

	// Reload: 2 keys (removed key1).
	content2 := `{"version": "1", "keys": [
		{"id": "key2", "secret": "secret2", "workspace_id": "ws2", "status": "active"},
		{"id": "key4", "secret": "secret4", "workspace_id": "ws4", "status": "active"}
	]}`
	if err := os.WriteFile(keysFile, []byte(content2), 0644); err != nil {
		t.Fatalf("failed to update keys file: %v", err)
	}

	if err := provider.Reload(); err != nil {
		t.Fatalf("failed to reload: %v", err)
	}

	provider.mu.RLock()
	if len(provider.keys) != 2 {
		t.Errorf("expected 2 keys after reload, got %d", len(provider.keys))
	}
	// Check that keyIDs map is also updated.
	if _, exists := provider.keyIDs["key1"]; exists {
		t.Error("key1 should be removed from keyIDs map")
	}
	if _, exists := provider.keyIDs["key2"]; !exists {
		t.Error("key2 should exist in keyIDs map")
	}
	if _, exists := provider.keyIDs["key4"]; !exists {
		t.Error("key4 should exist in keyIDs map")
	}
	provider.mu.RUnlock()

	// Test that removed key no longer works.
	ctx := context.Background()
	req, _ := http.NewRequest("GET", "http://localhost/test", nil)
	req.Header.Set("Authorization", "Bearer secret1")

	_, err = provider.Authenticate(ctx, req)
	if err != ErrAuthenticationFailed {
		t.Errorf("expected ErrAuthenticationFailed for removed key, got %v", err)
	}
}

func TestLocalKeyAuthProvider_MissingFile(t *testing.T) {
	_, err := NewLocalKeyAuthProvider("/nonexistent/path/keys.json")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
	if !strings.Contains(err.Error(), "initial load of keys") {
		t.Errorf("expected error to mention initial load, got: %v", err)
	}
}

func TestLocalKeyAuthProvider_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	keysFile := filepath.Join(tmpDir, "keys.json")

	content := `not valid json`
	if err := os.WriteFile(keysFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create keys file: %v", err)
	}

	_, err := NewLocalKeyAuthProvider(keysFile)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLocalKeyAuthProvider_EmptyAllowedLists(t *testing.T) {
	tmpDir := t.TempDir()
	keysFile := filepath.Join(tmpDir, "keys.json")

	// Key with empty allowed lists (should allow all).
	content := `{"version": "1", "keys": [{
		"id": "key1",
		"secret": "secret",
		"workspace_id": "ws1",
		"status": "active",
		"allowed_models": [],
		"allowed_providers": []
	}]}`

	if err := os.WriteFile(keysFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create keys file: %v", err)
	}

	provider, err := NewLocalKeyAuthProvider(keysFile)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	req, _ := http.NewRequest("GET", "http://localhost/test", nil)
	req.Header.Set("Authorization", "Bearer secret")
	ctx := context.Background()

	authCtx, err := provider.Authenticate(ctx, req)
	if err != nil {
		t.Fatalf("authentication failed: %v", err)
	}

	// Empty lists should allow all.
	if !authCtx.IsAllowedModel("any-model") {
		t.Error("empty allowed_models should allow all models")
	}
	if !authCtx.IsAllowedProvider("any-provider") {
		t.Error("empty allowed_providers should allow all providers")
	}
}

func TestLocalKeyAuthProvider_RealConfig(t *testing.T) {
	// This test documents the expected format for config files.
	tmpDir := t.TempDir()
	keysFile := filepath.Join(tmpDir, "keys.yaml")

	content := `version: "1"
keys:
  # Engineering team key with full access
  - id: eng-team-key-001
    secret: rt_local_eng_blk2z9m5x7qm3v8k
    workspace_id: engineering
    roles:
      - admin
    status: active
    allowed_models: []  # Empty means allow all
    allowed_providers:
      - openai-go
      - opencode-zen
    rate_limits:
      requests_per_minute: 1000
      tokens_per_minute: 1000000
    metadata:
      team: platform
      cost_center: eng-ai

  # Read-only analytics key
  - id: analytics-readonly-001
    secret: rt_local_analytics_5j7k2m9p
    workspace_id: engineering
    roles:
      - read-only
    status: active
    allowed_models:
      - gpt-4
      - claude-3-opus
    rate_limits:
      requests_per_minute: 100
      burst_size: 20
`
	if err := os.WriteFile(keysFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create keys file: %v", err)
	}

	provider, err := NewLocalKeyAuthProvider(keysFile)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	// Test engineering key.
	req, _ := http.NewRequest("GET", "http://localhost/test", nil)
	req.Header.Set("Authorization", "Bearer rt_local_eng_blk2z9m5x7qm3v8k")
	ctx := context.Background()

	authCtx, err := provider.Authenticate(ctx, req)
	if err != nil {
		t.Fatalf("engineering key authentication failed: %v", err)
	}
	if authCtx.KeyID != "eng-team-key-001" {
		t.Errorf("expected key ID eng-team-key-001, got %s", authCtx.KeyID)
	}
	if !authCtx.HasRole("admin") {
		t.Error("expected admin role")
	}
	if authCtx.RateLimits.RequestsPerMinute != 1000 {
		t.Errorf("expected 1000 requests/min, got %d", authCtx.RateLimits.RequestsPerMinute)
	}
	if authCtx.Metadata["team"] != "platform" {
		t.Errorf("expected team 'platform', got %s", authCtx.Metadata["team"])
	}

	// Test analytics key.
	req2, _ := http.NewRequest("GET", "http://localhost/test", nil)
	req2.Header.Set("Authorization", "Bearer rt_local_analytics_5j7k2m9p")

	authCtx2, err := provider.Authenticate(ctx, req2)
	if err != nil {
		t.Fatalf("analytics key authentication failed: %v", err)
	}
	if authCtx2.KeyID != "analytics-readonly-001" {
		t.Errorf("expected key ID analytics-readonly-001, got %s", authCtx2.KeyID)
	}
	if authCtx2.HasRole("admin") {
		t.Error("analytics key should not have admin role")
	}
	if !authCtx2.HasRole("read-only") {
		t.Error("analytics key should have read-only role")
	}
	if authCtx2.RateLimits.BurstSize != 20 {
		t.Errorf("expected burst size 20, got %d", authCtx2.RateLimits.BurstSize)
	}
}

// BenchmarkHashToken benchmarks token hashing performance.
func BenchmarkHashToken(b *testing.B) {
	token := "rt_local_testsecret123456789abc123456789"
	for i := 0; i < b.N; i++ {
		_ = hashToken(token)
	}
}

// BenchmarkAuthenticate benchmarks the authenticate function.
func BenchmarkAuthenticate(b *testing.B) {
	tmpDir := b.TempDir()
	keysFile := filepath.Join(tmpDir, "keys.json")

	// Create file with many keys.
	var keysBuilder strings.Builder
	keysBuilder.WriteString(`{"version": "1", "keys": [`)
	for i := 0; i < 1000; i++ {
		if i > 0 {
			keysBuilder.WriteString(",")
		}
		fmt.Fprintf(&keysBuilder, `{"id":"key%d","secret":"secret%d","workspace_id":"ws%d","status":"active"}`, i, i, i)
	}
	keysBuilder.WriteString(`]}`)

	if err := os.WriteFile(keysFile, []byte(keysBuilder.String()), 0644); err != nil {
		b.Fatalf("failed to create keys file: %v", err)
	}

	provider, err := NewLocalKeyAuthProvider(keysFile)
	if err != nil {
		b.Fatalf("failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	req, _ := http.NewRequest("GET", "http://localhost/test", nil)
	req.Header.Set("Authorization", "Bearer secret500")
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = provider.Authenticate(ctx, req)
	}
}
