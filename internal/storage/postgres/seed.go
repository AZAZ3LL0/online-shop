package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/domain/analytics"
	"github.com/qzq-kiim/shop/internal/domain/order"
	"github.com/qzq-kiim/shop/internal/storage/postgres/sqlcgen"
)

// sizes is the size run every model carries, tech.md §15.
var sizes = []string{"S", "M", "L", "XL", "XXL"}

type seedProduct struct {
	slug        string
	title       string
	description string
	priceCents  int64
	imageFront  string
	imageBack   string
	stock       []int32
}

// seedProducts is the fixed demo collection: three models, tech.md §1.
var seedProducts = []seedProduct{
	{
		slug:        "qzq-black",
		title:       "QZQ Black",
		description: "Heavyweight cotton tee, black, oversized fit. Screen print front and back.",
		priceCents:  3500,
		imageFront:  "/static/img/qzqblackfront-removebg-preview.png",
		imageBack:   "/static/img/qzqblackback-removebg-preview.png",
		stock:       []int32{6, 12, 14, 8, 3},
	},
	{
		slug:        "qzq-white",
		title:       "QZQ White",
		description: "Heavyweight cotton tee, off-white, oversized fit. Screen print front and back.",
		priceCents:  3500,
		imageFront:  "/static/img/qzqwhitefront-removebg-preview.png",
		imageBack:   "/static/img/qzqwhiteback-removebg-preview.png",
		stock:       []int32{4, 10, 11, 7, 0},
	},
	{
		slug:        "besh",
		title:       "Besh",
		description: "Limited drop, mid-weight cotton, regular fit. Full back print.",
		priceCents:  3900,
		imageFront:  "/static/img/beshfront-removebg-preview.png",
		imageBack:   "/static/img/beshback-removebg-preview.png",
		stock:       []int32{9, 9, 5, 2, 1},
	},
}

var seedSources = []struct{ source, medium, campaign string }{
	{"instagram", "social", "drop-1"},
	{"telegram", "social", "drop-1"},
	{"google", "cpc", "brand"},
	{"", "", ""},
}

// Seeder fills a fresh database with the demo collection and 30 days of traffic
// so the admin charts are never developed against an empty database.
type Seeder struct {
	q *sqlcgen.Queries
}

// NewSeeder returns the seeder bound to the store.
func NewSeeder(s *Store) *Seeder { return &Seeder{q: s.q} }

// Run wipes shop data and rebuilds the demo dataset deterministically.
func (s *Seeder) Run(ctx context.Context, now time.Time) error {
	if err := s.q.PurgeDemoData(ctx); err != nil {
		return fmt.Errorf("purge demo data: %w", err)
	}
	if err := s.seedCatalog(ctx, now); err != nil {
		return err
	}
	variants, err := s.q.ListVariantIDs(ctx)
	if err != nil {
		return fmt.Errorf("list variants: %w", err)
	}
	return s.seedTraffic(ctx, now, variants)
}

func (s *Seeder) seedCatalog(ctx context.Context, now time.Time) error {
	for i, p := range seedProducts {
		productID, err := s.q.InsertProduct(ctx, sqlcgen.InsertProductParams{
			Slug:        p.slug,
			Title:       p.title,
			Description: p.description,
			PriceCents:  p.priceCents,
			Currency:    "USD",
			ImageFront:  p.imageFront,
			ImageBack:   p.imageBack,
			SortOrder:   int32of(i),
		})
		if err != nil {
			return fmt.Errorf("insert product %s: %w", p.slug, err)
		}
		for j, size := range sizes {
			err := s.q.InsertVariant(ctx, sqlcgen.InsertVariantParams{
				ProductID: productID,
				Size:      size,
				Sku:       fmt.Sprintf("%s-%s", p.slug, size),
				Stock:     p.stock[j],
			})
			if err != nil {
				return fmt.Errorf("insert variant %s/%s: %w", p.slug, size, err)
			}
		}
		// One price change per product so the analytics markers have data.
		reason := "launch price correction"
		err = s.q.InsertPriceHistory(ctx, sqlcgen.InsertPriceHistoryParams{
			ProductID:     productID,
			OldPriceCents: p.priceCents + 400,
			NewPriceCents: p.priceCents,
			Reason:        &reason,
			CreatedAt:     ts(now.AddDate(0, 0, -14)),
		})
		if err != nil {
			return fmt.Errorf("insert price history %s: %w", p.slug, err)
		}
	}
	return nil
}

