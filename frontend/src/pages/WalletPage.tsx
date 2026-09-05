import { EmptyState, LoadingState, PageHeader } from '@/components/ui'
import { XCoinAmount } from '@/components/XCoinMark'
import {
  useGetWalletEntriesQuery,
  useGetWalletQuery
} from '@/services/cloudApi'

function label(type: string) {
  return type === 'purchase'
    ? '服务购买'
    : type === 'renewal'
      ? '服务续费'
      : type === 'manual_credit'
        ? '管理员充值'
        : type === 'manual_debit'
          ? '管理员扣减'
          : type === 'refund'
            ? '订单退款'
            : '余额变动'
}

export function WalletPage() {
  const { data: wallet, isLoading: walletLoading } = useGetWalletQuery()
  const { data: entries = [], isLoading: entriesLoading } =
    useGetWalletEntriesQuery()
  return (
    <section className="page me-page">
      <PageHeader
        title="钱包"
        description="每一笔充值、扣减、购买和续费都可追溯。"
      />
      <section className="overflow-hidden rounded-xl border border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-800">
        <div className="flex items-center justify-between gap-4 border-b border-slate-100 px-5 py-4 dark:border-slate-700 max-[560px]:items-start">
          <div>
            <h2 className="m-0 text-sm font-bold">账本流水</h2>
            <p className="mt-1 text-xs text-slate-500 dark:text-slate-300">
              余额永久有效
            </p>
          </div>
          <p className="m-0 text-right text-xs text-slate-500 dark:text-slate-300">
            可用余额
            <br />
            <strong className="inline-flex items-center gap-1 text-lg text-slate-900 dark:text-white">
              {walletLoading ? (
                '—'
              ) : (
                <XCoinAmount
                  value={((wallet?.balanceFen ?? 0) / 100).toFixed(2)}
                />
              )}
            </strong>
          </p>
        </div>
        {entriesLoading ? (
          <LoadingState>正在加载钱包流水…</LoadingState>
        ) : entries.length === 0 ? (
          <EmptyState
            title="暂无钱包流水"
            description="充值、扣减和服务消费记录将显示在这里。"
          />
        ) : (
          <div className="divide-y divide-slate-100 dark:divide-slate-700">
            {entries.map(entry => (
              <article
                className="grid gap-2 px-5 py-4 sm:grid-cols-[1fr_auto]"
                key={entry.id}
              >
                <div>
                  <b className="text-sm">{label(entry.type)}</b>
                  <p className="mt-1 text-xs text-slate-500 dark:text-slate-300">
                    {entry.note || '—'}
                  </p>
                  <small className="mt-2 block text-[11px] text-slate-400">
                    {new Date(entry.createdAt).toLocaleString('zh-CN')} ·
                    变动后余额 {(entry.balanceAfterFen / 100).toFixed(2)} XCoin
                    {entry.orderId
                      ? ` · 订单 ${entry.orderId.slice(0, 14)}`
                      : ''}
                  </small>
                </div>
                <strong
                  className={
                    entry.amountFen >= 0
                      ? 'text-emerald-700 dark:text-emerald-300'
                      : 'text-red-700 dark:text-red-300'
                  }
                >
                  {entry.amountFen >= 0 ? '+' : ''}
                  {(entry.amountFen / 100).toFixed(2)} XCoin
                </strong>
              </article>
            ))}
          </div>
        )}
      </section>
    </section>
  )
}
