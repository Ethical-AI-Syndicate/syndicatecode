package tools

import (
	"encoding/json"
	"log"
	"net/http"
)

type ListToolsResponse struct {
	Tools []ToolDefinition `json:"tools"`
}

type ExecuteRequest = ToolCall

type ExecuteResponse struct {
	Result *ToolResult `json:"result"`
	Error  string      `json:"error,omitempty"`
}

func (r *Registry) HandleList(w http.ResponseWriter, req *http.Request) {
	tools := r.List()
	response := ListToolsResponse{
		Tools: tools,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

func (e *Executor) HandleExecute(w http.ResponseWriter, req *http.Request) {
	var executeReq ExecuteRequest
	if err := json.NewDecoder(req.Body).Decode(&executeReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	result, err := e.Execute(req.Context(), executeReq)
	if err != nil {
		response := ExecuteResponse{
			Error: err.Error(),
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("failed to encode error response: %v", err)
		}
		return
	}

	response := ExecuteResponse{
		Result: result,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}
