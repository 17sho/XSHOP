import type { PaymentCreateResult, WalletRechargeResult } from '../../src/api/types.ts'
import type { PaymentChannelFee } from '../../src/utils/customerFee.ts'

export const visibleSurchargeChannel: PaymentChannelFee = {
  fee_policy: 'customer_surcharge',
  fee_rate: '2.00',
  fixed_fee: '1.00',
  show_fee_details: true,
}

export const hiddenSurchargeChannel: PaymentChannelFee = {
  ...visibleSurchargeChannel,
  show_fee_details: false,
}

export const customerSurchargePayment: PaymentCreateResult = {
  fee_policy: 'customer_surcharge',
  fee_amount: '3.00',
}

export const legacyCustomerSurchargePayment: PaymentCreateResult = {
  fee_policy: 'legacy_customer_surcharge',
  fee_amount: '3.00',
}

export const merchantPayment: PaymentCreateResult = {
  fee_policy: 'merchant_absorbed',
  fee_amount: '3.00',
}

export const customerSurchargeRecharge: WalletRechargeResult = {
  fee_policy: 'customer_surcharge',
  recharge: {
    id: 1,
    recharge_no: 'WR-FIXTURE',
    amount: '100.00',
    payable_amount: '103.00',
    fee_amount: '3.00',
    currency: 'CNY',
    status: 'pending',
    remark: '',
    created_at: '2026-01-01T00:00:00Z',
  },
}
