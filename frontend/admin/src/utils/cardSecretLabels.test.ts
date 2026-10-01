import { describe, expect, it } from 'vitest'
import { buildProductLabel, buildSkuLabel, formatSkuSpecValues } from './cardSecretLabels'

describe('shared card secret labels', () => {
  it('formats specifications including empty keys and arrays', () => {
    expect(formatSkuSpecValues(null)).toBe('')
    expect(formatSkuSpecValues({ region: 'US', '': 'standalone', extras: ['one', '', 'two'] } as unknown as Record<string, string>)).toBe('region:US / standalone / extras:one, two')
    expect(formatSkuSpecValues({ region: '' })).toBe('')
  })
  it('preserves SKU code, spec and identifier fallbacks', () => {
    expect(buildSkuLabel(null)).toBe('-')
    expect(buildSkuLabel({ id: 7, sku_code: 'ABC', spec_values: { region: 'US' } } as never)).toBe('ABC · region:US')
    expect(buildSkuLabel({ id: 7, sku_code: '', spec_values: {} } as never)).toBe('#7')
  })
  it('preserves localized product label and fallbacks', () => {
    expect(buildProductLabel(null)).toBe('-')
    expect(buildProductLabel({ id: 8, title: { 'zh-CN': '示例' } } as never)).toBe('#8 示例')
    expect(buildProductLabel({ id: 8, title: {} } as never)).toBe('#8')
  })
})
