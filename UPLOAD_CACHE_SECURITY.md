# Upload and sensitive-response hardening

Test-site source changes only; no deployment or production validation.

## Upload policy

- All scenes require an extension matching a positively sniffed supported content type. Empty configured allowlists no longer mean arbitrary file formats are accepted.
- Normal scenes still enforce configured extension and MIME allowlists.
- Telegram retains supported binary attachments beyond the image-only configuration: ZIP, gzip, RAR, PDF, MP3, WAV, Ogg, MP4, WebM and AVI. No generic `.bin`/octet-stream, HTML/XHTML, JavaScript, CSS, or plain-text exception. ZIP contents are not extracted or malware-scanned; recognizing a container does **not** certify its contents safe to execute after download.
- PNG/JPEG/GIF/WebP keep image dimension validation. SVG retains script/event/protocol checks and additionally requires a well-formed single passive SVG root, SVG-only elements, no SMIL mutations, no external resource links, and no dynamic/external CSS. Ordinary shapes, filters, gradients, local references and inline styling remain supported. SVGs with external images/fonts, animation, foreign namespaces, CSS escapes/at-rules must be converted to passive artwork before upload.
- The old archive test used plain text named `.zip`; it now uses a real locally generated ZIP fixture.

## Static delivery

`registerUploadRoutes` preserves `/uploads/...` paths and GET/HEAD handling. All responses receive `nosniff` and a sandboxed restrictive CSP. Raster formats retain fixed image MIME types and inline delivery; SVG retains its image MIME plus attachment policy. Other extensions (including legacy unsafe uploads) receive `application/octet-stream` and forced attachment. CSP permits inline styling for normal SVG graphics but no scripts, objects, forms, base changes or external resources.

Any proxy/CDN that serves `/uploads` directly instead of forwarding to Gin must independently enforce the same headers. No proxy configuration or live storage was inspected or changed. Existing browser/client copies are not revoked.

## Sensitive API cache policy

`SensitiveResponseCacheMiddleware` is attached centrally to `/api/v1` before route authentication. It sets `Cache-Control: private, no-store` before any response bytes for registered order, fulfillment, card, procurement and payment route families, including guest browser-order summaries, member/guest downloads, admin exports, reseller order views, and channel/upstream order responses. Errors are protected as well. Exact route-segment matching leaves public catalog/content/configuration, upstream/channel catalog, and unrelated endpoints untouched.

This prevents future compliant caches from storing these responses; it does not erase already downloaded files or previously cached responses, and depends on intermediaries honoring response headers.

## Local verification

- Upload tests cover rejected active/unknown/disguised formats, SVG structure/reference/CSS attacks, real PNG/JPEG/GIF/WebP images, passive SVGs, real ZIP and PDF-signature attachments, and ordinary-scene policy enforcement.
- Static tests use a temporary fixture-only directory and verify GET/HEAD bytes, MIME, disposition, `nosniff`, and CSP for normal images and legacy active files.
- Cache tests inspect committed response headers for success and authorization/error responses, all relevant route families and methods, and unchanged public-cache policies.
- Existing per-directory architecture file budgets are preserved by consolidating tests into existing test files.
