package admin

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/qzq-kiim/shop/internal/domain/analytics"
	"github.com/qzq-kiim/shop/internal/httpx/reqctx"
	"github.com/qzq-kiim/shop/internal/money"
)

// timeLayout is the wire format of every instant in the metrics payload.
const timeLayout = time.RFC3339

// metricsResponse is the contract of GET /admin/api/metrics: the shape Chart.js
// is fed from, tech.md §8.2. Money crosses the wire as minor units plus the
// rendered string, never as a float.
type metricsResponse struct {
	Range        rangeDTO         `json:"range"`
	Totals       totalsDTO        `json:"totals"`
	Series       []bucketDTO      `json:"series"`
	PriceChanges []priceChangeDTO `json:"price_changes"`
	Funnel       []funnelStageDTO `json:"funnel"`
	Sources      []sourceDTO      `json:"sources"`
}

type rangeDTO struct {
	Period      string `json:"period"`
	Granularity string `json:"granularity"`
	From        string `json:"from"`
	To          string `json:"to"`
	Days        int    `json:"days"`
}

type moneyDTO struct {
	Cents    int64  `json:"cents"`
	Currency string `json:"currency"`
	Display  string `json:"display"`
}

type totalsDTO struct {
	Visits             int64    `json:"visits"`
	Visitors           int64    `json:"visitors"`
	Orders             int64    `json:"orders"`
	PaidOrders         int64    `json:"paid_orders"`
	Revenue            moneyDTO `json:"revenue"`
	AverageOrder       moneyDTO `json:"average_order"`
	Conversion         float64  `json:"conversion"`
	AbandonedCartShare float64  `json:"abandoned_cart_share"`
}

type bucketDTO struct {
	At         string   `json:"at"`
	Orders     int64    `json:"orders"`
	PaidOrders int64    `json:"paid_orders"`
	Revenue    moneyDTO `json:"revenue"`
}

type priceChangeDTO struct {
	ProductID    string   `json:"product_id"`
	ProductTitle string   `json:"product_title"`
	At           string   `json:"at"`
	OldPrice     moneyDTO `json:"old_price"`
	NewPrice     moneyDTO `json:"new_price"`
	Delta        moneyDTO `json:"delta"`
	Reason       string   `json:"reason"`
}

type funnelStageDTO struct {
	Stage            string  `json:"stage"`
	Label            string  `json:"label"`
	Count            int64   `json:"count"`
	RateFromPrevious float64 `json:"rate_from_previous"`
	RateFromTop      float64 `json:"rate_from_top"`
}

type sourceDTO struct {
	Source     string   `json:"source"`
	Medium     string   `json:"medium"`
	Campaign   string   `json:"campaign"`
	Referrer   string   `json:"referrer"`
	Label      string   `json:"label"`
	Visitors   int64    `json:"visitors"`
	Orders     int64    `json:"orders"`
	Revenue    moneyDTO `json:"revenue"`
	Conversion float64  `json:"conversion"`
}

// Metrics answers the chart requests of the dashboard, tech.md §5.3.
func (h *Handler) Metrics(w http.ResponseWriter, r *http.Request) {
	rng, err := h.parseRange(r)
	if err != nil {
		http.Error(w, "bad range", http.StatusBadRequest)
		return
	}
	report, err := h.metrics.Metrics(r.Context(), rng)
	if err != nil {
		h.log.Error("load metrics failed",
			slog.String("request_id", reqctx.RequestID(r.Context())),
			slog.String("error", err.Error()))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(metricsPayload(report)); err != nil {
		h.log.Error("encode metrics failed",
			slog.String("request_id", reqctx.RequestID(r.Context())),
			slog.String("error", err.Error()))
	}
}

// parseRange reads the range selector out of the query string.
func (h *Handler) parseRange(r *http.Request) (analytics.Range, error) {
	q := r.URL.Query()
	rng, err := analytics.ParseRange(
		q.Get("range"), q.Get("granularity"), q.Get("from"), q.Get("to"), time.Now().UTC())
	if err != nil {
		if errors.Is(err, analytics.ErrBadRange) {
			h.log.Warn("metrics range rejected",
				slog.String("request_id", reqctx.RequestID(r.Context())),
				slog.String("error", err.Error()))
		}
		return analytics.Range{}, err
	}
	return rng, nil
}

func metricsPayload(m analytics.Metrics) metricsResponse {
	out := metricsResponse{
		Range: rangeDTO{
			Period:      string(m.Range.Period),
			Granularity: string(m.Range.Granularity),
			From:        m.Range.From.Format(timeLayout),
			To:          m.Range.To.Format(timeLayout),
			Days:        m.Range.Days(),
		},
		Totals: totalsDTO{
			Visits:             m.Totals.Visits,
			Visitors:           m.Totals.Visitors,
			Orders:             m.Totals.Orders,
			PaidOrders:         m.Totals.PaidOrders,
			Revenue:            amount(m.Totals.Revenue),
			AverageOrder:       amount(m.Totals.AverageOrder()),
			Conversion:         m.Totals.Conversion(),
			AbandonedCartShare: m.Totals.AbandonedCartShare(),
		},
		Series:       make([]bucketDTO, 0, len(m.Series)),
		PriceChanges: make([]priceChangeDTO, 0, len(m.PriceChanges)),
		Funnel:       make([]funnelStageDTO, 0, len(m.Funnel)),
		Sources:      make([]sourceDTO, 0, len(m.Sources)),
	}
	for _, b := range m.Series {
		out.Series = append(out.Series, bucketDTO{
			At:         b.At.Format(timeLayout),
			Orders:     b.Orders,
			PaidOrders: b.PaidOrders,
			Revenue:    amount(b.Revenue),
		})
	}
	for _, c := range m.PriceChanges {
		out.PriceChanges = append(out.PriceChanges, priceChangeDTO{
			ProductID:    c.ProductID.String(),
			ProductTitle: c.ProductTitle,
			At:           c.At.Format(timeLayout),
			OldPrice:     amount(c.OldPrice),
			NewPrice:     amount(c.NewPrice),
			Delta:        amount(c.Delta()),
			Reason:       c.Reason,
		})
	}
	for i, s := range m.Funnel {
		out.Funnel = append(out.Funnel, funnelStageDTO{
			Stage:            string(s.Stage),
			Label:            s.Label(),
			Count:            s.Count,
			RateFromPrevious: m.Funnel.RateFromPrevious(i),
			RateFromTop:      m.Funnel.RateFromTop(i),
		})
	}
	for _, s := range m.Sources {
		out.Sources = append(out.Sources, sourceDTO{
			Source:     s.Source,
			Medium:     s.Medium,
			Campaign:   s.Campaign,
			Referrer:   s.Referrer,
			Label:      s.Label(),
			Visitors:   s.Visitors,
			Orders:     s.Orders,
			Revenue:    amount(s.Revenue),
			Conversion: s.Conversion(),
		})
	}
	return out
}

func amount(a money.Amount) moneyDTO {
	return moneyDTO{Cents: a.Cents, Currency: a.Currency, Display: a.String()}
}
