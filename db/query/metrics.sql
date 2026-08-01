-- Aggregates behind GET /admin/api/metrics, tech.md §8.2. Every number the admin
-- panel shows is computed here, in SQL: nothing is counted in a Go loop.

-- MetricsTotals is the top row of the dashboard. Bot sessions never enter the
-- numbers: they are filtered out of visits and, through the session join, out of
-- the events as well (tech.md §5.6).
-- name: MetricsTotals :one
WITH sessions AS (
    SELECT visits.visitor_id, visits.session_id
    FROM visits
    WHERE visits.created_at >= @from_at AND visits.created_at < @to_at AND NOT visits.is_bot
),
paid AS (
    SELECT total_cents
    FROM orders
    WHERE orders.created_at >= @from_at AND orders.created_at < @to_at
      AND orders.status = ANY(@revenue_statuses::text[])
)
SELECT
    (SELECT count(*) FROM sessions)::bigint AS visits,
    (SELECT count(DISTINCT visitor_id) FROM sessions)::bigint AS visitors,
    (SELECT count(*) FROM orders
      WHERE orders.created_at >= @from_at AND orders.created_at < @to_at)::bigint AS orders,
    (SELECT count(*) FROM paid)::bigint AS paid_orders,
    (SELECT coalesce(sum(total_cents), 0) FROM paid)::bigint AS revenue_cents,
    (SELECT count(DISTINCT e.visitor_id)
       FROM events e JOIN sessions s ON s.session_id = e.session_id
      WHERE e.type = 'add_to_cart'
        AND e.created_at >= @from_at AND e.created_at < @to_at)::bigint AS cart_visitors,
    (SELECT count(DISTINCT e.visitor_id)
       FROM events e JOIN sessions s ON s.session_id = e.session_id
      WHERE e.type = 'order_paid'
        AND e.created_at >= @from_at AND e.created_at < @to_at)::bigint AS paid_visitors;

-- MetricsSeries buckets orders and revenue by the requested unit. The buckets
-- are generated, not derived from the rows, so a day without orders still shows
-- up on the chart as a zero instead of collapsing the axis. Truncation happens
-- in UTC, independent of the database session time zone.
-- name: MetricsSeries :many
WITH params AS (
    SELECT @unit::text AS unit,
           @from_at::timestamptz AS from_at,
           @to_at::timestamptz AS to_at,
           @revenue_statuses::text[] AS revenue_statuses
),
bounds AS (
    SELECT date_trunc(p.unit, timezone('UTC', p.from_at)) AS start_at,
           date_trunc(p.unit, timezone('UTC', p.to_at - interval '1 microsecond')) AS end_at,
           ('1 ' || p.unit)::interval AS step
    FROM params p
),
buckets AS (
    SELECT generate_series(b.start_at, b.end_at, b.step) AS bucket
    FROM bounds b
),
grouped AS (
    SELECT date_trunc(p.unit, timezone('UTC', o.created_at)) AS bucket,
           count(*) AS orders,
           count(*) FILTER (WHERE o.status = ANY(p.revenue_statuses)) AS paid_orders,
           coalesce(sum(o.total_cents) FILTER (WHERE o.status = ANY(p.revenue_statuses)), 0) AS revenue_cents
    FROM orders o
    CROSS JOIN params p
    WHERE o.created_at >= p.from_at AND o.created_at < p.to_at
    GROUP BY 1
)
SELECT b.bucket::timestamp AS bucket,
       coalesce(g.orders, 0)::bigint AS orders,
       coalesce(g.paid_orders, 0)::bigint AS paid_orders,
       coalesce(g.revenue_cents, 0)::bigint AS revenue_cents
FROM buckets b
LEFT JOIN grouped g ON g.bucket = b.bucket
ORDER BY b.bucket;

