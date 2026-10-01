import { describe, expect, it } from 'vitest'
// Captured from actual smtp.Service.buildVerificationContent via a temporary Go
// test overlay (no backend source edits or mail sends), with synthetic 000000.
import verificationRuntimeCases from './verificationEmailRuntime.fixture.json'
import runtimeCases from './registrationEmailRuntime.fixture.json'
import type { RegistrationLanguage, VerificationScene } from './registrationEmailTemplates'
import { registrationBrandLogo, registrationPreviewDocument, registrationDefaultBodies, registrationSecurityFooter, renderRegistrationEmailPreview } from './registrationEmailPreview'
import { isSafeRegistrationHTML, registrationLanguages, validateRegistrationTemplates, type RegistrationEmailSettings } from './registrationEmailTemplates'

const settings = (html: string): RegistrationEmailSettings => ({ templates: Object.fromEntries(registrationLanguages.map(lang => [lang, { subject: '{{site_name}}', body: '{{code}} {{expire_minutes}}', custom_html: html, custom_html_enabled: true }])) } as RegistrationEmailSettings)

describe('registration renderer parity and isolation', () => {
  it('matches real SMTP renderer for every new purpose/locale/default/edited/advanced and copied registration-body edge', () => {
    const normalize = (html: string) => {
      const doc = new DOMParser().parseFromString(html, 'text/html')
      doc.querySelectorAll('meta[http-equiv="Content-Security-Policy"]').forEach(node => node.remove())
      doc.querySelectorAll('[href]').forEach(node => node.removeAttribute('href'))
      return doc.documentElement.outerHTML.replace(/© \d{4}/g, '© YEAR')
    }
    expect(verificationRuntimeCases).toHaveLength(48)
    for (const sample of verificationRuntimeCases) {
      const actual = renderRegistrationEmailPreview(sample.template, sample.lang as RegistrationLanguage, sample.brand, sample.minutes, sample.scene as VerificationScene)
      const context = `${sample.scene}/${sample.lang}/${sample.mode}`
      expect(actual.subject, context + ' subject').toBe(sample.subject)
      expect(actual.plain, context + ' plain').toBe(sample.plain)
      expect(normalize(actual.html), context + ' HTML').toBe(normalize(sample.html))
    }
  })
  it('matches actual SMTP output for all three locales in default, edited-body and advanced modes', () => {
    const normalize = (html: string) => {
      const doc = new DOMParser().parseFromString(html, 'text/html')
      doc.querySelectorAll('meta[http-equiv="Content-Security-Policy"]').forEach(node => node.remove())
      doc.querySelectorAll('[href]').forEach(node => node.removeAttribute('href'))
      return doc.documentElement.outerHTML.replace(/© \d{4}/g, '© YEAR')
    }
    expect(runtimeCases).toHaveLength(9)
    for (const sample of runtimeCases) {
      const actual = renderRegistrationEmailPreview(sample.template, sample.lang as RegistrationLanguage, sample.brand, sample.minutes)
      expect(actual.subject, `${sample.lang}/${sample.mode} subject`).toBe(sample.subject)
      expect(actual.plain, `${sample.lang}/${sample.mode} plain`).toBe(sample.plain)
      expect(normalize(actual.html), `${sample.lang}/${sample.mode} HTML`).toBe(normalize(sample.html))
    }
  })
  it('removes navigation even in noscript content in the script-disabled sandbox', () => {
    const html = registrationPreviewDocument('<p>{{code}} {{expire_minutes}}</p><noscript><a href="https://example.test">Visit</a></noscript>', { code: '000000', expire_minutes: '17' })
    expect(html).not.toContain('href=')
  })
  it('matches brand resolver and card logo preference without loading any images', () => {
    expect(registrationBrandLogo({ site_url: 'https://shop.example.com', site_logo: '/xshop-logo-v3.svg', site_icon: '/icon.png' })).toBe('https://shop.example.com/xshop-logo-v3-email.png')
    expect(registrationBrandLogo({ site_url: 'https://tenant.test', site_logo: '/xshop-logo-v3.svg', site_icon: '/icon.png' })).toBe('https://tenant.test/icon.png')
    expect(registrationBrandLogo({ site_url: 'https://tenant.test', site_logo: '/logo.png', site_icon: '/icon.png' })).toBe('https://tenant.test/logo.png')
    for (const brand of [
      { site_url: 'https://user:pass@tenant.test', site_logo: '/logo.png' },
      { site_url: 'http://tenant.test', site_logo: '/logo.png' },
      { site_url: 'https://tenant.test', site_logo: '//other.test/logo.png' },
      { site_url: 'https://tenant.test', site_logo: '/\\other.test/logo.png' },
      { site_url: 'https://tenant.test', site_logo: 'https://other.test/logo.png' },
    ]) expect(registrationBrandLogo(brand)).toBe('')
    const preview = renderRegistrationEmailPreview({ subject: '{{site_name}}', body: registrationDefaultBodies['en-US'], custom_html: '', custom_html_enabled: false }, 'en-US', { site_name: '<Tenant>', site_url: 'https://tenant.test', site_logo: '/logo.png' }, 17)
    const doc = new DOMParser().parseFromString(preview.html, 'text/html')
    expect(doc.querySelector('img')?.getAttribute('src')).toBe('https://tenant.test/logo.png')
    expect(doc.querySelector('img')?.getAttribute('alt')).toBe('<Tenant>')
    expect(doc.querySelector('img')?.getAttribute('width')).toBe('46')
    expect(doc.querySelector('meta[http-equiv="Content-Security-Policy"]')?.getAttribute('content')).toContain("img-src 'none'")
    expect(doc.querySelector('[href]')).toBeNull()
    expect(preview.plain.endsWith(registrationSecurityFooter('en-US'))).toBe(true)
    expect(preview.subject).toBe('<Tenant>')
  })
  it('accepts safe image URLs, srcset and full document attributes while enforcing original-source security', () => {
    for (const source of ['https://example.test/a.png', '/uploads/a.png', 'cid:logo@example.test']) {
      const html = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Static title</title></head><body bgcolor="#ffffff"><img src="${source}" alt="Safe" srcset="https://example.test/a.png 1x, https://example.test/b.png 2x"><p>{{code}} {{expire_minutes}}</p></body></html>`
      expect(validateRegistrationTemplates(settings(html))).toEqual([])
    }
    for (const html of [
      '<html onclick="alert(1)"><body>{{code}} {{expire_minutes}}</body></html>',
      '<body><img src="https://safe.test" onerror="alert(1)"><p>{{code}} {{expire_minutes}}</p></body>',
      '<img src="javascript:alert(1)">', '<img src="data:image/svg+xml,bad">', '<img srcset="https://safe.test/a 1x, javascript:alert(1) 2x">',
      '<body style="background:url(https://evil.test)">{{code}} {{expire_minutes}}</body>',
      '<html style="background:image-set(\'https://evil.test\' 1x)"><body>{{code}} {{expire_minutes}}</body></html>',
      '<p title="a > b" onclick="alert(1)">{{code}} {{expire_minutes}}</p>',
      '<img src="https://safe.test" src="javascript:alert(1)">',
      '<head><title>{{site_name}}</title></head><body>{{code}} {{expire_minutes}}</body>',
      '<template>{{code}}</template><p>{{expire_minutes}}</p>',
      '<noscript>{{code}}</noscript><p>{{expire_minutes}}</p>',
      '<template>{{code}}</template><p>&#114;egistration-template-marker-0-end {{expire_minutes}}</p>',
      '<p>{{code}} {{expire_minutes}}</p><!-- {{site_name}} -->',
      '<img alt="{{site_name}}" src="https://safe.test/a">',
      '<noscript><script>alert(1)</script></noscript>',
      '<noscript><p style="background:url(https://evil.test)">x</p></noscript>',
    ]) expect(isSafeRegistrationHTML(html), html).toBe(false)
    for (const html of ['<p>&#123;&#123;code&#125;&#125; {{expire_minutes}}</p>', '<p>{{code}} &#123;&#123;expire_minutes&#125;&#125;</p>']) {
      expect(validateRegistrationTemplates(settings(html))).toContainEqual({ lang: 'en-US', reason: 'requiredTokens' })
    }
  })
})
