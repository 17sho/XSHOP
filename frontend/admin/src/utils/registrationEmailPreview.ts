import { escapeEmailHTML, renderRegistrationText, type RegistrationLanguage, type RegistrationTemplate, type VerificationScene } from './registrationEmailTemplates'
import { isSafeRegistrationHTML, registrationBodyHTML, sandboxRegistrationDocument } from './registrationEmailHTML'

export type RegistrationBrand = { site_name?: string; site_url?: string; site_logo?: string; site_icon?: string }
// Mirrors smtp/registration_email_template.go and verify_code_card.go. Security copy
// is deliberately not editable. These defaults only select the SMTP card intro;
// they are never used as a fallback for failed settings loads.
export const registrationDefaultBodies: Record<RegistrationLanguage, string> = {
  'zh-CN': '您的验证码是：{{code}}\n\n请使用此验证码完成账号注册，{{expire_minutes}} 分钟内有效。\n\n站点：{{site_name}}\n网址：{{site_url}}',
  'zh-TW': '您的驗證碼是：{{code}}\n\n請使用此驗證碼完成帳號註冊，{{expire_minutes}} 分鐘內有效。\n\n站點：{{site_name}}\n網址：{{site_url}}',
  'en-US': 'Your verification code is: {{code}}\n\nUse this code to finish creating your account. This code expires in {{expire_minutes}} minutes.\n\nSite: {{site_name}}\nURL: {{site_url}}',
}
const copy = {
  'zh-CN': { label: '注册验证', title: '邮箱验证码', intro: '请使用下方验证码完成账号注册。', expiry: '验证码 {minutes} 分钟内有效', warning: '请勿向任何人透露验证码。', ignore: '如果不是您本人操作，请忽略此邮件。', signature: '团队', greeting: '祝好', siteLabel: '站点地址', slogan: '安心选购 · 即时交付', security: '本邮件由系统自动发送。我们不会索要您的密码、支付信息或验证码，请谨防冒充客服的消息。' },
  'zh-TW': { label: '註冊驗證', title: '郵箱驗證碼', intro: '請使用下方驗證碼完成帳號註冊。', expiry: '驗證碼 {minutes} 分鐘內有效', warning: '請勿向任何人透露驗證碼。', ignore: '如果不是您本人操作，請忽略此郵件。', signature: '團隊', greeting: '祝好', siteLabel: '站點地址', slogan: '安心選購 · 即時交付', security: '本郵件由系統自動發送。我們不會索取您的密碼、付款資訊或驗證碼，請留意冒充客服的訊息。' },
  'en-US': { label: 'Registration', title: 'Email verification code', intro: 'Use this code to finish creating your account.', expiry: 'This code expires in {minutes} minutes.', warning: 'Do not share this code with anyone.', ignore: "If you didn't request this, you can ignore this email.", signature: 'Team', greeting: 'Best regards,', siteLabel: 'Store', slogan: 'Shop with confidence', security: 'This is an automated email. We will never ask for your password, payment details, or verification code.' },
}
export const registrationSecurityFooter = (lang: RegistrationLanguage) => [copy[lang].warning, copy[lang].ignore, copy[lang].security].join('\n')

export function registrationPreviewDocument(html: string, variables: Record<string, string>, lang: RegistrationLanguage = 'zh-CN'): string {
  const rendered = isSafeRegistrationHTML(html) ? renderRegistrationText(html, Object.fromEntries(Object.entries(variables).map(([key, value]) => [key, escapeEmailHTML(value)]))) : ''
  // SMTP extracts only parsed body children into a fresh wrapper, then appends the
  // footer outside it. Unclosed tables and document styles cannot swallow it.
  const content = registrationBodyHTML(rendered)
  const footer = escapeEmailHTML(registrationSecurityFooter(lang)).replace(/\n/g, '<br>')
  return sandboxRegistrationDocument(`<!doctype html><html><body><div>${content}</div><p style="display:block;color:#475569;font:13px Arial;line-height:1.8">${footer}</p></body></html>`)
}

