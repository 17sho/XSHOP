import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const read = (path: string) => fs.readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8')

const banners = read('views/admin/Banners.vue')
const posts = read('views/admin/Posts.vue')

test('banner list exposes an immediate enable switch backed by the update API', () => {
  assert.match(banners, /const pendingBannerIds = ref\(new Set<number>\(\)\)/)
  assert.match(banners, /if \(pendingBannerIds\.value\.has\(banner\.id\)\) return/)
  assert.match(banners, /const toggleBannerActive\s*=\s*async/)
  assert.match(banners, /adminAPI\.updateBanner\(banner\.id/)
  assert.match(banners, /<Switch[\s\S]*?:model-value="banner\.is_active"[\s\S]*?toggleBannerActive/)
})

test('notification list exposes an immediate publish switch backed by the update API', () => {
  assert.match(posts, /const pendingPostIds = ref\(new Set<number>\(\)\)/)
  assert.match(posts, /if \(pendingPostIds\.value\.has\(post\.id\)\) return/)
  assert.match(posts, /const togglePostPublished\s*=\s*async/)
  assert.match(posts, /adminAPI\.updatePost\(post\.id/)
  assert.match(posts, /<Switch[\s\S]*?:model-value="post\.is_published"[\s\S]*?togglePostPublished/)
})
