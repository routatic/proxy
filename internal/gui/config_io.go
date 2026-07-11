package gui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/routatic/proxy/internal/config"
)

var sensitiveFieldPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)api[_-]?key`),
	regexp.MustCompile(`(?i)token`),
	regexp.MustCompile(`(?i)secret`),
	regexp.MustCompile(`(?i)password`),
	regexp.MustCompile(`(?i)credential`),
}

func anonymizeConfig(cfg *config.Config) *config.Config {
	data, err := json.Marshal(cfg)
	if err != nil {
		return cfg
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return cfg
	}

	anonymizeMap(raw)

	result := &config.Config{}
	data, _ = json.Marshal(raw)
	_ = json.Unmarshal(data, result)
	return result
}

func anonymizeMap(m map[string]interface{}) {
	for key, value := range m {
		if shouldAnonymize(key) {
			switch v := value.(type) {
			case string:
				if v != "" {
					m[key] = "***REDACTED***"
				}
			case []interface{}:
				m[key] = []string{"***REDACTED***"}
			}
		} else if nested, ok := value.(map[string]interface{}); ok {
			anonymizeMap(nested)
		}
	}
}

func shouldAnonymize(key string) bool {
	lowerKey := strings.ToLower(key)
	for _, pattern := range sensitiveFieldPatterns {
		if pattern.MatchString(lowerKey) {
			return true
		}
	}
	return false
}

func (s *Server) handleConfigExport(w http.ResponseWriter, r *http.Request) {
	if s.atomicCfg == nil {
		http.Error(w, "config not available", http.StatusServiceUnavailable)
		return
	}

	cfg := s.atomicCfg.Get()

	anonymize := r.URL.Query().Get("anonymize") == "true"
	if anonymize {
		cfg = anonymizeConfig(cfg)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=routatic-proxy-config.json")

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(cfg)
}

func (s *Server) handleConfigImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.atomicCfg == nil {
		http.Error(w, "config not available", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Config json.RawMessage `json:"config"`
		Apply  bool            `json:"apply"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	var cfg config.Config
	if err := json.Unmarshal(req.Config, &cfg); err != nil {
		http.Error(w, fmt.Sprintf("invalid config: %v", err), http.StatusBadRequest)
		return
	}

	if cfg.Host == "" {
		http.Error(w, "host is required", http.StatusBadRequest)
		return
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		http.Error(w, "port must be between 1 and 65535", http.StatusBadRequest)
		return
	}

	resp := map[string]interface{}{
		"valid":  true,
		"config": cfg,
	}

	if req.Apply {
		configPath := s.atomicCfg.Path()
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to marshal config: %v", err), http.StatusInternalServerError)
			return
		}
		if err := writeFileAtomic(configPath, data, 0600); err != nil {
			http.Error(w, fmt.Sprintf("failed to write config: %v", err), http.StatusInternalServerError)
			return
		}

		if err := s.atomicCfg.Reload(); err != nil {
			http.Error(w, fmt.Sprintf("failed to reload config: %v", err), http.StatusInternalServerError)
			return
		}

		resp["applied"] = true
	}

	writeJSON(w, resp)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, perm); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
