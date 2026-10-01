// Local-only synthetic API fixture for the real dev or production application.
// Run from frontend/user: node tests/browser/frontend-fixture-server.mjs [--built]
// Build first for --built. No backend requests or writes are allowed.
import { createServer, preview } from 'vite'
const title = 'LongUnbrokenFixtureProductTitle'.repeat(12)
const category = { id: 1, slug: 'fixture', name: { 'zh-CN': '测试分类', 'en-US': 'Fixture category' } }
const product = { id: 1, slug: 'fixture', title: { 'zh-CN': title, 'en-US': title }, category_id: 1, category, price_amount: '19.99', stock_status: 'in_stock', fulfillment_type: 'auto', images: [], skus: [] }
const built = process.argv.includes('--built')
function installFixtures(server) {
  server.middlewares.use((req, res, next) => {
    if (!req.url.startsWith('/api/')) return next()
    if (req.method !== 'GET') { res.statusCode = 405; return res.end('Fixture server forbids writes') }
    const url = new URL(req.url, 'http://localhost')
    let data = []
    if (url.pathname.endsWith('/config')) data = { brand: { site_name: 'OFFLINE QA' }, currency: 'CNY', storefront_template: 'vault', product_catalog_layout: /(?:^|;\s*)qa_catalog_layout=card(?:;|$)/.test(req.headers.cookie || '') ? 'card' : 'list', registration_enabled: true, email_verification_enabled: false, personal_center_visibility: Object.fromEntries(['overview','orders','wallet','affiliate','reseller','gift_cards','security','api','profile'].map(k => [k,true])) }
    else if (url.pathname.endsWith('/products')) data = [product]
    else if (url.pathname.endsWith('/categories')) data = [category]
    else if (url.pathname.endsWith('/products/fixture')) data = product
    res.setHeader('Content-Type', 'application/json')
    res.setHeader('Cache-Control', 'no-store')
    res.end(JSON.stringify({ status_code: 0, data, pagination: { page: 1, page_size: 12, total: 1, total_page: 1 } }))
  })
}
const options = {
  // Null removes inherited development proxies rather than merging an empty map.
  server: { host: '127.0.0.1', port: 5197, strictPort: true, hmr: false, proxy: null },
  preview: { host: '127.0.0.1', port: 5198, strictPort: true, proxy: null },
  plugins: [{ name: 'offline-frontend-fixtures', configureServer: installFixtures, configurePreviewServer: installFixtures }],
}
const server = built ? await preview(options) : await createServer(options)
if (!built) await server.listen()
console.log(`FRONTEND_FIXTURE_READY http://127.0.0.1:${built ? 5198 : 5197} (${built ? 'production build' : 'development'})`)
for (const signal of ['SIGTERM', 'SIGINT']) process.on(signal, async () => {
  if (built) server.httpServer.close(() => process.exit(0))
  else { await server.close(); process.exit(0) }
})
