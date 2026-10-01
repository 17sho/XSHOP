import test from 'node:test'
import assert from 'node:assert/strict'
import { nextTick } from 'vue'
import { setup } from './helpers/motion17BActualStores.ts'

for (const leave of ['dispose', 'provider-replaced', 'provider-replaced-before-flush', 'provider-roundtrip', 'session', 'disabled', 'oidc']) {
  test(`COUNTEREXAMPLE: retained Telegram script error after ${leave} cannot overwrite current notice`, async () => {
    const s = setup()
    try {
      s.app.config.telegram_auth.enabled = true
      s.g.telegramWidgetRef.value = { innerHTML: '', appendChild() {} }
      await s.r.mount(); await nextTick()
      const oldScript = s.scripts.at(-1)
      const retainedError = oldScript.onerror
      const retainedLoad = oldScript.onload
      assert.equal(typeof retainedError, 'function')
      if (leave === 'dispose') s.r.dispose()
      else if (leave === 'session') s.auth.acceptOAuthLogin({ token: 'new-inert-session', user: { id: 2 } })
      else if (leave === 'disabled') s.app.config.telegram_auth.enabled = false
      else if (leave === 'oidc') s.app.config.telegram_auth.mode = 'oidc'
      else s.app.config.telegram_auth.bot_username = 'replacement_bot'
      if (leave !== 'provider-replaced-before-flush') await nextTick()
      if (leave === 'provider-roundtrip') {
        s.app.config.telegram_auth.bot_username = 'fixture_bot'
        await nextTick()
      }
      s.g.error.value = 'current notice'
      // Retain the actual handler, not just the element property or latest global.
      retainedError.call(oldScript, new Event('error'))
      retainedLoad?.call(oldScript, new Event('load'))
      assert.equal(s.g.error.value, 'current notice')
      assert.equal(s.calls.length, 0)
      assert.equal(s.pushes.length, 0)
      if (leave !== 'session' && leave !== 'provider-replaced-before-flush') {
        assert.equal(oldScript.onerror, null, 'clear must detach error handler too')
        assert.equal(oldScript.onload, null, 'clear must detach any load handler too')
      }
    } finally { s.dispose() }
  })
}

test('current and replacement Telegram script errors still display; typing does not obsolete the widget', async () => {
  const s = setup()
  try {
    s.app.config.telegram_auth.enabled = true
    s.g.telegramWidgetRef.value = { innerHTML: '', appendChild() {} }
    await s.r.mount(); await nextTick()
    let script = s.scripts.at(-1)
    s.g.email.value = 'changed@example.invalid'
    script.onerror()
    assert.equal(s.g.error.value, 'auth.login.telegramWidgetLoadFailed')
    s.app.config.telegram_auth.bot_username = 'replacement_bot'
    await nextTick()
    const replacement = s.scripts.at(-1)
    assert.notEqual(replacement, script)
    s.g.error.value = 'current notice'
    replacement.onerror()
    assert.equal(s.g.error.value, 'auth.login.telegramWidgetLoadFailed')
    // Telegram has no application onload continuation; success does not clear notices.
    assert.equal(typeof replacement.onload === 'function', false)
    assert.equal(s.calls.length, 0)
  } finally { s.dispose() }
})
