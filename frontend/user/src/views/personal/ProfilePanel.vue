<template>
  <div class="rounded-2xl border bg-card p-7 shadow-sm">
    <PanelHeading :title="t('personalCenter.profile.title')" :description="t('personalCenter.profile.subtitle')" :icon="UserCircle">
      <template #actions>
        <Badge variant="accent" size="sm">{{ t('personalCenter.tabs.profile') }}</Badge>
      </template>
    </PanelHeading>

    <Alert v-if="profileAlert" class="mb-5" :variant="pageAlertVariant(profileAlert.level)" :class="pageAlertToneClass(profileAlert.level)">
      <AlertDescription>{{ profileAlert.message }}</AlertDescription>
    </Alert>

    <form class="space-y-6" @submit.prevent="handleSaveProfile">
      <div class="grid grid-cols-1 gap-5 md:grid-cols-2">
        <div class="md:col-span-2">
          <Label class="mb-2 block">{{ t('personalCenter.profile.emailLabel') }}</Label>
          <Input :model-value="userProfileStore.profile?.email || ''" disabled class="h-11" />
        </div>

        <div>
          <Label class="mb-2 block">{{ t('personalCenter.profile.nicknameLabel') }}</Label>
          <Input
            v-model="profileForm.nickname"
            :placeholder="t('personalCenter.profile.nicknamePlaceholder')"
            class="h-11"
          />
        </div>

        <div>
          <Label class="mb-2 block">{{ t('personalCenter.profile.localeLabel') }}</Label>
          <Select v-model="profileForm.locale">
            <SelectTrigger class="h-11 w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="zh-CN">简体中文</SelectItem>
              <SelectItem value="zh-TW">繁體中文</SelectItem>
              <SelectItem value="en-US">English</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      <div class="flex flex-col gap-3 border-t pt-5 sm:flex-row sm:items-center sm:justify-between">
        <p class="text-xs text-muted-foreground">{{ t('personalCenter.profile.subtitle') }}</p>
        <Button type="submit" :disabled="userProfileStore.savingProfile" class="h-11 font-bold">
          {{ userProfileStore.savingProfile ? t('personalCenter.profile.saving') : t('personalCenter.profile.save') }}
        </Button>
      </div>
    </form>
  </div>
</template>

<script setup lang="ts">
import { onScopeDispose, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { UserCircle } from 'lucide-vue-next'
import { pageAlertVariant, pageAlertToneClass, type PageAlert } from '../../utils/alerts'
import { useUserAuthStore } from '../../stores/userAuth'
import { useUserProfileStore } from '../../stores/userProfile'
import PanelHeading from '../../components/shared/PanelHeading.vue'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

const { t } = useI18n()
const userProfileStore = useUserProfileStore()
const userAuthStore = useUserAuthStore()
let disposed = false
let saveRequest = 0
onScopeDispose(() => { disposed = true })

const profileForm = reactive({
  nickname: '',
  locale: 'zh-CN',
})

const profileAlert = ref<PageAlert | null>(null)
const editGeneration = { nickname: 0, locale: 0 }
let hydrating = false
let pendingSave: { nickname: number; locale: number } | null = null
for (const key of ['nickname', 'locale'] as const) {
  watch(() => profileForm[key], () => {
    if (!hydrating) editGeneration[key]++
  }, { flush: 'sync' })
}

const handleSaveProfile = async () => {
  if (disposed) return
  const request = ++saveRequest
  const session = userAuthStore.sessionGeneration
  const account = userProfileStore.profile?.id
  const submittedGeneration = { ...editGeneration }
  pendingSave = submittedGeneration
  profileAlert.value = null
  const payload = {
    nickname: profileForm.nickname.trim(),
    locale: profileForm.locale,
  }
  const ok = await userProfileStore.saveProfile(payload)
  if (pendingSave === submittedGeneration) pendingSave = null
  if (disposed || request !== saveRequest || session !== userAuthStore.sessionGeneration || account !== userProfileStore.profile?.id) return
  if (!ok) {
    profileAlert.value = {
      level: 'error',
      message: userProfileStore.profileError || t('personalCenter.common.saveFailed'),
    }
    return
  }
  hydrating = true
  for (const key of ['nickname', 'locale'] as const) {
    if (editGeneration[key] === submittedGeneration[key]) profileForm[key] = profileBaseline[key]
  }
  hydrating = false
  profileAlert.value = {
    level: 'success',
    message: t('personalCenter.profile.saveSuccess'),
  }
}

let profileAccount: number | undefined
let profileBaseline = { nickname: '', locale: 'zh-CN' }
watch(
  () => userProfileStore.profile,
  (profile) => {
    const next = { nickname: profile?.nickname || '', locale: profile?.locale || 'zh-CN' }
    const accountChanged = profile?.id !== profileAccount
    if (!profile || accountChanged) pendingSave = null
    hydrating = true
    for (const key of ['nickname', 'locale'] as const) {
      const editedDuringSave = pendingSave && editGeneration[key] !== pendingSave[key]
      if (!profile || accountChanged || (!editedDuringSave && profileForm[key] === profileBaseline[key])) {
        profileForm[key] = next[key]
      }
    }
    hydrating = false
    if (!profile || accountChanged) profileAlert.value = null
    profileAccount = profile?.id
    profileBaseline = next
  },
  { immediate: true, flush: 'sync' }
)
</script>
