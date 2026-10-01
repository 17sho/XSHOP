export { fixture, installCss, media, rootEnd, end, wait, vue, settle, kinds } from './motionPolishOverlayReversalHarness.ts'
// Longest painted layer owns completion, even when panel motion is shorter.
export const budgets = (kind: string, _width: number) => kind === 'ConfirmDialog' ? [200, 150] : [300, 200]
