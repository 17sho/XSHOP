import { describe, expect, it } from 'vitest'
import * as templates from './registrationEmailTemplates'

describe('registration email HTML policy', () => {
  it('builds an inert preview, escapes replacements, disables links and rejects unsafe input', () => {
    const html = templates.registrationPreviewDocument('<p>{{site_name}} {{code}} {{expire_minutes}}</p><a href="https://example.test">site</a>', {
      code: '000000', expire_minutes: '17', site_name: '<img src=x onerror=alert(1)>', site_url: 'https://example.test',
    })
    const doc = new DOMParser().parseFromString(html, 'text/html')
    expect(doc.querySelector('img')).toBeNull()
    expect(doc.body.textContent).toContain('<img src=x onerror=alert(1)> 000000 17')
    expect(doc.querySelector('a')?.hasAttribute('href')).toBe(false)
    const csp = doc.querySelector('meta[http-equiv="Content-Security-Policy"]')?.getAttribute('content')
    for (const directive of ["default-src 'none'", "script-src 'none'", "form-action 'none'", "base-uri 'none'", "navigate-to 'none'"]) expect(csp).toContain(directive)
    expect(templates.registrationPreviewDocument('<script>alert(1)</script>', {})).not.toContain('<script>')
  })
  it('validates every language, required tokens, unknown variables and byte limits', () => {
    const data = { templates: Object.fromEntries(['zh-CN', 'zh-TW', 'en-US'].map(lang => [lang, {
      subject: 'Registration {{site_name}}', body: '{{code}} {{expire_minutes}}', custom_html: '', custom_html_enabled: false,
    }])) } as templates.RegistrationEmailSettings
    expect(templates.validateRegistrationTemplates(data)).toEqual([])
    data.templates['en-US'].body = '{{code}} {{unknown}}'
    expect(templates.validateRegistrationTemplates(data)).toEqual(expect.arrayContaining([
      expect.objectContaining({ lang: 'en-US', reason: 'requiredTokens' }),
      expect.objectContaining({ lang: 'en-US', reason: 'unknownVariables' }),
    ]))
    data.templates['zh-CN'].custom_html_enabled = true
    expect(templates.validateRegistrationTemplates(data)).toContainEqual(expect.objectContaining({ lang: 'zh-CN', reason: 'requiredTokens' }))
    data.templates['zh-TW'].custom_html = '汉'.repeat(70000)
    expect(templates.validateRegistrationTemplates(data)).toContainEqual(expect.objectContaining({ lang: 'zh-TW', reason: 'htmlTooLarge' }))
    data.templates['zh-TW'].subject = ' '
    expect(templates.validateRegistrationTemplates(data)).toContainEqual(expect.objectContaining({ lang: 'zh-TW', reason: 'subjectRequired' }))
    data.templates['zh-TW'].subject = 'Subject\r\nBcc: other@example.test'
    expect(templates.validateRegistrationTemplates(data)).toContainEqual(expect.objectContaining({ lang: 'zh-TW', reason: 'subjectRequired' }))
    data.templates['zh-CN'].body = '{{ code }} {{ expire_minutes }}'
    expect(templates.validateRegistrationTemplates(data)).toContainEqual(expect.objectContaining({ lang: 'zh-CN', reason: 'unknownVariables' }))
  })
  it('accepts inline formatting and text-node variables but rejects active and non-text contexts', () => {
    expect(templates.isSafeRegistrationHTML('<p style="color:#1766ba"><strong>{{code}}</strong> / {{expire_minutes}}</p>')).toBe(true)
    for (const html of [
      '<script>alert(1)</script>', '<body onload="alert(1)"><p>x</p></body>', '<html onclick="alert(1)"><p>x</p></html>', '<meta http-equiv="refresh" content="0;url=https://evil.test">',
      '<p onclick="alert(1)">x</p>', '<svg><a href="javascript:alert(1)">x</a></svg>',
      '<a href="{{site_url}}">x</a>', '<p title="{{code}}">x</p>', '<!-- {{code}} -->', '<{{code}}>x</{{code}}>',
      '<textarea>{{code}}</textarea>', '<style>{{code}}</style>', '<p style="background:image-set(\'https://evil.test\' 1x)">x</p>',
      '<p style="background:u/**/rl(https://evil.test)">x</p>', '<p style="color:\\72 ed">x</p>',
      '<a href="java&#x73;cript:alert(1)">x</a>', '<a href="%2f%2fevil.test">x</a>', '<a href="https://user:pass@evil.test">x</a>',
    ]) expect(templates.isSafeRegistrationHTML(html), html).toBe(false)
  })
})
