import test from 'node:test'
import assert from 'node:assert/strict'
import { loader, vue, settle } from './helpers/motion17DHarness.ts'

test('footer text renders current year as escaped text, coexists with links and hides when blank', async () => {
  const store = vue.reactive({ config: { footer_text: ' © {year} 雪糕数卡 {year} ', footer_links: [] }, locale: 'zh-CN' })
  const Component = loader({ '../stores/app': { useAppStore: () => store } })('components/CustomFooterLinks.vue').default
  const host = document.createElement('div')
  document.body.append(host)
  const app = vue.createApp(Component)
  app.mount(host)
  try {
    assert.equal(host.querySelector('[data-footer-text]')?.textContent, `© ${new Date().getFullYear()} 雪糕数卡 ${new Date().getFullYear()}`)
    store.config.footer_text = '<img src=x onerror=alert(1)> <script>alert(1)</script>'
    store.config.footer_links = [{ name: '支持', url: 'https://example.invalid' }] as any
    await settle()
    assert.equal(host.querySelector('[data-footer-text]')?.textContent, store.config.footer_text)
    assert.equal(host.querySelectorAll('img,script').length, 0)
    assert.equal(host.querySelector('a')?.textContent, '支持')
    store.config.footer_text = '  '
    await settle()
    assert.equal(host.querySelector('[data-footer-text]'), null)
    assert.ok(host.querySelector('footer'))
    store.config.footer_links = []
    await settle()
    assert.equal(host.querySelector('footer'), null)
    for (const invalid of [null, {}, 2026]) {
      store.config.footer_text = invalid as any
      await settle()
      assert.equal(host.querySelector('footer'), null)
    }
  } finally { app.unmount(); host.remove() }
})
