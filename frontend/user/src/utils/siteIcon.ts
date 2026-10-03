type IconConfig = { [key: string]: unknown; brand?: { site_icon?: unknown } } | null
export function siteIconLinks(config: IconConfig): Array<{rel: string; href: string; key: string}> {
 if (!config) return []
 const raw = String(config.brand?.site_icon || '').trim()
 const custom = /^(\/[^/]|https?:\/\/)/i.test(raw) ? raw : ''
 return [
  {rel:'icon',href:custom || '/favicon-v5.svg',key:'site-icon'},
  {rel:'shortcut icon',href:custom || '/favicon-v5.ico',key:'site-shortcut-icon'},
  {rel:'apple-touch-icon',href:custom || '/apple-touch-icon-v5.png',key:'site-touch-icon'},
 ]
}
