package analytics

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/money"
)

// ErrBadRange is returned when the requested period cannot be read.
var ErrBadRange = errors.New("analytics: unsupported range")

// Period is the range selector of the dashboard, tech.md §8.1.
type Period string

// Periods.
const (
	PeriodToday   Period = "today"
	Period7Days   Period = "7d"
	Period30Days  Period = "30d"
	PeriodCustom  Period = "custom"
	DefaultPeriod        = Period30Days
)

// Granularity is the bucket width of the revenue series.
type Granularity string

// Granularities.
const (
	GranularityHour Granularity = "hour"
	GranularityDay  Granularity = "day"
	GranularityWeek Granularity = "week"
)

// Stage is one step of the purchase funnel, tech.md §8.2.
type Stage string

// Funnel stages, in order.
const (
	StageVisits       Stage = "visits"
	StageProductViews Stage = "product_views"
	StageCartAdds     Stage = "cart_adds"
	StageCheckouts    Stage = "checkouts"
	StagePayments     Stage = "payments"
)

// dateLayout is the wire format of a custom range bound.
const dateLayout = "2006-01-02"

// Range is the half-open interval [From, To) the dashboard reports on.
type Range struct {
	Period      Period
	Granularity Granularity
	From        time.Time
	To          time.Time
}

// ParseRange reads the query of GET /admin/api/metrics. An empty period falls
// back to the default; anything unknown is refused rather than guessed at, so a
// typo in the query cannot silently report the wrong period.
func ParseRange(period, granularity, from, to string, now time.Time) (Range, error) {
	day := now.UTC().Truncate(24 * time.Hour)
	end := day.AddDate(0, 0, 1)

	r := Range{Period: Period(period), To: end}
	switch r.Period {
	case "":
		r.Period = DefaultPeriod
		r.From = end.AddDate(0, 0, -30)
	case PeriodToday:
		r.From = day
	case Period7Days:
		r.From = end.AddDate(0, 0, -7)
	case Period30Days:
		r.From = end.AddDate(0, 0, -30)
	case PeriodCustom:
		parsed, err := parseCustom(from, to)
		if err != nil {
			return Range{}, err
		}
		r.From, r.To = parsed.From, parsed.To
	default:
		return Range{}, fmt.Errorf("%w: period %q", ErrBadRange, period)
	}

	g, err := parseGranularity(granularity, r.Period)
	if err != nil {
		return Range{}, err
	}
	r.Granularity = g
	return r, nil
}

// Days is the length of the range in whole days, at least one.
func (r Range) Days() int {
	days := int(r.To.Sub(r.From).Hours() / 24)
	return max(days, 1)
}

func parseCustom(from, to string) (Range, error) {
	fromAt, err := time.ParseInLocation(dateLayout, from, time.UTC)
	if err != nil {
		return Range{}, fmt.Errorf("%w: from %q", ErrBadRange, from)
	}
	toAt, err := time.ParseInLocation(dateLayout, to, time.UTC)
	if err != nil {
		return Range{}, fmt.Errorf("%w: to %q", ErrBadRange, to)
	}
	// The bound the operator types is inclusive; the interval is half-open.
	toAt = toAt.AddDate(0, 0, 1)
	if !toAt.After(fromAt) {
		return Range{}, fmt.Errorf("%w: %s is not before %s", ErrBadRange, from, to)
	}
	return Range{From: fromAt, To: toAt}, nil
}

// parseGranularity defaults the bucket width to what the period can show: one
// day has no daily bars to draw, a month of hourly ones is unreadable.
func parseGranularity(granularity string, period Period) (Granularity, error) {
	switch Granularity(granularity) {
	case "":
		if period == PeriodToday {
			return GranularityHour, nil
		}
		return GranularityDay, nil
	case GranularityHour:
		return GranularityHour, nil
	case GranularityDay:
		return GranularityDay, nil
	case GranularityWeek:
		return GranularityWeek, nil
	default:
		return "", fmt.Errorf("%w: granularity %q", ErrBadRange, granularity)
	}
}

// Totals is the top row of the dashboard, tech.md §8.1. Bots are already out of
// every count: they never reach the aggregates.
type Totals struct {
	Visits       int64
	Visitors     int64
	Orders       int64
	PaidOrders   int64
	Revenue      money.Amount
	CartVisitors int64
	PaidVisitors int64
}

