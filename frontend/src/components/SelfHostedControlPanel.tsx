import { useState } from 'react'
import { Alert, Button, EmptyState, LoadingState, StatusBadge } from '@/components/ui'
import {
  useCreateControlEnrollmentTokenMutation,
  useCreateSelfHostedNodeInstanceMutation,
  useGetCatalogQuery,
  useGetSelfHostedNodesQuery,
  useGetSelfHostedNodeInstancesQuery,
  useGetSelfHostedNodeQuery,
  useRevokeSelfHostedControlMutation
} from '@/services/cloudApi'

type Props = {
  detail?: boolean
  nodeID?: string
  onOpen?: (id: string) => void
  onBack?: () => void
}

function NodeStatus({ online }: { online: boolean }) {
  return <StatusBadge tone={online ? 'success' : 'neutral'}>{online ? '节点在线' : '节点离线'}</StatusBadge>
}

export function SelfHostedControlPanel({ detail = false, nodeID, onOpen, onBack }: Props) {
  const { data: nodes = [], isLoading: nodesLoading } = useGetSelfHostedNodesQuery()
  const { data: node, isLoading: nodeLoading } = useGetSelfHostedNodeQuery(nodeID ?? '', { skip: !detail || !nodeID })
  const { data: nodeInstances = [] } = useGetSelfHostedNodeInstancesQuery(nodeID ?? '', { skip: !detail || !nodeID })
  const { data: catalog } = useGetCatalogQuery(undefined, { skip: !detail })
  const [createToken, { isLoading: creatingToken }] = useCreateControlEnrollmentTokenMutation()
  const [createInstance, { isLoading: creatingInstance }] = useCreateSelfHostedNodeInstanceMutation()
  const [revoke, { isLoading: revoking }] = useRevokeSelfHostedControlMutation()
  const [enrollmentToken, setEnrollmentToken] = useState('')
  const [message, setMessage] = useState('')
  const [instanceName, setInstanceName] = useState('')
  const [imageID, setImageID] = useState('')
  const [imageVersion, setImageVersion] = useState('latest')
  const [cpu, setCPU] = useState('1')
  const [memoryMB, setMemoryMB] = useState('1024')

  const selectedImage = catalog?.images.find(image => image.id === imageID)

  async function generateToken() {
    setMessage('')
    try {
      const result = await createToken().unwrap()
      setEnrollmentToken(result.token)
    } catch {
      setMessage('接入 Token 生成失败，请稍后重试。')
    }
  }

  async function submitInstance() {
    if (!node || !instanceName || !imageID) return
    setMessage('')
    try {
      await createInstance({
        nodeID: node.id,
        name: instanceName,
        imageId: imageID,
        imageVersion,
        cpu: Number(cpu),
        memoryMB: Number(memoryMB)
      }).unwrap()
      setInstanceName('')
      setMessage('自建实例已进入部署队列。')
    } catch {
      setMessage('创建失败，请检查节点状态、资源配额与镜像版本。')
    }
  }

  if (!detail) {
    return <section className="page me-page space-y-4">
      <div>
        <h1 className="m-0 text-xl font-bold text-slate-900 dark:text-white">自建节点</h1>
        <p className="mb-0 mt-1 text-sm text-slate-500 dark:text-slate-300">接入自己的服务器，在节点资源配额内免费创建平台审核镜像实例。</p>
      </div>
      {nodesLoading ? <LoadingState>正在加载自建节点…</LoadingState> : nodes.length > 0 ? (
        <div className="grid gap-3">
          {nodes.map(item => <button key={item.id} type="button" onClick={() => onOpen?.(item.id)} className="flex w-full flex-wrap items-center justify-between gap-4 rounded-xl border border-slate-200 bg-white p-5 text-left transition-colors hover:border-blue-300 hover:bg-blue-50/40 dark:border-slate-700 dark:bg-slate-800 dark:hover:border-blue-700 dark:hover:bg-slate-900">
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

  return <section className="page me-page space-y-4">
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div><Button tone="secondary" onClick={onBack}>返回自建节点</Button><h1 className="mb-0 mt-3 text-xl font-bold text-slate-900 dark:text-white">{node.name}</h1><p className="mb-0 mt-1 text-sm text-slate-500 dark:text-slate-300">自建 Agent 节点 · 所有实例共享最高 10 Mbps 出口带宽</p></div>
      <NodeStatus online={node.status === 'online'} />
    </div>
    <section className="grid grid-cols-1 gap-3 rounded-xl border border-slate-200 bg-white p-5 text-sm text-slate-700 sm:grid-cols-2 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-200"><span>CPU：{node.cpuUsed} / {node.cpuQuota} 核</span><span>内存：{node.memoryUsedMB} / {node.memoryQuotaMB} MB</span><span>检测资源：{node.cpuDetected} 核 / {node.memoryDetectedMB} MB</span><span>Agent：{node.agentVersion || '等待上报'}</span></section>
    <section className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-700 dark:bg-slate-800"><h2 className="m-0 text-base font-bold text-slate-900 dark:text-white">创建自建实例</h2><p className="mb-4 mt-1 text-xs text-slate-500 dark:text-slate-300">不创建订单、不扣钱包余额；仅可选择平台审核镜像。</p><div className="grid grid-cols-1 gap-3 sm:grid-cols-2"><label className="text-xs font-semibold text-slate-600 dark:text-slate-200">实例名称<input value={instanceName} onChange={event => setInstanceName(event.target.value)} className="mt-1 w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm font-normal dark:border-slate-600 dark:bg-slate-900" /></label><label className="text-xs font-semibold text-slate-600 dark:text-slate-200">审核镜像<select value={imageID} onChange={event => { const next = event.target.value; setImageID(next); setImageVersion(catalog?.images.find(image => image.id === next)?.versions[0]?.tag ?? 'latest') }} className="mt-1 w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm font-normal dark:border-slate-600 dark:bg-slate-900"><option value="">请选择镜像</option>{catalog?.images.map(image => <option key={image.id} value={image.id}>{image.name}</option>)}</select></label><label className="text-xs font-semibold text-slate-600 dark:text-slate-200">版本<select value={imageVersion} onChange={event => setImageVersion(event.target.value)} disabled={!selectedImage} className="mt-1 w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm font-normal disabled:cursor-not-allowed disabled:opacity-60 dark:border-slate-600 dark:bg-slate-900">{selectedImage?.versions.map(version => <option key={version.tag} value={version.tag}>{version.tag}</option>)}</select></label><label className="text-xs font-semibold text-slate-600 dark:text-slate-200">CPU 核数<input type="number" min="0.1" step="0.1" value={cpu} onChange={event => setCPU(event.target.value)} className="mt-1 w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm font-normal dark:border-slate-600 dark:bg-slate-900" /></label><label className="text-xs font-semibold text-slate-600 dark:text-slate-200">内存 MB<input type="number" min="256" step="256" value={memoryMB} onChange={event => setMemoryMB(event.target.value)} className="mt-1 w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm font-normal dark:border-slate-600 dark:bg-slate-900" /></label></div><Button className="mt-4" loading={creatingInstance} disabled={node.status !== 'online' || !instanceName || !imageID} onClick={() => void submitInstance()}>{node.status === 'online' ? '创建实例' : '节点离线，暂不可创建'}</Button>{message && <p className="mb-0 mt-2 text-xs text-slate-600 dark:text-slate-300">{message}</p>}</section>
    <section className="rounded-xl border border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-800"><div className="border-b border-slate-100 px-5 py-3 dark:border-slate-700"><h2 className="m-0 text-base font-bold text-slate-900 dark:text-white">该节点实例</h2></div>{nodeInstances.length === 0 ? <p className="px-5 py-4 text-sm text-slate-500">暂无自建实例。</p> : <div className="divide-y divide-slate-100 dark:divide-slate-700">{nodeInstances.map(instance => <div key={instance.id} className="flex flex-wrap items-center justify-between gap-3 px-5 py-3 text-sm"><span><b className="block text-slate-900 dark:text-white">{instance.name}</b><span className="text-xs text-slate-500">{instance.spec} · {instance.status}</span></span>{instance.ip && <a href={instance.ip} target="_blank" rel="noreferrer" className="text-xs font-semibold text-blue-700 underline dark:text-blue-300">访问</a>}</div>)}</div>}</section>
    <section className="rounded-xl border border-rose-200 bg-rose-50 p-5 dark:border-rose-900 dark:bg-rose-950/30"><h2 className="m-0 text-base font-bold text-rose-900 dark:text-rose-100">撤销节点</h2><p className="mb-3 mt-1 text-sm text-rose-800 dark:text-rose-200">撤销会立即断开自建 Agent，已部署实例将无法继续管理。</p><Button tone="danger" loading={revoking} onClick={() => { if (window.confirm('确认撤销该自建节点吗？')) void revoke() }}>撤销节点</Button></section>
  </section>
}
