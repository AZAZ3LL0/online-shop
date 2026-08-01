package postgres

import (
	"context"
	"fmt"

	"github.com/qzq-kiim/shop/internal/domain/analytics"
	"github.com/qzq-kiim/shop/internal/domain/order"
	"github.com/qzq-kiim/shop/internal/money"
	"github.com/qzq-kiim/shop/internal/storage/postgres/sqlcgen"
)

// sourceRowLimit caps the traffic table. Campaign tagging is a handful of
// combinations; a referrer list long enough to hit this cap is spam.
const sourceRowLimit = 50

// currency is the shop currency every reported amount is in, tech.md §15.
const currency = "USD"

// MetricsRepo answers the admin panel with SQL aggregates. It runs one query per
// panel and nothing is counted in a Go loop, tech.md §8.2.
type MetricsRepo struct {
	q *sqlcgen.Queries
}

// NewMetricsRepo returns the metrics repository bound to the store.
func NewMetricsRepo(s *Store) *MetricsRepo { return &MetricsRepo{q: s.q} }

var _ analytics.MetricsRepository = (*MetricsRepo)(nil)

// Metrics loads every panel of one range.
func (r *MetricsRepo) Metrics(ctx context.Context, rng analytics.Range) (analytics.Metrics, error) {
	from, to := ts(rng.From), ts(rng.To)
	statuses := revenueStatuses()

	totals, err := r.q.MetricsTotals(ctx, sqlcgen.MetricsTotalsParams{
		FromAt: from, ToAt: to, RevenueStatuses: statuses,
	})
	if err != nil {
		return analytics.Metrics{}, fmt.Errorf("metrics totals: %w", err)
	}
	series, err := r.q.MetricsSeries(ctx, sqlcgen.MetricsSeriesParams{
		Unit: string(rng.Granularity), FromAt: from, ToAt: to, RevenueStatuses: statuses,
	})
	if err != nil {
		return analytics.Metrics{}, fmt.Errorf("metrics series: %w", err)
	}
	changes, err := r.q.MetricsPriceChanges(ctx, sqlcgen.MetricsPriceChangesParams{
		FromAt: from, ToAt: to,
	})
	if err != nil {
		return analytics.Metrics{}, fmt.Errorf("metrics price changes: %w", err)
	}
	funnel, err := r.q.MetricsFunnel(ctx, sqlcgen.MetricsFunnelParams{FromAt: from, ToAt: to})
	if err != nil {
		return analytics.Metrics{}, fmt.Errorf("metrics funnel: %w", err)
	}
	sources, err := r.q.MetricsSources(ctx, sqlcgen.MetricsSourcesParams{
		FromAt: from, ToAt: to, RevenueStatuses: statuses, RowLimit: sourceRowLimit,
	})
	if err != nil {
		return analytics.Metrics{}, fmt.Errorf("metrics sources: %w", err)
	}

	return analytics.Metrics{
		Range:        rng,
		Totals:       metricsTotals(totals),
		Series:       metricsSeries(series),
		PriceChanges: metricsPriceChanges(changes),
		Funnel:       metricsFunnel(funnel),
		Sources:      metricsSources(sources),
	}, nil
}

func metricsTotals(row sqlcgen.MetricsTotalsRow) analytics.Totals {
	return analytics.Totals{
		Visits:       row.Visits,
		Visitors:     row.Visitors,
		Orders:       row.Orders,
		PaidOrders:   row.PaidOrders,
		Revenue:      money.New(row.RevenueCents, currency),
		CartVisitors: row.CartVisitors,
		PaidVisitors: row.PaidVisitors,
	}
}

func metricsSeries(rows []sqlcgen.MetricsSeriesRow) []analytics.Bucket {
	buckets := make([]analytics.Bucket, 0, len(rows))
	for _, row := range rows {
		buckets = append(buckets, analytics.Bucket{
			// The query truncates in UTC and returns a bare timestamp.
			At:         row.Bucket.Time.UTC(),
			Orders:     row.Orders,
			PaidOrders: row.PaidOrders,
			Revenue:    money.New(row.RevenueCents, currency),
		})
	}
	return buckets
}

func metricsPriceChanges(rows []sqlcgen.MetricsPriceChangesRow) []analytics.PriceChange {
	changes := make([]analytics.PriceChange, 0, len(rows))
	for _, row := range rows {
		change := analytics.PriceChange{
			ProductID:    row.ProductID,
			ProductTitle: row.Title,
			At:           row.CreatedAt.Time.UTC(),
			OldPrice:     money.New(row.OldPriceCents, row.Currency),
			NewPrice:     money.New(row.NewPriceCents, row.Currency),
		}
		if row.Reason != nil {
			change.Reason = *row.Reason
		}
		changes = append(changes, change)
	}
	return changes
}

// metricsFunnel keeps the stages in the order they are walked in: the template
// and the JSON both rely on it, and the counts are subsets of one another.
func metricsFunnel(row sqlcgen.MetricsFunnelRow) analytics.Funnel {
	return analytics.Funnel{
		{Stage: analytics.StageVisits, Count: row.Visits},
		{Stage: analytics.StageProductViews, Count: row.ProductViews},
		{Stage: analytics.StageCartAdds, Count: row.CartAdds},
		{Stage: analytics.StageCheckouts, Count: row.Checkouts},
		{Stage: analytics.StagePayments, Count: row.Payments},
	}
}

func metricsSources(rows []sqlcgen.MetricsSourcesRow) []analytics.Source {
	sources := make([]analytics.Source, 0, len(rows))
	for _, row := range rows {
		sources = append(sources, analytics.Source{
			Source:   row.Source,
			Medium:   row.Medium,
			Campaign: row.Campaign,
			Referrer: row.Referrer,
			Visitors: row.Visitors,
			Orders:   row.Orders,
			Revenue:  money.New(row.RevenueCents, currency),
		})
	}
	return sources
}

// revenueStatuses renders the domain list for the SQL parameter, so the set of
// statuses that count as money is defined once, in the domain.
func revenueStatuses() []string {
	statuses := order.RevenueStatuses()
	out := make([]string, 0, len(statuses))
	for _, s := range statuses {
		out = append(out, string(s))
	}
	return out
}
