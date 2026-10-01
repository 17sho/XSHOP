import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import {
  describeChannelFee,
  estimateChannelFeeAmount,
  hasActualCustomerSurcharge,
  shouldShowChannelFee,
} from '../src/utils/customerFee.ts'
import {
  customerSurchargePayment,
  customerSurchargeRecharge,
  hiddenSurchargeChannel,
  legacyCustomerSurchargePayment,
  merchantPayment,
  visibleSurchargeChannel,
} from './fixtures/paymentFeeFixtures.ts'

const file = (name: string) => readFileSync(new URL(name, import.meta.url), 'utf8')

test('pre-payment channel disclosure is separate from actual post-payment surcharge', () => {
  assert.equal(shouldShowChannelFee(visibleSurchargeChannel), true)
  assert.equal(shouldShowChannelFee(hiddenSurchargeChannel), false)
  assert.equal(shouldShowChannelFee({ ...visibleSurchargeChannel, fee_rate: 0, fixed_fee: 0 }), false)
  assert.equal(shouldShowChannelFee({ ...visibleSurchargeChannel, fee_policy: 'merchant' }), false)
  assert.equal(shouldShowChannelFee(null), false)

  assert.equal(hasActualCustomerSurcharge(customerSurchargePayment), true)
  assert.equal(hasActualCustomerSurcharge(legacyCustomerSurchargePayment), true)
  assert.equal(hasActualCustomerSurcharge(merchantPayment), false)
  assert.equal(hasActualCustomerSurcharge({ fee_policy: 'customer_surcharge', fee_amount: '0.00' }), false)
  assert.equal(hasActualCustomerSurcharge(null), false)
  assert.equal(hasActualCustomerSurcharge({
    fee_policy: customerSurchargeRecharge.fee_policy,
    fee_amount: customerSurchargeRecharge.recharge?.fee_amount,
  }), true)
})

test('shared channel description preserves selector output', () => {
  assert.equal(describeChannelFee(visibleSurchargeChannel), '2.00% + 1.00')
  assert.equal(describeChannelFee({ ...visibleSurchargeChannel, fixed_fee: 0 }), '2.00%')
  assert.equal(describeChannelFee({ ...visibleSurchargeChannel, fee_rate: 0 }), '1.00')
  assert.equal(describeChannelFee(null), '')
})

test('wallet fee estimation preserves cent rounding and invalid amount fallback', () => {
  assert.equal(estimateChannelFeeAmount('100.00', visibleSurchargeChannel), '3.00')
  assert.equal(estimateChannelFeeAmount('0.01', { ...visibleSurchargeChannel, fee_rate: '50.00', fixed_fee: 0 }), '0.01')
  assert.equal(estimateChannelFeeAmount('', visibleSurchargeChannel), '0.00')
  assert.equal(estimateChannelFeeAmount('invalid', visibleSurchargeChannel), '0.00')
})

test('classic and vault checkout plus selector use pre-payment channel visibility', () => {
  for (const name of ['../src/views/Checkout.vue', '../src/templates/vault/Checkout.vue']) {
    assert.match(file(name), /v-if="shouldShowChannelFee\(channel\)"/)
  }
  const selector = file('../src/components/payment/PaymentChannelSelector.vue')
  assert.match(selector, /v-if="shouldShowChannelFee\(channel\)"/)
  assert.match(selector, /describeChannelFee\(channel\)/)
})

test('wallet uses shared channel visibility and fee estimate logic', () => {
  assert.match(file('../src/components/wallet/WalletRechargeForm.vue'), /v-if="shouldShowChannelFee\(selectedChannel\)"/)
  const wallet = file('../src/views/personal/WalletPanel.vue')
  assert.match(wallet, /estimateChannelFeeAmount\(rechargeForm\.amount, selectedChannel\.value\)/)
  assert.doesNotMatch(wallet, /calculateFeeCents/)
})

test('payment and recharge details use actual surcharge predicate', () => {
  assert.match(file('../src/composables/usePayment.ts'), /hasActualCustomerSurcharge\(paymentResult\.value\)/)
  assert.match(file('../src/composables/useRechargeOrderDetail.ts'), /hasActualCustomerSurcharge\(\{[\s\S]*?fee_policy:[\s\S]*?fee_amount:/)
})

test('auto-open input type contains only fields it reads', () => {
  const source = file('../src/utils/paymentResumePolicy.ts')
  const signature = source.match(/export const shouldAutoOpenPaymentLink = \(payment\?: \{([^}]*)\}/)?.[1] || ''
  assert.doesNotMatch(signature, /fee_policy/)
})
