# Fulfillment release and downstream callback revocation

## Scope

Test-source remediation of audit A2/A4. No production data, configuration, migrations, callbacks, deployments or credentials were used. Worktree: `/path/to/xshop-source`, base `179183d45ea88297ac50641dd3cc23a9436001e8` (the delegated concatenated path `/path/to/xshop-source179183d` did not exist).

## Callback authorization and recovery

Every `SendCallback` reloads the referenced credential and its owner. The credential must still exist, be non-deleted, approved and active, and its matching owner must exist, be non-deleted and active. The credential reader deliberately reloads through `GetByApiKey`, because the existing `GetByID` repository method does not preload the owner. A zero-valued authorization projection denies delivery.

Revoked/missing credentials or owners **pause** the reference (`callback_status=paused`) without sending even a status-only callback. The paid order, original callback URL, reference linkage, retry count and last actual callback timestamp are retained. The job is acknowledged after pause persistence, no failure backoff is scheduled, and paused references are excluded from `ListPendingCallbacks`. Already queued duplicate jobs recheck authority and do not cause a busy retry loop. Database read/write failures still fail closed and return an error for normal worker error handling.

Re-enabling a credential alone does not trigger a catch-up sweep. After reviewing the destination, an authorized operational retry can invoke the existing application `EnqueueCallback(orderID)` entrypoint, or a legitimate order-status event can invoke it. This resets the paused reference to pending and the normal worker reloads the latest state again. `SendCallback(refID)` also permits an explicit one-shot retry after re-enablement. This change does not add a public/admin retry HTTP endpoint or a new UI control. Restoration of deleted records is not automated.

Key rotation intentionally preserves the reference and uses the latest signing key; it does **not** establish that an old callback destination is safe. Review/update destinations before re-enabling compromised integrations. This check cannot recall data sent before revocation or cancel a request already in flight; revocation racing after the final read remains a normal read/send race.

## Shared release policy

`internal/modules/order/domain/order.go` gates the parent and **each individual child** independently. It is used by member/guest detail presentation, the shared member/guest download collector, channel detail payloads, the downstream order projection and callback payload selection, order-status email payload/child deliveries/attachment source, and synchronous upstream `GetOrder` fulfillment selection. It never mutates fulfillment history.

A non-deleted order and non-deleted fulfillment with `status=delivered` are required. Non-empty payload alone is never evidence. Structured delivery data is withheld along with payload.

| Order state | Additional release evidence |
|---|---|
| paid / fulfilling / partially_delivered | nonzero `PaidAt` |
| delivered / completed | matching delivered fulfillment snapshot; retains historical records without timestamps |
| refunded / partially_refunded | nonzero `PaidAt` **or** nonzero fulfillment `DeliveredAt` |
| canceled | both nonzero `PaidAt` and fulfillment `DeliveredAt` (historical delivery only) |
| pending_payment / unknown | never release, even with inconsistent timestamps |

Legacy delivered/completed records intentionally trust the mutually corroborating terminal order and delivered fulfillment states; no transaction-history backfill is invented. A canceled historical record missing payment proof is withheld for operator review. Refund-before-delivery and pending snapshots remain hidden. Parent payment cannot authorize an unpaid child; a parent's inconsistent snapshot does not suppress an independently proven child delivery.

Item instructions retain their existing payment-based visibility semantics (not a new delivery prerequisite). Channel child instructions now use the child's own payment timestamp instead of inheriting the parent's.

Order-status emails retain their existing instruction handling: instructions are included only for delivered/completed email scenes, independently of the card-release gate. Recipient lookup, queued status/template selection, item snapshots, cancellation tombstones and delivery formatting are unchanged. The current loaded order, not the queued email status, authorizes card release. Both inline text and child delivery cards are filtered; large attachments are derived only from the same filtered payload. A blocked parent's payload cannot suppress legitimate child delivery content.

## Wire/log compatibility

