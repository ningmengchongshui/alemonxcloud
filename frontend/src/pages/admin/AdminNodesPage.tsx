import { NodeConfigButton, NodeEditor } from '@/components/NodeEditor'
import { useGetAdminNodesQuery } from '@/services/cloudApi'
import {
  Button,
  EmptyState,
  LoadingState,
  PageHeader,
  StatusBadge
} from '@/components/ui'
import type { Node } from '@/types/cloud'

const cpuValue = (value: number) =>
  Number.isInteger(value) ? String(value) : value.toFixed(1)
const memoryValue = (value: number) => {
  const gb = value / 1024
  return Number.isInteger(gb) ? String(gb) : gb.toFixed(1)
}
const percent = (used: number, total: number) =>
  total > 0 ? Math.min(100, Math.round((used / total) * 100)) : 0
const diskGBValue = (bytes: number) => {
  const value = Math.max(0, bytes) / 1024 ** 3
  return Number.isInteger(value) ? String(value) : value.toFixed(1)
}

function nodeState(node: Node) {
  if (!node.enabled) return { label: '未启用', tone: 'neutral' as const }
  if (!node.lastHeartbeatAt)
    return { label: '等待心跳', tone: 'pending' as const }
  return { label: '在线', tone: 'success' as const }
}

function Capacity({
  label,
  used,
  total,
  suffix,
  format = value => String(value)
}: {
  label: string
  used: number
  total: number
  suffix: string
  format?: (value: number) => string
}) {
  const ratio = percent(used, total)
  return (
    <div className="min-w-0">
      <div className="flex items-baseline justify-between gap-2 text-[10px] text-slate-500 dark:text-slate-300">
        <span>{label}</span>
        <b className="shrink-0 text-slate-700 dark:text-slate-100">
          {format(used)}/{format(total)} {suffix}
        </b>
      </div>
      <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-slate-100 dark:bg-slate-700">
        <span
          className={`block h-full rounded-full ${ratio >= 90 ? 'bg-red-500' : ratio >= 75 ? 'bg-amber-500' : 'bg-blue-600'}`}
          style={{ width: `${ratio}%` }}
        />
      </div>
    </div>
  )
}

