# Dependency security maintenance

## Reproducible toolchain

The application build is pinned through `go.mod` to Go 1.26.8. CI and Docker use the matching Go toolchain and Node 24.21.0. Both frontend manifests pin pnpm 10.34.6; install with `pnpm install --frozen-lockfile` from each frontend directory. This does not replace the host's system Go or change a running production binary.

The updated `golang.org/x/net`, `x/crypto`, `x/image` and supporting dependencies, and frontend runtime/build lockfiles, were scanned after updating. `go mod verify` and both offline frozen-lockfile checks passed. Full application release gates are still required after all changes are integrated.

## Scanners are not silently suppressed

At the verification snapshot, `govulncheck` using Go 1.26.8 reported no affected imported package or called symbol. Its sole module-level finding was GO-2026-5932 for the deprecated `golang.org/x/crypto/openpgp` package. This application does not import that package; the module remains necessary for other cryptographic packages. Future additions must not introduce OpenPGP imports without a maintained replacement and review.

Both frontend full `pnpm audit --json` results retained one moderate record: GHSA-cp6q-959q-f8rh for `@tiptap/core`. The advisory's affected range is `>=2.0.0-alpha.0 <3.30.4`, which includes the installed v2.27.3 backport. Do not hide or ignore this record in package-manager configuration.

The registry's v2.27.3 package implements `Object.defineProperty` for the `__proto__` key, rather than invoking the legacy prototype setter. The installed-package regression gate verifies actual behavior, rather than concluding safety from the version number:

```
node scripts/check_frontend_dependency_safety.cjs user
node scripts/check_frontend_dependency_safety.cjs admin
```

The gate fails against cached v2.27.2 with an altered-prototype assertion and passes against the installed v2.27.3 packages. It checks prototype isolation, ProseMirror DOM serialization with inert canary attributes, legitimate class merging, normal rich-text/image/link roundtrips and the storefront sanitizer. CI and tagged release workflows run the gate. No executable attack payload or network request is needed.

Keep the compatible v2 backport rather than silently changing the editor's major version solely to clear a version-range alert. Revisit the advisory and backport on future upgrades. Passing this focused regression does not certify the absence of other editor or application vulnerabilities.

Registry artifact metadata for `@tiptap/core@2.27.3`:
- Tarball: https://registry.npmjs.org/@tiptap/core/-/core-2.27.3.tgz
- Integrity: sha512-a5LfRbLpfaGI3hbL/LPHUYHI0I+FQHdSHsy8L4YnVIuu3hXcm3QZgkWbpEGf8hCz8krk6zEiu0+iFOjTySU2FA==
- Advisory: https://github.com/advisories/GHSA-cp6q-959q-f8rh

## Release provenance

Always commit reviewed source **before** the release build. Run `scripts/check_release_source.py` before and after building. Embed frontends built from the same commit with the admin fullstack script; inspect `go version -m` and require `vcs.revision` to equal the release commit and `vcs.modified=false`. Record artifact hashes. A clean Git HEAD alone does not establish the running binary's provenance.
