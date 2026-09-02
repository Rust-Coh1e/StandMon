package product_service

import (
	"PriceMon/internal/parser"
	"context"
	"fmt"
	"time"
)

// Product — товар, который наша система мониторит.
type Product struct {
	ID        int64
	URL       string
	Store     string
	CreatedAt time.Time
}

// PriceSnapshot — цена конкретного Product в конкретный момент времени.
type PriceSnapshot struct {
	ID        int64
	ProductID int64
	Price     int
	CheckedAt time.Time
}

func NewProduct(url string, store string) Product {
	return Product{
		URL:       url,
		Store:     store,
		CreatedAt: time.Now(),
	}
}

// Что Service хочет от хранилища.
type ProductRepository interface {
	Create(ctx context.Context, product Product) (Product, error)
	CreatePriceSnapshot(ctx context.Context, snapshot PriceSnapshot) error
}

// Что Service хочет от Registry.
type Registry interface {
	Resolve(productURL string) (parser.Parser, error)
}

type Service struct {
	registry Registry
	repo     ProductRepository
}

func NewService(reg Registry, repo ProductRepository) *Service {
	return &Service{
		registry: reg,
		repo:     repo,
	}
}

func (s *Service) Create(ctx context.Context, productURL string) (Product, error) {

	// 1. Определяем парсер.
	p, err := s.registry.Resolve(productURL)
	if err != nil {
		return Product{}, fmt.Errorf("resolve parser: %w", err)
	}

	// 2. Получаем текущую цену.
	info, err := p.Parse(ctx, productURL)
	if err != nil {
		return Product{}, fmt.Errorf("parse product: %w", err)
	}

	// 3. Создаём нашу доменную сущность Product.
	product := NewProduct(productURL, info.Store)

	// 4. Сохраняем Product.
	savedProduct, err := s.repo.Create(ctx, product)
	if err != nil {
		return Product{}, fmt.Errorf("create product: %w", err)
	}

	// 5. Создаём первый снимок цены.
	snapshot := PriceSnapshot{
		ProductID: savedProduct.ID,
		Price:     info.Price,
		CheckedAt: info.CheckedAt,
	}

	// 6. Сохраняем снимок.
	err = s.repo.CreatePriceSnapshot(ctx, snapshot)
	if err != nil {
		return Product{}, fmt.Errorf("create price snapshot: %w", err)
	}

	return savedProduct, nil
}