function verifiedSiteURL(raw: string): string {
  if (/[\\\u0000-\u001f\u007f]/.test(raw)) return ''
  try {
    const url = new URL(raw.trim())
    return ['https:', 'http:'].includes(url.protocol) && url.hostname && !url.username && !url.password ? raw.trim() : ''
  } catch { return '' }
}

// Mirrors bootstrap/mailbrand.absoluteBrandLogo: the settings form stores local
// root-relative assets, not already resolved SMTP Brand URLs. Never borrow a
// different site's logo or resolve against the administrator's current origin.
export function registrationBrandLogo(brand: RegistrationBrand): string {
  const site = verifiedSiteURL(brand.site_url || '')
  if (!site.startsWith('https://')) return ''
  const resolve = (value: string) => {
    const path = value.trim()
    if (!path.startsWith('/') || path.startsWith('//') || /[\\\u0000-\u001f\u007f]/.test(path)) return ''
    try { return new URL(path, site).href } catch { return '' }
  }
  const logo = resolve(brand.site_logo || '')
  const icon = resolve(brand.site_icon || '')
  const url = new URL(site)
  if (url.hostname === 'shop.example.com' && logo && new URL(logo).pathname === '/xshop-logo-v3.svg') return `${url.origin}/xshop-logo-v3-email.png`
  return [logo, icon].find(candidate => candidate && !candidate.split('?')[0]!.toLowerCase().endsWith('.svg')) || ''
}

export function renderRegistrationEmailPreview(template: RegistrationTemplate, lang: RegistrationLanguage, brand: RegistrationBrand, expireMinutes: number, scene: VerificationScene = 'registration') {
  const minutes = expireMinutes > 0 ? expireMinutes : 10
  const variables = { code: '000000', expire_minutes: String(minutes), site_name: (brand.site_name || '').trim(), site_url: (brand.site_url || '').trim().replace(/\/+$/, '') }
  const body = renderRegistrationText(template.body, variables)
  return {
    subject: renderRegistrationText(template.subject, variables),
    plain: `${body}\n\n${registrationSecurityFooter(lang)}`,
    html: template.custom_html_enabled ? registrationPreviewDocument(template.custom_html, variables, lang) : sandboxRegistrationDocument(renderRegistrationCard(lang, brand, variables, template.body === registrationDefaultBodies[lang] ? undefined : body, scene)),
  }
}

