<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api } from '@/api/client'
import { useAdminAuthStore } from '@/stores/auth'
import { Button } from '@/components/ui/button'
defineProps<{ version?: string }>()
const authStore = useAdminAuthStore()
interface UpgradeStatus { state: string; version?: string; sequence?: number; digest?: string; message?: string; need_restart?: boolean; phase?: string }
const status = ref<UpgradeStatus>({ state: 'idle' })
const open = ref(false)
const root = ref<HTMLElement | null>(null)
const busy = ref(false)
const error = ref('')
const active = computed(() => ['checking', 'installing', 'restarting'].includes(status.value.state))
const needRestart = computed(() => status.value.state === 'prepared' && status.value.need_restart === true)
let disposed = false
let refreshing = false
let awaitingRestart = false
let timer: ReturnType<typeof setInterval> | undefined
const labels: Record<string, string> = { idle: '等待检查', checking: '检查中', available: '可升级', installing: '下载并应用中', prepared: '已应用，需要重启', restarting: '重启中', installed: '升级完成', failed: '操作失败' }
async function refresh() {
  if (!authStore.isSuper || disposed || refreshing) return
  refreshing = true
  try {
    const result = await api.get('/admin/xshop-upgrade/status')
    if (disposed || !authStore.isSuper) return
    status.value = result.data.data as UpgradeStatus
    error.value = ''
    if (awaitingRestart && status.value.state === 'installed' && !status.value.need_restart) {
      awaitingRestart = false
      window.dispatchEvent(new Event('appversionrefresh'))
    }
  } catch { if (!disposed && authStore.isSuper) error.value = awaitingRestart ? '服务正在重启或暂时不可用，正在等待状态确认。' : '读取升级状态失败，请稍后刷新。' }
  finally { refreshing = false }
}
async function check() {
  if (!authStore.isSuper || disposed || busy.value || active.value || needRestart.value) return
  busy.value = true; error.value = ''
  try { await api.post('/admin/xshop-upgrade/check'); await refresh() }
  catch { if (!disposed) error.value = '检查更新失败，请稍后重试。' }
  finally { busy.value = false }
}
async function install() {
  if (!authStore.isSuper || disposed || busy.value || status.value.state !== 'available' || !status.value.digest) return
  const digest = status.value.digest
  if (!window.confirm(`下载并应用 ${status.value.version}？应用后旧进程继续服务，需另行点击立即重启。请先备份配置和数据库。`)) return
  if (!authStore.isSuper || disposed || status.value.digest !== digest || status.value.state !== 'available') return
  busy.value = true; error.value = ''
  try { await api.post('/admin/xshop-upgrade/install', { digest }); await refresh() }
  catch { if (!disposed) error.value = '应用请求未确认，请刷新状态，不要重复提交。' }
  finally { busy.value = false }
}
async function restart() {
  if (!authStore.isSuper || disposed || busy.value || !needRestart.value) return
  const digest = status.value.digest
  if (!window.confirm('立即重启以启用已应用的版本？服务将短暂中断。')) return
  if (!authStore.isSuper || disposed || !needRestart.value || digest !== status.value.digest) return
  busy.value = true; error.value = ''; awaitingRestart = true
  try { await api.post('/admin/xshop-upgrade/restart'); await refresh() }
  catch { if (!disposed) { error.value = '重启请求未确认，正在等待服务状态；请勿重复提交。'; await refresh() } }
  finally { busy.value = false }
}
function toggle() { if (!authStore.isSuper) return; open.value = !open.value; if (open.value) void refresh() }
function outside(event: MouseEvent) { if (open.value && event.target instanceof Node && !root.value?.contains(event.target)) open.value = false }
function escape(event: KeyboardEvent) { if (event.key === 'Escape') { open.value = false; root.value?.querySelector('button')?.focus() } }
onMounted(() => { void refresh(); timer = setInterval(() => { if (open.value || active.value || awaitingRestart) void refresh() }, 2500); document.addEventListener('click', outside); document.addEventListener('keydown', escape) })
onBeforeUnmount(() => { disposed = true; if (timer) clearInterval(timer); document.removeEventListener('click', outside); document.removeEventListener('keydown', escape) })
</script>
<template>
  <div ref="root" class="relative shrink-0">
    <button v-if="authStore.isSuper" type="button" class="h-8 max-w-24 md:max-w-40 truncate rounded-md border border-border bg-secondary/40 px-2 text-xs" aria-label="版本与在线升级" :aria-expanded="open" aria-haspopup="dialog" @click="toggle">{{ version || '版本' }} <span v-if="status.state === 'available' || needRestart" class="text-amber-600">●</span></button>
    <span v-else-if="version" class="text-xs text-muted-foreground">{{ version }}</span>
    <section v-if="open && authStore.isSuper" role="dialog" aria-label="XSHOP 在线升级" class="fixed right-4 top-16 sm:absolute sm:right-0 sm:top-auto z-50 mt-2 w-80 max-w-[calc(100vw-2rem)] max-h-[calc(100dvh-5rem)] overflow-y-auto rounded-xl border border-border bg-card p-4 shadow-lg space-y-3 text-sm break-words">
      <div class="flex items-center justify-between"><h2 class="font-semibold">XSHOP 在线升级</h2><button type="button" aria-label="关闭升级面板" @click="open = false">×</button></div>
      <p>当前版本：{{ version || '—' }}</p>
      <p v-if="status.version">目标版本：{{ status.version }}<span v-if="status.sequence">（序号 {{ status.sequence }}）</span></p>
      <p role="status" aria-live="polite">状态：{{ labels[status.state] || status.state }}</p>
      <p v-if="status.phase">阶段：{{ status.phase }}</p>
      <p v-if="status.message" :role="status.state === 'failed' ? 'alert' : undefined">{{ status.message }}</p>
      <p v-if="needRestart" class="text-amber-600">下载并应用完成，需要重启。旧进程仍在提供服务。</p>
      <p v-if="error" role="alert" class="text-destructive">{{ error }}</p>
      <p class="text-xs text-muted-foreground">仅超级管理员可操作，更新来源为私有 XSHOP 定制通道。升级前请备份配置和数据库；重启不会覆盖业务数据。</p>
      <div class="flex flex-wrap gap-2">
        <Button size="sm" :disabled="busy || active || needRestart" @click="check">检查更新</Button>
        <Button v-if="!needRestart" size="sm" :disabled="busy || status.state !== 'available' || !status.digest" @click="install">下载并应用</Button>
        <Button v-else size="sm" :disabled="busy || awaitingRestart" @click="restart">立即重启</Button>
        <Button size="sm" variant="outline" :disabled="busy" @click="refresh">刷新状态</Button>
      </div>
    </section>
  </div>
</template>
