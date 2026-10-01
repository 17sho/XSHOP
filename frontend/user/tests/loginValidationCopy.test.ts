import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const zhCN = JSON.parse(fs.readFileSync(new URL('../src/i18n/locales/zh-CN.json', import.meta.url), 'utf8'))
const login = fs.readFileSync(new URL('../src/composables/useLogin.ts', import.meta.url), 'utf8')
const register = fs.readFileSync(new URL('../src/composables/useRegister.ts', import.meta.url), 'utf8')

test('login validation uses field-specific natural Chinese prompts', () => {
  assert.equal(zhCN.formValidation.emailRequired, '请输入邮箱')
  assert.equal(zhCN.formValidation.passwordRequired, '请输入密码')
  assert.match(login, /requiredRule\('formValidation\.emailRequired'\)/)
  assert.match(login, /requiredRule\('formValidation\.passwordRequired'\)/)
})

test('registration uses the same field-specific prompts', () => {
  assert.match(register, /requiredRule\('formValidation\.emailRequired'\)/)
  assert.match(register, /requiredRule\('formValidation\.passwordRequired'\)/)
})
