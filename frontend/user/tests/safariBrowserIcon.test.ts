import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8')

test('Safari icon links follow saved branding without competing static defaults', () => {
 assert.doesNotMatch(html, /<link rel="(?:icon|shortcut icon|apple-touch-icon|manifest)"/)
 const source = fs.readFileSync(new URL('../src/utils/siteIcon.ts', import.meta.url), 'utf8')
 assert.match(source, /site_icon/)
 assert.match(source, /apple-touch-icon/)
 assert.match(source, /favicon-v5\.svg/)
})