-- MetricsPriceChanges feeds the markers drawn over the revenue chart, so a drop
-- or a jump in sales can be read against the price it was sold at (tech.md §8.2).
-- name: MetricsPriceChanges :many
SELECT h.product_id, p.title, p.currency,
       h.old_price_cents, h.new_price_cents, h.reason, h.created_at
FROM price_history h
JOIN products p ON p.id = h.product_id
WHERE h.created_at >= @from_at AND h.created_at < @to_at
ORDER BY h.created_at;

-- MetricsFunnel counts the visitors that reached each step. A step counts every
-- visitor that got at least that far, so the stages are subsets of one another
-- and the numbers can only fall from step to step, whatever order the events
-- were written in.
-- name: MetricsFunnel :one
WITH sessions AS (
    SELECT visits.visitor_id, visits.session_id
    FROM visits
    WHERE visits.created_at >= @from_at AND visits.created_at < @to_at AND NOT visits.is_bot
),
reached AS (
    SELECT DISTINCT e.visitor_id, e.type
    FROM events e
    JOIN sessions s ON s.session_id = e.session_id
    WHERE e.created_at >= @from_at AND e.created_at < @to_at
)
SELECT
    (SELECT count(DISTINCT visitor_id) FROM sessions)::bigint AS visits,
    (SELECT count(DISTINCT visitor_id) FROM reached
      WHERE type IN ('product_view', 'add_to_cart', 'checkout_started',
                     'payment_created', 'order_paid'))::bigint AS product_views,
    (SELECT count(DISTINCT visitor_id) FROM reached
      WHERE type IN ('add_to_cart', 'checkout_started',
                     'payment_created', 'order_paid'))::bigint AS cart_adds,
    (SELECT count(DISTINCT visitor_id) FROM reached
      WHERE type IN ('checkout_started', 'payment_created',
                     'order_paid'))::bigint AS checkouts,
    (SELECT count(DISTINCT visitor_id) FROM reached
      WHERE type = 'order_paid')::bigint AS payments;

-- MetricsSources reports traffic and money side by side. Visitors are grouped by
-- what the visits recorded; orders are grouped by the last touch stored on the
-- order itself, so one order is credited to exactly one source no matter how
-- many times its buyer came back.
-- name: MetricsSources :many
WITH visit_groups AS (
    SELECT coalesce(utm_source, '') AS source,
           coalesce(utm_medium, '') AS medium,
           coalesce(utm_campaign, '') AS campaign,
           coalesce(referrer, '') AS referrer,
           count(DISTINCT visitor_id) AS visitors
    FROM visits
    WHERE visits.created_at >= @from_at AND visits.created_at < @to_at AND NOT visits.is_bot
    GROUP BY 1, 2, 3, 4
),
order_groups AS (
    SELECT coalesce(last_touch->>'utm_source', '') AS source,
           coalesce(last_touch->>'utm_medium', '') AS medium,
           coalesce(last_touch->>'utm_campaign', '') AS campaign,
           coalesce(last_touch->>'referrer', '') AS referrer,
           count(*) AS orders,
           coalesce(sum(total_cents), 0) AS revenue_cents
    FROM orders
    WHERE orders.created_at >= @from_at AND orders.created_at < @to_at
      AND orders.status = ANY(@revenue_statuses::text[])
    GROUP BY 1, 2, 3, 4
)
SELECT coalesce(v.source, o.source)::text AS source,
       coalesce(v.medium, o.medium)::text AS medium,
       coalesce(v.campaign, o.campaign)::text AS campaign,
       coalesce(v.referrer, o.referrer)::text AS referrer,
       coalesce(v.visitors, 0)::bigint AS visitors,
       coalesce(o.orders, 0)::bigint AS orders,
       coalesce(o.revenue_cents, 0)::bigint AS revenue_cents
FROM visit_groups v
FULL JOIN order_groups o
       ON o.source = v.source AND o.medium = v.medium
      AND o.campaign = v.campaign AND o.referrer = v.referrer
ORDER BY 7 DESC, 5 DESC, 1
LIMIT @row_limit;
