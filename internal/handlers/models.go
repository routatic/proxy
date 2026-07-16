package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/routatic/proxy/internal/router"
)

// ModelsHandler serves the OpenAI-compatible model listing endpoint.
//
// Tools that manage Claude Code providers — notably CC-Switch's "Fetch Models"
// button — call GET /v1/models to populate a model picker. The proxy answers
// with every model identifier a client may put in the request "model" field:
// config aliases, model_overrides keys, and catalog canonical names.
type ModelsHandler struct {
	modelRouter *router.ModelRouter
}

// NewModelsHandler creates a new models listing handler.
func NewModelsHandler(modelRouter *router.ModelRouter) *ModelsHandler {
	return &ModelsHandler{modelRouter: modelRouter}
}

// openAIModel mirrors an entry in the OpenAI /v1/models response. Fields beyond
// "id" are informational; clients key off "id".
//
// display_name is included for Claude Code's gateway model discovery, which
// reads that field when it queries GET /v1/models?limit=1000. Note that Claude
// Code only surfaces discovered models whose id begins with "claude" or
// "anthropic"; other ids are silently filtered from its picker.
type openAIModel struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	OwnedBy     string `json:"owned_by,omitempty"`
	Name        string `json:"name,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

// openAIModelList is the OpenAI /v1/models envelope: {"object":"list","data":[...]}.
type openAIModelList struct {
	Object string        `json:"object"`
	Data   []openAIModel `json:"data"`
}

// HandleListModels handles GET /v1/models.
func (h *ModelsHandler) HandleListModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	infos := h.modelRouter.ListModels(r.Context())
	data := make([]openAIModel, 0, len(infos))
	for _, info := range infos {
		data = append(data, openAIModel{
			ID:          info.ID,
			Object:      "model",
			OwnedBy:     info.Provider,
			Name:        info.DisplayName,
			DisplayName: info.DisplayName,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(openAIModelList{Object: "list", Data: data})
}