function renderRegistrationCard(lang: RegistrationLanguage, brand: RegistrationBrand, variables: Record<string, string>, configuredBody?: string, scene: VerificationScene = 'registration'): string {
  const labels = {
    'zh-CN': { reset: '重置密码', telegram_bind: '绑定 Telegram', change_email_old: '更换邮箱', change_email_new: '更换邮箱' },
    'zh-TW': { reset: '重置密碼', telegram_bind: '綁定 Telegram', change_email_old: '更換郵箱', change_email_new: '更換郵箱' },
    'en-US': { reset: 'Password reset', telegram_bind: 'Telegram binding', change_email_old: 'Change email', change_email_new: 'Change email' },
  }
  const intros = {
    'zh-CN': { reset: '请使用下方验证码重置密码。', telegram_bind: '请使用下方验证码绑定 Telegram。', change_email_old: '请使用下方验证码更换邮箱。', change_email_new: '请使用下方验证码更换邮箱。' },
    'zh-TW': { reset: '請使用下方驗證碼重置密碼。', telegram_bind: '請使用下方驗證碼綁定 Telegram。', change_email_old: '請使用下方驗證碼更換郵箱。', change_email_new: '請使用下方驗證碼更換郵箱。' },
    'en-US': { reset: 'Use this code to reset your password.', telegram_bind: 'Use this code to bind Telegram.', change_email_old: 'Use this code to change your email.', change_email_new: 'Use this code to change your email.' },
  }
  const c = { ...copy[lang], ...(scene === 'registration' ? {} : { label: labels[lang][scene], intro: intros[lang][scene] }) }
  const clean = escapeEmailHTML
  const name = clean(variables.site_name || '商城')
  const logo = registrationBrandLogo(brand)
  const mark = logo ? `<img src="${clean(logo)}" alt="${name}" width="46" height="46" style="display:block;width:46px;height:46px;border:0;object-fit:contain">` : ''
  const link = clean(verifiedSiteURL(variables.site_url || ''))
  const siteRow = link ? `<p style="margin:10px 0 0;color:#64748b;font-size:12px;line-height:1.6">${c.siteLabel}：<a href="${link}" style="color:#2563eb;text-decoration:none;font-weight:500">${link}</a></p>` : ''
  const intro = configuredBody === undefined ? clean(c.intro) : clean(configuredBody).replace(/\n/g, '<br>')
  const expiry = clean(c.expiry.replace('{minutes}', variables.expire_minutes || '10'))
  return `<!doctype html><html lang="${lang}"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>${clean(c.title)}</title></head><body style="margin:0;padding:24px 12px;background:#f4f7fc;font-family:Arial,'Microsoft YaHei',sans-serif;color:#18243b"><table role="presentation" cellpadding="0" cellspacing="0" style="width:100%;max-width:560px;margin:0 auto;background:#ffffff;border-collapse:separate;border-spacing:0;border-radius:20px;overflow:hidden;box-shadow:0 12px 30px #dce5f4"><tr><td style="padding:32px 24px 38px;text-align:center;background:#2563eb;background-image:linear-gradient(125deg,#3b82f6,#1d4ed8);color:#ffffff"><div style="width:60px;height:60px;border-radius:999px;background:#ffffff;margin:0 auto 17px;text-align:center"><table role="presentation" cellpadding="0" cellspacing="0" style="width:100%;height:60px"><tr><td align="center" valign="middle">${mark}</td></tr></table></div><div style="color:#ffffff;font-size:20px;font-weight:700;line-height:1.5">${name}</div><div style="display:inline-block;margin:15px 0 22px;padding:5px 14px;border-radius:999px;border:1px solid #bfdbfe;color:#ffffff;font-size:12px;letter-spacing:1px">${clean(c.label)}</div><h1 style="margin:0;color:#ffffff;font-size:27px;line-height:1.35">${clean(c.title)}</h1><p style="margin:13px 0 0;color:#dbeafe;font-size:14px;line-height:1.7">${intro}</p></td></tr><tr><td style="padding:31px 27px 34px;background:#ffffff"><table role="presentation" cellpadding="0" cellspacing="0" style="width:100%;background:#172554;border-collapse:separate;border-spacing:0;border-radius:14px"><tr><td style="padding:24px 12px;text-align:center"><div style="color:#bfdbfe;font-size:12px">${clean(c.title)}</div><div style="margin:11px 0;color:#dbeafe;font-size:34px;font-weight:700;letter-spacing:6px;word-break:break-all">${clean(variables.code || '000000')}</div><div style="color:#bfdbfe;font-size:12px;line-height:1.7">${expiry} · ${clean(c.warning)}</div></td></tr></table><p style="margin:27px 0 0;color:#475569;font-size:13px;line-height:1.8">${clean(c.ignore)}</p><div style="margin:20px 0 17px;border-top:1px dashed #cbd5e1"></div><p style="margin:0 0 4px;color:#64748b;font-size:12px;line-height:1.5">${clean(c.greeting)}</p><p style="margin:0;color:#1e293b;font-size:14px;font-weight:600;line-height:1.5">${name} ${clean(c.signature)}</p>${siteRow}</td></tr><tr><td style="padding:29px 25px 25px;text-align:center;background:#172554;color:#cbd5e1"><div style="color:#93c5fd;font-size:15px;font-weight:700">${name}</div><div style="margin:8px 0 17px;color:#93c5fd;font-size:13px">${clean(c.slogan)}</div><p style="margin:0;color:#cbd5e1;font-size:11px;line-height:1.8">${clean(c.security)}</p><p style="margin:15px 0 0;color:#94a3b8;font-size:11px">© ${new Date().getFullYear()} ${name}</p></td></tr></table></body></html>`
}
