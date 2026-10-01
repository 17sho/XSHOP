export const VAULT_PRODUCT_COVER_CLASSES = [
  'bg-[linear-gradient(135deg,#7b74f2,var(--red))]',
  'bg-[linear-gradient(135deg,#1cc0bf,var(--teal))]',
  'bg-[linear-gradient(135deg,#9b6cf5,var(--plum))]',
  'bg-[linear-gradient(135deg,#f7bd4e,var(--gold))]',
  'bg-[linear-gradient(135deg,#3a3950,var(--ink))]',
] as const

export const getVaultProductCoverClass = (index: number) => (
  VAULT_PRODUCT_COVER_CLASSES[index % VAULT_PRODUCT_COVER_CLASSES.length]
)

type VaultStockIcons<Icon> = {
  soldOut: Icon
  lowStock: Icon
  available: Icon
}

type VaultStockPresentationInput<Icon> = {
  soldOut: boolean
  stockStatus?: string
  stockLabel: string
  soldOutLabel: string
  icons: VaultStockIcons<Icon>
}

export const getVaultStockPresentation = <Icon>({
  soldOut,
  stockStatus,
  stockLabel,
  soldOutLabel,
  icons,
}: VaultStockPresentationInput<Icon>) => {
  if (soldOut) {
    return {
      tone: 'bg-secondary text-muted-foreground',
      icon: icons.soldOut,
      label: soldOutLabel,
    }
  }
  if (stockStatus === 'low_stock') {
    return {
      tone: 'bg-[color:var(--gold-soft)] text-[color:var(--gold-strong)]',
      icon: icons.lowStock,
      label: stockLabel,
    }
  }
  return {
    tone: 'bg-[color:var(--teal-soft)] text-[color:var(--teal-strong)]',
    icon: icons.available,
    label: stockLabel,
  }
}
