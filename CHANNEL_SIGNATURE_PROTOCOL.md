# Channel request signatures: query-bound v2

## Scope and migration

This protocol applies **only** to inbound `/api/v1/channel/**` requests using
`Dujiao-Next-Channel-*` authentication headers. Ordinary upstream/downstream API
request and callback signatures are unchanged; do not change their shared
`internal/upstream/signer.go` algorithm or clients.

Channel clients sending **any query string** must migrate to v2 and send exactly
one `Dujiao-Next-Channel-Signature-Version: 2` header. This includes pagination,
locale, catalog filters, wallet/affiliate identity lookups, and both current
`channel_user_id` and legacy `telegram_user_id` identity parameters. There is no
fallback to legacy path-only verification for query requests, even if their JSON
body also contains an identity. Missing/`1` version is accepted only when there is
no query string (not even a bare `?`). Existing no-query signed JSON requests keep
their exact method/path/timestamp/body semantics; v2 also supports those requests.
Unknown versions and repeated authentication headers fail closed with HTTP 401.

Coordinate the server/client rollout in a test environment; older servers do not
understand v2, and after this change old path-only channel query clients receive
401 until upgraded. Preserve the complete query through proxies; the verifier
signs the request target it receives. Never silently retry a failed v2 request
with legacy query signing. No deployment or external bot changes are performed
by this source change.

Repository inspection found no production outbound channel API request signer
or bot client here: the prior channel-signing call is a service test. The real
outbound `downstreamcallback/.../callbackclient` is an **ordinary API callback**
and must not be migrated to this protocol. External channel bots/SDKs must be
updated by their owners. `ChannelSignatureTarget` is the channel-only helper for
Go integrations; it is not a replacement for the global signer.

## Canonical request target

1. Parse the URL query with `url.ParseQuery(requestURL.RawQuery)` and reject any
   error. Do not use `URL.Query()` without validation: it silently discards parse
   errors. Invalid percent escapes and unescaped semicolons are rejected.
2. Reject empty parameter names and any decoded name with other than one value,
   including identical repeats and differently escaped copies of the same name.
3. If both `channel_user_id` and `telegram_user_id` appear, their trimmed values
   must agree; otherwise reject the request. Prefer sending only one identity.
   Both original values, including whitespace, remain in the signed query.
4. Encode the validated values with `url.Values.Encode()` (equivalent to
   `URL.Query().Encode()` after successful validation). Names are sorted,
   parameter values are query-escaped, spaces become `+`, literal `+` becomes
   `%2B`. Include **every** parameter, not an allowlist of today's identity fields.
5. Start with `requestURL.EscapedPath()`. Append `?` plus the encoded query if it
   is nonempty. No scheme, authority, fragment, or trailing empty `?` is included
   in this v2 target. Thus query ordering and equivalent escapes for decoded
   query values are normalized; path escaping is preserved, not decoded into
   potential query separators. Empty values are included as `name=`.

The empty-query normalization in v2 is intentional: a bare `?` has no query
parameters. The legacy migration gate remains stricter and rejects it.

## Signing and verification

Keep the existing HMAC-SHA256 primitive and lowercase hex output. For v2, pass
this **channel-only signing context** as the shared signer's `path` argument:

```text
dujiao-next-channel-v2\n{channel_key}\n{canonical_request_target}
```

The complete UTF-8 HMAC input is therefore (literal LF separators, no final LF):

```text
{method}
dujiao-next-channel-v2
{channel_key}
{canonical_request_target}
{timestamp_decimal}
{lowercase_hex_md5_of_exact_body_bytes}
```

The secret is the channel client's secret. The explicit protocol marker and
channel key prevent v2 signatures from being interpreted as ordinary signatures
or as another channel context, even if credentials are accidentally reused.
The version header selects this format; it is not sufficient to relabel an old
signature. Method, escaped path, all query parameters, timestamp, channel key,
and body digest are authenticated. Signing does not canonicalize JSON: sign the
exact bytes sent. The server restores those bytes for downstream handlers.

Go usage inside this repository (pure signing, no network):

```go
signingTarget, err := channelclientapp.ChannelSignatureTarget(channelKey, "2", requestURL)
if err != nil {
    // Refuse to send an ambiguous/malformed request.
    return err
}
signature := upstream.Sign(channelSecret, method, signingTarget, timestamp, body)
// Send the normal channel key/timestamp/signature headers plus
// Dujiao-Next-Channel-Signature-Version: 2.
```

### Synthetic interoperability vector

These values are test fixtures, not credentials; the timestamp is deliberately
expired and cannot be used as an authenticated request.

- Secret: `fixture-only-secret`
- Channel key: `fixture-key`
- Method: `GET`
- URL: `/api/v1/channel/orders?locale=en%20US&channel_user_id=111`
- Canonical target: `/api/v1/channel/orders?channel_user_id=111&locale=en+US`
- Timestamp: `1700000000`
- Body: zero bytes
- Body MD5: `d41d8cd98f00b204e9800998ecf8427e`
- v2 signature: `3bcadcf13423fa2f603b36f3d36de0abba4655321d3d0c454e52b907e5b89dd5`

The protocol test also fixes the ordinary legacy signer vector for method `GET`,
path `/api/v1/channel/orders`, the same synthetic secret/timestamp and empty body:
`6e94053a777520e9df58284dfc36c0583ab2366aba1af5702d3e27674f08d52e`.
That is a shared primitive compatibility vector, **not** a valid v2 query signature.

## Authority and replay limits

This patch authenticates the identity asserted by the bot; it does not prove the
human operating that bot owns the asserted external identity. Existing storage
resolves external identities globally by provider/provider-user-ID, not by
channel client. **Trusted bots still have channel-wide identity authority.** A
holder of a valid channel secret can sign requests for other identities. Existing
order ownership checks still run against the resolved local user; this patch
does not introduce client/user isolation or order-source scoping. Separate keys
per integration, key rotation/revocation, and secure bot-side user verification
remain necessary. Restrict access to signers; never expose credentials to users.

The existing 60-second timestamp-skew window remains unchanged. There is no
nonce store or one-time signature consumption: an unchanged, still-fresh request
may be retried and is accepted again. Canonical equivalents have the same
signature. Query binding prevents changing authenticated parameters; it is not
full replay prevention, TLS, or rate limiting. TLS and secret/header protection
remain required, and host/origin is not signed. Avoid sharing keys across hosts.
For side-effecting operations, keep application idempotency/retry rules; adding
nonce deduplication requires a durable design that can return the original
result for legitimate retries rather than blindly rejecting them. This patch
deliberately does not add an unreliable process-local replay cache.

## Local verification

Tests use synthetic in-memory credentials, `httptest` requests, and fixture
identity/order ports, with no remote requests, real accounts, or real orders.
They cover legitimate owners and non-owners, current and legacy identity names,
all-query binding, malformed/duplicate/ambiguous query rejection, wrong
method/path/body/channel/protocol/timestamp contexts, legacy JSON compatibility,
fresh retries, expiration, fixed protocol vectors, and unchanged ordinary API
authentication including existing path-only query semantics.

```sh
go test -count=1 ./internal/modules/channelclient/... ./internal/app/httpserver/middleware ./internal/modules/channelapi/... ./internal/upstream ./internal/modules/upstreamapi/...
go test -race -count=1 ./internal/modules/channelclient/application ./internal/app/httpserver/middleware
go test -count=1 ./...
git diff --check
```
