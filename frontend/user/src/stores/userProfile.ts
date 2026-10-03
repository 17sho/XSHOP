import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'
import { memberLevelAPI, userOrderAPI, userProfileAPI } from '../api'
import type {
    ChangeEmailPayload,
    ChangeUserPasswordPayload,
    GoogleBindingData,
    PublicMemberLevel,
    SendChangeEmailCodePayload,
    TelegramAuthPayload,
    TelegramBindingData,
    UpdateUserProfilePayload,
    UserLoginLogItem,
    UserProfileData,
} from '../api'
import { useUserAuthStore } from './userAuth'
import { normalizePersonalCenterVisibility } from '../utils/personalCenterVisibility'

interface PersonalOrderSummary {
    id: number
    order_no: string
    total_amount?: string
    currency?: string
    status?: string
    created_at?: string
}

const normalizeErrorMessage = (error: unknown, fallback: string) => {
    if (error instanceof Error && error.message.trim() !== '') {
        return error.message
    }
    return fallback
}

export const useUserProfileStore = defineStore('user-profile', () => {
    const userAuthStore = useUserAuthStore()

    const profile = ref<UserProfileData | null>(null)
    const recentOrders = ref<PersonalOrderSummary[]>([])
    const ordersTotal = ref(0)
    const recentLoginLogs = ref<UserLoginLogItem[]>([])
    const telegramBinding = ref<TelegramBindingData | null>(null)
    const googleBinding = ref<GoogleBindingData | null>(null)
    const memberLevels = ref<PublicMemberLevel[]>([])

    const loadingProfile = ref(false)
    const savingProfile = ref(false)
    const loadingOrders = ref(false)
    const loadingLoginLogs = ref(false)
    const loadingTelegramBinding = ref(false)
    const bindingTelegram = ref(false)
    const unbindingTelegram = ref(false)
    const loadingGoogleBinding = ref(false)
    const bindingGoogle = ref(false)
    const unbindingGoogle = ref(false)
    const sendingCode = ref(false)
    const changingEmail = ref(false)
    const changingPassword = ref(false)

    const profileError = ref('')
    const securityError = ref('')
    let profileRequest: { token: string; promise: Promise<boolean> } | null = null


    // Reset synchronously, before any old-account promise can commit again.
    watch(() => userAuthStore.sessionGeneration, () => {
        profile.value = null
        recentOrders.value = []
        ordersTotal.value = 0
        recentLoginLogs.value = []
        telegramBinding.value = null
        googleBinding.value = null
        profileError.value = ''
        securityError.value = ''
        profileRequest = null
        loadingProfile.value = false
        savingProfile.value = false
        loadingOrders.value = false
        loadingLoginLogs.value = false
        loadingTelegramBinding.value = false
        bindingTelegram.value = false
        unbindingTelegram.value = false
        loadingGoogleBinding.value = false
        bindingGoogle.value = false
        unbindingGoogle.value = false
        sendingCode.value = false
        changingEmail.value = false
        changingPassword.value = false
    }, { flush: 'sync' })

    const displayName = computed(() => {
        if (profile.value?.nickname && profile.value.nickname.trim() !== '') {
            return profile.value.nickname
        }
        return profile.value?.email || '-'
    })

    const personalCenterVisibility = computed(() => normalizePersonalCenterVisibility(profile.value?.personal_center_visibility))

    const currentLevel = computed(() => {
        const levelId = profile.value?.member_level_id
        if (memberLevels.value.length === 0) return null
        if (!levelId) return memberLevels.value.find((l) => l.is_default) || null
        return memberLevels.value.find((l) => l.id === levelId) || null
    })

    const nextLevel = computed(() => {
        const sorted = [...memberLevels.value].sort((a, b) => a.sort_order - b.sort_order)
        if (!currentLevel.value) {
            return sorted.length > 0 ? sorted[0] : null
        }
        const idx = sorted.findIndex((l) => l.id === currentLevel.value!.id)
        if (idx < 0 || idx >= sorted.length - 1) return null
        return sorted[idx + 1]
    })

    const upgradeProgress = computed(() => {
        const next = nextLevel.value
        if (!next) return null
        const recharged = Number(profile.value?.total_recharged || 0)
        const spent = Number(profile.value?.total_spent || 0)
        const rechargeThreshold = next.recharge_threshold
        const spendThreshold = next.spend_threshold
        return {
            rechargePercent: rechargeThreshold > 0 ? Math.min(100, (recharged / rechargeThreshold) * 100) : null,
            spendPercent: spendThreshold > 0 ? Math.min(100, (spent / spendThreshold) * 100) : null,
            recharged,
            spent,
            rechargeThreshold,
            spendThreshold,
        }
    })

    const loadMemberLevels = async () => {
        try {
            const response = await memberLevelAPI.list()
            memberLevels.value = Array.isArray(response.data.data) ? response.data.data.map((level: PublicMemberLevel) => {
                const bundled = ['standard', 'silver', 'gold', 'diamond'].includes(level.slug)
                const previewIcon = `/uploads/member-icons/${level.slug}-v1.svg`
                return { ...level, icon: bundled && (!level.icon || level.icon === previewIcon) ? `/assets/member-icons/${level.slug}-v1.svg` : level.icon }
            }) : []
        } catch {
            memberLevels.value = []
        }
    }

    const clearProfileError = () => {
        profileError.value = ''
    }

    const clearSecurityError = () => {
        securityError.value = ''
    }

    const loadProfile = async () => {
        const session = userAuthStore.sessionGeneration
        const isCurrent = () => Boolean(userAuthStore.token) && session === userAuthStore.sessionGeneration
        const requestToken = userAuthStore.token
        loadingProfile.value = true
        clearProfileError()
        try {
            const response = await userProfileAPI.current()
            if (!isCurrent()) return false
            const data = response.data.data
            if (!requestToken || requestToken !== userAuthStore.token) return false
            profile.value = data
            userAuthStore.syncUserProfile(data)
            return true
        } catch (error) {
            if (!isCurrent()) return false
            if (requestToken !== userAuthStore.token) return false
            profile.value = null
            profileError.value = normalizeErrorMessage(error, '加载个人资料失败')
            return false
        } finally {
            if (isCurrent()) loadingProfile.value = false
        }
    }

    const ensureProfileLoaded = (fresh = false) => {
        if (!fresh && profile.value) return Promise.resolve(true)
        const requestToken = userAuthStore.token
        if (profileRequest?.token === requestToken) return profileRequest.promise
        const promise = loadProfile().finally(() => {
            if (profileRequest?.promise === promise) profileRequest = null
        })
        profileRequest = { token: requestToken, promise }
        return promise
    }

    const saveProfile = async (payload: UpdateUserProfilePayload) => {
        const session = userAuthStore.sessionGeneration
        const isCurrent = () => Boolean(userAuthStore.token) && session === userAuthStore.sessionGeneration
        savingProfile.value = true
        clearProfileError()
        try {
            const response = await userProfileAPI.updateProfile(payload)
            if (!isCurrent()) return false
            const data = response.data.data
            profile.value = data
            userAuthStore.syncUserProfile(data)
            return true
        } catch (error) {
            if (!isCurrent()) return false
            profileError.value = normalizeErrorMessage(error, '保存个人资料失败')
            return false
        } finally {
            if (isCurrent()) savingProfile.value = false
        }
    }

    const sendChangeEmailCode = async (payload: SendChangeEmailCodePayload) => {
        const session = userAuthStore.sessionGeneration
        const isCurrent = () => Boolean(userAuthStore.token) && session === userAuthStore.sessionGeneration
        sendingCode.value = true
        clearSecurityError()
        try {
            await userProfileAPI.sendChangeEmailCode(payload)
            if (!isCurrent()) return false
            return true
        } catch (error) {
            if (!isCurrent()) return false
            securityError.value = normalizeErrorMessage(error, '发送验证码失败')
            return false
        } finally {
            if (isCurrent()) sendingCode.value = false
        }
    }

    const changeEmail = async (payload: ChangeEmailPayload) => {
        const session = userAuthStore.sessionGeneration
        const isCurrent = () => Boolean(userAuthStore.token) && session === userAuthStore.sessionGeneration
        changingEmail.value = true
        clearSecurityError()
        try {
            const response = await userProfileAPI.changeEmail(payload)
            if (!isCurrent()) return false
            const data = response.data.data
            profile.value = data
            userAuthStore.syncUserProfile(data)
            return true
        } catch (error) {
            if (!isCurrent()) return false
            securityError.value = normalizeErrorMessage(error, '更换邮箱失败')
            return false
        } finally {
            if (isCurrent()) changingEmail.value = false
        }
    }

    const changePassword = async (payload: ChangeUserPasswordPayload) => {
        const session = userAuthStore.sessionGeneration
        const isCurrent = () => Boolean(userAuthStore.token) && session === userAuthStore.sessionGeneration
        changingPassword.value = true
        clearSecurityError()
        try {
            await userProfileAPI.changePassword(payload)
            if (!isCurrent()) return false
            return true
        } catch (error) {
            if (!isCurrent()) return false
            securityError.value = normalizeErrorMessage(error, '修改密码失败')
            return false
        } finally {
            if (isCurrent()) changingPassword.value = false
        }
    }

    const loadRecentOrders = async (limit = 5) => {
        const session = userAuthStore.sessionGeneration
        const isCurrent = () => Boolean(userAuthStore.token) && session === userAuthStore.sessionGeneration
        loadingOrders.value = true
        try {
            const response = await userOrderAPI.list({ page: 1, page_size: limit })
            if (!isCurrent()) return false
            const data = response.data.data
            recentOrders.value = Array.isArray(data) ? (data as PersonalOrderSummary[]) : []
            ordersTotal.value = response.data.pagination?.total ?? recentOrders.value.length
            return true
        } catch {
            if (!isCurrent()) return false
            recentOrders.value = []
            ordersTotal.value = 0
            return false
        } finally {
            if (isCurrent()) loadingOrders.value = false
        }
    }


    const loadRecentLoginLogs = async (limit = 5) => {
        const session = userAuthStore.sessionGeneration
        const isCurrent = () => Boolean(userAuthStore.token) && session === userAuthStore.sessionGeneration
        loadingLoginLogs.value = true
        try {
            const response = await userProfileAPI.loginLogs({ page: 1, page_size: limit })
            if (!isCurrent()) return false
            const data = response.data.data
            recentLoginLogs.value = Array.isArray(data) ? (data as UserLoginLogItem[]) : []
            return true
        } catch {
            if (!isCurrent()) return false
            recentLoginLogs.value = []
            return false
        } finally {
            if (isCurrent()) loadingLoginLogs.value = false
        }
    }

    const loadTelegramBinding = async () => {
        const session = userAuthStore.sessionGeneration
        const isCurrent = () => Boolean(userAuthStore.token) && session === userAuthStore.sessionGeneration
        loadingTelegramBinding.value = true
        clearSecurityError()
        try {
            const response = await userProfileAPI.getTelegramBinding()
            if (!isCurrent()) return false
            telegramBinding.value = response.data.data || { bound: false }
            return true
        } catch (error) {
            if (!isCurrent()) return false
            telegramBinding.value = null
            securityError.value = normalizeErrorMessage(error, '加载 Telegram 绑定信息失败')
            return false
        } finally {
            if (isCurrent()) loadingTelegramBinding.value = false
        }
    }

    const bindTelegram = async (payload: TelegramAuthPayload) => {
        const session = userAuthStore.sessionGeneration
        const isCurrent = () => Boolean(userAuthStore.token) && session === userAuthStore.sessionGeneration
        bindingTelegram.value = true
        clearSecurityError()
        try {
            const response = await userProfileAPI.bindTelegram(payload)
            if (!isCurrent()) return false
            telegramBinding.value = response.data.data || { bound: true }
            return true
        } catch (error) {
            if (!isCurrent()) return false
            securityError.value = normalizeErrorMessage(error, '绑定 Telegram 失败')
            return false
        } finally {
            if (isCurrent()) bindingTelegram.value = false
        }
    }

    const bindTelegramMiniApp = async (initData: string) => {
        const session = userAuthStore.sessionGeneration
        const isCurrent = () => Boolean(userAuthStore.token) && session === userAuthStore.sessionGeneration
        bindingTelegram.value = true
        clearSecurityError()
        try {
            const response = await userProfileAPI.bindTelegramMiniApp({ init_data: initData })
            if (!isCurrent()) return false
            telegramBinding.value = response.data.data || { bound: true }
            return true
        } catch (error) {
            if (!isCurrent()) return false
            securityError.value = normalizeErrorMessage(error, '绑定 Telegram 失败')
            return false
        } finally {
            if (isCurrent()) bindingTelegram.value = false
        }
    }

    const unbindTelegram = async () => {
        const session = userAuthStore.sessionGeneration
        const isCurrent = () => Boolean(userAuthStore.token) && session === userAuthStore.sessionGeneration
        unbindingTelegram.value = true
        clearSecurityError()
        try {
            await userProfileAPI.unbindTelegram()
            if (!isCurrent()) return false
            telegramBinding.value = { bound: false }
            return true
        } catch (error) {
            if (!isCurrent()) return false
            securityError.value = normalizeErrorMessage(error, '解绑 Telegram 失败')
            return false
        } finally {
            if (isCurrent()) unbindingTelegram.value = false
        }
    }

    const loadGoogleBinding = async () => {
        const session = userAuthStore.sessionGeneration
        const isCurrent = () => Boolean(userAuthStore.token) && session === userAuthStore.sessionGeneration
        loadingGoogleBinding.value = true
        clearSecurityError()
        try {
            const response = await userProfileAPI.getGoogleBinding()
            if (!isCurrent()) return false
            googleBinding.value = response.data.data || { bound: false }
            return true
        } catch (error) {
            if (!isCurrent()) return false
            googleBinding.value = null
            securityError.value = normalizeErrorMessage(error, '加载 Google 绑定信息失败')
            return false
        } finally {
            if (isCurrent()) loadingGoogleBinding.value = false
        }
    }

    const bindGoogle = async (credential: string) => {
        const session = userAuthStore.sessionGeneration
        const isCurrent = () => Boolean(userAuthStore.token) && session === userAuthStore.sessionGeneration
        bindingGoogle.value = true
        clearSecurityError()
        try {
            const response = await userProfileAPI.bindGoogle({ credential })
            if (!isCurrent()) return false
            googleBinding.value = response.data.data || { bound: true }
            return true
        } catch (error) {
            if (!isCurrent()) return false
            securityError.value = normalizeErrorMessage(error, '绑定 Google 失败')
            return false
        } finally {
            if (isCurrent()) bindingGoogle.value = false
        }
    }

    const exchangeGoogleRedirectBind = async () => {
        const session = userAuthStore.sessionGeneration
        const isCurrent = () => Boolean(userAuthStore.token) && session === userAuthStore.sessionGeneration
        bindingGoogle.value = true
        clearSecurityError()
        try {
            const response = await userProfileAPI.googleRedirectBindExchange()
            if (!isCurrent()) return false
            googleBinding.value = response.data.data || { bound: true }
            return true
        } catch (error) {
            if (!isCurrent()) return false
            securityError.value = normalizeErrorMessage(error, '绑定 Google 失败')
            return false
        } finally {
            if (isCurrent()) bindingGoogle.value = false
        }
    }

    const unbindGoogle = async () => {
        const session = userAuthStore.sessionGeneration
        const isCurrent = () => Boolean(userAuthStore.token) && session === userAuthStore.sessionGeneration
        unbindingGoogle.value = true
        clearSecurityError()
        try {
            await userProfileAPI.unbindGoogle()
            if (!isCurrent()) return false
            googleBinding.value = { bound: false }
            return true
        } catch (error) {
            if (!isCurrent()) return false
            securityError.value = normalizeErrorMessage(error, '解绑 Google 失败')
            return false
        } finally {
            if (isCurrent()) unbindingGoogle.value = false
        }
    }

    return {
        profile,
        recentOrders,
        ordersTotal,
        recentLoginLogs,
        telegramBinding,
        googleBinding,
        memberLevels,
        currentLevel,
        nextLevel,
        upgradeProgress,
        loadingProfile,
        savingProfile,
        loadingOrders,
        loadingLoginLogs,
        loadingTelegramBinding,
        bindingTelegram,
        unbindingTelegram,
        loadingGoogleBinding,
        bindingGoogle,
        unbindingGoogle,
        sendingCode,
        changingEmail,
        changingPassword,
        profileError,
        securityError,
        displayName,
        personalCenterVisibility,
        clearProfileError,
        clearSecurityError,
        loadProfile,
        ensureProfileLoaded,
        saveProfile,
        sendChangeEmailCode,
        changeEmail,
        changePassword,
        loadRecentOrders,
        loadMemberLevels,
        loadRecentLoginLogs,
        loadTelegramBinding,
        bindTelegram,
        bindTelegramMiniApp,
        unbindTelegram,
        loadGoogleBinding,
        bindGoogle,
        exchangeGoogleRedirectBind,
        unbindGoogle,
    }
})
