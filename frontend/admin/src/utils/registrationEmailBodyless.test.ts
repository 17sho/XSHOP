import { expect, it } from 'vitest'
import { isSafeRegistrationHTML, registrationBodyHTML, registrationHTMLTokensInBody } from './registrationEmailHTML'

it('bodyless frameset documents fail closed without breaking validation or preview', () => {
  for (const html of ['<frameset>', '<html><head><title>draft</title></head><frameset><frame src="https://example.test"></frameset></html>']) {
    expect(() => isSafeRegistrationHTML(html)).not.toThrow()
    expect(isSafeRegistrationHTML(html)).toBe(false)
    expect(registrationHTMLTokensInBody(html, true)).toBe(false)
    expect(registrationBodyHTML(html)).toBe('')
  }
})
