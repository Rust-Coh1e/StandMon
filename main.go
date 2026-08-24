package main

import (
	"PriceMon/internal/registry"
	"context"
	"fmt"
	"log"
	"time"
)

func main() {
	reg := registry.NewParserRegistry()

	// productURL := "https://www.baltopttorg.ru/goods/37357"
	productURL := "https://www.regard.ru/product/7005/nakopitel-ssd-480gb-kingston-a400-sa400s37-480g"

	p, err := reg.Resolve(productURL)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()

	result, err := p.Parse(ctx, productURL)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf(
		"Магазин: %s\nЦена: %d коп.\nПроверено: %s\n",
		result.Store,
		result.Price,
		result.CheckedAt.Format(time.RFC3339),
	)
}
