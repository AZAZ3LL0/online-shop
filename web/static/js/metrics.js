// Admin charts. The numbers are already server-rendered in the cards and the
// tables; this only draws them. Data comes from GET /admin/api/metrics, so the
// chart and the page always report the same period.
document.addEventListener("DOMContentLoaded", () => {
  const root = document.getElementById("metrics-root");
  if (!root || typeof window.Chart === "undefined") {
    return;
  }
  const url = root.dataset.metricsUrl;
  if (!url) {
    return;
  }

  const ink = "#171717";
  const muted = "#a3a3a3";

  fetch(url, { headers: { "X-Requested-With": "fetch" } })
    .then((response) => (response.ok ? response.json() : null))
    .then((data) => {
      if (!data) {
        return;
      }
      drawRevenue(data);
      drawFunnel(data);
    })
    .catch(() => {
      // The tables next to the chart carry the same numbers, so a failed
      // fetch degrades the page instead of breaking it.
    });

  function bucketLabel(at, granularity) {
    const date = new Date(at);
    if (granularity === "hour") {
      return date.toISOString().slice(11, 16);
    }
    return date.toISOString().slice(0, 10);
  }

  // priceMarkers turns price changes into one point per bucket they fall in, so
  // a change is drawn where the revenue it affected is (tech.md §8.2).
  function priceMarkers(data, labels) {
    const byLabel = new Map();
    data.price_changes.forEach((change) => {
      const label = bucketLabel(change.at, data.range.granularity);
      const nearest = labels.includes(label)
        ? label
        : labels.find((candidate) => candidate >= label);
      if (nearest === undefined) {
        return;
      }
      const existing = byLabel.get(nearest) || [];
      existing.push(`${change.product_title}: ${change.old_price.display} -> ${change.new_price.display}`);
      byLabel.set(nearest, existing);
    });
    return byLabel;
  }

  function drawRevenue(data) {
    const canvas = document.getElementById("revenue-chart");
    if (!canvas) {
      return;
    }
    const labels = data.series.map((bucket) => bucketLabel(bucket.at, data.range.granularity));
    const revenue = data.series.map((bucket) => bucket.revenue.cents / 100);
    const orders = data.series.map((bucket) => bucket.orders);
    const markers = priceMarkers(data, labels);
    const markerPoints = labels.map((label) => (markers.has(label) ? Math.max(...revenue, 1) : null));

    new window.Chart(canvas, {
      type: "bar",
      data: {
        labels,
        datasets: [
          { label: "Revenue", data: revenue, backgroundColor: ink, order: 3 },
          {
            label: "Orders",
            data: orders,
            type: "line",
            yAxisID: "orders",
            borderColor: muted,
            backgroundColor: muted,
            tension: 0.2,
            order: 1,
          },
          {
            label: "Price change",
            data: markerPoints,
            type: "scatter",
            pointStyle: "triangle",
            pointRadius: 7,
            borderColor: ink,
            backgroundColor: "#ffffff",
            showLine: false,
            order: 0,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        interaction: { mode: "index", intersect: false },
        scales: {
          y: { beginAtZero: true, title: { display: true, text: "Revenue" } },
          orders: { beginAtZero: true, position: "right", grid: { drawOnChartArea: false } },
        },
        plugins: {
          legend: { labels: { usePointStyle: true } },
          tooltip: {
            callbacks: {
              afterBody: (items) => markers.get(items[0].label) || [],
            },
          },
        },
      },
    });
  }

  function drawFunnel(data) {
    const canvas = document.getElementById("funnel-chart");
    if (!canvas) {
      return;
    }
    new window.Chart(canvas, {
      type: "bar",
      data: {
        labels: data.funnel.map((stage) => stage.label),
        datasets: [
          {
            label: "Visitors",
            data: data.funnel.map((stage) => stage.count),
            backgroundColor: ink,
          },
        ],
      },
      options: {
        indexAxis: "y",
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
          legend: { display: false },
          tooltip: {
            callbacks: {
              afterLabel: (item) => {
                const stage = data.funnel[item.dataIndex];
                return `${(stage.rate_from_previous * 100).toFixed(1)}% of the previous step`;
              },
            },
          },
        },
        scales: { x: { beginAtZero: true } },
      },
    });
  }
});
