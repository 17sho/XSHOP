import fs from 'node:fs'
import ts from 'typescript'
import * as vue from 'vue'
export { deferred } from './sourceRuntime.ts'

// Execute the real setup/composable body; only browser/API/store boundaries are fixtures.
export function runtime(file: string, names: string[], bindings: Record<string, any> = {}) {
  let source = fs.readFileSync(new URL('../../' + file, import.meta.url), 'utf8')
  if (file.endsWith('.vue')) source = source.match(/<script setup lang="ts">([\s\S]*?)<\/script>/)![1]!
  const ast = ts.createSourceFile(file, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS)
  const printer = ts.createPrinter()
  source = ast.statements.filter(s => !ts.isImportDeclaration(s)).map(s => printer.printNode(ts.EmitHint.Unspecified, s, ast)).join('\n').replace(/export /g, '')
  const code = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.None } }).outputText
  const disposals: Function[] = []
  const mounts: Function[] = []
  const scope = vue.effectScope()
  const all = Object.fromEntries(Object.entries({ ...vue, onMounted: (fn: Function) => mounts.push(fn), onUnmounted: (f: Function) => disposals.push(f), onScopeDispose: (f: Function) => disposals.push(f), ...bindings }).filter(([key]) => /^[A-Za-z_$][\w$]*$/.test(key) && key !== 'default'))
  const result = scope.run(() => Function(...Object.keys(all), code + '\nreturn {' + names.join(',') + '}')(...Object.values(all)))
  return { ...result, mount: () => Promise.all(mounts.map(f => f())), run: (fn: () => any) => scope.run(fn), dispose: () => { disposals.forEach(f => f()); scope.stop() } }
}
