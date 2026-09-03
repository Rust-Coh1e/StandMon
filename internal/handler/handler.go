package handler

import (
	"PriceMon/internal/parser"
	postgres "PriceMon/internal/repository"
	"encoding/json"
	"log"
	"net/http"
)

type Registry interface {
	Resolve(productURL string) (parser.Parser, error)
}

type ParseHandler struct {
	registry Registry
	repo     *postgres.TaskRepository
}

func NewParseHandler(
	registry Registry,
	repo *postgres.TaskRepository,
) *ParseHandler {
	return &ParseHandler{
		registry: registry,
		repo:     repo,
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

type CreateProductRequest struct {
	URL                  string `json:"url"`
	CheckIntervalSeconds int64  `json:"check_interval_seconds"`
}

type CreateProductResponse struct {
	ID int64 `json:"id"`
}

func (h *ParseHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateProductRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		log.Printf("decode error: %v", err)
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	log.Printf("request: %+v", req)

	if req.URL == "" || req.CheckIntervalSeconds <= 0 {
		log.Printf("validation failed")
		http.Error(w, "invalid params", http.StatusBadRequest)
		return
	}

	_, err = h.registry.Resolve(req.URL)
	if err != nil {
		log.Printf("resolve error: %v", err)
		http.Error(w, "unsupported url", http.StatusBadRequest)
		return
	}

	id, err := h.repo.CreateProduct(
		r.Context(),
		req.URL,
		req.CheckIntervalSeconds,
	)
	if err != nil {
		log.Printf("CreateProduct error: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	_ = json.NewEncoder(w).Encode(CreateProductResponse{
		ID: id,
	})
}
