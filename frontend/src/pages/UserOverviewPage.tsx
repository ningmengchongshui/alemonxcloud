import { useGetWalletQuery } from '@/services/cloudApi'
import { Button, InlineAction, PageHeader } from '@/components/ui'
import { XCoinAmount } from '@/components/XCoinMark'
import type { Instance } from '@/types/cloud'

function isRunning(status: string) {
  return ['running', 'active', 'online'].includes(status.toLowerCase())
}
function isProgressing(status: string) {
  return ['creating', 'deploying', 'pending'].includes(status.toLowerCase())
}

export function UserOverviewPage({
  instances,
  loading,
  onCreate,
  onInstances,
  onWallet
}: {
  instances: Instance[]
  loading: boolean
  onCreate: () => void
  onInstances: () => void
  onWallet: () => void
}) {
  const { data: wallet } = useGetWalletQuery()
  const running = instances.filter(item => isRunning(item.status)).length
  const progressing = instances.filter(item =>
    isProgressing(item.status)
  ).length
  return (
    <section className="page me-page dashboard-page">
      <PageHeader
        title="工作台"
        description="服务状态、余额和待处理事项一目了然。"
        actions={
          <Button onClick={onCreate}>
            <span aria-hidden="true">＋</span>创建服务
          </Button>
        }
      />
      <section
        className="flex flex-wrap items-center gap-x-6 gap-y-2 border-y border-slate-200 py-3 text-xs text-slate-500 dark:border-slate-700 dark:text-slate-300"
        aria-label="服务概览"
      >
        <span>
          运行{' '}
          <b className="text-slate-900 dark:text-white">
            {loading ? '—' : `${running} / ${instances.length}`}
          </b>
        </span>
        <InlineAction onClick={onInstances}>管理实例 →</InlineAction>
        <span>
          余额{' '}
          <b className="inline-flex items-center gap-1 text-slate-900 dark:text-white">
            {wallet ? (
              <XCoinAmount value={(wallet.balanceFen / 100).toFixed(2)} />
            ) : (
              '同步中'
            )}
          </b>
        </span>
        <InlineAction onClick={onWallet}>查看流水 →</InlineAction>
        <span
          className={
            progressing ? 'text-amber-700 dark:text-amber-200' : undefined
          }
        >
          {progressing ? `${progressing} 个服务部署中` : '没有待处理服务'}
        </span>
      </section>
    </section>
  )
}
