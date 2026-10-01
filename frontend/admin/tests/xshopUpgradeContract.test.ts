import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
const root = new URL('../', import.meta.url)
const read = (p: string) => readFileSync(new URL(p, root), 'utf8')
test('XSHOP custom updater is separately registered behind JWT/RBAC and requires super admin', () => {
 const routes = read('../../internal/app/httpserver/routes_admin.go')
 assert.match(routes, /authorized := admin.Use\(middleware.JWTAuthMiddleware[\s\S]*middleware.AdminRBACMiddleware[\s\S]*xshophttp.Register\(authorized/)
 assert.match(read('../../internal/xshophttp/routes.go'), /admin_is_super/)
 assert.doesNotMatch(read('../../internal/authz/bootstrap.go'), /xshop-upgrade/)
})
test('Chinese upgrade panel rechecks permission in actions and requires confirmation', () => {
 const panel = read('src/components/XshopVersionBadge.vue')
 assert.match(panel, /XSHOP 在线升级/)
 assert.match(panel, /authStore.isSuper/)
 assert.match(panel, /if \(!authStore.isSuper/)
 assert.match(panel, /window.confirm/)
 assert.match(panel, /\/admin\/xshop-upgrade\//)
 assert.match(read('src/router/index.ts'), /XshopUpgrade.vue/)
 const layout = read('src/layouts/AdminLayout.vue')
 assert.match(layout, /<header[\s\S]*<XshopVersionBadge :version="appVersion"[\s\S]*<\/header>/)
 assert.doesNotMatch(layout, /id: 'xshop-upgrade'/)
 assert.match(layout, /addEventListener\('appversionrefresh'/)
 assert.match(read('src/views/admin/XshopUpgrade.vue'), /<XshopVersionBadge/)
 assert.match(panel, /api.post\('\/admin\/xshop-upgrade\/restart', undefined, \{ expectedRestartUntil: restartUntil \}\)/)
 assert.match(panel, /fixed right-4 top-16 sm:absolute sm:right-0 sm:top-auto/)
 assert.match(panel, /max-h-\[calc\(100dvh-5rem\)\]/)
 assert.doesNotMatch(panel, /github_pat_|ghp_|gho_|Authorization.*token/i)
})
