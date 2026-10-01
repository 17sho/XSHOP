import test from 'node:test'
import assert from 'node:assert/strict'
import { loader, mountSetup, vue } from './helpers/motion17DHarness.ts'

test('post-list page scrolling delegates reduced motion to the shared policy', () => {
  const load = loader({ 'vue-router': { useRouter: () => ({}) }, '../stores/app': { useAppStore: () => ({ locale: 'en-US' }) }, './usePageSeo': { usePageSeo() {} }, '../api': { postAPI: { list: async () => ({ data: { data: [] } }) } } })
  const f = mountSetup(() => load('composables/usePostList.ts').usePostList('notice', { title: () => 'Fixture', canonicalPath: '/notice' }))
  const original = window.scrollTo; const media = window.matchMedia
  let options: any
  window.scrollTo = (v: any) => { options = v }; window.matchMedia = (() => ({ matches: true })) as any
  try {
    f.state.totalPages.value = 3
    f.state.changePage(2)
    assert.deepEqual(options, { top: 0, behavior: 'auto' })
  } finally { f.unmount(); window.scrollTo = original; window.matchMedia = media }
})
