<template>
  <Transition name="back-to-top">
    <Button
      v-if="visible"
      variant="secondary"
      size="icon"
      @click="scrollPageToTop"
      class="back-to-top fixed right-4 md:right-6 z-30 lg:z-40 h-11 w-11 rounded-full border shadow-lg transition-none hover:shadow-xl [&_svg]:size-5"
      :aria-label="t('common.backToTop')"
    >
      <ChevronUp />
    </Button>
  </Transition>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { scrollPageToTop } from '../utils/motionPolicy'
import { useI18n } from 'vue-i18n'
import { ChevronUp } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'

const { t } = useI18n()
const visible = ref(false)

const onScroll = () => {
  visible.value = window.scrollY > 400
}

onMounted(() => window.addEventListener('scroll', onScroll, { passive: true }))
onUnmounted(() => window.removeEventListener('scroll', onScroll))
</script>

<style scoped>
.back-to-top-enter-active,
.back-to-top-leave-active {
  transition: opacity 160ms ease-out, transform 160ms ease-out;
}
.back-to-top-enter-from,
.back-to-top-leave-to {
  opacity: 0;
  transform: scale(0.95);
}

/* Mobile: sit above bottom nav (h-14 = 3.5rem) + safe area, with breathing room */
.back-to-top {
  bottom: calc(3.5rem + env(safe-area-inset-bottom, 0px) + 1rem);
}
@media (min-width: 1024px) {
  .back-to-top {
    bottom: 1.5rem;
  }
}
</style>
