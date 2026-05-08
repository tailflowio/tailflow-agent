export function fmtDur(ms: number | null | undefined): string {
  if (ms == null) return '—'
  if (ms < 1000) return ms + 'ms'
  if (ms < 60000) return (ms / 1000).toFixed(1) + 's'
  return Math.floor(ms / 60000) + 'm' + Math.floor((ms % 60000) / 1000) + 's'
}

export function fmtAgo(ts: string | number | Date): string {
  const t = typeof ts === 'string' || ts instanceof Date ? new Date(ts).getTime() : ts
  const s = Math.floor((Date.now() - t) / 1000)
  if (s < 5) return 'now'
  if (s < 60) return s + 's ago'
  if (s < 3600) return Math.floor(s / 60) + 'm ago'
  if (s < 86400) return Math.floor(s / 3600) + 'h ago'
  return Math.floor(s / 86400) + 'd ago'
}

export function statusDot(s: string): string {
  if (s === 'success') return 'bg-emerald-400'
  if (s === 'failed') return 'bg-red-400'
  if (s === 'cancelled') return 'bg-orange-400'
  if (s === 'running') return 'bg-amber-400 animate-pulse'
  if (s === 'waiting') return 'bg-amber-400 animate-pulse'
  if (s === 'skipped') return 'bg-g-7'
  return 'bg-g-6'
}

export function statusBadgeClass(s: string): string {
  if (s === 'success') return 'bg-emerald-400/15 text-emerald-400'
  if (s === 'failed') return 'bg-red-400/15 text-red-400'
  if (s === 'cancelled') return 'bg-orange-400/15 text-orange-400'
  if (s === 'running') return 'bg-amber-400/15 text-amber-400'
  if (s === 'waiting') return 'bg-amber-400/15 text-amber-400'
  if (s === 'pending') return 'bg-g-4 text-g-9'
  if (s === 'skipped') return 'bg-g-4 text-g-8'
  return 'bg-g-4 text-g-9'
}
