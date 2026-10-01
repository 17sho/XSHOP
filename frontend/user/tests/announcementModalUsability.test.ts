import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const modal = fs.readFileSync(new URL('../src/components/AnnouncementModal.vue', import.meta.url), 'utf8')
const behavior = fs.readFileSync(new URL('../src/composables/useAnnouncement.ts', import.meta.url), 'utf8')
const lifecycle = fs.readFileSync(new URL('../src/composables/overlayMotionLifecycle.ts', import.meta.url), 'utf8')

test('announcement backdrop closes from the actual painted overlay', () => {
  assert.match(modal, /class="[^"]*announcement-modal-overlay[^"]*"[\s\S]*@click\.self="handleClose"/)
  assert.doesNotMatch(modal, /<div class="fixed inset-0 bg-black\/55[^>]*aria-hidden="true"/)
})

test('announcement modal locks and restores page scrolling', () => {
  assert.match(modal, /useOverlayMotionLifecycle\(\(\) => props\.visible, dialogRef, handleClose, 120\)/)
  assert.match(modal, /@before-enter="beforeEnter" @before-leave="beforeLeave"/)
  assert.match(lifecycle, /document\.body\.style\.overflow = 'hidden'/)
  assert.match(lifecycle, /document\.documentElement\.style\.overflow = 'hidden'/)
  assert.match(lifecycle, /if \(index < 0\) return/)
  assert.match(lifecycle, /if \(!owners\.length\) \{[\s\S]*document\.body\.style\.overflow = bodyOverflow/)
  assert.match(lifecycle, /if \(document\.body\.style\.overflow === 'hidden'\)/)
  assert.match(lifecycle, /if \(document\.documentElement\.style\.overflow === 'hidden'\)/)
  assert.match(lifecycle, /onBeforeUnmount\([\s\S]*release\(\)/)
})

test('announcement modal traps focus and restores the previous focus target', () => {
  assert.match(lifecycle, /owner\.previous = document\.activeElement/)
  assert.match(lifecycle, /event\.key !== 'Tab'/)
  assert.match(lifecycle, /const nodes = focusables\(panel\.value\)/)
  assert.match(lifecycle, /topOwner\(\) !== owner/)
  assert.match(lifecycle, /owner\.previous\?\.isConnected[\s\S]*owner\.previous\.focus\(\)/)
  assert.match(lifecycle, /element\.setAttribute\('inert', ''\)/)
  assert.match(lifecycle, /style\.pointerEvents = 'none'/)
})

test('announcement actions are mobile-safe and content cannot overflow', () => {
  assert.match(modal, /min-h-11/)
  assert.match(modal, /max-h-\[calc\(100dvh-2rem\)\]/)
  assert.match(modal, /overscroll-contain/)
  assert.match(modal, /break-words/)
  assert.match(modal, /pb-\[max\(1\.25rem,env\(safe-area-inset-bottom\)\)\]/)
})

test('today dismissal is an explicit checkbox applied when closing', () => {
  assert.match(modal, /dismissTodayChecked/)
  assert.match(modal, /<Checkbox[^>]*v-model="dismissTodayChecked"/)
  assert.match(modal, /if \(dismissTodayChecked\.value\)[\s\S]*dismissToday\(props\.announcement\.version\)/)
  assert.doesNotMatch(modal, /dismissForever|handleDismissForever/)
})

test('ordinary close is not remembered across reloads or sessions', () => {
  assert.doesNotMatch(behavior, /SESSION_KEY|sessionStorage|closeForSession|mode: 'forever'|dismissForever/)
  assert.match(behavior, /dismiss\.mode === 'today'/)
})
