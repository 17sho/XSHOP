<template>
  <div
    v-if="enabled"
    :class="variant === 'vault'
      ? 'mb-4 rounded-sm border bg-secondary p-3.5'
      : 'mb-4 space-y-2 rounded-xl border bg-card p-4'"
  >
    <p :class="variant === 'vault' ? 'mb-2 text-xs font-bold text-muted-foreground' : 'text-xs font-semibold text-muted-foreground'">
      {{ t('auth.common.captchaLabel') }}
    </p>
    <ImageCaptcha
      v-if="provider === 'image'"
      ref="imageCaptchaRef"
      v-model="imagePayload"
      :disabled="disabled"
      @config-stale="emit('config-stale')"
    />
    <TurnstileCaptcha
      v-else-if="provider === 'turnstile'"
      ref="turnstileRef"
      v-model="turnstileToken"
      :site-key="turnstileSiteKey"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { CaptchaPayload } from '../../api/types'
import ImageCaptcha from '../captcha/ImageCaptcha.vue'
import TurnstileCaptcha from '../captcha/TurnstileCaptcha.vue'

const props = defineProps<{
  enabled: boolean
  provider: string
  imagePayload: CaptchaPayload
  turnstileToken: string
  turnstileSiteKey: string
  disabled: boolean
  variant: 'classic' | 'vault'
}>()

const emit = defineEmits<{
  'update:imagePayload': [value: CaptchaPayload]
  'update:turnstileToken': [value: string]
  'config-stale': []
}>()

const { t } = useI18n()
const imageCaptchaRef = ref<InstanceType<typeof ImageCaptcha> | null>(null)
const turnstileRef = ref<InstanceType<typeof TurnstileCaptcha> | null>(null)

const imagePayload = computed({
  get: () => props.imagePayload,
  set: (value) => emit('update:imagePayload', value),
})
const turnstileToken = computed({
  get: () => props.turnstileToken,
  set: (value) => emit('update:turnstileToken', value),
})

defineExpose({
  refresh: () => imageCaptchaRef.value?.refresh(),
  reset: () => turnstileRef.value?.reset(),
})
</script>