func (s *Seeder) seedTraffic(ctx context.Context, now time.Time, variants []sqlcgen.ListVariantIDsRow) error {
	if len(variants) == 0 {
		return nil
	}
	// Demo data must be reproducible, so the seed is fixed. Nothing here
	// protects anything; real tokens come from crypto/rand.
	rng := rand.New(rand.NewSource(20260727)) //nolint:gosec // deterministic demo data
	var orderSeq int

	for day := 30; day >= 1; day-- {
		date := now.AddDate(0, 0, -day)
		for range rng.Intn(25) + 15 {
			visitorID := uuid.New()
			sessionID := uuid.New()
			src := seedSources[rng.Intn(len(seedSources))]
			at := date.Add(time.Duration(rng.Intn(24*60)) * time.Minute)
			touch := analytics.Touch{
				VisitorID:   visitorID,
				UTMSource:   src.source,
				UTMMedium:   src.medium,
				UTMCampaign: src.campaign,
				LandingPath: "/",
			}

			err := s.q.InsertVisit(ctx, sqlcgen.InsertVisitParams{
				VisitorID:   visitorID,
				SessionID:   sessionID,
				LandingPath: "/",
				UtmSource:   nullString(src.source),
				UtmMedium:   nullString(src.medium),
				UtmCampaign: nullString(src.campaign),
				UserAgent:   nullString("Mozilla/5.0 (seed)"),
				CreatedAt:   ts(at),
			})
			if err != nil {
				return fmt.Errorf("insert visit: %w", err)
			}

			funnel := []analytics.EventType{analytics.EventPageView}
			depth := rng.Intn(100)
			switch {
			case depth > 85:
				funnel = append(funnel,
					analytics.EventProductView, analytics.EventAddToCart,
					analytics.EventCheckoutStarted, analytics.EventPaymentCreated,
					analytics.EventOrderPaid)
			case depth > 70:
				funnel = append(funnel,
					analytics.EventProductView, analytics.EventAddToCart, analytics.EventCheckoutStarted)
			case depth > 45:
				funnel = append(funnel, analytics.EventProductView, analytics.EventAddToCart)
			case depth > 20:
				funnel = append(funnel, analytics.EventProductView)
			}
			for step, ev := range funnel {
				err := s.q.InsertEvent(ctx, sqlcgen.InsertEventParams{
					VisitorID: visitorID,
					SessionID: sessionID,
					Type:      string(ev),
					Payload:   []byte("{}"),
					CreatedAt: ts(at.Add(time.Duration(step) * time.Minute)),
				})
				if err != nil {
					return fmt.Errorf("insert event: %w", err)
				}
			}

			if funnel[len(funnel)-1] != analytics.EventOrderPaid {
				continue
			}
			orderSeq++
			if err := s.insertPaidOrder(ctx, rng, variants, touch, at, orderSeq); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Seeder) insertPaidOrder(
	ctx context.Context,
	rng *rand.Rand,
	variants []sqlcgen.ListVariantIDsRow,
	touch analytics.Touch,
	at time.Time,
	seq int,
) error {
	v := variants[rng.Intn(len(variants))]
	qty := int32of(rng.Intn(2) + 1)
	subtotal := v.PriceCents * int64(qty)

	tj, err := json.Marshal(touch)
	if err != nil {
		return fmt.Errorf("marshal touch: %w", err)
	}
	paidAt := at.Add(20 * time.Minute)

	orderID, err := s.q.InsertOrder(ctx, sqlcgen.InsertOrderParams{
		Number:          fmt.Sprintf("ORD-%s-%04d", at.Format("060102"), seq),
		PublicToken:     randomHex32(rng),
		TgLinkCode:      randomHex16(rng),
		Status:          string(order.StatusPaid),
		SubtotalCents:   subtotal,
		ShippingCents:   0,
		TotalCents:      subtotal,
		Currency:        "USD",
		CustomerName:    "Seed Customer",
		CustomerContact: "seed@example.com",
		ShippingAddress: "Seed street 1",
		VisitorID:       nullUUID(&touch.VisitorID),
		FirstTouch:      tj,
		LastTouch:       tj,
		PaidAt:          ts(paidAt),
		CreatedAt:       ts(at),
	})
	if err != nil {
		return fmt.Errorf("insert order: %w", err)
	}

	err = s.q.InsertOrderItem(ctx, sqlcgen.InsertOrderItemParams{
		OrderID:        orderID,
		VariantID:      v.ID,
		ProductTitle:   v.Title,
		Size:           v.Size,
		UnitPriceCents: v.PriceCents,
		Qty:            qty,
	})
	if err != nil {
		return fmt.Errorf("insert order item: %w", err)
	}
	return nil
}

// randomHex32 and randomHex16 use the seeded rng on purpose: demo tokens must be
// reproducible and never protect anything. Real tokens come from crypto/rand.
func randomHex32(rng *rand.Rand) string { return randomHex(rng, 32) }

func randomHex16(rng *rand.Rand) string { return randomHex(rng, 16) }

func randomHex(rng *rand.Rand, n int) string {
	const digits = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = digits[rng.Intn(len(digits))]
	}
	return string(b)
}
