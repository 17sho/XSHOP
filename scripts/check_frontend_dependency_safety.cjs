// Offline installed-package regression gate; never fetches or executes supplied markup.
// Usage: node scripts/check_frontend_dependency_safety.cjs [user|admin] [core-module-path]
// The optional module path enables verifying this gate against a cached vulnerable baseline.
const assert = require('node:assert/strict')
const path = require('node:path')
const { createRequire } = require('node:module')
const app = process.argv[2] || 'user'
assert(['user', 'admin'].includes(app), 'invalid frontend')
const appRequire = createRequire(path.resolve(__dirname, '..', 'frontend', app, 'package.json'))
const tiptapRequire = createRequire(appRequire.resolve('@tiptap/extension-image'))
const core = process.argv[3] ? require(path.resolve(process.argv[3])) : tiptapRequire('@tiptap/core')
const { JSDOM } = appRequire('jsdom')
const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'https://fixture.invalid/' })
for (const name of ['window', 'document', 'navigator', 'Node', 'Element', 'HTMLElement', 'MutationObserver', 'DOMParser', 'getComputedStyle']) {
  Object.defineProperty(globalThis, name, {
    value: name === 'getComputedStyle' ? dom.window.getComputedStyle.bind(dom.window) : dom.window[name], configurable: true,
  })
}
try {
  const input = JSON.parse('{"__proto__":{"data-regression-canary":"inherited"},"title":"legitimate"}')
  const attributes = core.mergeAttributes({ class: 'first' }, { class: 'second' }, input)
  assert.equal(Object.getPrototypeOf(attributes), Object.prototype, 'untrusted attribute object changed prototype')
  assert.equal(attributes['data-regression-canary'], undefined, 'inherited attributes escaped own-property boundary')
  assert.equal(attributes.title, 'legitimate')
  assert.equal(attributes.class, 'first second')
  const { Schema, DOMSerializer } = tiptapRequire('@tiptap/pm/model')
  const schema = new Schema({ nodes: { doc: { content: 'image' }, image: { toDOM: () => ['img', attributes] }, text: {} } })
  const doc = schema.node('doc', null, [schema.node('image')])
  const fragment = DOMSerializer.fromSchema(schema).serializeFragment(doc.content)
  assert.equal(fragment.firstChild.getAttribute('data-regression-canary'), null)
  assert.equal(fragment.firstChild.getAttribute('title'), 'legitimate')
  const StarterKit = appRequire('@tiptap/starter-kit').default
  const Image = appRequire('@tiptap/extension-image').default
  const Link = appRequire('@tiptap/extension-link').default
  const editor = new core.Editor({
    element: document.createElement('div'), extensions: [StarterKit, Image, Link],
    content: '<p>fixture <strong>bold</strong> <a href="https://fixture.invalid/help">help</a></p><img src="/uploads/fixture.png">',
  })
  try {
    const html = editor.getHTML()
    assert.match(html, /<strong>bold<\/strong>/)
    assert.match(html, /href="https:\/\/fixture\.invalid\/help"/)
    assert.match(html, /src="\/uploads\/fixture\.png"/)
  } finally { editor.destroy() }
  if (app === 'user') {
    const purify = appRequire('dompurify')
    assert.equal(purify.sanitize('<p>fixture</p><script>inert fixture text</script>'), '<p>fixture</p>')
  }
  console.log(`${app}: installed editor prototype isolation, DOM serialization, normal rich-text roundtrip and sanitizer PASS`)
} finally { dom.window.close() }
