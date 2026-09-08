import { useState } from 'react'
import { InstancesPage } from '@/pages/InstancesPage'
import { SelfHostedInstanceCreateDialog } from '@/components/SelfHostedInstanceCreateDialog'
import { Alert, Button, EmptyState, LoadingState, PageHeader, StatusBadge } from '@/components/ui'
import {
  useCreateControlEnrollmentTokenMutation,
  useGetSelfHostedNodeInstancesQuery,
  useGetSelfHostedNodeQuery,
  useGetSelfHostedNodesQuery,
  useRevokeSelfHostedControlMutation
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

function NodeStatus({ online }: { online: boolean }) {
  return <StatusBadge tone={online ? 'success' : 'neutral'}>{online ? '节点在线' : '节点离线'}</StatusBadge>
}

export function SelfHostedControlPanel({ detail = false, nodeID, onOpen, onBack, onOpenLogs = () => undefined, onOpenTerminal = () => undefined, onOpenExecutions = () => undefined }: Props) {
  const { data: nodes = [], isLoading: nodesLoading } = useGetSelfHostedNodesQuery(undefined, { skip: detail })
  const { data: node, isLoading: nodeLoading } = useGetSelfHostedNodeQuery(nodeID ?? '', { skip: !detail || !nodeID })
  const { data: nodeInstances = [], isLoading: instancesLoading } = useGetSelfHostedNodeInstancesQuery(nodeID ?? '', { skip: !detail || !nodeID })
  const [createToken, { isLoading: creatingToken }] = useCreateControlEnrollmentTokenMutation()
  const [revoke, { isLoading: revoking }] = useRevokeSelfHostedControlMutation()
  const [enrollmentToken, setEnrollmentToken] = useState('')
  const [message, setMessage] = useState('')
  const [createOpen, setCreateOpen] = useState(false)

  async function generateToken() {
    setMessage('')
    try {
      const result = await createToken().unwrap()
      setEnrollmentToken(result.token)
    } catch {
      setMessage('接入 Token 生成失败，请稍后重试。')
    }
  }

  if (!detail) {
    return <section className="page me-page space-y-4">
      <PageHeader eyebrow="自建 Agent" title="自建节点" description="接入自己的服务器，在节点资源配额内免费创建平台审核镜像实例。当前账户最多启用 1 个节点。" />
      {nodesLoading ? <LoadingState>正在加载自建节点…</LoadingState> : nodes.length > 0 ? (
        <div className="grid gap-3">
          {nodes.map(item => <button key={item.id} type="button" onClick={() => onOpen?.(item.id)} className="flex w-full flex-wrap items-center justify-between gap-4 rounded-xl border border-slate-200 bg-white p-5 text-left transition-colors hover:border-blue-300 hover:bg-blue-50/40 focus-visible:outline-3 focus-visible:outline-blue-200 dark:border-slate-700 dark:bg-slate-800 dark:hover:border-blue-700 dark:hover:bg-slate-900">
            <span><b className="block text-sm text-slate-900 dark:text-white">{item.name}</b><span className="mt-1 block text-xs text-slate-500 dark:text-slate-300">已用 {item.cpuUsed} / {item.cpuQuota} 核 · {item.memoryUsedMB} / {item.memoryQuotaMB} MB · 所有实例共享最高 10 Mbps</span></span>
            <span className="flex items-center gap-3"><NodeStatus online={item.status === 'online'} /><span className="text-sm font-semibold text-blue-700 dark:text-blue-300">进入节点 ›</span></span>
          </button>)}
        </div>
      ) : <section className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-700 dark:bg-slate-800">
        <EmptyState title="还没有自建节点" description="当前账户可接入 1 台运行 xcloud-control 的服务器。生成 Token 后按部署说明启动 Agent。" />
        <div className="mx-auto mt-4 max-w-xl space-y-3">
          <Alert tone="info">Token 仅显示一次，10 分钟内有效。写入 <code>/etc/xcloud-control/config.json</code> 后启动服务。</Alert>
          <Button loading={creatingToken} onClick={() => void generateToken()}>生成接入 Token</Button>
          {enrollmentToken && <label className="block text-xs font-semibold text-slate-600 dark:text-slate-200">请立即保存 Token<input readOnly value={enrollmentToken} className="mt-1.5 w-full rounded-lg border border-amber-300 bg-amber-50 px-3 py-2 font-mono text-xs text-slate-900 dark:border-amber-800 dark:bg-amber-950 dark:text-white" /></label>}
          {message && <Alert tone="error">{message}</Alert>}
        </div>
      </section>}
    </section>
  }

  if (nodeLoading) return <LoadingState>正在加载节点详情…</LoadingState>
  if (!node) return <EmptyState title="自建节点不存在" description="节点可能已被撤销或你没有访问权限。" action={<Button tone="secondary" onClick={onBack}>返回自建节点</Button>} />

  return <section className="page me-page space-y-5">
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div><Button tone="secondary" onClick={onBack}>返回自建节点</Button><p className="mb-1 mt-4 text-[10px] font-extrabold tracking-widest text-blue-600">自建 Agent 节点</p><h1 className="m-0 text-2xl font-bold tracking-tight text-slate-900 dark:text-white">{node.name}</h1></div>
      <NodeStatus online={node.status === 'online'} />
    </div>
    <section className="grid grid-cols-1 gap-px overflow-hidden rounded-xl border border-slate-200 bg-slate-200 sm:grid-cols-2 lg:grid-cols-4 dark:border-slate-700 dark:bg-slate-700">
      {[
        ['CPU 配额', `${node.cpuUsed} / ${node.cpuQuota} 核`],
        ['内存配额', `${node.memoryUsedMB} / ${node.memoryQuotaMB} MB`],
        ['检测资源', `${node.cpuDetected} 核 / ${node.memoryDetectedMB} MB`],
        ['Agent 版本', node.agentVersion || '等待上报']
      ].map(([label, value]) => <div key={label} className="bg-white px-4 py-3 dark:bg-slate-800"><span className="block text-[10px] font-bold text-slate-400">{label}</span><b className="mt-1 block text-xs text-slate-700 dark:text-slate-100">{value}</b></div>)}
    </section>
    <InstancesPage instances={nodeInstances} orders={[]} loading={instancesLoading} onCreate={() => setCreateOpen(true)} onOpenLogs={onOpenLogs} onOpenTerminal={onOpenTerminal} onOpenExecutions={onOpenExecutions} workspace={{ eyebrow: '节点实例', title: '我的实例', description: '与平台托管实例使用相同的管理能力；不创建订单、不扣 XCoin，所有实例共享该节点最高 10 Mbps 出口带宽。', createLabel: '创建', selfHosted: true }} />
    <section className="rounded-xl border border-rose-200 bg-rose-50 p-5 dark:border-rose-900 dark:bg-rose-950/30"><h2 className="m-0 text-base font-bold text-rose-900 dark:text-rose-100">撤销节点</h2><p className="mb-3 mt-1 text-sm text-rose-800 dark:text-rose-200">撤销会立即断开自建 Agent，已部署实例将无法继续管理。</p><Button tone="danger" loading={revoking} onClick={() => { if (window.confirm('确认撤销该自建节点吗？')) void revoke() }}>撤销节点</Button></section>
    {createOpen && <SelfHostedInstanceCreateDialog node={node} onClose={() => setCreateOpen(false)} />}
  </section>
}
