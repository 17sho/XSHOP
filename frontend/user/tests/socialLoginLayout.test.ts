import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'

const classic = fs.readFileSync(new URL('../src/views/auth/Login.vue', import.meta.url), 'utf8')
const vault = fs.readFileSync(new URL('../src/templates/vault/auth/Login.vue', import.meta.url), 'utf8')

for (const [name, source] of [['classic', classic], ['vault', vault]] as const) {
  test(`${name} third-party login groups Google and GitHub consistently`, () => {
    assert.match(source, /auth\.login\.socialHint/)
    assert.doesNotMatch(source, /auth\.login\.googleHint/)
    assert.match(source, /Github[^\n]*class="h-5 w-5"/)
    assert.match(source, /auth\.login\.githubButton/)
    assert.match(source, /showGoogleLogin[\s\S]*showGitHubLogin/)
    assert.match(source, /class="social-google-button"/)
    assert.match(source, /Github[^\n]*class="h-5 w-5"/)
    assert.match(source, /bg-\[#202124\]/)
    assert.match(source, /text-\[#e8eaed\]/)
    assert.match(source, /h-10 w-full/)
    assert.match(source, /shape="pill"/)
    assert.match(source, /rounded-full/)
  })
}

const googleButton = fs.readFileSync(new URL('../src/components/auth/GoogleIdentityButton.vue', import.meta.url), 'utf8')
const loginComposable = fs.readFileSync(new URL('../src/composables/useLogin.ts', import.meta.url), 'utf8')
const appShell = fs.readFileSync(new URL('../src/App.vue', import.meta.url), 'utf8')
test('Google button wrapper clips the official control to the shared mobile shape', () => {
  assert.match(googleButton, /social-google-button/)
  assert.match(googleButton, /overflow-hidden/)
  assert.match(googleButton, /h-10 w-full/)
  assert.match(googleButton, /\[&_div\.S9gUrf-YoZ4jf\]:w-full/)
})

test('Google uses the same custom visual shell as GitHub while keeping GIS as the click target', () => {
  assert.match(googleButton, /social-google-visual/)
  assert.match(googleButton, /social-google-native/)
  assert.match(googleButton, /pointer-events-none/)
  assert.match(googleButton, /opacity-0/)
  assert.match(googleButton, /viewBox="0 0 48 48"/)
  assert.match(googleButton, /shape === 'pill' \? 'rounded-full' : 'rounded-md'/)
  assert.match(googleButton, /@click="focusNativeGoogleButton"/)
  assert.match(googleButton, /click\?\.\(\)/)
})

test('returning from a cancelled Google flow clears only the stale widget-load notice', () => {
  assert.match(loginComposable, /window\.addEventListener\('pageshow', handlePageShow\)/)
  assert.match(loginComposable, /event\.persisted/)
  assert.match(loginComposable, /googleWidgetLoadFailed/)
  assert.match(loginComposable, /window\.removeEventListener\('pageshow', handlePageShow\)/)
})

test('custom Google button does not report native GIS rendering failures as form errors', () => {
  assert.match(googleButton, /const isCustomVisualShell = true/)
  assert.match(googleButton, /if \(!isCustomVisualShell\) \{\n\s*emit\('error'/)
})

test('mobile navigation remains available on auth routes without covering the login form', () => {
  assert.match(appShell, /<MobileBottomNav v-if="!isResellerConsole"/)
  assert.doesNotMatch(appShell, /isAuthRoute/)
  assert.match(classic, /pb-28/)
  assert.match(vault, /pb-28/)
})

test('login page clears the fixed top bar before placing its return-home control', () => {
  assert.match(classic, /pt-24[\s\S]*sm:pt-16/)
  assert.match(vault, /pt-20[\s\S]*sm:py-12/)
})
