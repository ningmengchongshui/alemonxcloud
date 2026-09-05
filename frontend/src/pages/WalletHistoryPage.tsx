import { Button, EmptyState, LoadingState, PageHeader } from '@/components/ui'
import {
  useGetAdminWalletEntriesQuery,
  useSearchAdminUsersQuery
} from '@/services/cloudApi'

function entryLabel(type: string) {
  return type === 'purchase'
    ? '服务购买'
    : type === 'renewal'
      ? '服务续费'
      : type === 'manual_credit'
        ? '管理员充值'
        : type === 'manual_debit'
          ? '管理员扣减'
          : '余额变动'
}

export function WalletHistoryPage({
  userID,
  onBack
}: {
  userID: string
  onBack: () => void
}) {
  const { data: users = [] } = useSearchAdminUsersQuery(userID)
  const { data: entries = [], isLoading } =
    useGetAdminWalletEntriesQuery(userID)
  const user = users.find(item => item.id === userID)

  return (
    <section className="page super-page">
      <PageHeader
        title="钱包流水"
        description={
          user
            ? `${user.username} · 当前余额 ${(user.balanceFen / 100).toFixed(2)} XCoin`
            : `用户 ${userID}`
        }
        actions={
          <Button tone="secondary" onClick={onBack}>
            返回用户与钱包
          </Button>
        }
      />
      <section className="overflow-hidden rounded-xl border border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-800">
        <div className="border-b border-slate-100 px-5 py-4 dark:border-slate-700">
          <p className="m-0 text-xs text-slate-500 dark:text-slate-300">
            每笔变动均记录金额、操作原因、变动后余额及操作人。
          </p>
        </div>
        {isLoading ? (
          <LoadingState>正在加载钱包流水…</LoadingState>
        ) : entries.length === 0 ? (
          <EmptyState
            title="暂无钱包流水"
            description="该用户尚未产生充值、消费或余额调整记录。"
          />
        ) : (
          <div className="divide-y divide-slate-100 dark:divide-slate-700">
            {entries.map(entry => (
              <article
                className="grid gap-x-5 gap-y-2 px-5 py-4 sm:grid-cols-[minmax(0,1fr)_auto]"
                key={entry.id}
              >
                <div>
                  <b className="text-sm text-slate-800 dark:text-white">
                    {entryLabel(entry.type)}
                  </b>
                  <p className="mt-1 text-xs text-slate-500 dark:text-slate-300">
                    {entry.note || '—'}
                  </p>
                  <small className="mt-2 block text-[11px] text-slate-400">
                    {new Date(entry.createdAt).toLocaleString('zh-CN')} ·
                    变动后余额 {(entry.balanceAfterFen / 100).toFixed(2)} XCoin
                    {entry.actorId ? ` · 操作人 ${entry.actorId}` : ''}
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
