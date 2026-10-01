import { computed, onScopeDispose, readonly, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

// One shared name; existing page loaders still own icons, captcha and config.
const siteName = ref('')
let generation = 0
const checkedName = (value: unknown) => typeof value === 'string' ? value.trim() : ''

export function useAdminBrand() {
  const { t, locale } = useI18n()
  let disposed = false
  onScopeDispose(() => { disposed = true })
  const name = computed(() => siteName.value || t('admin.brandFallback'))
  const title = computed(() => t('admin.brand', { name: name.value }))
  const controlRoom = computed(() => t('admin.layout.controlRoom', { name: name.value }))
  const initials = computed(() => Array.from(name.value).slice(0, 2).join(''))
  watch([locale, siteName], () => { document.title = title.value }, { immediate: true })
  // A ticket also fences older public reads when a save/new page takes ownership.
  function beginLoad() {
    const ticket = ++generation
    return (value: unknown) => {
      if (disposed || ticket !== generation) return false
      siteName.value = checkedName(value)
      return true
    }
  }
  return { siteName: readonly(siteName), name, title, controlRoom, initials, beginLoad }
}
