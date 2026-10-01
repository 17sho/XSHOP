<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref } from 'vue'
import { api } from '@/api/client'
import { useAdminAuthStore } from '@/stores/auth'
import { Button } from '@/components/ui/button'
const authStore = useAdminAuthStore()
interface UpgradeStatus { state: string; version?: string; sequence?: number; digest?: string; message?: string; repository?: string }
const status = ref<UpgradeStatus>({ state: 'idle' })
const busy = ref(false)
const error = ref('')
let timer: ReturnType<typeof setInterval> | undefined
let disposed = false
async function refresh() {
  if (!authStore.isSuper || disposed) return
  try { const result = await api.get('/admin/xshop-upgrade/status'); if (!disposed) status.value = result.data.data as UpgradeStatus }
  catch { if (!disposed) error.value = '升级服务尚未就绪，或服务正在重启；请稍后刷新。' }
}
async function check() {
  if (!authStore.isSuper || busy.value) return
  busy.value = true; error.value = ''
  try { await api.post('/admin/xshop-upgrade/check'); await refresh() }
  catch { error.value = '检查更新失败，请稍后重试。' }
  finally { busy.value = false }
}
async function install() {
  if (!authStore.isSuper || busy.value || status.value.state !== 'available' || !status.value.digest) return
  const digest = status.value.digest
  if (!window.confirm(`安装 ${status.value.version}？将短暂重启测试站，保留配置、数据库和上传文件；失败时仅回退程序。`)) return
  // The confirmation can outlive permission refresh or a new update check.
  if (!authStore.isSuper || status.value.digest !== digest || status.value.state !== 'available') return
  busy.value = true; error.value = ''
  try { await api.post('/admin/xshop-upgrade/install', { digest }); await refresh() }
  catch { error.value = '安装请求未确认，请先刷新状态，不要重复提交。' }
  finally { busy.value = false }
}
onMounted(() => { void refresh(); timer = setInterval(() => { void refresh() }, 2500) })
onBeforeUnmount(() => { disposed = true; if (timer) clearInterval(timer) })
const labels: Record<string, string> = { idle: '等待检查', checking: '检查中', available: '可升级', installing: '安装中', installed: '升级完成', failed: '操作失败' }
</script>
<template>
  <section class="mx-auto max-w-3xl space-y-5 p-4">
    <h1 class="text-2xl font-semibold">XSHOP 在线升级</h1>
    <p class="text-muted-foreground">仅超级管理员可操作。更新来源固定为私有 17sho/XSHOP，不使用官方上游程序覆盖定制功能。</p>
    <div v-if="authStore.isSuper" class="rounded-xl border p-5 space-y-4">
      <p>状态：{{ labels[status.state] || status.state }}</p>
      <p v-if="status.version">目标版本：{{ status.version }}（序号 {{ status.sequence }}）</p>
      <p>{{ status.message }}</p>
      <p class="text-sm text-muted-foreground">升级前保存程序、配置和数据库检查点；自动恢复不会覆盖业务数据。本通道只允许数据库结构不变的嵌入式测试版本。</p>
      <p v-if="error" role="alert" class="text-destructive">{{ error }}</p>
      <div class="flex flex-wrap gap-3">
        <Button :disabled="busy || ['checking','installing'].includes(status.state)" @click="check">检查 XSHOP 更新</Button>
        <Button :disabled="busy || status.state !== 'available'" @click="install">安装已验证版本</Button>
        <Button variant="outline" @click="refresh">刷新状态</Button>
      </div>
    </div>
    <p v-else role="alert">无升级权限。</p>
  </section>
</template>
