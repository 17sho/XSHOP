import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import ts from 'typescript'
import { JSDOM } from 'jsdom'
import { parse, compileScript } from 'vue/compiler-sfc'
const dom = new JSDOM('<body></body>', { url: 'https://synthetic.invalid' })
for (const key of ['window', 'document', 'Element', 'HTMLElement', 'SVGElement', 'Node']) Object.defineProperty(globalThis, key, { configurable: true, writable: true, value: (dom.window as any)[key] })
const vue = await import('vue')
const { harness, response, settle } = await import('./helpers/motion17AHarness.ts')
const { evaluate } = await import('./helpers/sourceRuntime.ts')
function consumer(path: string, bindings: any) {
  const source = fs.readFileSync(new URL('../src/' + path, import.meta.url), 'utf8')
  const { descriptor } = parse(source)
  const script = compileScript(descriptor, { id: 'synthetic-consumer', inlineTemplate: true })
  const ast = ts.createSourceFile(path + '.ts', script.content, ts.ScriptTarget.Latest, true)
  const external: any = {}
  const stub = vue.defineComponent({ setup(_, { slots }) { return () => vue.h('div', {}, slots.default?.()) } })
  for (const statement of ast.statements) if (ts.isImportDeclaration(statement)) {
    const fromVue = (statement.moduleSpecifier as any).text === 'vue'
    const clause = statement.importClause
    if (clause?.name) external[clause.name.text] = bindings[clause.name.text] ?? stub
    if (clause?.namedBindings && ts.isNamedImports(clause.namedBindings)) for (const spec of clause.namedBindings.elements) {
      if (spec.isTypeOnly) continue
      const imported = spec.propertyName?.text || spec.name.text
      external[spec.name.text] = fromVue ? (vue as any)[imported] : bindings[imported] ?? stub
    }
  }
  const printer = ts.createPrinter()
  const body = ast.statements.filter(s => !ts.isImportDeclaration(s)).map(s => printer.printNode(ts.EmitHint.Unspecified, s, ast)).join('\n').replace('export default', 'const Component =')
  const code = ts.transpileModule(body, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.None } }).outputText
  return Function(...Object.keys(external), code + '; return Component')(...Object.values(external))
}
for (const path of ['views/BlogDetail.vue', 'templates/vault/BlogDetail.vue']) test(`actual notice consumer ${path} replaces A with B without remount`, async () => {
  const h = harness('useBlogDetail'); h.unmount(); h.route.name = 'notice-detail'
  Object.defineProperty(globalThis, 'window', { configurable: true, writable: true, value: dom.window })
  const real = evaluate('src/composables/useBlogDetail.ts', ['useBlogDetail'], { ...h.bindings, onMounted: vue.onMounted, onUnmounted: vue.onUnmounted })
  const component = consumer(path, { ...h.bindings, ...real, sanitizeRichHtml: (s: string) => s || '' })
  const host = document.createElement('div'); document.body.append(host)
  const app = vue.createApp(component)
  app.component('RouterLink', { setup(_: any, { slots }: any) { return () => vue.h('a', {}, slots.default?.()) } }); app.mount(host)
  const post = (slug: string) => ({ slug, type: 'notice', title: { 'en-US': `Notice ${slug}` }, content: { 'en-US': `Synthetic body ${slug}` } })
  try {
    h.calls.at(-1).resolve(response(post('A'))); await settle(); assert.match(host.textContent || '', /Notice A/)
    h.route.params.slug = 'B'; await settle(); assert.doesNotMatch(host.textContent || '', /Notice A/)
    h.calls.at(-1).resolve(response(post('B'))); await settle(); assert.match(host.textContent || '', /Notice B/)
    const before = h.calls.length; h.route.query = { tracking: 'same' }; await settle(); assert.equal(h.calls.length, before)
  } finally { app.unmount(); host.remove() }
})
for (const path of ['views/ProductDetail.vue', 'templates/vault/ProductDetail.vue']) test(`actual ${path} reuses instance for A -> B, rendered title/SKU and observer track B`, async () => {
  const h = harness('useProductDetail'); h.unmount()
  Object.defineProperty(globalThis, 'window', { configurable: true, writable: true, value: dom.window })
  let observed = 0
  Object.assign(globalThis, { IntersectionObserver: class { observe() { observed++ } disconnect() {} } })
  const labels = new Proxy({}, { get: (_, key) => key === 'getStockBadgeVariant' ? () => 'default' : () => false })
  const real = evaluate('src/composables/useProductDetail.ts', ['useProductDetail'], { ...h.bindings, onMounted: vue.onMounted, onUnmounted: vue.onUnmounted, useProductLabels: () => labels })
  const component = consumer(path, { ...h.bindings, ...real, sanitizeRichHtml: (s: string) => s || '' })
  const host = document.createElement('div'); document.body.append(host)
  const app = vue.createApp(component)
  app.component('RouterLink', { setup(_, { slots }: any) { return () => vue.h('a', {}, slots.default?.()) } })
  app.mount(host)
  const product = (slug: string) => ({ id: slug === 'A' ? 1 : 2, slug, title: { 'en-US': `Synthetic Product ${slug}` }, fulfillment_type: 'manual', purchase_type: 'guest', images: [], skus: [{ id: slug === 'A' ? 1 : 2, sku_code: `SKU-${slug}`, is_active: true, price_amount: '1.00', manual_stock_total: -1 }] })
  try {
    h.calls.at(-1).resolve(product('A')); await settle()
    assert.match(host.textContent || '', /Synthetic Product A/)
    assert.equal(observed, 1, 'loaded callback attaches actual purchase-actions ref')
    h.route.params.slug = 'B'; await settle()
    assert.doesNotMatch(host.textContent || '', /Synthetic Product A/)
    h.calls.at(-1).resolve(product('B')); await settle()
    assert.match(host.textContent || '', /Synthetic Product B/); assert.doesNotMatch(host.textContent || '', /Synthetic Product A/)
    assert.equal(observed, 2)
  } finally { app.unmount(); host.remove() }
})
