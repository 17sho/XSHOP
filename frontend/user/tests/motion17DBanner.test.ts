import test from 'node:test'
import assert from 'node:assert/strict'
import { loader, deferred, mountSetup } from './helpers/motion17DHarness.ts'

test('a late banner response after unmount cannot recreate autoplay', async () => {
  const request = deferred()
  const load = loader({ '../utils/image': { getImageUrl: (v: string) => v }, 'vue-router': { useRouter: () => ({}) }, 'vue-i18n': { useI18n: () => ({ t: (v: string) => v }) }, '../api': { bannerAPI: { list: () => request.promise } }, './useProduct': { useLocalized: () => ({ getLocalizedText: () => '' }) } })
  const fixture = mountSetup(() => load('composables/useBannerCarousel.ts').useBannerCarousel())
  const original = globalThis.setInterval
  let created = 0
  globalThis.setInterval = (() => { created++; return 123 }) as any
  try {
    const pending = fixture.state.loadBanners()
    fixture.unmount()
    request.resolve({ data: { data: [{ image: 'a' }, { image: 'b' }] } })
    await pending
    assert.equal(created, 0)
    assert.deepEqual(fixture.state.banners.value, [])
  } finally { globalThis.setInterval = original }
})
