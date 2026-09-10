import { useState } from 'react'
import { InstancesPage } from '@/pages/InstancesPage'
import { SelfHostedInstanceCreateDialog } from '@/components/SelfHostedInstanceCreateDialog'
import { SelfHostedNodeCreateDialog } from '@/components/SelfHostedNodeCreateDialog'
import { SelfHostedNodeRenameDialog } from '@/components/SelfHostedNodeRenameDialog'
import {
  Alert,
  Button,
  EmptyState,
  FilterTabs,
  LoadingState,
  StatusBadge
} from '@/components/ui'
import {
  useGetSelfHostedNodeInstancesQuery,
  useGetSelfHostedNodeQuery,
  useGetSelfHostedNodesQuery,
  useGetSelfHostedReadinessEventsQuery,
  useRevokeSelfHostedNodeMutation
} from '@/services/cloudApi'

type Props = {
  detail?: boolean
  nodeID?: string
  onOpen?: (id: string) => void
  onBack?: () => void
  onOpenLogs?: (instanceID: string) => void
  onOpenTerminal?: (instanceID: string) => void
  onOpenExecutions?: (instanceID: string) => void
}

function NodeStatus({
  online,
  ready = true
}: {
  online: boolean
  ready?: boolean
}) {
  if (online && !ready) {
    return <StatusBadge tone="danger">节点需修复</StatusBadge>
  }
  return (
    <StatusBadge tone={online ? 'success' : 'neutral'}>
      {online ? '节点在线' : '节点离线'}
    </StatusBadge>
  )
}

