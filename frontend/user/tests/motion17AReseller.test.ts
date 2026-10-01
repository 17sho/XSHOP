import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import ts from 'typescript'
import * as vue from 'vue'
import { evaluate, deferred } from './helpers/sourceRuntime.ts'
import { settle, response } from './helpers/motion17AHarness.ts'
for (const exit of ['replace', 'unmount']) test(`actual reseller detail consumer ${exit} ignores old response`, async () => {
  const route = vue.reactive({ params: { order_no: 'A' }, query: {} })
  const pending: any[] = [], mounts: Function[] = [], unmounts: Function[] = []
  const resellerAPI = { orderDetail: (no: string) => { const d = deferred(); pending.push({ no, ...d }); return d.promise } }
  const { useResellerOrders } = evaluate('src/composables/reseller/useResellerOrders.ts', ['useResellerOrders'], { resellerAPI })
  const source = fs.readFileSync(new URL('../src/views/reseller/ResellerOrderDetail.vue', import.meta.url), 'utf8').split('<script setup lang="ts">')[1]!.split('</script>')[0]!
  const ast = ts.createSourceFile('detail.ts', source, ts.ScriptTarget.Latest, true)
  const printer = ts.createPrinter()
  const body = ast.statements.filter(s => !ts.isImportDeclaration(s)).map(s => printer.printNode(ts.EmitHint.Unspecified, s, ast)).join('\n')
  const code = ts.transpileModule(body, { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText
  const bindings = { ...Object.fromEntries(Object.entries(vue).filter(([key]) => /^[A-Za-z_$][\w$]*$/.test(key) && key !== 'default')), resellerAPI, useResellerOrders, useRoute: () => route, useI18n: () => ({ t: String, locale: vue.ref('en-US') }), onMounted: (f: Function) => mounts.push(f), onUnmounted: (f: Function) => unmounts.push(f) }
  const scope = vue.effectScope()
  const state = scope.run(() => Function(...Object.keys(bindings), code + ';return {detail, detailLoading, detailError, reload}')(...Object.values(bindings)))
  const unmount = () => { unmounts.forEach(f => f()); scope.stop() }
  mounts.forEach(f => f())
  try {
    if (exit === 'replace') { route.params.order_no = 'B'; await settle(); assert.equal(pending.length, 2) }
    else unmount()
    pending[0].resolve(response({ order_no: 'A' })); await settle()
    assert.equal(state.detail.value, null)
    if (exit === 'replace') {
      pending[1].resolve(response({ order_no: 'B' })); await settle()
      assert.equal(state.detail.value.order_no, 'B')
      route.query = { tab: 'same' }; await settle(); assert.equal(pending.length, 2)
    }
  } finally { unmount() }
})
