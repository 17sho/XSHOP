<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useOverlayMotionLifecycle } from '../composables/overlayMotionLifecycle'
import { useI18n } from 'vue-i18n'
import { AlertTriangle, CheckCircle2, Info, Megaphone, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { useLocalized } from '../composables/useProduct'
import { sanitizeRichHtml } from '../utils/richContent'
import { useAnnouncement, type HomeAnnouncement } from '../composables/useAnnouncement'

const props = defineProps<{
  announcement: HomeAnnouncement
  visible: boolean
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
}>()

const { t } = useI18n()
const { getLocalizedText } = useLocalized()
const { dismissToday } = useAnnouncement()
const dismissTodayChecked = ref(false)

const title = computed(() => getLocalizedText(props.announcement.title))

const sanitizedContent = computed(() => {
  const raw = getLocalizedText(props.announcement.content)
  return sanitizeRichHtml(raw)
})

const typeStyle = computed(() => {
  switch (props.announcement.type) {
    case 'warning':
      return {
        iconWrap: 'bg-warning-soft text-warning ring-warning/20',
        icon: AlertTriangle,
      }
    case 'info':
      return {
        iconWrap: 'bg-info-soft text-info ring-info/20',
        icon: Info,
      }
    case 'success':
      return {
        iconWrap: 'bg-success-soft text-success ring-success/20',
        icon: CheckCircle2,
      }
    default:
      return {
        iconWrap: 'bg-primary-soft text-primary ring-primary/20',
        icon: Megaphone,
      }
  }
})

const close = () => emit('update:visible', false)

const handleClose = () => {
  if (dismissTodayChecked.value) dismissToday(props.announcement.version)
  close()
}

// Vue must wait for the full panel budget: CSS reversal can end the
// independently eased root early. Sample on each render/interaction, not once
// at setup. The visible argument tracks ref-driven visibility outside the
// Transition slot too. Vue remains the only completion/cancellation timer owner.
const transitionDuration = (_visible: boolean) => {
  if (typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return 0
  return { enter: 300, leave: 200 }
}

const dialogRef = ref<HTMLElement | null>(null)
watch(() => props.visible, open => { if (open) dismissTodayChecked.value = false }, { immediate: true })
const { beforeEnter, beforeLeave } = useOverlayMotionLifecycle(() => props.visible, dialogRef, handleClose, 120)
</script>

<template>
  <Teleport to="body">
    <Transition name="announcement-motion" :duration="transitionDuration(visible)" appear @before-enter="beforeEnter" @before-leave="beforeLeave">
      <div
        v-if="visible"
        data-overlay-root
        class="announcement-modal-overlay fixed inset-0 z-[120] flex items-center justify-center bg-black/60 p-4 sm:p-6"
        @click.self="handleClose"
      >
          <div
            ref="dialogRef"
            data-overlay-panel
            role="dialog"
            aria-modal="true"
            aria-labelledby="announcement-modal-title"
            tabindex="-1"
            class="bg-card text-card-foreground relative z-10 flex max-h-[calc(100dvh-2rem)] w-full max-w-lg flex-col overflow-hidden rounded-2xl border sm:max-h-[86vh] sm:rounded-3xl"
            style="box-shadow: var(--ui-shadow-card)"
          >
            <!-- Header -->
            <div class="flex items-center gap-3 px-4 pb-4 pt-4 sm:gap-4 sm:px-6 sm:pb-5 sm:pt-6">
              <div
                class="flex size-12 shrink-0 items-center justify-center rounded-2xl ring-1 ring-inset"
                :class="typeStyle.iconWrap"
              >
                <component :is="typeStyle.icon" class="size-6" :stroke-width="2" />
              </div>
              <h3
                id="announcement-modal-title"
                class="line-clamp-2 min-w-0 flex-1 text-lg font-semibold leading-snug text-foreground"
              >
                {{ title }}
              </h3>
              <button
                type="button"
                class="flex size-11 shrink-0 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
                :aria-label="t('announcement.close')"
                @click="handleClose"
              >
                <X class="size-4" :stroke-width="2" />
              </button>
            </div>

            <!-- Body -->
            <div class="min-h-0 flex-1 overscroll-contain overflow-y-auto px-4 pb-3 sm:px-6">
              <div class="theme-prose prose prose-sm max-w-none break-words dark:prose-invert [&_a]:break-all [&_img]:h-auto [&_img]:max-w-full [&_table]:block [&_table]:max-w-full [&_table]:overflow-x-auto" v-html="sanitizedContent"></div>
            </div>

            <!-- Footer -->
            <div class="flex flex-col gap-2 border-t px-4 pb-[max(1.25rem,env(safe-area-inset-bottom))] pt-3 sm:flex-row sm:items-center sm:justify-between sm:gap-3 sm:px-6 sm:pt-4">
              <label for="announcement-dismiss-today" class="flex min-h-11 cursor-pointer items-center gap-2 rounded-lg px-1 text-sm text-muted-foreground hover:text-foreground">
                <Checkbox id="announcement-dismiss-today" v-model="dismissTodayChecked" />
                <span>{{ t('announcement.dismissToday') }}</span>
              </label>
              <Button class="min-h-11 w-full rounded-xl sm:w-auto" @click="handleClose">
                {{ t('announcement.close') }}
              </Button>
            </div>
          </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.announcement-motion-enter-active { transition:opacity .3s ease-out; }
.announcement-motion-leave-active { transition:opacity .2s ease-in; }
.announcement-motion-enter-active [data-overlay-panel] { transition:transform .3s cubic-bezier(.34,1.56,.64,1); }
.announcement-motion-leave-active [data-overlay-panel] { transition:transform .2s ease-in; }
.announcement-motion-enter-from,.announcement-motion-leave-to { opacity:0; }
.announcement-motion-enter-from [data-overlay-panel] { transform:translateY(12px) scale(.95); }
.announcement-motion-leave-to [data-overlay-panel] { transform:translateY(8px) scale(.95); }
@media (prefers-reduced-motion: reduce) {
  .announcement-motion-enter-active,.announcement-motion-leave-active,.announcement-motion-enter-active [data-overlay-panel],.announcement-motion-leave-active [data-overlay-panel] { transition:none; }
  .announcement-motion-enter-from [data-overlay-panel],.announcement-motion-leave-to [data-overlay-panel] { transform:none; }
}
</style>
