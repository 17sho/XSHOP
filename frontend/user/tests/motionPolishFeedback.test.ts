import test from 'node:test'
import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'

const source = (file: string) => readFileSync(new URL(`../src/${file}`, import.meta.url), 'utf8')

for (const [dir, top, preserved] of [
  ['views/auth', 'pt-24', '45e77ff5ce237ab1bd443c113ad3f0237d422d41e2556ed326da54bac34195bc'],
  ['templates/vault/auth', 'pt-20', 'dd59db30d71cdca5ca031fce331dafd659379a1e57a7a286fdd0933b05aa9aca'],
]) test(`${dir} Forgot shares mobile top alignment and preserves all policy/inputs/content`, () => {
  for (const name of ['Forgot', 'Login', 'Register']) {
    const shell = source(`${dir}/${name}.vue`).split('\n')[1].match(/class="([^"]+)"/)![1].split(' ')
    for (const token of ['items-start', top, 'pb-28', 'sm:items-center']) assert.ok(shell.includes(token), `${name} missing ${token}`)
    assert.ok(!shell.includes('items-center'))
  }
  // Only the opening shell line is authorized; all policy, inputs and actions stay byte-identical.
  const rest = source(`${dir}/Forgot.vue`).trimEnd().split('\n').slice(2).join('\n')
  assert.equal(createHash('sha256').update(rest).digest('hex'), preserved)
})

test('success feedback uses the original panel-local 450ms bursts and global reduced policy', () => {
  for (const [file, name] of [['ApiPanel', 'new-secret-burst'], ['GiftCardPanel', 'gift-card-success-burst']]) {
    const panel = source(`views/personal/${file}.vue`)
    assert.ok(panel.includes(`animation: ${name} 0.45s ease backwards;`))
    assert.ok(panel.includes(`@keyframes ${name}`))
    assert.match(panel, /transform: translateY\(8px\) scale\(0\.98\)/)
    assert.match(panel, /transform: none;/)
    assert.doesNotMatch(panel, /motion-success-feedback|ease both|forwards/)
  }
  assert.doesNotMatch(source('style.css'), /motion-success-feedback/)
  assert.match(source('style.css'), /prefers-reduced-motion: reduce[\s\S]*animation: none !important/)
})
