<template>
  <nav
    class="fixed top-0 left-0 right-0 z-50 border-b bg-card/95 py-3 shadow-sm">
    <div class="container mx-auto px-4 flex items-center justify-between gap-4">
      <!-- Logo -->
      <router-link to="/" class="theme-wordmark group relative gap-3" :title="brandSiteName">
        <svg class="h-8 w-8 shrink-0" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" role="img" aria-label="XSHOP"><rect width="64" height="64" rx="14" fill="#111827"/><path d="M18 18L46 46M46 18L18 46" stroke="#ffffff" stroke-width="8" stroke-linecap="round"/></svg>
        <span class="theme-wordmark-text">{{ brandSiteName }}</span>
      </router-link>

      <!-- Desktop Menu -->
      <div class="hidden lg:flex items-center space-x-1 min-w-0 overflow-x-auto scrollbar-hide">
        <template v-for="item in menuItems" :key="item.key">
          <Button v-if="item.type === 'route'" as-child variant="ghost" size="sm"
            class="gap-1.5 text-muted-foreground whitespace-nowrap shrink-0">
            <router-link :to="item.path" active-class="!text-primary !bg-primary/10">
              <component :is="item.icon" class="w-4 h-4 shrink-0 opacity-70" />
              <span>{{ item.label }}</span>
            </router-link>
          </Button>
          <Button v-else as-child variant="ghost" size="sm"
            class="gap-1.5 text-muted-foreground whitespace-nowrap shrink-0">
            <a :href="item.path" :target="item.target" rel="noopener noreferrer">
              <component :is="item.icon" class="w-4 h-4 shrink-0 opacity-70" />
              <span>{{ item.label }}</span>
            </a>
          </Button>
        </template>
      </div>

      <!-- Right Side Actions -->
      <div class="flex items-center shrink-0 space-x-2 lg:space-x-4">

        <Button v-if="!userAuthStore.isAuthenticated" as-child variant="ghost" size="sm"
          class="hidden lg:inline-flex gap-1.5 text-muted-foreground whitespace-nowrap">
          <router-link to="/guest/orders">
            <ClipboardList class="w-4 h-4 shrink-0 opacity-70" />
            {{ t('navbar.guestOrders') }}
          </router-link>
        </Button>
        <Button v-if="!userAuthStore.isAuthenticated" as-child variant="ghost" size="sm"
          class="hidden lg:inline-flex gap-1.5 text-muted-foreground whitespace-nowrap">
          <router-link to="/auth/login">
            <LogIn class="w-4 h-4 shrink-0 opacity-70" />
            {{ t('navbar.login') }}
          </router-link>
        </Button>
        <Button v-if="userAuthStore.isAuthenticated" as-child variant="ghost" size="sm"
          class="hidden lg:inline-flex gap-1.5 text-muted-foreground whitespace-nowrap">
          <router-link to="/me">
            <User class="w-4 h-4 shrink-0 opacity-70" />
            {{ t('navbar.personalCenter') }}
          </router-link>
        </Button>
        <Button v-if="userAuthStore.isAuthenticated" variant="ghost" size="sm"
          class="hidden lg:inline-flex gap-1.5 whitespace-nowrap text-destructive hover:text-destructive hover:bg-destructive/10"
          @click="userAuthStore.logout()">
          <LogOut class="w-4 h-4 shrink-0 opacity-70" />
          {{ t('navbar.logout') }}
        </Button>
        <!-- Theme Switcher -->
        <Button variant="ghost" size="icon" class="text-muted-foreground" @click="toggleTheme">
          <Sun v-if="theme === 'dark'" class="w-4 h-4" />
          <Moon v-else class="w-4 h-4" />
        </Button>

        <Popover v-if="mobileMenuItems.length" v-model:open="mobileMenuOpen">
          <PopoverTrigger as-child>
            <Button variant="ghost" size="icon" class="text-muted-foreground lg:hidden" :aria-label="t('navbar.more')">
              <Menu class="w-4 h-4" />
            </Button>
          </PopoverTrigger>
          <PopoverContent align="end" class="w-52 p-2 lg:hidden">
            <template v-for="item in mobileMenuItems" :key="`mobile-${item.key}`">
              <router-link v-if="item.type === 'route'" :to="item.path" class="flex min-h-10 items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent hover:text-accent-foreground" @click="mobileMenuOpen = false">
                <component :is="item.icon" class="h-4 w-4 shrink-0" />
                <span>{{ item.label }}</span>
              </router-link>
              <a v-else :href="item.path" :target="item.target" rel="noopener noreferrer" class="flex min-h-10 items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent hover:text-accent-foreground" @click="mobileMenuOpen = false">
                <component :is="item.icon" class="h-4 w-4 shrink-0" />
                <span>{{ item.label }}</span>
              </a>
            </template>
          </PopoverContent>
        </Popover>

        <!-- Language Switcher -->
        <Popover v-model:open="langOpen">
          <PopoverTrigger as-child>
            <Button variant="ghost" size="sm" class="inline-flex gap-1.5 px-2 text-muted-foreground">
              <Languages class="w-4 h-4" />
              <span class="text-xs font-medium">{{ currentLocale }}</span>
            </Button>
          </PopoverTrigger>
          <PopoverContent align="end" class="w-40 p-2">
            <button v-for="lang in languages" :key="lang.code" @click="changeLanguage(lang.code)"
              class="flex min-h-10 w-full items-center justify-between rounded-md px-3 py-2 text-left text-sm transition-colors hover:bg-accent hover:text-accent-foreground"
              :class="{ 'text-primary font-semibold': appStore.locale === lang.code }">
              {{ lang.name }}
              <span v-if="appStore.locale === lang.code" class="w-1.5 h-1.5 rounded-full bg-primary"></span>
            </button>
          </PopoverContent>
        </Popover>
      </div>
    </div>

  </nav>

</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'

import { useUserAuthStore } from '../stores/userAuth'
import { useTheme } from '../utils/theme'
import { useNavConfig } from '../composables/useNavConfig'
import {
  Sun, Moon, ClipboardList, LogIn, User, LogOut, Languages, Menu,
} from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'

const { t } = useI18n()
const appStore = useAppStore()

const userAuthStore = useUserAuthStore()
const { theme, toggleTheme } = useTheme()
const { primaryNavItems, secondaryNavItems } = useNavConfig()

const langOpen = ref(false)
const mobileMenuOpen = ref(false)



const menuItems = computed(() => primaryNavItems.value.filter((item) => item.path !== '/'))
const mobileMenuItems = computed(() => secondaryNavItems.value)

const languages = [
  { code: 'zh-CN', name: '简体中文' },
  { code: 'zh-TW', name: '繁體中文' },
  { code: 'en-US', name: 'English' },
]

const currentLocale = computed(() => {
  const lang = languages.find(l => l.code === appStore.locale)
  if (!lang) return 'CN'
  return lang.code === 'en-US' ? 'EN' : (lang.code === 'zh-CN' ? '简' : '繁')
})


const brandSiteName = computed(() => {
  const text = String(appStore.config?.brand?.site_name || '').trim()
  return text !== '' ? text : 'XSHOP'
})

const changeLanguage = (langCode: string) => {
  appStore.setLocale(langCode)
  langOpen.value = false
}


</script>

<style scoped>
.scrollbar-hide {
  -ms-overflow-style: none;
  scrollbar-width: none;
}
.scrollbar-hide::-webkit-scrollbar {
  display: none;
}
</style>
