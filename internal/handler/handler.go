package handler

import (
	"PriceMon/internal/registry"
	"encoding/json"
	"net/http"
)

type Registry interface {
	Resolve(productURL string) (registry.Parser, error)
}

type ParseHandler struct {
	registry Registry
}

func NewParseHandler(registry Registry) *ParseHandler {
	return &ParseHandler{
		registry: registry,
	}
}

type parseRequest struct {
	URL string `json:"url"`
}

func (h *ParseHandler) Parse(w http.ResponseWriter, r *http.Request) {
	var req parseRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	p, err := h.registry.Resolve(req.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	result, err := p.Parse(r.Context(), req.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	err = json.NewEncoder(w).Encode(result)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

	}
}