function storage(bytes: number) {
  if (bytes <= 0) return '等待上报'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value >= 10 || unit === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[unit]}`
}

function CapacityMeter({
  label,
  used,
  total,
  unit
}: {
  label: string
  used: number
  total: number
  unit: string
}) {
  const percent =
    total > 0 ? Math.min(100, Math.round((used / total) * 100)) : 0
  return (
    <div className="min-w-0">
      <div className="flex items-baseline justify-between gap-3 text-xs">
        <span className="font-semibold text-slate-600 dark:text-slate-200">
          {label}
        </span>
        <span className="shrink-0 font-bold tabular-nums text-slate-900 dark:text-white">
          {used} / {total} {unit}
        </span>
      </div>
      <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-slate-100 dark:bg-slate-700">
        <i
          className="block h-full rounded-full bg-blue-600"
          style={{ width: `${percent}%` }}
          aria-label={`${label}已使用 ${percent}%`}
        />
      </div>
    </div>
  )
}

function DiskMeter({ available, total }: { available: number; total: number }) {
  const used = Math.max(0, total - available)
  const percent =
    total > 0 ? Math.min(100, Math.round((used / total) * 100)) : 0
  return (
    <div className="min-w-0">
      <div className="flex items-baseline justify-between gap-3 text-xs">
        <span className="font-semibold text-slate-600 dark:text-slate-200">
          实例数据盘已用
        </span>
        <span className="shrink-0 font-bold tabular-nums text-slate-900 dark:text-white">
          {total > 0
            ? `${storage(used)} / ${storage(total)}`
            : '等待 Agent 上报'}
        </span>
      </div>
      <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-slate-100 dark:bg-slate-700">
        <i
          className="block h-full rounded-full bg-blue-600"
          style={{ width: `${percent}%` }}
          aria-label={`实例数据盘已使用 ${percent}%`}
        />
      </div>
    </div>
  )
}

export function SelfHostedControlPanel({
  detail = false,
  nodeID,
  onOpen,
  onBack,
  onOpenLogs = () => undefined,
  onOpenTerminal = () => undefined,
  onOpenExecutions = () => undefined
}: Props) {
  const { data: nodes = [], isLoading: nodesLoading } =
    useGetSelfHostedNodesQuery(undefined, { skip: detail })
  const { data: node, isLoading: nodeLoading } = useGetSelfHostedNodeQuery(
    nodeID ?? '',
    { skip: !detail || !nodeID }
  )
  const { data: nodeInstances = [], isLoading: instancesLoading } =
    useGetSelfHostedNodeInstancesQuery(nodeID ?? '', {
      skip: !detail || !nodeID
    })
  const [revoke, { isLoading: revoking }] = useRevokeSelfHostedNodeMutation()
  const [nodeCreateOpen, setNodeCreateOpen] = useState(false)
  const [renameOpen, setRenameOpen] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const [activeTab, setActiveTab] = useState<'overview' | 'instances'>('overview')
  const { data: readinessEvents = [] } = useGetSelfHostedReadinessEventsQuery(
    nodeID ?? '',
    { skip: !detail || !nodeID }
  )

  if (!detail) {
    return (
      <section className="page me-page space-y-4">
        {nodesLoading ? (
          <LoadingState>正在加载自建节点…</LoadingState>
        ) : nodes.length > 0 ? (
          <div className="space-y-4">
            <div className="flex justify-end">
              <Button onClick={() => setNodeCreateOpen(true)}>
                <span aria-hidden="true">＋</span> 新建节点
              </Button>
            </div>
            <div className="grid gap-3">{nodes.map(item => (
              <button
                key={item.id}
                type="button"
                onClick={() => onOpen?.(item.id)}
                className="flex w-full flex-wrap items-center justify-between gap-4 rounded-xl border border-slate-200 bg-white p-5 text-left transition-colors hover:border-blue-300 hover:bg-blue-50/40 focus-visible:outline-3 focus-visible:outline-blue-200 dark:border-slate-700 dark:bg-slate-800 dark:hover:border-blue-700 dark:hover:bg-slate-900"
              >
                <span>
                  <b className="block text-sm text-slate-900 dark:text-white">
                    {item.name}
                  </b>
                  <span className="mt-1 block text-xs text-slate-500 dark:text-slate-300">
                    实例已分配 {item.cpuUsed} / {item.cpuQuota} 核 ·{' '}
                    {item.memoryUsedMB} / {item.memoryQuotaMB} MB
                    {item.diskTotalBytes > 0 &&
                      ` · 数据盘可用 ${storage(item.diskAvailableBytes)}`}{' '}
                    ·
                  </span>
                </span>
                <span className="flex items-center gap-3">
                  <NodeStatus
                    online={item.status === 'online'}
                    ready={item.ready}
                  />
                  <span className="text-sm font-semibold text-blue-700 dark:text-blue-300">
                    进入节点 ›
                  </span>
                </span>
              </button>
            ))}</div>
          </div>
        ) : (
          <section className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-700 dark:bg-slate-800">
            <EmptyState
              title="还没有自建节点"
              description="生成 Token 后按部署说明启动 Agent。"
            />
            <div className="mt-4 text-center"><Button onClick={() => setNodeCreateOpen(true)}>新建节点</Button></div>
          </section>
        )}
        {nodeCreateOpen && <SelfHostedNodeCreateDialog onClose={() => setNodeCreateOpen(false)} />}
      </section>
    )
  }

  if (nodeLoading) return <LoadingState>正在加载节点详情…</LoadingState>
  if (!node)
    return (
      <EmptyState
        title="自建节点不存在"
        description="节点可能已被撤销或你没有访问权限。"
        action={
          <Button tone="secondary" onClick={onBack}>
            返回自建节点
          </Button>
        }
      />
    )

  return (
    <section className="page me-page space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Button tone="ghost" className="min-h-9 px-2" onClick={onBack}>
          ← 自建节点
        </Button>
        <FilterTabs
          label="节点详情"
          value={activeTab}
          onChange={setActiveTab}
          items={[
            { value: 'overview', label: '概览' },
            { value: 'instances', label: `实例（${nodeInstances.length}）` }
          ]}
        />
      </div>
      {activeTab === 'overview' && <>
      <section className="rounded-xl border border-slate-200 bg-white px-5 py-4 dark:border-slate-700 dark:bg-slate-800">
        <div className="mb-5 flex flex-wrap items-start justify-between gap-3 border-b border-slate-100 pb-4 dark:border-slate-700">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="m-0 truncate text-lg font-bold text-slate-900 dark:text-white">{node.name}</h1>
              <NodeStatus online={node.status === 'online'} ready={node.ready} />
            </div>
            <p className="m-0 mt-1 text-xs text-slate-500 dark:text-slate-300">
              Agent {node.agentVersion || '等待连接上报'} · 自建节点
            </p>
          </div>
          <Button tone="secondary" onClick={() => setRenameOpen(true)}>
            修改名称
          </Button>
        </div>
        <div className="grid gap-5 md:grid-cols-3">
          <CapacityMeter
            label="实例 CPU 已分配"
            used={node.cpuUsed}
            total={node.cpuQuota}
            unit="核"
          />
          <CapacityMeter
            label="实例内存已分配"
            used={node.memoryUsedMB}
            total={node.memoryQuotaMB}
            unit="MB"
          />
          <DiskMeter
            available={node.diskAvailableBytes}
            total={node.diskTotalBytes}
          />
        </div>
      </section>
      {!node.ready && (
        <Alert tone="error">
          <b>节点已连接，但暂不可部署。</b>{' '}
          {node.readinessIssues?.map(issue => issue.message).join('；') ||
            node.lastAgentError ||
            (node.agentVersion
              ? 'Agent 尚未上报运行环境检查结果。请等待数秒；若仍未恢复，请检查 xcloud-control 服务日志。'
              : '当前连接的 Agent 未上报部署就绪状态，通常是旧版 xcloud-control。请升级 Agent 后重新连接。')}
        </Alert>
      )}
      <section className="rounded-xl border border-slate-200 bg-white px-5 py-4 dark:border-slate-700 dark:bg-slate-800">
        <h2 className="text-sm font-bold text-slate-900 dark:text-white">运行环境检查记录</h2>
        <div className="mt-3 space-y-2 text-xs">
          {readinessEvents.length === 0 ? <p className="text-slate-500">尚未收到 Agent 检查记录。</p> : readinessEvents.map(event => (
            <div key={`${event.createdAt}-${event.code}`} className="flex flex-wrap justify-between gap-2 rounded-lg bg-slate-50 px-3 py-2 dark:bg-slate-900">
              <span className={event.ready ? 'text-emerald-700 dark:text-emerald-300' : 'text-rose-700 dark:text-rose-300'}>{event.ready ? '环境就绪' : event.message || '环境检查失败'}</span>
              <time className="text-slate-500">{new Date(event.createdAt).toLocaleString()}</time>
            </div>
          ))}
        </div>
      </section>
      <details className="border-t border-slate-200 pt-4 dark:border-slate-700">
        <summary className="cursor-pointer text-xs font-bold text-rose-700 dark:text-rose-300">
          危险操作：撤销节点
        </summary>
        <div className="mt-3 flex flex-wrap items-center justify-between gap-3 rounded-lg bg-rose-50 px-4 py-3 text-xs text-rose-800 dark:bg-rose-950/30 dark:text-rose-100">
          <span>撤销会立即断开 Agent，已部署实例将无法继续管理。</span>
          <Button
            tone="danger"
            loading={revoking}
            onClick={() => {
              if (window.confirm('确认撤销该自建节点吗？')) void revoke(node.id)
            }}
          >
            撤销节点
          </Button>
        </div>
      </details>
      </>}
      {activeTab === 'instances' && (
        <InstancesPage
          instances={nodeInstances}
          orders={[]}
          loading={instancesLoading}
          onCreate={() => setCreateOpen(true)}
          onOpenLogs={onOpenLogs}
          onOpenTerminal={onOpenTerminal}
          onOpenExecutions={onOpenExecutions}
          workspace={{
            title: '实例',
            description: '运行在当前自建节点。',
            createLabel: '创建实例',
            selfHosted: true,
            compact: true
          }}
        />
      )}
      {createOpen && (
        <SelfHostedInstanceCreateDialog
          node={node}
          onClose={() => setCreateOpen(false)}
        />
      )}
      {renameOpen && (
        <SelfHostedNodeRenameDialog
          id={node.id}
          name={node.name}
          onClose={() => setRenameOpen(false)}
        />
      )}
    </section>
  )
}
