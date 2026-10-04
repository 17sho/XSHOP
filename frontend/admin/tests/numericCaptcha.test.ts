import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const captchaTab = readFileSync(new URL('../src/views/admin/components/SettingsCaptchaTab.vue', import.meta.url), 'utf8')
const settings = readFileSync(new URL('../src/views/admin/Settings.vue', import.meta.url), 'utf8')

test('image captcha UI selects digits or alphanumeric and enforces a secure length', () => {
  assert.match(captchaTab, /length:\s*6/)
  assert.match(captchaTab, /character_type:\s*'digits'/)
  assert.match(captchaTab, /v-model="form\.image\.character_type"/)
  assert.match(captchaTab, /SelectItem value="digits"/)
  assert.match(captchaTab, /SelectItem value="alphanumeric"/)
  assert.match(captchaTab, /character_type:\s*form\.image\.character_type/)
  assert.match(settings, /captchaData\.image\.character_type = captchaImage\?\.character_type === 'alphanumeric' \? 'alphanumeric' : 'digits'/)
  assert.match(captchaTab, /form\.image\.length" type="number" min="4" max="8"/)
  assert.doesNotMatch(captchaTab, /form\.image\.length" type="number" min="6"/)
  assert.match(settings, /image:\s*\{\s*character_type:\s*'digits',[\s\S]*?length:\s*6/)
})
