<template>
  <div id="app" class="min-h-screen bg-background text-foreground flex flex-col">
    <!-- vault 模板：自带顶栏/页脚的外壳包裹页面（控制台仍走下方分支） -->
    <VaultLayout v-if="isVault && !isResellerConsole">
      <ErrorBoundary>
        <RouterView v-slot="{ Component }">
          <Transition name="page-fade" mode="out-in" @before-enter="prepareEnteringPage" @before-leave="guardRetiringPage" @leave-cancelled="restoreRetiringPage" @after-leave="restoreRetiringPage">
            <component :is="Component" />
          </Transition>
        </RouterView>
      </ErrorBoundary>
    </VaultLayout>

    <!-- classic 模板 / 分销控制台（保持原有结构不变） -->
    <template v-else>
      <Navbar v-if="!isResellerConsole" />
      <main class="flex-1" :class="isResellerConsole ? '' : 'pb-14 lg:pb-0'">
        <ErrorBoundary>
          <RouterView v-slot="{ Component }">
            <Transition name="page-fade" mode="out-in" @before-enter="prepareEnteringPage" @before-leave="guardRetiringPage" @leave-cancelled="restoreRetiringPage" @after-leave="restoreRetiringPage">
              <component :is="Component" />
            </Transition>
          </RouterView>
        </ErrorBoundary>
      </main>
      <BackToTop v-if="!isResellerConsole" />
      <MobileBottomNav v-if="!isResellerConsole" />
    </template>

    <CustomFooterLinks v-if="!isResellerConsole" />
    <Toast />
    <ConfirmDialog />
  </div>
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthRouteScroll } from './utils/authRouteScroll'

import { getActiveTemplate } from './templates/registry'
import Navbar from './components/Navbar.vue'
import CustomFooterLinks from './components/CustomFooterLinks.vue'

import Toast from './components/Toast.vue'
import ConfirmDialog from './components/ConfirmDialog.vue'
import ErrorBoundary from './components/ErrorBoundary.vue'
import BackToTop from './components/BackToTop.vue'
import MobileBottomNav from './components/MobileBottomNav.vue'

// vault 外壳按需加载，classic 用户不会拉取其 chunk/样式
const VaultLayout = defineAsyncComponent(() => import('./templates/vault/layout/VaultLayout.vue'))

// config 由 router.beforeEach 统一加载，无需在此重复调用
const route = useRoute()
const authScroll = useAuthRouteScroll(useRouter())
function prepareEnteringPage(element: Element) {
  restoreRetiringPage(element)
  authScroll.beforeEnter()
}
const isResellerConsole = computed(() => route.meta.resellerConsole === true)
// getActiveTemplate 读取 appStore.config（响应式），config 加载后会重新计算
const isVault = computed(() => getActiveTemplate() === 'vault')

// Vue disposes the view before its CSS leave removes the retained DOM.
const retiringPages = new Map<Element, () => void>()
function restoreRetiringPage(element: Element) {
  retiringPages.get(element)?.()
  retiringPages.delete(element)
}
function guardRetiringPage(element: Element) {
  if (!(element instanceof HTMLElement) || retiringPages.has(element)) return
  const attributes = ['inert', 'aria-hidden'] as const
  const previous = attributes.map(name => element.getAttribute(name))
  const pointerEvents = element.style.getPropertyValue('pointer-events')
  const pointerPriority = element.style.getPropertyPriority('pointer-events')
  const hadStyle = element.hasAttribute('style')
  const block = (event: Event) => {
    event.preventDefault()
    event.stopImmediatePropagation()
  }
  const events = ['click', 'submit', 'keydown', 'keyup', 'keypress']
  events.forEach(type => element.addEventListener(type, block, true))
  retiringPages.set(element, () => {
    events.forEach(type => element.removeEventListener(type, block, true))
    attributes.forEach((name, index) => {
      if (element.getAttribute(name) !== (name === 'inert' ? '' : 'true')) return
      const value = previous[index]
      if (value == null) element.removeAttribute(name)
      else element.setAttribute(name, value)
    })
    if (element.style.getPropertyValue('pointer-events') === 'none' && element.style.getPropertyPriority('pointer-events') === 'important') {
      if (pointerEvents) element.style.setProperty('pointer-events', pointerEvents, pointerPriority)
      else element.style.removeProperty('pointer-events')
    }
    if (!hadStyle && element.style.length === 0) element.removeAttribute('style')
  })
  const focused = element.ownerDocument.activeElement
  if (focused instanceof HTMLElement && element.contains(focused)) focused.blur()
  element.setAttribute('inert', '')
  element.setAttribute('aria-hidden', 'true')
  element.style.setProperty('pointer-events', 'none', 'important')
}
onUnmounted(() => retiringPages.forEach((_, element) => restoreRetiringPage(element)))
</script>

<style>
.page-fade-enter-active,
.page-fade-leave-active {
  transition: opacity 200ms ease;
}

.page-fade-enter-from,
.page-fade-leave-to {
  opacity: 0;
}
</style>
