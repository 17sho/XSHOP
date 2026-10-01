import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { CheckoutItem } from '../types/checkout'
import { readBuyNowItem, writeBuyNowItem, readDurableBuyNowItem, writeDurableBuyNowItem } from '../utils/buyNowPersistence'
import { getBrowserStorage } from '../utils/browserStorage'

const buyNowStorage = getBrowserStorage('sessionStorage')
const durableStorage = getBrowserStorage('localStorage')

export const useBuyNowStore = defineStore('buyNow', () => {
    const sessionItem = readBuyNowItem(buyNowStorage)
    const durableItem = readDurableBuyNowItem(durableStorage)
    if (sessionItem && !durableItem) writeDurableBuyNowItem(durableStorage, sessionItem)
    const item = ref<CheckoutItem | null>(sessionItem ?? durableItem)

    const hasItem = computed(() => item.value !== null)

    const setItem = (newItem: CheckoutItem) => {
        item.value = { ...newItem }
        writeBuyNowItem(buyNowStorage, item.value)
        writeDurableBuyNowItem(durableStorage, item.value)
    }

    const clear = () => {
        item.value = null
        writeBuyNowItem(buyNowStorage, null)
        writeDurableBuyNowItem(durableStorage, null)
    }

    return {
        item,
        hasItem,
        setItem,
        clear,
    }
})
