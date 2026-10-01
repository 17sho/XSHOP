import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { shouldShowChannelFee } from '../src/utils/customerFee.ts'

const file = (name: string) => readFileSync(new URL(name, import.meta.url), 'utf8')

test('hide zero-fee labels but disclose nonzero customer surcharges', () => {
  assert.equal(shouldShowChannelFee({ fee_policy: 'customer_surcharge', fee_rate: 0, fixed_fee: 0 }), false)
  assert.equal(shouldShowChannelFee({ fee_policy: 'customer_surcharge', fee_rate: '2.00', fixed_fee: 0 }), true)
  assert.equal(shouldShowChannelFee({ fee_policy: 'customer_surcharge', fee_rate: 0, fixed_fee: '1.00' }), true)
  assert.equal(shouldShowChannelFee({ fee_policy: 'customer_surcharge', fee_rate: 2, fixed_fee: 0, show_fee_details: false }), false)
  assert.equal(shouldShowChannelFee({ fee_policy: 'customer_surcharge', fee_rate: 2, fixed_fee: 0, show_fee_details: true }), true)
  assert.equal(shouldShowChannelFee({ fee_policy: 'merchant', fee_rate: 2, fixed_fee: 1 }), false)
})

test('both checkout themes gate the fee summary on actual customer fee', () => {
  for (const name of ['../src/views/Checkout.vue', '../src/templates/vault/Checkout.vue']) {
    const source = file(name)
    assert.match(source, /v-if="shouldShowChannelFee\(channel\)"/)
  }
})

test('payment selector and wallet recharge share fee visibility policy', () => {
  assert.match(file('../src/components/payment/PaymentChannelSelector.vue'), /v-if="shouldShowChannelFee\(channel\)"/)
  const wallet = file('../src/components/wallet/WalletRechargeForm.vue')
  assert.match(wallet, /v-if="shouldShowChannelFee\(selectedChannel\)"/)
  assert.match(wallet, /v-if="Number\(selectedChannel\?\.fee_rate\) > 0"/)
  assert.match(wallet, /v-if="Number\(selectedChannel\?\.fixed_fee\) > 0"/)
  assert.match(file('../src/views/personal/WalletPanel.vue'), /show_fee_details: channel\.show_fee_details !== false/)
})
