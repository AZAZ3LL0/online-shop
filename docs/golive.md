# Go-live rehearsal on a real payment (S8.3)

One manual pass over the whole path on the live domain with real provider keys
and the smallest payment the coin allows. It is not automated (TASKS.md S8.3);
the filled-in checklist goes into the pull request description.

Run it after [deploy.md](deploy.md) is done and `https://<domain>/healthz`
answers `ok`.

## Before you start

- [ ] `PAYMENTS_PROVIDER=nowpayments` and `TELEGRAM_PROVIDER=telegram` in `.env`
      on the server, container restarted, startup log shows
      `providers selected payments=nowpayments`.
- [ ] `getWebhookInfo` reports no `last_error_message`.
- [ ] One product is repriced to the smallest amount that still clears the
      NOWPayments minimum for the coin you will pay with. Use the admin price
      form with a `reason` — it is written to `price_history` and shows up on the
      dashboard chart, which is the point of the field.
- [ ] A wallet with enough of that coin plus the network fee.

Nothing here is a test mode. The payment is real money and the order is a real
row; cancel it from the admin panel afterwards rather than deleting it.

## The pass

| # | Step | Expected |
|---|---|---|
| 1 | Open `https://<domain>/` in a fresh private window | Three cards, cover flips on hover, padlock in the address bar |
| 2 | Open a product, pick a size | Sold-out sizes are disabled |
| 3 | Add to cart, change the quantity, open `/cart` | Totals match the line items, shipping from settings |
| 4 | Fill in checkout and submit | 303 to the NOWPayments invoice, order is `awaiting_payment`, stock `reserved` went up |
| 5 | Pay the invoice from the wallet | NOWPayments shows the payment confirming |
| 6 | Watch `/order/<token>` | The 10 s poll flips the status to `paid` without a reload |
| 7 | Admin → the order | `payment_events` holds the IPN with `signature_ok=true`, exactly one row per delivery |
| 8 | Open the bot deep link, `/start <code>` | Bot answers with the order status (needs S4.2, see below) |
| 9 | Admin → move `paid → shipped` | Bot delivers the status message once, `notifications` row is `sent` |
| 10 | Stock check | `stock` decreased by the quantity, `reserved` is back to 0 |
| 11 | Second look at the IPN | Re-delivering the same callback from the dashboard changes nothing: one transition, one stock movement, one message |

## After the pass

- [ ] Price restored to the real one, again with a `reason`.
- [ ] Test order moved to `cancelled` (or `delivered` if you actually shipped it).
- [ ] `./scripts/backup-db.sh` run once and the dump restored on a throwaway
      host — a backup nobody restored is not a backup.
- [ ] Uptime monitor green, alerting mail received once during a deliberate
      `docker compose stop app`.
- [ ] Paste this checklist, the order number and the transaction hash into the
      pull request description.

## Known blocker

Steps 8 and 9 depend on S4 (`tg_link_code`, the bot webhook, the outbox
notification on every transition). Those slices live on the unmerged branch
`claude/telegram-statuses-6f849c` and are not on `main`. Until S4 is merged the
rehearsal can only be completed up to step 7, and the Telegram half has to be
repeated afterwards.
