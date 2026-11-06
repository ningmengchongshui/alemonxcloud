import type { ReactNode } from 'react'
import { Alert, Button, LoadingState, PageHeader } from '@/components/ui'
import { useGetInstanceResourceQuery } from '@/services/cloudApi'
import type { Instance, Order } from '@/types/cloud'
import { InstancesPage } from './InstancesPage'
import { InstanceLogsPage } from './InstanceLogsPage'
import { InstanceExecutionPage } from './InstanceExecutionPage'

const tabs = [
  ['overview', '概览'],
  ['resources', '资源与状态'],
  ['manage', '实例管理'],
  ['logs', '运行日志'],
  ['executions', '执行记录']
] as const
const time = (value?: string) =>
  value ? new Date(value).toLocaleString('zh-CN') : '暂无记录'
const statuses: Record<string, string> = {
  running: '运行中',
  stopped: '已关机',
  deploying: '部署中',
  pending: '等待中',
  destroyed: '已销毁',
  destroy_scheduled: '待销毁',
  deployment_failed: '部署失败',
  missing: '资源异常',
  purged: '已清除'
}
function Facts({
  title,
  items
}: {
  title: string
  items: [string, ReactNode][]
}) {
  return (
    <article className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-700 dark:bg-slate-800">
      <h2 className="mb-4 mt-0 text-base font-bold">{title}</h2>
      <dl className="m-0 grid gap-4">
        {items.map(([label, value]) => (
          <div key={label} className="grid gap-1 sm:grid-cols-3">
            <dt className="text-sm text-slate-500 dark:text-slate-400">
              {label}
            </dt>
            <dd className="m-0 break-all text-sm sm:col-span-2">
              {value ?? '暂未上报'}
            </dd>
          </div>
        ))}
      </dl>
    </article>
  )
}
export function InstanceDetailPage({
  instanceID,
  instance,
  orders,
  loading,
  tab,
  navigate
}: {
  instanceID: string
  instance?: Instance
  orders: Order[]
  loading: boolean
  tab: string
  navigate: (path: string) => void
}) {
  const { data, isLoading, isError, isFetching, refetch } =
    useGetInstanceResourceQuery(instanceID, {
      pollingInterval: 15000,
      skipPollingIfUnfocused: true
    })
  const base = `/me/instances/${encodeURIComponent(instanceID)}`
  const runtime =
    data?.status.runtimeState ||
    instance?.runtimeStatus ||
    instance?.status ||
    ''
  const back = () => navigate('/me/instances')
  return (
    <section className="page me-page">
      <PageHeader
        title={instance?.name || '实例详情'}
        description={instanceID}
        actions={
          <div className="flex flex-wrap gap-2">
            <Button tone="secondary" onClick={back}>
              返回我的实例
            </Button>
            <Button
              tone="secondary"
              loading={isFetching}
              onClick={() => void refetch()}
            >
              刷新状态
            </Button>
            <Button
              disabled={runtime !== 'running' || Boolean(instance?.activeTask)}
              onClick={() =>
                navigate(`/terminal/${encodeURIComponent(instanceID)}`)
              }
            >
              连接终端
            </Button>
          </div>
        }
      />
      <nav
        aria-label="实例功能"
        className="flex gap-1 overflow-x-auto border-b border-slate-200 dark:border-slate-700"
      >
        {tabs.map(([id, label]) => (
          <a
            key={id}
            href={`${base}/${id}`}
            aria-current={tab === id ? 'page' : undefined}
            onClick={event => {
              if (
                event.button === 0 &&
                !event.metaKey &&
                !event.ctrlKey &&
                !event.shiftKey &&
                !event.altKey
              ) {
                event.preventDefault()
                navigate(`${base}/${id}`)
              }
            }}
            className={`shrink-0 border-b-2 px-4 py-3 text-sm font-semibold focus-visible:outline-2 ${tab === id ? 'border-blue-600 text-blue-600 dark:text-blue-300' : 'border-transparent text-slate-500 hover:text-blue-600 dark:text-slate-300'}`}
          >
            {label}
          </a>
        ))}
      </nav>
      {isError && (
        <Alert tone="error">
          实例状态读取失败，请点击刷新重试。实例可能已移除，或你没有访问权限。
        </Alert>
      )}
      {(loading || isLoading) && <LoadingState>正在读取实例信息…</LoadingState>}
      {!loading && !instance && (
        <Alert>暂无此实例的基本信息，请返回列表确认实例是否存在。</Alert>
      )}
      {tab === 'logs' ? (
        <InstanceLogsPage
          instanceID={instanceID}
          instance={instance}
          onBack={back}
        />
      ) : tab === 'executions' ? (
        <InstanceExecutionPage
          instanceID={instanceID}
          instance={instance}
          onBack={back}
        />
      ) : tab === 'manage' ? (
        instance && (
          <InstancesPage
            instances={[instance]}
            orders={orders}
            loading={loading}
            onCreate={() => navigate('/me/create')}
            onOpenLogs={() => navigate(`${base}/logs`)}
            onOpenTerminal={() =>
              navigate(`/terminal/${encodeURIComponent(instanceID)}`)
            }
            onOpenExecutions={() => navigate(`${base}/executions`)}
          />
        )
      ) : (
        <div className="grid gap-4 py-4">
          <div className="grid gap-4 sm:grid-cols-3">
            {[
              ['运行状态', statuses[runtime] || runtime || '暂未上报'],
              [
                '已生效 CPU 配额',
                data?.status.observedCpu !== undefined
                  ? `${data.status.observedCpu} 核`
                  : '暂未上报'
              ],
              [
                '已生效内存配额',
                data?.status.observedMemoryMB !== undefined
                  ? `${data.status.observedMemoryMB} MB`
                  : '暂未上报'
              ]
            ].map(([label, value]) => (
              <article
                key={label}
                className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-700 dark:bg-slate-800"
              >
                <p className="m-0 text-sm text-slate-500 dark:text-slate-400">
                  {label}
                </p>
                <p className="mb-0 mt-3 text-xl font-bold">{value}</p>
              </article>
            ))}
          </div>
          <div className="rounded-xl border border-slate-200 bg-slate-50 p-5 dark:border-slate-700 dark:bg-slate-900">
            <h2 className="m-0 text-base font-bold">当前资源占用</h2>
            <p className="mb-0 text-sm text-slate-500 dark:text-slate-400">
              CPU
              使用率、内存用量、磁盘用量及网络流量暂未接入监控。上方配额表示实例可用资源上限，不代表实时占用。
            </p>
          </div>
          {tab === 'overview' ? (
            <div className="grid items-start gap-4 lg:grid-cols-2">
              <Facts
                title="基本信息"
                items={[
                  ['实例名称', instance?.name],
                  ['实例 ID', instanceID],
                  ['镜像', instance?.image],
                  ['版本', instance?.version],
                  ['资源规格', instance?.spec],
                  ['当前套餐', instance?.currentPlanName],
                  ['创建时间', time(instance?.createdAt)]
                ]}
              />
              <Facts
                title="服务与访问"
                items={[
                  ['服务开始', time(instance?.serviceStartsAt)],
                  ['服务到期', time(instance?.serviceExpiresAt)],
                  [
                    '服务地址',
                    instance?.terminalOnly
                      ? '仅支持终端访问'
                      : instance?.ip || '暂无地址'
                  ],
                  ['容器名称', instance?.containerName],
                  [
                    '宿主机访问',
                    instance?.hostAccessApplied ? '已生效' : '未启用'
                  ],
                  ['最近观测', time(data?.status.observedAt)],
                  [
                    '当前任务',
                    instance?.activeTask
                      ? '处理中，请在执行记录查看进度'
                      : '无进行中的任务'
                  ]
                ]}
              />
            </div>
          ) : (
            <>
              <Facts
                title="资源同步状态"
                items={[
                  [
                    '生命周期',
                    statuses[data?.status.lifecycle || ''] ||
                      data?.status.lifecycle
                  ],
                  ['最近观测', time(data?.status.observedAt)],
                  ['资源版本', data?.metadata.resourceVersion],
                  ['已观测版本', data?.status.observedGeneration]
                ]}
              />
              <Facts
                title="状态检查"
                items={
                  data?.conditions?.length
                    ? data.conditions.map(condition => [
                        condition.type,
                        `${condition.status} · ${condition.message || condition.reason || '无补充信息'} · ${time(condition.updatedAt)}`
                      ])
                    : [['检查结果', '暂无状态检查记录']]
                }
              />
            </>
          )}
        </div>
      )}
    </section>
  )
}
