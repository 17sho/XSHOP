import { onBeforeUnmount, watch } from 'vue'
import type { RouteLocationNormalized, Router } from 'vue-router'

const isAuthSwitch = (to: RouteLocationNormalized, from: RouteLocationNormalized) =>
  (to.name === 'user-login' && from.name === 'user-register') ||
  (to.name === 'user-register' && from.name === 'user-login')

type Owner = (to: RouteLocationNormalized, from: RouteLocationNormalized, position: ScrollToOptions) => ScrollToOptions | Promise<ScrollToOptions | false>
const owners = new WeakMap<Router, Owner>()
// Route objects inherit the auth barrier at commit, before queued scroll runs.
// Weak membership survives App disposal without retaining routes or old owners.
const authBarrierRoutes = new WeakSet<RouteLocationNormalized>()

// Keep vue-router responsible for applying the position. Only the auth pair
// waits for Vue's existing out-in boundary, never for a duplicated CSS timer.
export function deferAuthScroll(router: Router, to: RouteLocationNormalized, from: RouteLocationNormalized, position: ScrollToOptions) {
  if (authBarrierRoutes.has(to) && router.currentRoute.value !== to) return false
  return owners.get(router)?.(to, from, position) ?? (isAuthSwitch(to, from) || authBarrierRoutes.has(to) ? false : position)
}

export function useAuthRouteScroll(router: Router) {
  let active = true
  let ready = true
  let pending: ((ready: boolean) => void) | undefined
  const finish = (value: boolean) => {
    const resolve = pending
    pending = undefined
    resolve?.(value)
  }
  // currentRoute changes only on commit: aborted/failed guards cannot steal
  // ownership. Keep an in-flight auth barrier across query-only replacements.
  const stop = watch(router.currentRoute, (to, from) => {
    finish(false)
    ready = !isAuthSwitch(to, from) && (ready || to.path !== from.path)
    if (!ready) authBarrierRoutes.add(to)
  }, { flush: 'sync' })
  const owner: Owner = (to, from, position) => {
    if (!isAuthSwitch(to, from) && ready) return position
    if (!active || router.currentRoute.value !== to) return Promise.resolve(false)
    finish(false)
    const result = ready ? Promise.resolve(true) : new Promise<boolean>(resolve => { pending = resolve })
    return result.then(allowed => allowed && active && router.currentRoute.value === to ? position : false)
  }
  owners.set(router, owner)
  onBeforeUnmount(() => {
    active = false
    stop()
    finish(false)
    if (owners.get(router) === owner) owners.delete(router)
  })
  return {
    beforeEnter() {
      ready = true
      finish(true)
    },
  }
}
