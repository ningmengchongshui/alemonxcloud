import { XCoinAmount } from '@/components/XCoinMark'

export function BalanceSettlement({
  balanceFen,
  payableFen
}: {
  balanceFen?: number
  payableFen?: number
}) {
  if (balanceFen === undefined || payableFen === undefined) {
    return (
      <div className="rounded-lg border border-slate-200 px-3 py-2.5 text-xs text-slate-500 dark:border-slate-700 dark:text-slate-300">
        正在确认本次支付金额与可用余额…
      </div>
    )
  }
  const remaining = balanceFen - payableFen
  const sufficient = remaining >= 0
  return (
    <section
      className={`rounded-lg border px-3 py-3 ${sufficient ? 'border-emerald-200 bg-emerald-50/50 dark:border-emerald-900 dark:bg-emerald-950/30' : 'border-amber-200 bg-amber-50/60 dark:border-amber-900 dark:bg-amber-950/30'}`}
      aria-live="polite"
    >
      <div className="grid grid-cols-[1fr_auto_1fr_auto_1fr] items-center gap-2 text-[10px] text-slate-500 dark:text-slate-300">
        <span className="min-w-0">
          <span className="block">当前余额</span>
          <b className="mt-1 block text-xs text-slate-900 dark:text-white">
            <XCoinAmount value={(balanceFen / 100).toFixed(2)} />
          </b>
        </span>
        <span aria-hidden="true">−</span>
        <span className="min-w-0">
          <span className="block">本次应付</span>
          <b className="mt-1 block text-xs text-slate-900 dark:text-white">
            <XCoinAmount value={(payableFen / 100).toFixed(2)} />
          </b>
        </span>
        <span aria-hidden="true">＝</span>
        <span className="min-w-0">
          <span className="block">支付后</span>
          <b
            className={`mt-1 block text-xs ${sufficient ? 'text-emerald-700 dark:text-emerald-200' : 'text-amber-700 dark:text-amber-200'}`}
          >
            <XCoinAmount value={(Math.max(remaining, 0) / 100).toFixed(2)} />
          </b>
        </span>
      </div>
      <p
        className={`mb-0 mt-2 text-[11px] font-bold ${sufficient ? 'text-emerald-700 dark:text-emerald-200' : 'text-amber-800 dark:text-amber-100'}`}
      >
        {sufficient ? (
          ''
        ) : (
          <>
            余额不足，还差{' '}
            <XCoinAmount value={(Math.abs(remaining) / 100).toFixed(2)} />。
          </>
        )}
      </p>
    </section>
  )
}
