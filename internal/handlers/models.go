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
type openAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by,omitempty"`
	Name    string `json:"name,omitempty"`
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

	infos := h.modelRouter.ListModels()
	data := make([]openAIModel, 0, len(infos))
	for _, info := range infos {
		data = append(data, openAIModel{
			ID:      info.ID,
			Object:  "model",
			OwnedBy: info.Provider,
			Name:    info.DisplayName,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(openAIModelList{Object: "list", Data: data})
}
