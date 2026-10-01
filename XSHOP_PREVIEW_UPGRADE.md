# XSHOP preview upgrade candidate — independent review required

This custom updater is **not** the removed upstream self-updater. It is a separate
root-owned helper and a super-administrator-only proxy. Publishing, installing the
helper, bootstrapping the real preview, and production changes are separate release
operations. This source snapshot is a candidate, not deployment approval.

## Trust and privilege contract

- Product is exactly `XSHOP`; repository is exactly private `17sho/XSHOP`.
- Download authority lives only in a separate root helper. It uses the server's
  existing `/usr/bin/gh` credential backend, never a browser token, app setting,
  archive credential, arbitrary caller URL, shell command, or upstream repository.
  The existing root GitHub credential may have broader scope than this helper's
  protocol; a separately scoped repository read credential is preferable for later
  permanent operation. No new credential scopes are requested by this candidate.
- Signed Ed25519 manifest bytes are authenticated before strict recursive JSON
  parsing. Sequence is a durable root-owned high-water fence. A failed attempted
  install consumes its sequence, so republish a reviewed higher sequence to retry.
- Only `linux/amd64`, `embedded-preview`, channel `preview`, `minimum_updater: 1`,
  unchanged schema fingerprint, and an exact allowed current binary hash are
  accepted. Production/split profiles are refused. Signer and root policy are
  trusted; this is not a sandbox for malicious approved releases.
- Package format: one regular USTAR member named `dujiao-next`, mode 0755, exact
  signed length/hash; zero member padding; exactly two zero end blocks; one gzip
  stream; no PAX/GNU metadata, paths, links, extra members or appended streams.
  A package cannot write configuration, database, uploads or the helper itself.
- Local limits: manifest 1 MiB, signature exactly 64 bytes, binary archive 128 MiB,
  corresponding source 64 MiB, binary member 256 MiB. Downloads have subprocess
  deadlines and stream byte caps. Private download requests are fixed gh API
  endpoints, selected from repository release asset IDs, not manifest URLs.
- Source is downloaded, hash verified and retained in root-only helper storage
  before activation. Source authenticity does not itself establish GPL completeness;
  the release gate must check the source archive, LICENSE and build inputs.
- Application JWT and RBAC run first; an additional real `admin_is_super` and
  nonzero admin identity guard denies ordinary roles, even wildcard grants.
- Unix socket is root:dujiao 0660 in root-owned `/run/xshop-preview-upgrader`.
  Kernel SO_PEERCRED UID must be `dujiao` and peer cgroup must be exactly
  `0::/system.slice/dujiao-preview.service`. Production shares the UID but its
  different cgroup is denied. No application socket listener or new public port.
- Helper executable/policy/ancestors must be root-owned, nonsymlink,
  non-group/world-writable. Target binary must be root-owned at bootstrap.
  Runtime data directories retain their existing service-user ownership.
- Root helper has exclusive flock and serializes HTTP jobs. A service-managed
  runtime directory removes stale sockets on restart. Journal files are atomic,
  fsynced, mode 0600, in root-owned mode-0700 persistent state storage.
- Helper systemd proposal has `NoNewPrivileges`, `ProtectSystem=strict`,
  restricted capability/AF sets, `ProtectHome=read-only`, private tmp, task/memory
  bounds, and a read-only production directory. Its narrowly scoped installer
  code can only stop/start `dujiao-preview.service` and atomically replace its
  fixed executable. The mount-level writable allowance covers the preview root
  because a file-only bind mount prevents atomic rename; inspect this privilege
  tradeoff independently. **Do not loosen preview's egress restrictions.**

## Protocol

App routes, behind the existing authenticated administrator group:

- `GET /api/v1/admin/xshop-upgrade/status`, empty body.
- `POST /api/v1/admin/xshop-upgrade/check`, empty body, asynchronously checks the
  repository's latest **published non-prerelease** release. Tag must begin
  `xshop-preview-` and match signed manifest version.
- `POST /api/v1/admin/xshop-upgrade/install`, exact canonical JSON
  `{"digest":"<64 lowercase hex>"}` from a completed check. No extra fields,
  duplicate aliases, query parameters, URLs, filenames, commands or service names.

The helper equivalents are `/status`, `/check`, `/install`. Accepted jobs return
202 immediately; app wraps them in its usual response envelope. UI polls status
and tolerates preview restart. Install confirmation rechecks permission/digest.
Helper intentionally offers no HTTP restart, initialize, arbitrary rollback,
URL override, key/config edit, credential delivery or production operation.

## Data and recovery

Downloads and verification finish before stopping writers. The exact old binary
is retained, high-water/pending recovery journal is committed, then only preview
is stopped. Require MainPID zero and an empty cgroup. Save configuration and a
consistent SQLite backup. Stage/switch executable atomically, start preview, and
require both running `/proc/<MainPID>/exe` hash and real JSON health.

On failure or pending-journal recovery, stop writers, restore only the
hash-verified compatible old binary, start and verify it. **Never restore a database
checkpoint after any service-start attempt.** Configuration/uploads are not
modified. Failed recovery stays pending and refuses subsequent installs.
A/B candidates intentionally share one reviewed source/schema commit and differ
only by version ldflags. Rehearsal uses fresh synthetic data, not live preview
credentials, transactions or stock. Retained checkpoints are root-only.

## Build and independent-review commands

Use the Go toolchain required by go.mod and Node/pnpm required by each package.
All output artifacts should be outside the Git worktree. Start from a cold clean
copy; keep workflows in `source-workflows/` inactive.

