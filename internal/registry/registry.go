package registry

import (
	"PriceMon/internal/parser"
	"context"
	"fmt"
	"net/url"
	"strings"
)

type Parser interface {
	Parse(ctx context.Context, url string) (parser.ProductInfo, error)
}

type ParserRegistry struct {
	parsers map[string]Parser
}

func NewParserRegistry() *ParserRegistry {
	return &ParserRegistry{
		parsers: map[string]Parser{
			"ozon.ru":        &parser.OzonParser{},
			"baltopttorg.ru": &parser.BaltOptTorg{},
			"regard.ru":      &parser.RegardParser{},
		},
	}
}

func (p *ParserRegistry) Resolve(productURL string) (parser.Parser, error) {
	parsedURL, err := url.Parse(productURL)
	if err != nil {
		return nil, err
	}

	host := parsedURL.Hostname()

	mainDomain := strings.TrimPrefix(host, "www.")

	res, exist := p.parsers[mainDomain]
	if !exist {
		return nil, fmt.Errorf("Отсутствует поддерживаемый парсер")
	}
	return res, nil
}
