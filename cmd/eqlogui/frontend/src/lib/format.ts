const intFmt = new Intl.NumberFormat('en-US', {
  maximumFractionDigits: 0,
})

const float1Fmt = new Intl.NumberFormat('en-US', {
  minimumFractionDigits: 1,
  maximumFractionDigits: 1,
})

export function formatInt(n: number | bigint): string {
  if (typeof n === 'bigint') return intFmt.format(n)
  if (typeof n !== 'number' || !Number.isFinite(n)) return ''
  return intFmt.format(Math.round(n))
}

export function formatFloat1(n: number): string {
  if (typeof n !== 'number' || !Number.isFinite(n)) return ''
  return float1Fmt.format(n)
}

export function formatCompact(n: number): string {
  if (typeof n !== 'number' || !Number.isFinite(n)) return ''

  const sign = n < 0 ? '-' : ''
  const abs = Math.abs(n)

  // For small values, compact formatting tends to be harder to read than commas.
  if (abs < 10_000) return sign + formatInt(abs)

  const units = [
    { value: 1_000_000_000_000, suffix: 'T' },
    { value: 1_000_000_000, suffix: 'B' },
    { value: 1_000_000, suffix: 'M' },
    { value: 1_000, suffix: 'K' },
  ]

  for (const u of units) {
    if (abs >= u.value) {
      const v = abs / u.value
      let s = v.toFixed(1)
      if (s.endsWith('.0')) s = s.slice(0, -2)
      return sign + s + u.suffix
    }
  }

  return sign + formatInt(abs)
}

export function formatNumberForTable(n: number, mode: 'int' | 'dps' | 'compact'): string {
  switch (mode) {
    case 'int':
      return formatInt(n)
    case 'dps':
      return formatFloat1(n)
    case 'compact':
      return formatCompact(n)
    default:
      return ''
  }
}
