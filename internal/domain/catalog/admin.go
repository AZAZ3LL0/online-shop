package catalog

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/money"
)

// Bounds of a price change, counted in runes for the reason (tech.md §16.1).
const (
	MinReasonRunes = 3
	MaxReasonRunes = 200
	// MaxPriceCents caps a single t-shirt at a million dollars: a typo in the
	// price field must not become a five-figure invoice.
	MaxPriceCents = 100_000_000
	// MaxStock is the ceiling of one size, well above any real print run.
	MaxStock = 100_000
	// PriceHistoryLimit is how much of the price trail one product page shows.
	PriceHistoryLimit = 10
)

// Errors of the admin catalogue edits, recognised by the handler with errors.Is.
var (
	ErrReasonRequired     = errors.New("catalog: a price change needs a reason")
	ErrPriceOutOfRange    = errors.New("catalog: price out of range")
	ErrStockOutOfRange    = errors.New("catalog: stock out of range")
	ErrStockBelowReserved = errors.New("catalog: stock below the reserved units")
)

// PriceChange is one operator edit of a price, always with a reason on the
// record (tech.md §8.3).
type PriceChange struct {
	ProductID uuid.UUID
	NewPrice  money.Amount
	Reason    string
	ChangedBy uuid.UUID
}

// PriceEntry is one line of the price trail of a product.
type PriceEntry struct {
	OldPrice  money.Amount
	NewPrice  money.Amount
	Reason    string
	ChangedBy string // the administrator login, empty when the row predates them
	CreatedAt time.Time
}

// Delta is how much the price moved.
func (e PriceEntry) Delta() money.Amount {
	d, err := e.NewPrice.Sub(e.OldPrice)
	if err != nil {
		return money.Zero(e.NewPrice.Currency)
	}
	return d
}

// AdminRepository is the storage the admin catalogue needs. Reads for the panel
// include the deactivated models the storefront never shows.
type AdminRepository interface {
	// ListAll returns every product with its variants, in sort order.
	ListAll(ctx context.Context) ([]Product, error)
	// ChangePrice writes the new price and its price_history row in one
	// transaction: a price never moves without the trail behind it.
	ChangePrice(ctx context.Context, change PriceChange) error
	// PriceHistory returns the most recent price changes of one product.
	PriceHistory(ctx context.Context, productID uuid.UUID, limit int) ([]PriceEntry, error)
	// SetStock replaces the stock of one size. It fails with
	// ErrStockBelowReserved when reserved units would be left uncovered.
	SetStock(ctx context.Context, variantID uuid.UUID, stock int) error
	// ProductOfVariant reports which product a size belongs to.
	ProductOfVariant(ctx context.Context, variantID uuid.UUID) (uuid.UUID, error)
}

// AdminService is the catalogue side of the admin panel: it validates an edit
// before storage ever sees it.
type AdminService struct {
	repo     AdminRepository
	currency string
}

// NewAdminService wires the admin catalogue service.
func NewAdminService(repo AdminRepository, currency string) *AdminService {
	return &AdminService{repo: repo, currency: currency}
}

// Products returns every model with its sizes.
func (s *AdminService) Products(ctx context.Context) ([]Product, error) {
	return s.repo.ListAll(ctx)
}

// PriceHistory returns the price trail shown next to a model.
func (s *AdminService) PriceHistory(ctx context.Context, productID uuid.UUID) ([]PriceEntry, error) {
	return s.repo.PriceHistory(ctx, productID, PriceHistoryLimit)
}

// ChangePrice validates the edit and records it. The reason is mandatory: a
// price change without one tells nobody later why the number moved.
func (s *AdminService) ChangePrice(ctx context.Context, change PriceChange) error {
	change.Reason = strings.TrimSpace(change.Reason)
	if n := utf8.RuneCountInString(change.Reason); n < MinReasonRunes || n > MaxReasonRunes {
		return fmt.Errorf("reason of %d characters: %w", n, ErrReasonRequired)
	}
	if change.NewPrice.Cents <= 0 || change.NewPrice.Cents > MaxPriceCents {
		return fmt.Errorf("price %d: %w", change.NewPrice.Cents, ErrPriceOutOfRange)
	}
	if change.NewPrice.Currency == "" {
		change.NewPrice.Currency = s.currency
	}
	if err := s.repo.ChangePrice(ctx, change); err != nil {
		return fmt.Errorf("change price: %w", err)
	}
	return nil
}

// SetStock validates and replaces the stock of one size.
func (s *AdminService) SetStock(ctx context.Context, variantID uuid.UUID, stock int) error {
	if stock < 0 || stock > MaxStock {
		return fmt.Errorf("stock %d: %w", stock, ErrStockOutOfRange)
	}
	if err := s.repo.SetStock(ctx, variantID, stock); err != nil {
		return fmt.Errorf("set stock: %w", err)
	}
	return nil
}

// ProductOfVariant reports which product a size belongs to, so a stock edit can
// return to the page it came from.
func (s *AdminService) ProductOfVariant(ctx context.Context, variantID uuid.UUID) (uuid.UUID, error) {
	return s.repo.ProductOfVariant(ctx, variantID)
}

// ParsePrice reads a price typed as a decimal ("35", "35.5", "35.50") into minor
// units. The digits are counted, never divided: a price must not pass through a
// float on its way into the database (tech.md §6.1).
func ParsePrice(input, currency string) (money.Amount, error) {
	text := strings.TrimSpace(strings.ReplaceAll(input, ",", "."))
	text = strings.TrimPrefix(text, "$")
	if text == "" {
		return money.Amount{}, fmt.Errorf("empty price: %w", ErrPriceOutOfRange)
	}

	whole, frac, hasFrac := strings.Cut(text, ".")
	if hasFrac {
		switch len(frac) {
		case 1:
			frac += "0"
		case 2:
		default:
			return money.Amount{}, fmt.Errorf("price %q: %w", input, ErrPriceOutOfRange)
		}
	} else {
		frac = "00"
	}

	units, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || units < 0 {
		return money.Amount{}, fmt.Errorf("price %q: %w", input, ErrPriceOutOfRange)
	}
	cents, err := strconv.ParseInt(frac, 10, 64)
	if err != nil || cents < 0 {
		return money.Amount{}, fmt.Errorf("price %q: %w", input, ErrPriceOutOfRange)
	}
	if units > MaxPriceCents/100 {
		return money.Amount{}, fmt.Errorf("price %q: %w", input, ErrPriceOutOfRange)
	}
	return money.New(units*100+cents, currency), nil
}
