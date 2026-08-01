package order

// RevenueStatuses are the statuses whose money has actually arrived, and the
// only ones the admin reports count as revenue: a refund is not revenue and an
// unpaid order never was. The status machine lives here, so the list does too.
func RevenueStatuses() []Status {
	return []Status{StatusPaid, StatusShipped, StatusDelivered}
}
