<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch, nextTick } from 'vue'
import { api } from '@/api/client'
import { useAdminAuthStore } from '@/stores/auth'
import { Button } from '@/components/ui/button'
import { Download } from 'lucide-vue-next'
defineProps<{ version?: string }>()
const authStore = useAdminAuthStore()
const revoked = ref(false)
let generation = 0
const permitted = computed(() => authStore.isSuper && !revoked.value)
interface UpgradeStatus { state: string; version?: string; sequence?: number; digest?: string; message?: string; need_restart?: boolean; phase?: string; current_version?: string; previous_version?: string; rollback_available?: boolean }
const status = ref<UpgradeStatus>({ state: 'idle' })
const open = ref(false)
const root = ref<HTMLElement | null>(null)
const busy = ref(false)
const error = ref('')
const feedback = ref(false)
let feedbackTimer: ReturnType<typeof setTimeout> | undefined
function prepareFeedback() { feedback.value = true; if (feedbackTimer) clearTimeout(feedbackTimer); feedbackTimer = setTimeout(() => { feedback.value = false }, window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ? 0 : 450) }
const active = computed(() => ['checking', 'installing', 'restarting', 'rolling_back'].includes(status.value.state))
const needRestart = computed(() => status.value.state === 'prepared' && status.value.need_restart === true)
let disposed = false
let refreshing = false
const awaitingRestart = ref(false)
let restartUntil = 0
// DTO current_version is projected only from a helper-verified terminal journal.
// No operation ID is exposed: retain the write fence for unidentifiable outcomes.
type Activation = Readonly<{ kind: 'restart' | 'rollback'; start?: string; target?: string; digest?: string }>
let activation: Activation | undefined
function completed(s: UpgradeStatus) {
  if (!activation?.target || activation.target === activation.start || s.current_version !== activation.target || s.need_restart) return false
  return activation.kind === 'rollback'
    ? s.state === 'rolled_back' && s.rollback_available === false && !s.previous_version
    : s.state === 'installed'
}
let timer: ReturnType<typeof setInterval> | undefined
const labels: Record<string, string> = { idle: '等待检查', checking: '检查中', available: '可升级', installing: '下载并应用中', prepared: '已应用，需要重启', restarting: '重启中', installed: '升级完成', failed: '操作失败', up_to_date: '已是最新版本', rolling_back: '回退并核验中', rolled_back: '程序回退已确认' }
function rejectOperation(err: any) {
  if (err?.status === 401 || err?.status === 403) { revoked.value = true; generation++; awaitingRestart.value = false; busy.value = false; refreshing = false; open.value = false; return true }
  if (typeof err?.status === 'number' && ![0, 502, 503].includes(err.status)) { error.value = awaitingRestart.value ? '请求结果未确认，请只刷新状态；请勿重复提交。' : '操作失败，请核查服务状态后重试。'; return true }
  return false
}
async function refresh(manual = false) {
  if (!authStore.isSuper || !permitted.value || disposed || refreshing) return
  refreshing = true
  const owner = generation
  try {
    const expired = awaitingRestart.value && Date.now() >= restartUntil
    if (expired) { error.value = '运行状态确认超时，请核查服务后刷新；尚未确认成功，请勿重复提交。'; if (!manual) return }
    const result = await api.get('/admin/xshop-upgrade/status', awaitingRestart.value && !expired ? { expectedRestartUntil: restartUntil } : undefined)
    if (disposed || !permitted.value || owner !== generation) return
    status.value = result.data.data as UpgradeStatus
    if (!expired) error.value = ''
    if (awaitingRestart.value && completed(status.value)) {
      awaitingRestart.value = false; restartUntil = 0; activation = undefined; error.value = ''
      window.dispatchEvent(new Event('appversionrefresh'))
    }
  } catch (err: any) { if (!disposed && owner === generation && permitted.value) { if (rejectOperation(err)) return; if (!error.value.includes('确认超时')) error.value = awaitingRestart.value ? '等待服务恢复，正在核验实际运行状态；请勿重复提交。' : '读取升级状态失败，请稍后刷新。' } }
  finally { if (owner === generation) refreshing = false }
}
async function check() {
  if (!open.value || !authStore.isSuper || !permitted.value || disposed || busy.value || active.value || awaitingRestart.value || needRestart.value) return
  const owner = generation
  busy.value = true; error.value = ''
  try { await api.post('/admin/xshop-upgrade/check'); if (!disposed && owner === generation) await refresh() }
  catch (err: any) { if (!disposed && owner === generation && !rejectOperation(err)) error.value = '检查更新失败，请稍后重试。' }
  finally { if (!disposed && owner === generation) busy.value = false }
}
async function install() {
  if (!open.value || !authStore.isSuper || !permitted.value || disposed || busy.value || awaitingRestart.value || status.value.state !== 'available' || !status.value.digest) return
  const digest = status.value.digest
  if (!window.confirm(`下载并应用 ${status.value.version}？应用后旧进程继续服务，需另行点击立即重启。请先备份配置和数据库。`)) return
  if (!authStore.isSuper || !permitted.value || disposed || status.value.digest !== digest || status.value.state !== 'available') return
  const owner = generation
  busy.value = true; error.value = ''; prepareFeedback()
  try { await api.post('/admin/xshop-upgrade/install', { digest }); if (!disposed && owner === generation) await refresh() }
  catch (err: any) { if (!disposed && owner === generation && !rejectOperation(err)) error.value = '应用请求未确认，请刷新状态，不要重复提交。' }
  finally { if (!disposed && owner === generation) busy.value = false }
}
async function restart() {
  if (!open.value || !authStore.isSuper || !permitted.value || disposed || busy.value || awaitingRestart.value || !needRestart.value) return
  const intended = Object.freeze({ kind: 'restart' as const, start: status.value.current_version, target: status.value.version, digest: status.value.digest })
  const owner = generation
  if (!window.confirm('立即重启以启用已应用的版本？服务将短暂中断。')) return
  if (!authStore.isSuper || !permitted.value || disposed || owner !== generation || awaitingRestart.value || !needRestart.value || intended.digest !== status.value.digest || intended.target !== status.value.version || intended.start !== status.value.current_version) return
  activation = intended
  busy.value = true; error.value = ''; awaitingRestart.value = true; restartUntil = Date.now() + 120000
  try { await api.post('/admin/xshop-upgrade/restart', undefined, { expectedRestartUntil: restartUntil }); if (!disposed && owner === generation) await refresh() }
  catch (err: any) { if (!disposed && owner === generation) { if (rejectOperation(err)) return; error.value = '重启请求未确认，正在等待服务状态；请勿重复提交。'; if (!disposed && owner === generation) await refresh() } }
  finally { if (!disposed && owner === generation) busy.value = false }
}
const canRollback = computed(() => status.value.rollback_available === true && !!status.value.previous_version && ['installed', 'up_to_date', 'rolled_back', 'available'].includes(status.value.state))
async function rollback() {
  if (!open.value || !authStore.isSuper || !permitted.value || disposed || busy.value || active.value || awaitingRestart.value || !canRollback.value) return
  const intended = Object.freeze({ kind: 'rollback' as const, start: status.value.current_version, target: status.value.previous_version })
  const owner = generation
  if (!window.confirm('仅回退程序，不回滚数据库；会短暂重启')) return
  if (!authStore.isSuper || !permitted.value || disposed || !canRollback.value || owner !== generation || awaitingRestart.value || intended.target !== status.value.previous_version || intended.start !== status.value.current_version) return
  activation = intended
  busy.value = true; error.value = ''; awaitingRestart.value = true; restartUntil = Date.now() + 120000
  try { await api.post('/admin/xshop-upgrade/rollback', undefined, { expectedRestartUntil: restartUntil }); if (!disposed && owner === generation) await refresh() }
  catch (err: any) { if (!disposed && owner === generation && permitted.value && !rejectOperation(err)) await refresh() }
  finally { if (!disposed && owner === generation) busy.value = false }
}
watch(() => [authStore.isSuper, authStore.token], () => { generation++; open.value = false; status.value = { state: 'idle' }; busy.value = false; refreshing = false; awaitingRestart.value = false; activation = undefined; restartUntil = 0; feedback.value = false; if (feedbackTimer) clearTimeout(feedbackTimer) }, { flush: 'sync' })
const windowReduced = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches || false
const panelWidth = ref(296)
function measurePanel() { const width = Math.min(document.documentElement.clientWidth || 320, window.visualViewport?.width || Infinity); panelWidth.value = Math.max(0, width - 24) }
function toggle() { if (!authStore.isSuper || !permitted.value) return; open.value = !open.value; if (open.value) { measurePanel(); void refresh(); const owner = generation; void nextTick(() => { if (!disposed && owner === generation && open.value) root.value?.querySelector<HTMLElement>('[aria-label="关闭升级面板"]')?.focus() }) } }
function outside(event: MouseEvent) { if (open.value && event.target instanceof Node && !root.value?.contains(event.target)) open.value = false }
function escape(event: KeyboardEvent) { if (event.key === 'Escape') { open.value = false; root.value?.querySelector('button')?.focus() } }
onMounted(() => { measurePanel(); window.addEventListener('resize', measurePanel); window.visualViewport?.addEventListener('resize', measurePanel); void refresh(); timer = setInterval(() => { if (open.value || active.value || awaitingRestart.value) void refresh() }, 2500); document.addEventListener('click', outside); document.addEventListener('keydown', escape) })
onBeforeUnmount(() => { disposed = true; generation++; window.removeEventListener('resize', measurePanel); window.visualViewport?.removeEventListener('resize', measurePanel); if (timer) clearInterval(timer); if (feedbackTimer) clearTimeout(feedbackTimer); document.removeEventListener('click', outside); document.removeEventListener('keydown', escape) })
</script>
<template>
  <div ref="root" class="relative shrink-0">
    <Button v-if="permitted" type="button" size="icon-sm" variant="outline" class="relative" title="版本与在线升级" aria-label="版本与在线升级" :aria-expanded="open" aria-haspopup="dialog" @click="toggle"><Download class="h-4 w-4" aria-hidden="true" /><span v-if="status.state === 'available' || needRestart" class="absolute right-0 top-0 h-2 w-2 rounded-full bg-amber-500" aria-label="有更新待处理" /></Button>
    <span v-else-if="version" class="text-xs text-muted-foreground">{{ version }}</span>
    <Transition name="upgrade-panel" :duration="windowReduced ? 0 : 200">
    <section v-if="open && permitted" role="dialog" aria-label="XSHOP 在线升级" :inert="!open || !permitted" :aria-hidden="!open || !permitted ? true : undefined" :style="{ maxWidth: panelWidth + 'px', pointerEvents: open && permitted ? undefined : 'none' }" class="fixed right-4 top-16 sm:absolute sm:right-0 sm:top-auto z-50 mt-2 w-80 max-w-[calc(100vw-2rem)] max-h-[calc(100dvh-5rem)] overflow-y-auto rounded-xl border border-border bg-card p-4 shadow-lg space-y-3 text-sm break-words">
      <div class="flex items-center justify-between"><h2 class="font-semibold">XSHOP 在线升级</h2><button type="button" aria-label="关闭升级面板" @click="open = false">×</button></div>
      <p>当前版本：{{ status.current_version || version || '—' }}</p>
      <p v-if="status.previous_version">上一版本：{{ status.previous_version }}</p>
      <p v-if="status.version">目标版本：{{ status.version }}<span v-if="status.sequence">（序号 {{ status.sequence }}）</span></p>
      <p class="upgrade-step" :key="status.phase || status.state" role="status" aria-live="polite">状态：{{ awaitingRestart && activation?.kind === 'restart' && status.state === 'rolled_back' && status.current_version && status.rollback_available === false ? '旧程序已恢复；目标版本未确认' : awaitingRestart && status.state !== 'failed' ? '等待服务恢复与运行核验' : labels[status.state] || '等待状态确认' }}</p>
      <div v-if="busy || active || awaitingRestart || feedback" role="progressbar" aria-label="操作进行中，等待核验" class="upgrade-progress"><span /></div>
      <p v-if="feedback" role="status">{{ needRestart ? '下载与验证已完成，准备结果已确认' : '正在下载并验证；等待实际结果' }}</p>
      <p v-if="status.phase">阶段：{{ ({ downloading: '下载中', verifying: '签名与哈希验证中', preparing: '准备程序中', prepared: '准备完成', restarting: '重启并核验中', installed: '运行与健康确认完成' } as Record<string, string>)[status.phase] || '等待状态确认' }}</p>
      <p v-if="status.message" :role="status.state === 'failed' ? 'alert' : undefined">{{ status.message }}</p>
      <p v-if="needRestart" class="rounded-lg border border-green-200 bg-green-50 p-3 text-green-700">✓ 下载并应用完成，需要重启。旧进程仍在提供服务。</p>
      <p v-if="error" role="alert" class="text-destructive">{{ error }}</p>
      <p class="text-xs text-muted-foreground">仅超级管理员可操作，更新来源为私有 XSHOP 定制通道。升级前请备份配置和数据库；重启不会覆盖业务数据。</p>
      <div class="flex flex-wrap gap-2">
        <Button size="sm" :disabled="busy || active || awaitingRestart || needRestart" @click="check">检查更新</Button>
        <Button v-if="!needRestart" size="sm" :disabled="busy || awaitingRestart || status.state !== 'available' || !status.digest" @click="install">下载并应用</Button>
        <Button v-else size="sm" :disabled="busy || feedback || awaitingRestart" @click="restart">立即重启</Button>
        <Button size="sm" variant="outline" :disabled="busy || active || awaitingRestart || !canRollback" @click="rollback">回退程序</Button>
        <Button size="sm" variant="outline" :disabled="busy" @click="refresh(true)">刷新状态</Button>
      </div>
    </section>
    </Transition>
  </div>
</template>
<style scoped>
.upgrade-panel-enter-active,.upgrade-panel-leave-active { transition: opacity 200ms ease, transform 200ms ease; }
.upgrade-panel-enter-from,.upgrade-panel-leave-to { opacity: 0; transform: translateY(-6px); }
.upgrade-step { animation: upgrade-phase 200ms ease; }
.upgrade-progress { height: 3px; overflow: hidden; background: #e2e8f0; border-radius: 3px; }
.upgrade-progress span { display: block; width: 35%; height: 100%; background: #16a34a; animation: upgrade-pending 1.2s ease-in-out infinite alternate; }
@keyframes upgrade-phase { from { opacity: .3; transform: translateY(3px); } to { opacity: 1; transform: none; } }
@keyframes upgrade-pending { from { transform: translateX(0); } to { transform: translateX(180%); } }
@media (prefers-reduced-motion: reduce) { .upgrade-panel-enter-active,.upgrade-panel-leave-active { transition: none; } .upgrade-step,.upgrade-progress span { animation: none; } }
</style>