export function AdminNodesPage() {
  const nodes = useGetAdminNodesQuery()
  const values = nodes.data ?? []
  const online = values.filter(node => node.enabled && node.lastHeartbeatAt)
  const cpu = values.reduce((total, node) => total + node.cpuTotal, 0)
  const memory = values.reduce((total, node) => total + node.memoryTotalMB, 0)
  const reservedCPU = values.reduce(
    (total, node) => total + node.cpuReserved,
    0
  )
  const reservedMemory = values.reduce(
    (total, node) => total + node.memoryReservedMB,
    0
  )
  const nodesWithDiskCapacity = values.filter(node => (node.diskTotalBytes ?? 0) > 0)
  const diskTotal = nodesWithDiskCapacity.reduce(
    (total, node) => total + (node.diskTotalBytes ?? 0),
    0
  )
  const diskUsed = nodesWithDiskCapacity.reduce(
    (total, node) => total + Math.max(0, (node.diskTotalBytes ?? 0) - (node.diskAvailableBytes ?? 0)),
    0
  )

  return (
    <section className="page super-page">
      <PageHeader
        eyebrow="资源运营"
        title="节点管理"
        description="监控可调度容量、Agent 连通性与运行资源；新实例只会调度到健康节点。"
        actions={
          <Button
            tone="secondary"
            loading={nodes.isFetching}
            onClick={() => void nodes.refetch()}
          >
            ↻ 刷新
          </Button>
        }
      />
      <div className="mb-5 flex flex-wrap items-center justify-between gap-x-5 gap-y-3 border-y border-slate-200 py-3 text-xs dark:border-slate-700">
        <p className="m-0 text-slate-500 dark:text-slate-300">
          <b className="text-slate-900 dark:text-white">
            {online.length}/{values.length}
          </b>{' '}
          个节点在线
          <span className="mx-2 text-slate-300 dark:text-slate-600">·</span>
          已分配{' '}
          <b className="text-slate-900 dark:text-white">
            {cpuValue(reservedCPU)}/{cpuValue(cpu)} 核
          </b>
          <span className="mx-2 text-slate-300 dark:text-slate-600">·</span>
          <b className="text-slate-900 dark:text-white">
            {memoryValue(reservedMemory)}/{memoryValue(memory)} GB
          </b>
          {diskTotal > 0 && <>
            <span className="mx-2 text-slate-300 dark:text-slate-600">·</span>
            磁盘已用{' '}
            <b className="text-slate-900 dark:text-white">
              {diskGBValue(diskUsed)}/{diskGBValue(diskTotal)} GB
            </b>
          </>}
        </p>
        <NodeEditor />
      </div>
      {nodes.isLoading ? (
        <LoadingState>正在同步节点健康与资源容量…</LoadingState>
      ) : nodes.isError ? (
        <EmptyState
          title="节点数据加载失败"
          description="暂时无法获取节点状态，请刷新后重试。"
          action={
            <Button onClick={() => void nodes.refetch()}>重新加载</Button>
          }
        />
      ) : values.length === 0 ? (
        <EmptyState
          title="尚未注册节点"
          description="注册并完成 Agent 心跳后，节点才可承载新实例。"
          action={<NodeEditor />}
        />
      ) : (
        <div className="overflow-hidden rounded-xl border border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-800">
          <div className="hidden grid-cols-[minmax(15rem,1.35fr)_minmax(18rem,1.45fr)_minmax(13rem,1fr)_auto] gap-5 border-b border-slate-100 bg-slate-50 px-5 py-2.5 text-[10px] font-extrabold tracking-wide text-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 lg:grid">
            <span>节点与连接</span>
            <span>可调度容量</span>
            <span>运行信息</span>
            <span className="text-right">操作</span>
          </div>
          <div className="divide-y divide-slate-100 dark:divide-slate-700">
            {values.map(node => {
              const state = nodeState(node)
              const agentTone =
                node.agentCompatibility === 'compatible'
                  ? 'success'
                  : node.agentCompatibility === 'outdated'
                    ? 'pending'
                    : 'neutral'
              const hasRisk =
                Boolean(node.lastAgentError) ||
                (node.offlineInstanceCount ?? 0) > 0 ||
                (node.pendingCleanupTasks ?? 0) > 0
              return (
                <article
                  key={node.id}
                  className="grid gap-4 px-5 py-4 lg:grid-cols-[minmax(15rem,1.35fr)_minmax(18rem,1.45fr)_minmax(13rem,1fr)_auto] lg:items-center lg:gap-5"
                >
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <h2 className="m-0 truncate text-sm font-bold text-slate-900 dark:text-white">
                        {node.name}
                      </h2>
                      <StatusBadge tone={state.tone}>{state.label}</StatusBadge>
                    </div>
                    <p
                      className="mb-0 mt-1 truncate text-[11px] text-slate-500 dark:text-slate-300"
                      title={node.agentURL}
                    >
                      {node.agentURL}
                    </p>
                    <p className="mb-0 mt-1.5 text-[10px] text-slate-400">
                      最近心跳：
                      {node.lastHeartbeatAt
                        ? new Date(node.lastHeartbeatAt).toLocaleString('zh-CN')
                        : '尚未收到'}
                    </p>
                  </div>
                  <div className="grid gap-3 rounded-lg bg-slate-50 p-3 dark:bg-slate-900 sm:grid-cols-2 lg:grid-cols-3 lg:bg-transparent lg:p-0">
                    <Capacity
                      label="CPU 已分配"
                      used={node.cpuReserved}
                      total={node.cpuTotal}
                      suffix="核"
                    />
                    <Capacity
                      label="内存已分配"
                      used={node.memoryReservedMB / 1024}
                      total={node.memoryTotalMB / 1024}
                      suffix="GB"
                    />
                    {(node.diskTotalBytes ?? 0) > 0 ? (
                      <Capacity
                        label="磁盘已用"
                        used={Math.max(0, (node.diskTotalBytes ?? 0) - (node.diskAvailableBytes ?? 0))}
                        total={node.diskTotalBytes ?? 0}
                        suffix="GB"
                        format={diskGBValue}
                      />
                    ) : (
                      <div className="min-w-0 text-[10px] text-slate-500 dark:text-slate-300">
                        <span>磁盘容量</span>
                        <b className="mt-1.5 block text-slate-700 dark:text-slate-100">同步中</b>
                      </div>
                    )}
                  </div>
                  <div className="flex flex-wrap gap-x-3 gap-y-1.5 text-[11px] text-slate-500 dark:text-slate-300">
                    <span>{node.managedContainerCount ?? 0} 个容器</span>
                    <span>
                      {node.dockerVersion
                        ? `Docker ${node.dockerVersion}`
                        : 'Docker 同步中'}
                    </span>
                    <StatusBadge tone={agentTone}>
                      {node.agentCompatibility === 'compatible'
                        ? `Agent ${node.agentVersion ?? '已连接'}`
                        : node.agentCompatibility === 'outdated'
                          ? 'Agent 需要升级'
                          : 'Agent 协议未知'}
                    </StatusBadge>
                  </div>
                  <div className="flex justify-end lg:justify-self-end">
                    <NodeConfigButton node={node} />
                  </div>
                  {hasRisk && (
                    <div className="lg:col-span-4 flex flex-wrap gap-x-3 gap-y-1 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-[11px] text-amber-800 dark:border-amber-900 dark:bg-amber-950 dark:text-amber-100">
                      <b>需要关注</b>
                      {(node.offlineInstanceCount ?? 0) > 0 && (
                        <span>
                          离线/保留实例 {node.offlineInstanceCount} 个
                        </span>
                      )}
                      {(node.pendingCleanupTasks ?? 0) > 0 && (
                        <span>待清理任务 {node.pendingCleanupTasks} 个</span>
                      )}
                      {node.lastAgentError && (
                        <span
                          className="min-w-0 truncate"
                          title={node.lastAgentError}
                        >
                          Agent：{node.lastAgentError}
                        </span>
                      )}
                    </div>
                  )}
                </article>
              )
            })}
          </div>
        </div>
      )}
    </section>
  )
}
