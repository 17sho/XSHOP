<template>
  <footer v-if="links.length" data-custom-footer-links class="flex flex-wrap justify-center gap-x-4 gap-y-2 px-4 py-5 pb-20 lg:pb-5 text-xs text-muted-foreground">
    <a v-for="(link,index) in links" :key="index" :href="link.url" target="_blank" rel="noopener noreferrer" class="hover:text-foreground hover:underline">{{ link.name }}</a>
  </footer>
</template>
<script setup lang="ts">
import {computed} from 'vue'
import {useAppStore} from '../stores/app'
import {getLocalizedText} from '../utils/resellerSiteConfig'
const appStore=useAppStore()
const links=computed(()=>{
 const raw=appStore.config?.footer_links
 if(!Array.isArray(raw))return []
 return raw.map((item:any)=>({name:typeof item?.name==='string'?item.name.trim():getLocalizedText(item?.name,appStore.locale),url:String(item?.url||'').trim()})).filter(item=>item.name && /^https?:\/\//i.test(item.url))
})
</script>