- Callback `CallbackPayload` / `Fulfillment` JSON fields, event naming, timestamp, HMAC algorithm, signature headers and fixed signing path `/api/v1/upstream/callback` are unchanged.
- The existing singular fulfillment shape is preserved for callbacks and synchronous upstream `GetOrder`: first releasable parent, otherwise first releasable child. No child-array protocol extension is introduced. The synchronous response retains its status/refund/items fields and fulfillment `type`, `status`, `payload`, `delivery_data`, and `delivered_at` fields without payload rewriting.
- Destination URL/query values are not modified before sending or signing. Callback URLs are omitted from worker diagnostics entirely; raw remote response bodies and transport/request errors cannot be reflected into callback logs. Client errors retain only a fixed failure category and HTTP status where available.
- Normal HTTP failure retry/backoff behavior is unchanged. Pauses are not HTTP failures.

## Verification and boundaries

Regression tests exercise synthetic pending/paid/delivered/refunded/canceled/history cases, parent/child independence, deleted/disabled/missing owners and credentials, real SQLite credential/reference persistence, pause exclusion from pending queries, re-enablement, reflected response/query redaction, and an exact signed callback JSON fixture. HTTP fixtures use only local test servers or fake delivery ports, not live callbacks.

Security tests are consolidated within the existing architecture file budgets:

- `internal/modules/order/domain/refund_fee_test.go`: release boundary evidence alongside refund tests.
- `internal/modules/channelapi/transport/http/handler_test.go`: channel parent/child release isolation.
- `internal/modules/downstreamcallback/infrastructure/callbackclient/client_test.go`: exact wire/signature compatibility and error redaction.
- `internal/modules/downstreamcallback/infrastructure/orderreader/reader_test.go`: parent/child projection release isolation.
- `internal/modules/order/integrationtest/transport/fulfillment_release_test.go`: black-box guest and member download routes, preserving unpaid, independently released child, and historical parent cases without exposing private production helpers.
- `internal/app/jobs/consumer/asynq_worker_test.go`: parent/child evidence matrix, filtered attachment sources, and loopback-only SMTP MIME verification for inline/attachment delivery, stale queued status, instructions/items, recipient and state-specific subject preservation. Existing fulfillment fixtures now explicitly carry truthful delivered order/fulfillment states.
- `internal/modules/upstreamapi/transport/http/upstream_handler_test.go`: parent/child evidence matrix and fallback selection with exact response-schema/value comparisons, including structured delivery data and non-mutation of history.

Verified successfully on the test source:

```sh
go test ./... -count=1
go vet ./...
go test -race ./internal/modules/order/integrationtest/transport -count=1
go test ./internal/modules/downstreamcallback/... ./internal/modules/order/... ./internal/modules/channelapi/... ./internal/modules/apicredential/... ./internal/modules/fulfillment/... ./internal/modules/upstreamapi/... ./internal/upstream ./internal/app/container ./internal/app/jobs/consumer -count=1
go test -race ./internal/modules/downstreamcallback/... ./internal/modules/order/domain ./internal/modules/order/transport/... ./internal/modules/channelapi/transport/http -count=1
go vet ./internal/modules/downstreamcallback/... ./internal/modules/order/domain ./internal/modules/order/transport/... ./internal/modules/channelapi/transport/http
go test ./internal/app/jobs/consumer ./internal/modules/upstreamapi/... ./internal/architecture -count=1
go test -race ./internal/app/jobs/consumer ./internal/modules/upstreamapi/... ./internal/modules/notification/... -count=1
git diff --check
```

The email and synchronous upstream follow-through was verified with failing-then-passing defensive release tests, the full `go test ./... -count=1` suite, `go vet ./...`, architecture tests and the affected race suite above; it does not change the shared helper policy.

This policy does not retrofit every secret-bearing surface: admin privileged reads retain their existing policy and remain a separate follow-up review surface. Authorization/tenant ownership is still the responsibility of the existing caller/repository; this helper is not an ownership check. No cache/CDN behavior or production historical data was inspected.
