package product_model

import (
	"time"
)

type Product struct {
	ID        int64
	URL       string
	Store     string
	CreatedAt time.Time
}
