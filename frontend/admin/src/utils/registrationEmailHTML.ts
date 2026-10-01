import { parse, serialize, Tokenizer, TokenizerMode, type Token, type DefaultTreeAdapterTypes as Tree } from 'parse5'

// Pure-data HTML5 parsing: unlike DOMParser this cannot load images or other resources.
// Keep the full document until after validation; fragment parsing drops body/head attributes.
export function registrationHTMLBody(markup: string): Tree.Element | undefined {
  const doc = parse(markup)
  const root = doc.childNodes.find((node): node is Tree.Element => 'tagName' in node && node.tagName === 'html')
  return root?.childNodes.find((node): node is Tree.Element => 'tagName' in node && node.tagName === 'body')
}
export const registrationBodyHTML = (markup: string) => {
  const body = registrationHTMLBody(markup)
  return body ? serialize(body) : ''
}
const delimiters = /{{|}}/
const forbiddenTags = new Set('script style link iframe object embed form base svg math'.split(' '))
const rawTextTags = new Set('textarea title xmp noembed noframes plaintext'.split(' '))
const urlAttributes = new Set('href src action formaction poster background xlink:href'.split(' '))

function safeURL(value: string): boolean {
  let decoded: string
  try { decoded = decodeURIComponent(value.trim()) } catch { return false }
  if (!decoded || /[\s\\\u0000-\u001f\u007f]/.test(decoded) || decoded.startsWith('//')) return false
  try {
    const url = new URL(decoded, 'https://preview.invalid/')
    return ['http:', 'https:', 'mailto:', 'cid:'].includes(url.protocol) && !url.username && !url.password
  } catch { return false }
}

function bodyText(node: Tree.Node): string {
  if ('tagName' in node && ['head', 'template', 'noscript', ...rawTextTags].includes(node.tagName)) return ''
  if ('value' in node) return node.value
  return 'childNodes' in node ? node.childNodes.map(bodyText).join('') : ''
}

export function registrationHTMLTokensInBody(html: string, requireCode = false): boolean {
  // Substitute source tokens before parsing, as SMTP does. Encoded braces cannot
  // satisfy required variables. Unique markers also identify each occurrence.
  let marker = 'registration-template-marker-'
  const decoded = serialize(parse(html))
  while (html.includes(marker) || decoded.includes(marker)) {
    if (marker.length >= 128) return false
    marker += 'x'
  }
  const tokens: { key: string; marker: string }[] = []
  const replaced = html.replace(/{{(code|expire_minutes|site_name|site_url)}}/g, (_, key: string) => {
    const value = `${marker}${tokens.length}-end`
    tokens.push({ key, marker: value })
    return value
  })
  const body = registrationHTMLBody(replaced)
  if (!body) return false
  const text = bodyText(body)
  return tokens.every(token => text.includes(token.marker)) && (!requireCode || ['code', 'expire_minutes'].every(key => tokens.some(token => token.key === key && text.includes(token.marker))))
}

export function isSafeRegistrationHTML(html: string): boolean {
  if (new TextEncoder().encode(html).length > 200 * 1024) return false
  let safe = true
  const stack: string[] = []
  const raw = (token: Token.Token) => token.location ? html.slice(token.location.startOffset, token.location.endOffset) : ''
  const nonText = (token: Token.Token) => { if (delimiters.test(raw(token))) safe = false }
  const text = (token: Token.CharacterToken) => {
    if (delimiters.test(raw(token)) && (raw(token).includes('<') || rawTextTags.has(stack[stack.length - 1] || ''))) safe = false
  }
  const tokenizer = new Tokenizer({ sourceCodeLocationInfo: true }, {
    onStartTag(token) {
      nonText(token)
      if (forbiddenTags.has(token.tagName)) safe = false
      for (const attr of token.attrs) {
        const value = attr.value.trim()
        if (attr.name.startsWith('on') || delimiters.test(value) || (token.tagName === 'meta' && attr.name === 'http-equiv' && value.toLowerCase() === 'refresh')) safe = false
        if (urlAttributes.has(attr.name) && !safeURL(value)) safe = false
        if (attr.name === 'srcset' && !value.split(',').every(candidate => safeURL(candidate.trim().split(/\s+/)[0] || ''))) safe = false
        if (attr.name === 'style') {
          const compact = value.replace(/[\s\u0000-\u001f\u007f]/g, '').toLowerCase()
          // Retain stricter existing obfuscation checks; never relax CSS to gain roundtrip support.
          if (/\\|\/\*|\*\/|url\(|image-set\(|@|expression\(|behavior:|-moz-binding:/.test(compact)) safe = false
        }
      }
      if (!token.selfClosing) stack.push(token.tagName)
      if (['textarea', 'title'].includes(token.tagName)) tokenizer.state = TokenizerMode.RCDATA
      // Previews have scripting disabled, so noscript children must be checked
      // as markup too; otherwise scripts/CSS can hide in a raw-text token.
      else if (['style', 'xmp', 'iframe', 'noembed', 'noframes'].includes(token.tagName)) tokenizer.state = TokenizerMode.RAWTEXT
      else if (token.tagName === 'script') tokenizer.state = TokenizerMode.SCRIPT_DATA
      else if (token.tagName === 'plaintext') tokenizer.state = TokenizerMode.PLAINTEXT
    },
    onEndTag(token) { nonText(token); if (stack[stack.length - 1] === token.tagName) stack.pop() },
    onComment: nonText, onDoctype: nonText,
    onCharacter: text, onWhitespaceCharacter: text, onNullCharacter: text,
    onParseError(error) { if (error.code === 'duplicate-attribute') safe = false },
    onEof() {},
  })
  tokenizer.write(html, true)
  return safe && registrationHTMLTokensInBody(html)
}

export const registrationPreviewCSP = "default-src 'none'; script-src 'none'; style-src 'unsafe-inline'; img-src 'none'; connect-src 'none'; font-src 'none'; media-src 'none'; object-src 'none'; frame-src 'none'; base-uri 'none'; form-action 'none'; navigate-to 'none'"

export function sandboxRegistrationDocument(markup: string): string {
  // Match the iframe's empty sandbox when interpreting noscript descendants.
  const doc = parse(markup, { scriptingEnabled: false })
  const walk = (node: Tree.Node) => {
    if ('attrs' in node) {
      // An empty sandbox still permits self-navigation. Remove navigation attributes,
      // including less common form/ping routes; CSP blocks all image/network requests.
      node.attrs = node.attrs.filter(attr => !['href', 'action', 'formaction', 'ping', 'target', 'download'].includes(attr.name))
    }
    if ('childNodes' in node) node.childNodes.forEach(walk)
    if ('content' in node) walk(node.content)
  }
  walk(doc)
  return serialize(doc).replace('<head>', `<head><meta http-equiv="Content-Security-Policy" content="${registrationPreviewCSP}">`)
}
