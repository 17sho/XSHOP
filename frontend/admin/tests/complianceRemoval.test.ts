import test from 'node:test'
import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'

const root = new URL('../', import.meta.url)
const read = (path: string) => readFileSync(new URL(path, root), 'utf8')

const removedFrontendFiles = [
  'src/components/ComplianceAckDialog.vue',
  'src/components/ComplianceAckedBadge.vue',
  'src/components/ComplianceGuardWrapper.vue',
  'src/views/admin/ComplianceRequired.vue',
  'src/stores/compliance.ts',
]

test('admin frontend has no compliance acknowledgement gate', () => {
  for (const path of removedFrontendFiles) {
    assert.equal(existsSync(new URL(path, root)), false, path)
  }

  for (const path of [
    'src/router/index.ts',
    'src/stores/auth.ts',
    'src/api/client.ts',
    'src/api/admin.ts',
    'src/i18n/index.ts',
  ]) {
    assert.doesNotMatch(read(path), /compliance|Compliance|合规使用声明确认/, path)
  }
})

test('payment and finance pages render without compliance wrappers', () => {
  for (const path of [
    'src/views/admin/PaymentChannels.vue',
    'src/views/admin/Payments.vue',
    'src/views/admin/Wallet.vue',
    'src/views/admin/WalletRecharges.vue',
    'src/views/admin/Reconciliation.vue',
    'src/views/admin/AffiliateWithdraws.vue',
    'src/views/admin/AffiliateCommissions.vue',
    'src/views/admin/ResellerOperationsDashboard.vue',
    'src/views/admin/ResellerLedgerEntries.vue',
    'src/views/admin/ResellerBalanceAccounts.vue',
    'src/views/admin/ResellerWithdraws.vue',
  ]) {
    assert.doesNotMatch(read(path), /ComplianceGuardWrapper/, path)
  }
})

test('backend admin routes have no compliance middleware or endpoints', () => {
  const routes = read('../../internal/app/httpserver/routes_admin.go')
  assert.doesNotMatch(routes, /Compliance|compliance|paymentProtected/)
  assert.doesNotMatch(read('../../internal/authz/bootstrap.go'), /\/admin\/compliance\//)
})
