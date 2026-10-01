import test from 'node:test'
import assert from 'node:assert/strict'
import {
  getVaultProductCoverClass,
  getVaultStockPresentation,
} from '../src/templates/vault/components/vaultProductPresentation.ts'

const coverClasses = [
  'bg-[linear-gradient(135deg,#7b74f2,var(--red))]',
  'bg-[linear-gradient(135deg,#1cc0bf,var(--teal))]',
  'bg-[linear-gradient(135deg,#9b6cf5,var(--plum))]',
  'bg-[linear-gradient(135deg,#f7bd4e,var(--gold))]',
  'bg-[linear-gradient(135deg,#3a3950,var(--ink))]',
]

const icons = {
  soldOut: 'XCircle',
  lowStock: 'AlarmClock',
  available: 'Zap',
}

for (const [index, expected] of coverClasses.entries()) {
  test(`vault product cover ${index + 1} keeps its exact class`, () => {
    assert.equal(getVaultProductCoverClass(index), expected)
  })
}

test('sold-out stock keeps its muted presentation and explicit sold-out text', () => {
  assert.deepEqual(
    getVaultStockPresentation({
      soldOut: true,
      stockStatus: 'out_of_stock',
      stockLabel: '库存紧张',
      soldOutLabel: '已售罄',
      icons,
    }),
    {
      tone: 'bg-secondary text-muted-foreground',
      icon: 'XCircle',
      label: '已售罄',
    },
  )
})

test('low stock keeps its gold presentation and resolved text', () => {
  assert.deepEqual(
    getVaultStockPresentation({
      soldOut: false,
      stockStatus: 'low_stock',
      stockLabel: '库存紧张（仅剩 2 件）',
      soldOutLabel: '已售罄',
      icons,
    }),
    {
      tone: 'bg-[color:var(--gold-soft)] text-[color:var(--gold-strong)]',
      icon: 'AlarmClock',
      label: '库存紧张（仅剩 2 件）',
    },
  )
})

test('normal stock keeps its teal presentation and resolved text', () => {
  assert.deepEqual(
    getVaultStockPresentation({
      soldOut: false,
      stockStatus: 'in_stock',
      stockLabel: '有库存',
      soldOutLabel: '已售罄',
      icons,
    }),
    {
      tone: 'bg-[color:var(--teal-soft)] text-[color:var(--teal-strong)]',
      icon: 'Zap',
      label: '有库存',
    },
  )
})

test('hidden stock keeps generic resolved text without exposing a quantity', () => {
  const presentation = getVaultStockPresentation({
    soldOut: false,
    stockStatus: 'in_stock',
    stockLabel: '有库存',
    soldOutLabel: '已售罄',
    icons,
  })

  assert.deepEqual(presentation, {
    tone: 'bg-[color:var(--teal-soft)] text-[color:var(--teal-strong)]',
    icon: 'Zap',
    label: '有库存',
  })
  assert.doesNotMatch(presentation.label, /\d/)
})
