import type { PaymentCreateResult } from '../api/types.ts'
import { amountToCents, calculateFeeCents, centsToAmount, rateToBasisPoints } from './money.ts'

export interface PaymentChannelFee {
  fee_policy?: string
  fee_rate?: unknown
  fixed_fee?: unknown
  show_fee_details?: boolean
}

export type ActualPaymentFee = Pick<PaymentCreateResult, 'fee_policy' | 'fee_amount'>

const isPositive = (value: unknown) => Number(value || 0) > 0

/** Pre-payment disclosure policy for a configured payment channel. */
export const shouldShowChannelFee = (channel?: PaymentChannelFee | null): boolean =>
  channel?.fee_policy === 'customer_surcharge' &&
  channel.show_fee_details !== false &&
  (isPositive(channel.fee_rate) || isPositive(channel.fixed_fee))

/** Post-payment check based on the persisted payment result and charged fee. */
export const hasActualCustomerSurcharge = (payment?: ActualPaymentFee | null): boolean => {
  const policy = String(payment?.fee_policy || '').trim().toLowerCase()
  const customerPays = policy === 'customer_surcharge' || policy === 'legacy_customer_surcharge'
  return customerPays && isPositive(payment?.fee_amount)
}

export const describeChannelFee = (channel?: PaymentChannelFee | null): string => {
  const parts: string[] = []
  const rate = Number(channel?.fee_rate || 0)
  const fixed = Number(channel?.fixed_fee || 0)
  if (rate > 0) parts.push(`${rate.toFixed(2)}%`)
  if (fixed > 0) parts.push(fixed.toFixed(2))
  return parts.join(' + ')
}

export const estimateChannelFeeAmount = (amount: unknown, channel?: PaymentChannelFee | null): string => {
  const amountCents = amountToCents(amount)
  if (amountCents === null || amountCents <= 0) return '0.00'
  const rate = rateToBasisPoints(channel?.fee_rate) || 0
  const fixedFeeCents = amountToCents(channel?.fixed_fee) || 0
  const variableFeeCents = calculateFeeCents(amountCents, rate) || 0
  return centsToAmount(variableFeeCents + fixedFeeCents)
}
