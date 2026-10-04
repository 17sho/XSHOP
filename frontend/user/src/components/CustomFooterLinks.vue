<template>
  <footer v-if="footerText || links.length" data-custom-footer-links class="space-y-2 px-4 py-5 pb-20 lg:pb-5 text-center text-xs text-muted-foreground">
    <p v-if="footerText" data-footer-text class="whitespace-pre-line break-words">{{ footerText }}</p>
    <div v-if="links.length" class="flex flex-wrap justify-center gap-x-4 gap-y-2">
      <a v-for="(link,index) in links" :key="index" :href="link.url" target="_blank" rel="noopener noreferrer" class="hover:text-foreground hover:underline">{{ link.name }}</a>
    </div>
  </footer>
</template>
<script setup lang="ts">
import {computed} from 'vue'
import {useAppStore} from '../stores/app'
import {getLocalizedText} from '../utils/resellerSiteConfig'
const appStore=useAppStore()
const footerText=computed(()=>{
 const raw=appStore.config?.footer_text
 return typeof raw==='string' ? raw.trim().replace(/\{year\}/g, String(new Date().getFullYear())) : ''
})
const links=computed(()=>{
 const raw=appStore.config?.footer_links
 if(!Array.isArray(raw))return []
 return raw.map((item:any)=>({name:typeof item?.name==='string'?item.name.trim():getLocalizedText(item?.name,appStore.locale),url:String(item?.url||'').trim()})).filter(item=>item.name && /^https?:\/\//i.test(item.url))
})
</script>
