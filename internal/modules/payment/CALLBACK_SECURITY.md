# Payment callback hardening boundaries

## Reference policy

Reference validation happens both before the transaction and again on the locked
payment, for ordinary orders and wallet recharges. It does not replace signature,
channel, merchant-order-number, amount, or currency validation.

- **epay**: `trade_no` from API creation and callbacks names the gateway trade.
  Compare against the creation payload's `trade_no`, or a saved non-placeholder
  `ProviderRef`. Redirect creation returns no gateway trade; the application
  substitutes the submitted merchant order number. That placeholder may become
  the authenticated paid trade reference.
- **BEpusdt transaction mode**: creation `data.trade_id` and callback `trade_id`
  name the gateway transaction. Compare the snapshot, falling back to the saved
  non-placeholder reference for legacy records.
- **BEpusdt cashier mode**: when the creation snapshot explicitly says
  `data.order_mode=cashier`, creation-reference equality is not enforced before
  settlement. The separate cashier API supports currency reselection; the local
  fixtures do not establish that its creation ID and selected payment ID must
  remain identical. This is a deliberate compatibility boundary, not a claim
  that this transition is documented or validated against a live gateway.
- Both providers require a nonblank reference for success, including retries.
  After success, their stored paid reference is immutable, including cashier
  payments. Missing verified callback references must not be hidden by a fallback
  to the saved reference.
- **Excluded from new reference equality checks**: Stripe (Checkout Session and
  PaymentIntent are different objects), official WeChat (merchant-order/prepay
  placeholder to paid transaction), PayPal, Alipay, EPusdt, DujiaoPay, TokenPay,
  and OKPay. Their creation/paid reference contracts need separate review; this
  change does not infer equality merely because a field is called ProviderRef.

A missing historical creation snapshot cannot distinguish an actual gateway ID
that happens to equal the merchant order number from the application placeholder.
Until settlement, that case remains compatible. No global reference uniqueness
constraint or event replay ledger is introduced.

## Merchant and state policy

- EPusdt requires a nonblank configured PID and exact, nonblank callback PID
  equality. Callback identifiers are not trimmed or case-folded for comparison.
  HMAC-SHA256 signing is unchanged.
- PayPal success is restricted to `PAYMENT.CAPTURE.COMPLETED` and
  `CHECKOUT.ORDER.COMPLETED`. An explicit resource status must be `COMPLETED`;
  absent status remains supported for the existing event-only contract.
  `COMPLETED` on an unknown/refund event is not payment success. Existing
  non-success status handling and remote signature verification are unchanged.
- WeChat success requires trade state `SUCCESS`. A successful webhook must also
  be the authenticated `TRANSACTION.SUCCESS` event. `REFUND` is unsupported as
  a payment-state transition, including query results. This does not implement
  refund reconciliation.

## Metadata concurrency

All callback writes, including same-status, already-successful, and ignored wallet
terminal-state notifications, now use the existing transaction path and reload
payment/order or recharge rows under their existing locks. Metadata is merged
only from those fresh rows. The GORM store's ordinary Update API is unchanged;
there is no longer an unlocked callback metadata Save path.

Deterministic in-memory SQLite barriers reproduce an unlocked callback read,
commit a second callback, then resume the first. Tests verify no success/PaidAt/
reference rollback, no lost independent metadata keys, preserved creation
snapshots, consistent order/recharge state, and once-only wallet credit. A second
barrier test verifies reference validation is repeated after acquiring the lock.
This proves the application interleaving locally; it is not a PostgreSQL/MySQL
stress test or evidence about deployed fulfillment delivery.

## Verification and scope

Tests use synthetic keys, in-memory databases, and loopback fixture servers only.
Commands run in a new network namespace with only loopback enabled and
`GOPROXY=off`:

```sh
unshare -n sh -c 'ip link set lo up && GOPROXY=off go test -race -count=1 ./internal/modules/payment/...'
unshare -n sh -c 'ip link set lo up && GOPROXY=off go test -count=1 ./...'
unshare -n sh -c 'ip link set lo up && GOPROXY=off go vet ./...'
```

The new regression tests were observed failing before their respective fixes.
The existing underpayment, late success, superseded-payment, currency conversion,
wallet idempotency, signature, and adapter fixtures remain part of the suite.
No live callback/provider/payment requests, deployments, configuration changes,
commits, or external database access are required or performed.
