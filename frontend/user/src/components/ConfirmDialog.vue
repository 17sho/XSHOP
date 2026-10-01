<template>
  <Teleport to="body">
    <Transition name="confirm-motion" :duration="transitionDuration(visible)" appear @before-enter="beforeEnter" @before-leave="beforeLeave">
      <div
        v-if="visible"
        data-overlay-root
        class="fixed inset-0 z-[110] flex items-center justify-center bg-black/40 backdrop-blur-sm p-4"
        @click.self="handleCancel"
      >
          <div
            ref="dialogRef"
            data-overlay-panel
            role="alertdialog"
            aria-modal="true"
            tabindex="-1"
            aria-labelledby="confirm-dialog-title"
            class="relative z-10 w-full max-w-sm rounded-2xl bg-card text-card-foreground border shadow-2xl p-6"
          >
            <h3 id="confirm-dialog-title" class="text-lg font-bold text-foreground">{{ options.title }}</h3>
            <p class="mt-2 text-sm text-muted-foreground leading-relaxed">{{ options.message }}</p>
            <div class="mt-6 flex items-center justify-end gap-3">
              <Button variant="secondary" @click="handleCancel">
                {{ options.cancelText || t('common.cancel') }}
              </Button>
              <Button
                :variant="options.variant === 'danger' ? 'destructive' : 'default'"
                @click="handleConfirm"
              >
                {{ options.confirmText || t('common.confirm') }}
              </Button>
            </div>
          </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useOverlayMotionLifecycle } from '../composables/overlayMotionLifecycle'
import { useI18n } from 'vue-i18n'
import { useConfirmDialog } from '../composables/useConfirmDialog'
import { Button } from '@/components/ui/button'

const { t } = useI18n()
const { visible, options, handleConfirm, handleCancel } = useConfirmDialog()

// Vue must wait for the full panel budget: CSS reversal can end the
// independently eased root early. Sample on each render/interaction, not once
// at setup. The visible argument tracks ref-driven visibility outside the
// Transition slot too. Vue remains the only completion/cancellation timer owner.
const transitionDuration = (_visible: boolean) => {
  if (typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return 0
  return { enter: 200, leave: 150 }
}

const dialogRef = ref<HTMLElement | null>(null)
const { beforeEnter, beforeLeave } = useOverlayMotionLifecycle(() => visible.value, dialogRef, handleCancel, 110)
</script>

<style scoped>
.confirm-motion-enter-active { transition:opacity .2s ease-out; }
.confirm-motion-leave-active { transition:opacity .15s ease-in; }
.confirm-motion-enter-active [data-overlay-panel] { transition:transform .2s ease-out; }
.confirm-motion-leave-active [data-overlay-panel] { transition:transform .15s ease-in; }
.confirm-motion-enter-from,.confirm-motion-leave-to { opacity:0; }
.confirm-motion-enter-from [data-overlay-panel] { transform:translateY(8px) scale(.95); }
.confirm-motion-leave-to [data-overlay-panel] { transform:translateY(8px) scale(.95); }
@media (prefers-reduced-motion: reduce) {
  .confirm-motion-enter-active,.confirm-motion-leave-active,.confirm-motion-enter-active [data-overlay-panel],.confirm-motion-leave-active [data-overlay-panel] { transition:none; }
  .confirm-motion-enter-from [data-overlay-panel],.confirm-motion-leave-to [data-overlay-panel] { transform:none; }
}
</style>
