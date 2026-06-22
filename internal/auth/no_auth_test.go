package auth

import (
	"context"
	"net/http"
	"testing"
)

func TestNoAuthProvider_New(t *testing.T) {
	provider := NewNoAuthProvider("test-workspace")
	if provider == nil {
		t.Fatal("NewNoAuthProvider returned nil")
	}
	if provider.workspaceID != "test-workspace" {
		t.Errorf("expected workspaceID 'test-workspace', got '%s'", provider.workspaceID)
	}
}

func TestNoAuthProvider_New_EmptyWorkspaceID(t *testing.T) {
	provider := NewNoAuthProvider("")
	if provider.workspaceID != "local" {
		t.Errorf("expected default workspaceID 'local', got '%s'", provider.workspaceID)
	}
}

func TestNoAuthProvider_Authenticate(t *testing.T) {
	provider := NewNoAuthProvider("test-workspace")
	ctx := context.Background()

	// Test with nil request (allowed for testing)
	authCtx, err := provider.Authenticate(ctx, nil)
	if err != nil {
		t.Fatalf("Authenticate with nil request failed: %v", err)
	}
	if authCtx == nil {
		t.Fatal("Authenticate returned nil AuthContext")
	}

	// Verify AuthContext values
	if authCtx.Identity.Type != SubjectTypeLocal {
		t.Errorf("expected Identity.Type 'local', got '%s'", authCtx.Identity.Type)
	}
	if authCtx.Identity.ID != "test-workspace" {
		t.Errorf("expected Identity.ID 'test-workspace', got '%s'", authCtx.Identity.ID)
	}
	if authCtx.WorkspaceID != "test-workspace" {
		t.Errorf("expected WorkspaceID 'test-workspace', got '%s'", authCtx.WorkspaceID)
	}
	if authCtx.KeyID != "dev" {
		t.Errorf("expected KeyID 'dev', got '%s'", authCtx.KeyID)
	}
	if authCtx.KeyStatus != KeyStatusActive {
		t.Errorf("expected KeyStatus 'active', got '%s'", authCtx.KeyStatus)
	}
	if !authCtx.HasRole("admin") {
		t.Error("expected Roles to contain 'admin'")
	}
}

func TestNoAuthProvider_Authenticate_Localhost(t *testing.T) {
	provider := NewNoAuthProvider("test-workspace")
	ctx := context.Background()

	// Test with localhost request
	req, _ := http.NewRequest("GET", "http://localhost:8080/v1/chat/completions", nil)
	req.Host = "localhost:8080"
	req.RemoteAddr = "127.0.0.1:12345"

	authCtx, err := provider.Authenticate(ctx, req)
	if err != nil {
		t.Fatalf("Authenticate with localhost request failed: %v", err)
	}
	if authCtx == nil {
		t.Fatal("Authenticate returned nil AuthContext")
	}
}

func TestNoAuthProvider_Authenticate_NonLocalhost(t *testing.T) {
	provider := NewNoAuthProvider("test-workspace")
	ctx := context.Background()

	// Test with non-localhost request
	req, _ := http.NewRequest("GET", "http://example.com/v1/chat/completions", nil)
	req.Host = "example.com"
	req.RemoteAddr = "192.168.1.100:12345"

	_, err := provider.Authenticate(ctx, req)
	if err == nil {
		t.Error("Authenticate should reject non-localhost requests")
	}
}

func TestNoAuthProvider_RevokeCache(t *testing.T) {
	provider := NewNoAuthProvider("test-workspace")
	ctx := context.Background()

	err := provider.RevokeCache(ctx, "any-key")
	if err != nil {
		t.Errorf("RevokeCache should always return nil, got: %v", err)
	}
}

func TestNoAuthProvider_HealthCheck(t *testing.T) {
	provider := NewNoAuthProvider("test-workspace")
	ctx := context.Background()

	err := provider.HealthCheck(ctx)
	if err != nil {
		t.Errorf("HealthCheck should always return nil, got: %v", err)
	}
}

func TestNoAuthProvider_AllowsAllModels(t *testing.T) {
	provider := NewNoAuthProvider("test-workspace")
	ctx := context.Background()

	authCtx, _ := provider.Authenticate(ctx, nil)

	if !authCtx.IsAllowedModel("any-model") {
		t.Error("IsAllowedModel should return true for any model when AllowedModels is empty")
	}
	if !authCtx.IsAllowedModel("gpt-4") {
		t.Error("IsAllowedModel should return true for gpt-4")
	}
}

func TestNoAuthProvider_AllowsAllProviders(t *testing.T) {
	provider := NewNoAuthProvider("test-workspace")
	ctx := context.Background()

	authCtx, _ := provider.Authenticate(ctx, nil)

	if !authCtx.IsAllowedProvider("any-provider") {
		t.Error("IsAllowedProvider should return true for any provider when AllowedProviders is empty")
	}
	if !authCtx.IsAllowedProvider("openai") {
		t.Error("IsAllowedProvider should return true for openai")
	}
}

func TestNoAuthProvider_CopyIsolation(t *testing.T) {
	provider := NewNoAuthProvider("test-workspace")
	ctx := context.Background()

	authCtx1, _ := provider.Authenticate(ctx, nil)
	authCtx2, _ := provider.Authenticate(ctx, nil)

	// Modify first context
	authCtx1.Roles = append(authCtx1.Roles, "new-role")

	// Second context should not be affected
	if authCtx2.HasRole("new-role") {
		t.Error("AuthContext copies should be independent - modification leaked")
	}
}
