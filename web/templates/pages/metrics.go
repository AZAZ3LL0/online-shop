package pages

import (
	"net/url"
	"strconv"

	"github.com/a-h/templ"

	"github.com/qzq-kiim/shop/internal/domain/analytics"
	"github.com/qzq-kiim/shop/web/templates/components"
)

// metricsAPIPath is where the charts fetch their data, tech.md §5.3.
const metricsAPIPath = "/admin/api/metrics"

// dashboardSources is how many traffic rows fit next to the charts; the full
// table lives on the analytics page.
const dashboardSources = 5

// MetricsView is the admin report as the templates need it: the numbers plus
// where the page has to point its own links.
type MetricsView struct {
	Metrics analytics.Metrics
	// Path is the page the range picker reloads, so one picker serves both the
	// dashboard and the analytics page.
	Path string
	// Compact marks a Mini App screen: the same report, drawn for ~360px.
	Compact bool
}

// CompactAttr is how the chart bootstrap learns it is drawing for a Mini App.
func (v MetricsView) CompactAttr() string {
	if v.Compact {
		return "true"
	}
	return "false"
}

// StatCardData is one card of the top row.
type StatCardData struct {
	Label string
	Value string
	Delta string
	Tone  components.Tone
}

// StatCards is the top row of the dashboard, tech.md §8.1.
func (v MetricsView) StatCards() []StatCardData {
	t := v.Metrics.Totals
	return []StatCardData{
		{Label: "Revenue", Value: t.Revenue.String(), Delta: plural(t.PaidOrders, "paid order"), Tone: components.ToneSuccess},
		{Label: "Orders", Value: strconv.FormatInt(t.Orders, 10), Delta: strconv.FormatInt(t.PaidOrders, 10) + " paid", Tone: components.ToneNeutral},
		{Label: "Average order", Value: t.AverageOrder().String()},
		{Label: "Conversion", Value: percent(t.Conversion()), Delta: plural(t.Visits, "visit"), Tone: components.ToneNeutral},
		{Label: "Abandoned carts", Value: percent(t.AbandonedCartShare()), Delta: plural(t.CartVisitors, "cart"), Tone: abandonTone(t.AbandonedCartShare())},
	}
}

// PeriodOption is one entry of the range picker.
type PeriodOption struct {
	Label  string
	URL    string
	Active bool
}

// PeriodOptions builds the range picker. It is server-side navigation on
// purpose: the cards, the tables and the charts then always report the same
// period, whatever the browser did with the request before.
func (v MetricsView) PeriodOptions() []PeriodOption {
	named := []struct {
		period analytics.Period
		label  string
	}{
		{analytics.PeriodToday, "Today"},
		{analytics.Period7Days, "7 days"},
		{analytics.Period30Days, "30 days"},
	}
	options := make([]PeriodOption, 0, len(named))
	for _, n := range named {
		options = append(options, PeriodOption{
			Label:  n.label,
			URL:    v.Path + "?range=" + string(n.period),
			Active: v.Metrics.Range.Period == n.period,
		})
	}
	return options
}

// CustomRange reports whether the operator picked their own bounds.
func (v MetricsView) CustomRange() bool {
	return v.Metrics.Range.Period == analytics.PeriodCustom
}

// From and To are the custom bounds as the date inputs show them. The stored
// range is half-open, the input is inclusive.
func (v MetricsView) From() string { return v.Metrics.Range.From.Format("2006-01-02") }

// To is the inclusive upper bound of the range.
func (v MetricsView) To() string { return v.Metrics.Range.To.AddDate(0, 0, -1).Format("2006-01-02") }

// APIURL is the metrics endpoint for the period the page is showing.
func (v MetricsView) APIURL() string {
	q := url.Values{}
	q.Set("range", string(v.Metrics.Range.Period))
	q.Set("granularity", string(v.Metrics.Range.Granularity))
	if v.CustomRange() {
		q.Set("from", v.From())
		q.Set("to", v.To())
	}
	return metricsAPIPath + "?" + q.Encode()
}

// TopSources is the short traffic table shown next to the charts.
func (v MetricsView) TopSources() []analytics.Source {
	return v.Metrics.TopSources(dashboardSources)
}

// Sources is the full traffic table.
func (v MetricsView) Sources() []analytics.Source { return v.Metrics.Sources }

// HasSources reports whether any traffic was recorded in the period.
func (v MetricsView) HasSources() bool { return len(v.Metrics.Sources) > 0 }

// FunnelStep is one stage of the funnel with the rates the table prints.
type FunnelStep struct {
	Label            string
	Count            int64
	RateFromPrevious string
	RateFromTop      string
	// Width is the bar length in percent of the widest stage.
	Width string
}

// FunnelSteps is the funnel as the page draws it, tech.md §8.2.
func (v MetricsView) FunnelSteps() []FunnelStep {
	steps := make([]FunnelStep, 0, len(v.Metrics.Funnel))
	for i, stage := range v.Metrics.Funnel {
		steps = append(steps, FunnelStep{
			Label:            stage.Label(),
			Count:            stage.Count,
			RateFromPrevious: percent(v.Metrics.Funnel.RateFromPrevious(i)),
			RateFromTop:      percent(v.Metrics.Funnel.RateFromTop(i)),
			Width:            percent(max(v.Metrics.Funnel.RateFromTop(i), 0.01)),
		})
	}
	return steps
}

// PriceChanges are the markers drawn over the revenue chart.
func (v MetricsView) PriceChanges() []analytics.PriceChange { return v.Metrics.PriceChanges }

// funnelRows and sourceRows adapt the report to the shared Table component, so
// neither page writes its own table markup (tech.md §7.2).
func funnelRows(v MetricsView) []templ.Component {
	steps := v.FunnelSteps()
	rows := make([]templ.Component, 0, len(steps))
	for _, step := range steps {
		rows = append(rows, funnelRow(step))
	}
	return rows
}

func sourceRows(sources []analytics.Source) []templ.Component {
	rows := make([]templ.Component, 0, len(sources))
	for _, s := range sources {
		rows = append(rows, sourceRow(s))
	}
	return rows
}

// percent renders a rate with one decimal, the way both the cards and the
// funnel print it.
func percent(rate float64) string {
	return strconv.FormatFloat(rate*100, 'f', 1, 64) + "%"
}

// plural renders a count with its noun, so a card never reads "1 visits".
func plural(n int64, noun string) string {
	label := strconv.FormatInt(n, 10) + " " + noun
	if n == 1 {
		return label
	}
	return label + "s"
}

func abandonTone(share float64) components.Tone {
	if share >= 0.5 {
		return components.ToneWarning
	}
	return components.ToneNeutral
}
