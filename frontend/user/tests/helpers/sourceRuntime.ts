import fs from 'node:fs'
import ts from 'typescript'
import * as vue from 'vue'

// Execute unchanged TypeScript bodies with only external boundaries injected.
export function evaluate(relative: string, names: string[], bindings: Record<string, any> = {}) {
  let source = fs.readFileSync(new URL('../../' + relative, import.meta.url), 'utf8')
  source = source.replace(/import\.meta\.env/g, '{}')
  const ast = ts.createSourceFile(relative, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS)
  const printer = ts.createPrinter()
  source = ast.statements.filter(s => !ts.isImportDeclaration(s)).map(s => printer.printNode(ts.EmitHint.Unspecified, s, ast)).join('\n').replace(/export /g, '')
  const code = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.None } }).outputText
  const all = Object.fromEntries(Object.entries({ ...vue, onMounted: () => {}, onUnmounted: () => {}, ...bindings }).filter(([key]) => /^[A-Za-z_$][\w$]*$/.test(key) && key !== 'default'))
  return Function(...Object.keys(all), code + '\nreturn {' + names.join(',') + '}')(...Object.values(all))
}
export function deferred<T = any>() {
  let resolve!: (value: T) => void
  let reject!: (error: Error) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
