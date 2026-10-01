import test from 'node:test'
import assert from 'node:assert/strict'
import { loader, vue, settle } from './helpers/motion17DHarness.ts'

test('reseller mobile drawer owns focus, Escape and scroll until close or desktop resize', async () => {
  const nil = { render: () => null }
  const load = loader({
    'vue-i18n': { useI18n: () => ({ t: (v: string) => v }) },
    'vue-router': { useRoute: () => ({ path: '/reseller' }) },
    '@/components/ui/button': { Button: 'button' },
    '@/components/ui/popover': { Popover: nil, PopoverTrigger: nil, PopoverContent: nil },
    '../../stores/app': { useAppStore: () => ({ locale: 'en-US', config: {} }) },
    '../../stores/userAuth': { useUserAuthStore: () => ({ isAuthenticated: false }) },
    '../../utils/theme': { useTheme: () => ({ theme: vue.ref('light'), toggleTheme() {} }) },
    '../../utils/image': { getImageUrl: (v: string) => v },
  })
  const host = document.createElement('div'); document.body.append(host)
  const app = vue.createApp(load('components/reseller-console/ResellerConsoleTopbar.vue').default, { title: 'Fixture', navGroups: [] })
  app.component('RouterLink', { template: '<a><slot /></a>' }); app.mount(host)
  const trigger = host.querySelector('button')!
  try {
    trigger.focus(); trigger.click(); await settle()
    assert.equal(document.body.style.overflow, 'hidden')
    const panel = document.querySelector('aside')!
    assert.equal(panel.getAttribute('role'), 'dialog')
    assert.ok(panel.contains(document.activeElement))
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true })); await vue.nextTick()
    assert.equal(document.body.style.overflow, '')
    assert.ok(panel.closest('[inert]'))
    trigger.click(); await settle()
    window.dispatchEvent(new Event('resize')); await settle()
    assert.equal(document.body.style.overflow, '', 'desktop breakpoint must not retain an invisible modal lock')
  } finally { app.unmount(); host.remove() }
})
