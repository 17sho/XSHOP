import assert from 'node:assert/strict'
import test from 'node:test'
import { prefersReducedMotion, scrollPageToTop } from '../src/utils/motionPolicy.ts'

test('motion policy is safe outside the browser and follows live user preference', () => {
  assert.equal(prefersReducedMotion(), false)
  assert.doesNotThrow(scrollPageToTop)
  let reduce = true
  const calls: unknown[] = []
  Object.defineProperty(globalThis, 'window', { configurable: true, value: {
    matchMedia: (query: string) => { assert.equal(query, '(prefers-reduced-motion: reduce)'); return { matches: reduce } },
    scrollTo: (options: unknown) => calls.push(options),
  } })
  try {
    assert.equal(prefersReducedMotion(), true)
    scrollPageToTop()
    reduce = false
    assert.equal(prefersReducedMotion(), false)
    scrollPageToTop()
    assert.deepEqual(calls, [{ top: 0, behavior: 'auto' }, { top: 0, behavior: 'smooth' }])
  } finally { Reflect.deleteProperty(globalThis, 'window') }
})