// AverageOrder is revenue per paid order.
func (t Totals) AverageOrder() money.Amount {
	if t.PaidOrders == 0 {
		return money.Zero(t.Revenue.Currency)
	}
	return money.New(t.Revenue.Cents/t.PaidOrders, t.Revenue.Currency)
}

// Conversion is the visit to payment rate, tech.md §8.1.
func (t Totals) Conversion() float64 { return Rate(t.PaidOrders, t.Visits) }

// AbandonedCartShare is the share of visitors who filled a cart and never paid.
func (t Totals) AbandonedCartShare() float64 {
	return Rate(t.CartVisitors-t.PaidVisitors, t.CartVisitors)
}

// Bucket is one column of the revenue and orders chart.
type Bucket struct {
	At         time.Time
	Orders     int64
	PaidOrders int64
	Revenue    money.Amount
}

// PriceChange is one marker drawn over the revenue chart, tech.md §8.2.
type PriceChange struct {
	ProductID    uuid.UUID
	ProductTitle string
	At           time.Time
	OldPrice     money.Amount
	NewPrice     money.Amount
	Reason       string
}

// Delta is how much the price moved. Both sides come from the same product row,
// so the currencies cannot disagree.
func (c PriceChange) Delta() money.Amount {
	return money.New(c.NewPrice.Cents-c.OldPrice.Cents, c.NewPrice.Currency)
}

// FunnelStage is one step of the funnel with the visitors that reached it.
type FunnelStage struct {
	Stage Stage
	Count int64
}

// Label is the human name of the stage.
func (s FunnelStage) Label() string {
	switch s.Stage {
	case StageVisits:
		return "Visits"
	case StageProductViews:
		return "Product views"
	case StageCartAdds:
		return "Added to cart"
	case StageCheckouts:
		return "Checkouts"
	case StagePayments:
		return "Paid"
	default:
		return string(s.Stage)
	}
}

// Funnel is the ordered set of stages, widest first.
type Funnel []FunnelStage

// RateFromPrevious is the share of the previous stage that reached stage i.
func (f Funnel) RateFromPrevious(i int) float64 {
	if i <= 0 || i >= len(f) {
		return 1
	}
	return Rate(f[i].Count, f[i-1].Count)
}

// RateFromTop is the share of all visitors that reached stage i.
func (f Funnel) RateFromTop(i int) float64 {
	if len(f) == 0 || i < 0 || i >= len(f) {
		return 0
	}
	return Rate(f[i].Count, f[0].Count)
}

// Source is one row of the traffic table, tech.md §8.2.
type Source struct {
	Source   string
	Medium   string
	Campaign string
	Referrer string
	Visitors int64
	Orders   int64
	Revenue  money.Amount
}

// Label names the source the way the table shows it. Traffic with no campaign
// tagging at all is direct, not blank.
func (s Source) Label() string {
	parts := make([]string, 0, 4)
	for _, part := range []string{s.Source, s.Medium, s.Campaign} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		if s.Referrer != "" {
			return s.Referrer
		}
		return "direct"
	}
	label := parts[0]
	for _, part := range parts[1:] {
		label += " / " + part
	}
	return label
}

// Conversion is the visitor to order rate of this source.
func (s Source) Conversion() float64 { return Rate(s.Orders, s.Visitors) }

// Metrics is everything the dashboard and the metrics endpoint report on one
// range. Every number in it is produced by a SQL aggregate, tech.md §8.2.
type Metrics struct {
	Range        Range
	Totals       Totals
	Series       []Bucket
	PriceChanges []PriceChange
	Funnel       Funnel
	Sources      []Source
}

// TopSources returns at most n rows of the traffic table.
func (m Metrics) TopSources(n int) []Source {
	if len(m.Sources) <= n {
		return m.Sources
	}
	return m.Sources[:n]
}

// MetricsRepository is the read side the admin panel depends on: one call per
// rendered page, every aggregate computed in SQL.
type MetricsRepository interface {
	Metrics(ctx context.Context, r Range) (Metrics, error)
}

// Rate is part/whole guarded against an empty denominator.
func Rate(part, whole int64) float64 {
	if whole <= 0 || part <= 0 {
		return 0
	}
	return float64(part) / float64(whole)
}
