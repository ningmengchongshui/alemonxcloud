import {
  useGetAdminMetricsQuery,
  useGetAdminNodesQuery,
  useGetAdminOrdersQuery,
  useGetAdminTasksQuery
} from '@/services/cloudApi'
import { Alert, Button, PageHeader } from '@/components/ui'

export function AdminOverviewPage() {
  const orders = useGetAdminOrdersQuery()
  const nodes = useGetAdminNodesQuery()
  const tasks = useGetAdminTasksQuery()
  const metrics = useGetAdminMetricsQuery()
  const deploying = (orders.data ?? []).filter(
    order => order.status === 'deploying'
  )
  const failed = (tasks.data ?? []).filter(task => task.status === 'failed')
  const failedInstances = Array.from(
    failed.reduce((groups, task) => {
      const current = groups.get(task.instanceId) ?? []
      current.push(task)
      groups.set(task.instanceId, current)
      return groups
    }, new Map<string, typeof failed>())
  ).map(([instanceId, instanceTasks]) => ({
    instanceId,
    actions: [...new Set(instanceTasks.map(task => task.action))],
    count: instanceTasks.length,
    maxAttempts: Math.max(...instanceTasks.map(task => task.attempts))
  }))
  const online = (nodes.data ?? []).filter(
    node => node.enabled && node.lastHeartbeatAt
  )
  const lifecycleRisks = [
    [
      '部署失败',
      metrics.data?.deploymentFailed ?? 0,
      '请在实例任务中确认错误后重试部署。'
    ],
    [
      '运行资源丢失',
      metrics.data?.runtimeMissing ?? 0,
      '容器 404 不会自动重建；请先核对节点和数据目录。'
    ],
    [
      '销毁受阻',
      metrics.data?.destroyBlocked ?? 0,
      '等待节点恢复后会继续处理销毁任务。'
    ],
    [
      '离线节点影响实例',
      metrics.data?.offlineInstances ?? 0,
      '请前往节点管理查看受影响节点与清理任务。'
    ],
    [
      '24 小时租约恢复',
      metrics.data?.leaseRecoveries24h ?? 0,
      '请检查消费者进程与 RabbitMQ 连接稳定性。'
    ]
  ].filter(([, count]) => Number(count) > 0)
  async function refresh() {
    await Promise.all([
      orders.refetch(),
      nodes.refetch(),
      tasks.refetch(),
      metrics.refetch()
    ])
  }
  return (
    <section className="page super-page">
      <PageHeader
        eyebrow="平台运营"
        title="超级管理台"
        description="监控自动交付、资源健康和需要人工处理的任务。"
        actions={
          <Button
            tone="secondary"
            loading={
              orders.isFetching ||
              nodes.isFetching ||
              tasks.isFetching ||
              metrics.isFetching
            }
            onClick={() => void refresh()}
          >
            ↻ 刷新
          </Button>
        }
      />
      <section
        className="flex flex-wrap items-center gap-x-6 gap-y-2 border-y border-slate-200 py-3 text-xs text-slate-500 dark:border-slate-700 dark:text-slate-300"
        aria-label="平台概览"
      >
        <span>
          部署中{' '}
          <b className="text-slate-900 dark:text-white">{deploying.length}</b>
        </span>
        <span>
          任务积压{' '}
          <b className="text-slate-900 dark:text-white">
            {metrics.data?.taskBacklog ?? '—'}
          </b>
        </span>
        <span
          className={
            failed.length ? 'text-amber-700 dark:text-amber-200' : undefined
          }
        >
          失败任务 <b>{metrics.data?.taskFailures ?? '—'}</b>
        </span>
        <span>
          健康节点{' '}
          <b className="text-slate-900 dark:text-white">
            {nodes.data ? `${online.length}/${nodes.data.length}` : '—'}
          </b>
        </span>
        <span
          className={
            metrics.data?.urgentTickets
              ? 'text-red-700 dark:text-red-200'
              : undefined
          }
        >
          待处理工单 <b>{metrics.data?.openTickets ?? '—'}</b>
          {metrics.data?.urgentTickets
            ? `（紧急 ${metrics.data.urgentTickets}）`
            : ''}
        </span>
      </section>
      {failedInstances.length > 0 && (
        <section className="mt-5 rounded-xl border border-amber-200 bg-amber-50 p-5 dark:border-amber-900 dark:bg-amber-950">
          <h2 className="m-0 text-sm font-bold">需要处理的失败任务</h2>
          <p className="mb-0 mt-1 text-xs text-amber-800 dark:text-amber-100">
            同一实例的失败任务已合并展示。更新、重启、销毁等操作不会在这里被一键重放。
          </p>
          <div className="mt-3 grid gap-2">
            {failedInstances.slice(0, 5).map(item => (
              <article
                className="flex flex-wrap items-center justify-between gap-3 rounded-lg bg-white p-3 text-xs dark:bg-slate-900"
                key={item.instanceId}
              >
                <span>
                  <b>{item.actions.join(' / ')}</b> · 实例{' '}
                  {item.instanceId.slice(0, 14)} · {item.count} 个失败任务 ·
                  最高已尝试 {item.maxAttempts} 次
                </span>
                <Button
                  tone="secondary"
                  onClick={() => window.location.assign('/super/tasks')}
                >
                  查看并处理
                </Button>
              </article>
            ))}
          </div>
        </section>
      )}
      {lifecycleRisks.length > 0 && (
        <section className="mt-5 rounded-xl border border-red-200 bg-red-50 p-5 dark:border-red-900 dark:bg-red-950">
          <h2 className="m-0 text-sm font-bold">实例生命周期风险</h2>
          <div className="mt-3 grid gap-2 sm:grid-cols-2">
            {lifecycleRisks.map(([label, count, detail]) => (
              <article
                className="rounded-lg bg-white p-3 text-xs dark:bg-slate-900"
                key={String(label)}
              >
                <b>
                  {label} · {count}
                </b>
                <p className="mb-0 mt-1 text-slate-600 dark:text-slate-300">
                  {detail}
                </p>
              </article>
            ))}
          </div>
        </section>
      )}
      {deploying.length === 0 &&
        failed.length === 0 &&
        lifecycleRisks.length === 0 && (
          <Alert tone="success">当前没有需要人工处理的部署或失败任务。</Alert>
        )}
    </section>
  )
}