```sh
(cd frontend/user && pnpm install --offline --frozen-lockfile)
(cd frontend/admin && pnpm install --offline --frozen-lockfile)
node scripts/check_frontend_dependency_safety.cjs user
node scripts/check_frontend_dependency_safety.cjs admin
(cd frontend/user && pnpm build)
(cd frontend/admin && pnpm build:fullstack)
(cd frontend/admin && pnpm exec vitest run --maxWorkers=1)
(cd frontend/admin && node --experimental-strip-types --test --test-concurrency=1 tests/*.test.ts)
(cd frontend/user && node --experimental-strip-types --test --test-concurrency=1 tests/*.test.ts)
python3 scripts/check_frontend_assets.py frontend/user/dist frontend/admin/dist
CGO_ENABLED=1 go test ./...
go vet ./...
CGO_ENABLED=1 go test -race ./internal/customupgrade ./internal/xshopupgrade ./internal/xshophttp
```

Copy user/admin dist into `internal/web/dist/user` and `internal/web/dist/admin`,
then, after committing and checking source is clean:

```sh
CGO_ENABLED=0 go build -trimpath -tags release,fullstack -ldflags '-X github.com/dujiao-next/internal/version.Version=xshop-preview-a1' -o "$ARTIFACTS/A/dujiao-next" ./cmd/server
CGO_ENABLED=0 go build -trimpath -tags release,fullstack -ldflags '-X github.com/dujiao-next/internal/version.Version=xshop-preview-b1' -o "$ARTIFACTS/B/dujiao-next" ./cmd/server
CGO_ENABLED=0 go build -trimpath -o "$ARTIFACTS/xshop-preview-upgrader" ./cmd/xshop-preview-upgrader
python3 scripts/package_xshop_candidates.py . "$ARTIFACTS" "$PRIVATE_ED25519_KEY"
CGO_ENABLED=1 XSHOP_REHEARSAL_DIR="$ARTIFACTS" XSHOP_REHEARSAL_EVIDENCE="$EVIDENCE" go test -count=1 -v ./internal/xshopupgrade -run '^TestActualCandidateChain$'
go version -m "$ARTIFACTS/A/dujiao-next"
go version -m "$ARTIFACTS/B/dujiao-next"
go version -m "$ARTIFACTS/xshop-preview-upgrader"
```

The actual candidate ledger, executable evidence, source tar and hashes are
reported separately. Verify `vcs.modified=false` and exact source revision, full
frontend tree hashes, signed archive/source identities, safe source contents,
installer failure modes, authorization, preserved schemas and unchanged live
controls. Initial interrupted/concurrent tests do not replace final frozen gates.
The retired-updater tests retain **all original negative assertions**. Only their
descriptions distinguish retired machinery from XSHOP; their CI path is adjusted
to the already-reviewed inactive `source-workflows/ci.yml` relocation. No upstream
updater is restored and no role matrix wildcard is added.

## Parent/operator actions only AFTER independent acceptance

1. Read back private `17sho/XSHOP` and disabled Actions. Audit this exact source
   commit and Corresponding Source archive, then publish source and the B binary
   archive/source archive/manifest.json/manifest.sig and license as one matching
   release. Release tag `xshop-preview-b1` must be published **non-prerelease** so
   the private `/releases/latest` channel can find it. Never publish the signing
   private key, runtime config, test database, logs or local control baseline.
2. Read remote commit and release asset metadata back; download every asset via
   the same private gh path and compare exact local hashes. This remote path is
   necessarily unexercised until publication is independently authorized.
3. Preserve existing preview config/database/uploads/static asset roots and its
   no-egress unit unchanged. Rehearse these exact artifacts on a consistent
   preview database copy under isolation. Check frontend ingress: fullstack embeds
   both SPAs, but any separately filesystem-served admin must be bootstrapped with
   the approved matching admin dist to expose the new panel.
4. Back up only preview, then bootstrap A with its root-owned executable. Do not
   copy fixture data/settings or introduce production changes. Confirm all live
   protected-row fingerprints, config/uploads, origin/public health and A running
   `/proc` hash; retain a usable old-program checkpoint.
5. Install helper at `/usr/local/libexec/xshop-preview-upgrader`, root:root 0755;
   policy at `/etc/xshop-preview-upgrader/policy.json`, root:root 0600. Create
   root-owned `/var/lib/xshop-preview-upgrader` 0700 and runtime directory 0755.
   Policy fields: public key from `public-key.txt`, schema fingerprint from
   `schema-fingerprint.txt`, exact existing preview SQLite path beneath
   `/opt/dujiao-preview/db`, actual loopback port, `bootstrap_sequence: 1`.
   No private signing key is installed in the helper or app.
6. Run the installed helper once as root with `--initialize` (refuses existing
   state), then install/start the independently reviewed systemd proposal. Normal
   daemon start **never recreates missing high-water state**. Restart recovery
   preserves sequence and never rewinds the database.
7. Verify socket mode, SO_PEERCRED/cgroup positive preview and negative production
   boundary, real super-admin status/check and ordinary-admin/revoked-token denial.
   Root curl to the Unix socket is intentionally denied; use authenticated app
   routes. Confirm preview IPAddressDeny remains intact and all other services
   retain exact control fingerprints. Leave A running with B available; **do not
   click install on the user's behalf**. User then performs A→B in the Chinese
   XSHOP online-upgrade panel.

Publication, privileged deployment, live peer positive-path testing, real-preview
data acceptance and public UI checks are parent release gates, not implied by
synthetic candidate rehearsal. No live provider/payment/mail delivery is tested.
