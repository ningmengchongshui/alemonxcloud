import { useState } from 'react'
import { InstancesPage } from '@/pages/InstancesPage'
import { SelfHostedInstanceCreateDialog } from '@/components/SelfHostedInstanceCreateDialog'
import {
  Alert,
  Button,
  EmptyState,
  LoadingState,
  StatusBadge
} from '@/components/ui'
import {
  useCreateControlEnrollmentTokenMutation,
  useGetSelfHostedNodeInstancesQuery,
  useGetSelfHostedNodeQuery,
  useGetSelfHostedNodesQuery,
  useGetSelfHostedReadinessEventsQuery,
  useRevokeSelfHostedNodeMutation,
  useUpdateSelfHostedNodeMutation
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
  const [createToken, { isLoading: creatingToken }] =
    useCreateControlEnrollmentTokenMutation()
  const [revoke, { isLoading: revoking }] = useRevokeSelfHostedNodeMutation()
  const [rename, { isLoading: renaming }] = useUpdateSelfHostedNodeMutation()
  const [enrollmentToken, setEnrollmentToken] = useState('')
  const [nodeName, setNodeName] = useState('')
  const [editingName, setEditingName] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const { data: readinessEvents = [] } = useGetSelfHostedReadinessEventsQuery(
    nodeID ?? '',
    { skip: !detail || !nodeID }
  )

  async function generateToken() {
    try {
      const result = await createToken({ name: nodeName.trim() }).unwrap()
      setEnrollmentToken(result.token)
    } catch {
      /* cloudApi presents request errors through the global Toast. */
    }
  }

  if (!detail) {
    return (
      <section className="page me-page space-y-4">
        {nodesLoading ? (
          <LoadingState>正在加载自建节点…</LoadingState>
        ) : nodes.length > 0 ? (
          <div className="space-y-4">
            <div className="rounded-xl border border-slate-200 bg-white p-4 dark:border-slate-700 dark:bg-slate-800">
              <label className="block text-xs font-semibold text-slate-600 dark:text-slate-200">
                新节点名称
                <input value={nodeName} onChange={event => setNodeName(event.target.value)} placeholder="例如：香港云主机" className="mt-1.5 w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 dark:border-slate-700 dark:bg-slate-900 dark:text-white" />
              </label>
              <Button className="mt-3" loading={creatingToken} disabled={!nodeName.trim()} onClick={() => void generateToken()}>生成新节点接入 Token</Button>
              {enrollmentToken && <input readOnly value={enrollmentToken} className="mt-3 w-full rounded-lg border border-amber-300 bg-amber-50 px-3 py-2 font-mono text-xs text-slate-900 dark:border-amber-800 dark:bg-amber-950 dark:text-white" />}
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
            <div className="mx-auto mt-4 max-w-xl space-y-3">
              <label className="block text-xs font-semibold text-slate-600 dark:text-slate-200">
                节点名称
                <input
                  value={nodeName}
                  onChange={event => setNodeName(event.target.value)}
                  placeholder="例如：上海家用服务器"
                  className="mt-1.5 w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 dark:border-slate-700 dark:bg-slate-900 dark:text-white"
                />
              </label>
              <Button
                loading={creatingToken}
                disabled={!nodeName.trim()}
                onClick={() => void generateToken()}
              >
                生成接入 Token
              </Button>
              {enrollmentToken && (
                <label className="block text-xs font-semibold text-slate-600 dark:text-slate-200">
                  请立即保存 Token
                  <input
                    readOnly
                    value={enrollmentToken}
                    className="mt-1.5 w-full rounded-lg border border-amber-300 bg-amber-50 px-3 py-2 font-mono text-xs text-slate-900 dark:border-amber-800 dark:bg-amber-950 dark:text-white"
                  />
                </label>
              )}
            </div>
          </section>
        )}
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
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <input
            value={editingName || node.name}
            onChange={event => setEditingName(event.target.value)}
            className="min-w-0 rounded-md border border-slate-300 bg-white px-2 py-1 text-sm font-bold text-slate-900 dark:border-slate-700 dark:bg-slate-900 dark:text-white"
          />
          <Button
            tone="secondary"
            loading={renaming}
            disabled={!editingName.trim() || editingName.trim() === node.name}
            onClick={() => void rename({ id: node.id, name: editingName.trim() })}
          >保存名称</Button>
        </div>
        <NodeStatus online={node.status === 'online'} ready={node.ready} />
      </header>
      <section className="rounded-xl border border-slate-200 bg-white px-5 py-4 dark:border-slate-700 dark:bg-slate-800">
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
          description: '',
          createLabel: '创建实例',
          selfHosted: true,
          compact: true
        }}
      />
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
      {createOpen && (
        <SelfHostedInstanceCreateDialog
          node={node}
          onClose={() => setCreateOpen(false)}
        />
      )}
    </section>
  )
}
