import test from 'node:test'
import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'

const root = new URL('../', import.meta.url)
const read = (path: string) => readFileSync(new URL(path, root), 'utf8')

const removedFrontendFiles = [
  'src/components/SystemUpdateDialog.vue',
  'src/utils/releaseNotes.ts',
  'src/utils/releaseNotes.test.ts',
]

test('retired official updater controls and client calls remain absent', () => {
  const layout = read('src/layouts/AdminLayout.vue')
  for (const path of removedFrontendFiles) {
    assert.equal(existsSync(new URL(path, root)), false, path)
  }

  assert.doesNotMatch(layout, /SystemUpdateDialog|updateCheckOpen|admin\.updateCheck|RefreshCw/)
  assert.doesNotMatch(
    read('src/api/admin.ts'),
    /checkSystemUpdate|getUpdateCapability|getUpdateStatus|startSystemUpdate|rollbackSystemUpdate|restartSystemService/,
  )
  assert.match(layout, /appVersion[\s\S]*payload\?\.app_version/)
  assert.equal((layout.match(/\{\{ appVersion \}\}/g) || []).length, 2)
})

test('retired official updater endpoints remain absent', () => {
  for (const path of [
    '../../internal/platform/http/system/update_handler.go',
    '../../internal/platform/http/system/admin_handler.go',
    '../../internal/platform/http/system/routes.go',
    '../../internal/version/release.go',
    '../../internal/selfupdate',
  ]) {
    assert.equal(existsSync(new URL(path, root)), false, path)
  }
  assert.doesNotMatch(read('../../internal/app/httpserver/routes_admin.go'), /platform\/http\/system|RegisterAdminRoutes\(authorized, system/)
  assert.doesNotMatch(read('../../internal/authz/bootstrap.go'), /system\/version\/check|system\/update|system\/restart/)
})

test('retired official updater translations and dependencies remain absent', () => {
  assert.doesNotMatch(read('src/i18n/index.ts'), /updateCheck:|systemUpdate:|检测更新|一键升级|Check for updates/)
  assert.doesNotMatch(read('package.json'), /"dompurify"|"marked"/)
})

test('server startup remains free of in-process official updater machinery', () => {
  assert.doesNotMatch(read('../../cmd/server/main.go'), /internal\/selfupdate|selfupdate\.|runRollbackCommand|printUpdateStateRecoveryHint/)
  assert.doesNotMatch(read('../../internal/version/version.go'), /BuildType|IsReleaseBuild|self-update|一键升级/)
  assert.doesNotMatch(read('../../internal/i18n/messages.go'), /error\.update_|error\.restart_not_supported/)
  assert.doesNotMatch(read('../../.goreleaser.yaml'), /version\.BuildType|一键升级/)
  assert.doesNotMatch(read('../../Dockerfile'), /version\.BuildType/)
  assert.doesNotMatch(read('../../source-workflows/ci.yml'), /release notes.*sanitizer/i)
})
