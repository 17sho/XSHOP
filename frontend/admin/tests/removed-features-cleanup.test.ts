import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const read = (relativePath: string) =>
  readFileSync(new URL(`../src/${relativePath}`, import.meta.url), 'utf8')

test('removed Telegram Bot admin feature leaves no dead API, DTO, or navigation copy', () => {
  const adminApi = read('api/admin.ts')
  const types = read('api/types.ts')
  const messages = read('i18n/index.ts')

  assert.doesNotMatch(adminApi, /getChannelClients|createChannelClient|getChannelClient|updateChannelClient|resetChannelClientSecret/)
  assert.doesNotMatch(types, /AdminTelegramBotRuntimeStatus|AdminTelegramBroadcast|AdminTelegramBroadcastUser/)
  assert.doesNotMatch(messages, /telegramBot(?:Overview|Settings|HelpCenter|MenuSettings|Status|ChannelClients|Broadcasts)/)
})

test('notice-only admin editor does not carry removed blog relations', () => {
  const posts = read('views/admin/Posts.vue')
  const adminApi = read('api/admin.ts')

  assert.doesNotMatch(posts, /category_id:\s*null as|product_ids:\s*\[\] as/)
  assert.match(posts, /type:\s*'notice',[\s\S]*category_id:\s*null,[\s\S]*product_ids:\s*\[\]/)
  assert.doesNotMatch(adminApi, /getPostRelatedProducts/)
})

test('navigation settings expose only the retained notification destination', () => {
  const navigation = read('views/admin/components/SettingsNavigationTab.vue')
  assert.match(navigation, /builtin:\s*\{\s*notice:\s*true/)
  assert.doesNotMatch(navigation, /builtin\.blog|builtin\.about|\['blog', 'notice', 'about'\]/)
})

test('retained Telegram login, notification, and payment admin capabilities stay available', () => {
  const adminApi = read('api/admin.ts')

  assert.match(adminApi, /getTelegramAuthSettings/)
  assert.match(adminApi, /unbindUserTelegram/)
  assert.match(adminApi, /getNotificationCenterSettings/)
  assert.match(adminApi, /getPaymentChannels/)
})
