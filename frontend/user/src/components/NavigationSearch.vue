<template>
 <div v-if="appStore.config?.nav_search_enabled === true" class="relative shrink-0">
  <button type="button" class="grid h-9 w-9 place-items-center rounded-lg hover:bg-secondary" :aria-label="t('products.searchLabel')" :aria-expanded="open" @click="open=!open"><Search class="h-4 w-4" /></button>
  <form v-if="open" class="fixed top-16 left-3 right-3 z-[60] flex gap-2 rounded-xl border bg-card p-3 shadow-lg sm:absolute sm:left-auto sm:right-0 sm:top-full sm:w-80" @submit.prevent="submit" @keydown.esc="open=false">
   <input v-model="query" autofocus class="min-w-0 flex-1 rounded-lg border bg-background px-3 py-2 text-sm" :placeholder="t('products.searchBoxPlaceholder')" :aria-label="t('products.searchLabel')" />
   <button type="submit" class="rounded-lg bg-primary px-3 text-primary-foreground" :aria-label="t('products.searchLabel')"><Search class="h-4 w-4" /></button>
  </form>
 </div>
</template>
<script setup lang="ts">
import {ref,watch} from 'vue'
import {useRouter} from 'vue-router'
import {useI18n} from 'vue-i18n'
import {Search} from 'lucide-vue-next'
import {useAppStore} from '../stores/app'
const appStore=useAppStore(),router=useRouter(),{t}=useI18n()
const open=ref(false),query=ref('')
watch(()=>appStore.config?.nav_search_enabled,()=>{open.value=false})
function submit(){if(appStore.config?.nav_search_enabled!==true)return;const search=query.value.trim();open.value=false;void router.push({path:'/',query:search?{search}:{}})}
</script>
