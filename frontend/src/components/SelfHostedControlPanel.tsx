import { useEffect, useState } from 'react'
import { Alert, Button } from '@/components/ui'
import {
  useGetSelfHostedControlQuery,
  useGetSelfHostedNodesQuery,
  useGetSelfHostedNodeQuery,
  useGetSelfHostedNodeInstancesQuery,
  useGetCatalogQuery,
  useCreateSelfHostedNodeInstanceMutation,
  useCreateControlEnrollmentTokenMutation,
  useRevokeSelfHostedControlMutation,
  useSaveSelfHostedRouteMutation,
  useSelfHostedRouteActionMutation
} from '@/services/cloudApi'

export function SelfHostedControlPanel({ detail = false, nodeID, onOpen, onBack }: { detail?: boolean; nodeID?: string; onOpen?: (id: string) => void; onBack?: () => void }) {
  const { data, isLoading } = useGetSelfHostedControlQuery()
  const { data: nodes = [] } = useGetSelfHostedNodesQuery()
  const { data: node } = useGetSelfHostedNodeQuery(nodeID ?? '', { skip: !nodeID })
  const { data: nodeInstances = [] } = useGetSelfHostedNodeInstancesQuery(nodeID ?? '', { skip: !nodeID })
  const { data: catalog } = useGetCatalogQuery(undefined, { skip: !detail })
  const [createInstance, { isLoading: creatingInstance }] = useCreateSelfHostedNodeInstanceMutation()
  const [targetURL, setTargetURL] = useState('')
  const [message, setMessage] = useState('')
  const [save, { isLoading: saving }] = useSaveSelfHostedRouteMutation()
  const [action, { isLoading: acting }] = useSelfHostedRouteActionMutation()
  const [revoke, { isLoading: revoking }] = useRevokeSelfHostedControlMutation()
  const [createToken, { isLoading: creatingToken }] = useCreateControlEnrollmentTokenMutation()
  const [enrollmentToken, setEnrollmentToken] = useState('')
  const [instanceName, setInstanceName] = useState('')
  const [imageID, setImageID] = useState('')
  const [imageVersion, setImageVersion] = useState('latest')
  const [cpu, setCPU] = useState('1')
  const [memoryMB, setMemoryMB] = useState('1024')
  useEffect(() => setTargetURL(data?.route?.targetURL ?? ''), [data?.route?.targetURL])
  const busy = saving || acting || revoking
  const route = data?.route

  if (!detail && nodes[0]) return <section className="rounded-xl border border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-800">
    <button type="button" onClick={() => onOpen?.(nodes[0].id)} className="flex w-full items-center justify-between gap-4 px-5 py-4 text-left transition-colors hover:bg-slate-50 dark:hover:bg-slate-900">
      <span><b className="block text-sm text-slate-900 dark:text-white">{nodes[0].name}</b><span className="mt-1 block text-xs text-slate-500 dark:text-slate-300">自建节点 · {nodes[0].status === 'online' ? '在线' : '离线'} · 已用 {nodes[0].cpuUsed}/{nodes[0].cpuQuota} 核 · 最高共享带宽 10 Mbps</span></span><span className="text-sm text-sky-700 dark:text-sky-300">管理 ›</span>
    </button>
  </section>
  if (detail && node) return <section className="space-y-4">
    <section className="rounded-xl border border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-800">
    <div className="flex flex-wrap items-start justify-between gap-3 border-b border-slate-100 px-5 py-4 dark:border-slate-700"><div><h2 className="m-0 text-base font-bold text-slate-900 dark:text-white">{node.name}</h2><p className="mb-0 mt-1 text-xs text-slate-500 dark:text-slate-300">自建 Agent 节点 · 所有实例共享最高 10 Mbps 出口带宽</p></div><span className={node.status === 'online' ? 'rounded-full bg-emerald-100 px-2 py-1 text-xs font-semibold text-emerald-700 dark:bg-emerald-950 dark:text-emerald-200' : 'rounded-full bg-slate-100 px-2 py-1 text-xs font-semibold text-slate-600 dark:bg-slate-900 dark:text-slate-300'}>{node.status === 'online' ? '节点在线' : '节点离线'}</span></div>
    <div className="space-y-3 px-5 py-4 text-sm text-slate-700 dark:text-slate-200">{onBack && <Button tone="secondary" onClick={onBack}>返回自建节点列表</Button>}<div className="grid grid-cols-1 gap-2 sm:grid-cols-2"><span>CPU：{node.cpuUsed} / {node.cpuQuota} 核</span><span>内存：{node.memoryUsedMB} / {node.memoryQuotaMB} MB</span><span>检测资源：{node.cpuDetected} 核 / {node.memoryDetectedMB} MB</span><span>Agent：{node.agentVersion || '等待上报'}</span></div><Alert tone="info">在这里创建的实例不产生订单或钱包扣费；只能使用平台已审核镜像，且受此节点资源配额限制。</Alert></div>
    </section>
    <section className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-700 dark:bg-slate-800"><h3 className="m-0 text-sm font-bold text-slate-900 dark:text-white">创建自建实例</h3><div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2"><label className="text-xs font-semibold text-slate-600 dark:text-slate-200">实例名称<input value={instanceName} onChange={event => setInstanceName(event.target.value)} className="mt-1 w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm font-normal dark:border-slate-600 dark:bg-slate-900" /></label><label className="text-xs font-semibold text-slate-600 dark:text-slate-200">审核镜像<select value={imageID} onChange={event => { const next = event.target.value; setImageID(next); const selected = catalog?.images.find(item => item.id === next); setImageVersion(selected?.versions[0]?.tag ?? 'latest') }} className="mt-1 w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm font-normal dark:border-slate-600 dark:bg-slate-900"><option value="">请选择镜像</option>{catalog?.images.map(image => <option key={image.id} value={image.id}>{image.name}</option>)}</select></label><label className="text-xs font-semibold text-slate-600 dark:text-slate-200">版本<select value={imageVersion} onChange={event => setImageVersion(event.target.value)} className="mt-1 w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm font-normal dark:border-slate-600 dark:bg-slate-900">{catalog?.images.find(image => image.id === imageID)?.versions.map(version => <option key={version.tag} value={version.tag}>{version.tag}</option>)}</select></label><label className="text-xs font-semibold text-slate-600 dark:text-slate-200">CPU 核数<input type="number" min="0.1" step="0.1" value={cpu} onChange={event => setCPU(event.target.value)} className="mt-1 w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm font-normal dark:border-slate-600 dark:bg-slate-900" /></label><label className="text-xs font-semibold text-slate-600 dark:text-slate-200">内存 MB<input type="number" min="256" step="256" value={memoryMB} onChange={event => setMemoryMB(event.target.value)} className="mt-1 w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm font-normal dark:border-slate-600 dark:bg-slate-900" /></label></div><Button className="mt-4" disabled={node.status !== 'online' || !instanceName || !imageID} loading={creatingInstance} onClick={() => void createInstance({ nodeID: node.id, name: instanceName, imageId: imageID, imageVersion, cpu: Number(cpu), memoryMB: Number(memoryMB) }).unwrap().then(() => { setMessage('自建实例已进入部署队列。'); setInstanceName('') }).catch(() => setMessage('自建实例创建失败，请检查节点状态、资源配额与镜像版本。'))}>{node.status === 'online' ? '创建实例' : '节点离线，暂不可创建'}</Button>{message && <p className="mt-2 text-xs text-slate-600 dark:text-slate-300">{message}</p>}</section>
    <section className="rounded-xl border border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-800"><div className="border-b border-slate-100 px-5 py-3 dark:border-slate-700"><h3 className="m-0 text-sm font-bold text-slate-900 dark:text-white">该节点实例</h3></div>{nodeInstances.length === 0 ? <p className="px-5 py-4 text-sm text-slate-500">暂无自建实例。</p> : <div className="divide-y divide-slate-100 dark:divide-slate-700">{nodeInstances.map(instance => <div key={instance.id} className="flex items-center justify-between gap-3 px-5 py-3 text-sm"><span><b className="block text-slate-900 dark:text-white">{instance.name}</b><span className="text-xs text-slate-500">{instance.spec} · {instance.status}</span></span>{instance.ip && <a href={instance.ip} target="_blank" rel="noreferrer" className="text-xs text-sky-700 underline dark:text-sky-300">访问</a>}</div>)}</div>}</section>
  </section>

  async function saveTarget(enabled: boolean) {
    setMessage('')
    try {
      await save({ targetURL, enabled }).unwrap()
      setMessage(enabled ? '入口已启用。' : '本地服务已保存。')
    } catch { setMessage('保存失败，请确认地址为 http://127.0.0.1:端口。') }
  }

  if (!detail && data?.device) return <section className="rounded-xl border border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-800">
    <button type="button" onClick={() => onOpen?.(data.device!.id)} className="flex w-full items-center justify-between gap-4 px-5 py-4 text-left transition-colors hover:bg-slate-50 dark:hover:bg-slate-900">
      <span><b className="block text-sm text-slate-900 dark:text-white">{data.device.name}</b><span className="mt-1 block text-xs text-slate-500 dark:text-slate-300">自建节点 · {data.online ? '在线' : '离线'} · 最高共享带宽 {data.bandwidthMbps} Mbps</span></span><span className="text-sm text-sky-700 dark:text-sky-300">管理 ›</span>
    </button>
  </section>
  return <section className="rounded-xl border border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-800">
    <div className="flex flex-wrap items-start justify-between gap-3 border-b border-slate-100 px-5 py-4 dark:border-slate-700">
      <div><h2 className="m-0 text-base font-bold text-slate-900 dark:text-white">自建节点</h2><p className="mb-0 mt-1 text-xs text-slate-500 dark:text-slate-300">当前账户仅可启用 1 个 xcloud-control 节点 · 最高共享带宽 {data?.bandwidthMbps ?? 10} Mbps</p></div>
      {data?.device && <span className={data.online ? 'rounded-full bg-emerald-100 px-2 py-1 text-xs font-semibold text-emerald-700 dark:bg-emerald-950 dark:text-emerald-200' : 'rounded-full bg-slate-100 px-2 py-1 text-xs font-semibold text-slate-600 dark:bg-slate-900 dark:text-slate-300'}>{data.online ? '设备在线' : '设备离线'}</span>}
    </div>
    <div className="space-y-3 px-5 py-4 text-sm">
      {onBack && <Button tone="secondary" onClick={onBack}>返回自建节点列表</Button>}
      {isLoading ? <span className="text-slate-500">正在加载自建节点…</span> : !data?.device ? <><Alert tone="info">生成一次性接入 Token，写入服务器的 <code>/etc/xcloud-control/config.json</code> 后启动服务。Token 仅显示一次，10 分钟内有效。</Alert><Button disabled={creatingToken} onClick={() => void createToken().unwrap().then(result => setEnrollmentToken(result.token))}>生成接入 Token</Button>{enrollmentToken && <label className="block text-xs font-semibold text-slate-600 dark:text-slate-200">请立即复制 Token<input readOnly value={enrollmentToken} className="mt-1.5 w-full rounded-lg border border-amber-300 bg-amber-50 px-3 py-2 font-mono text-xs text-slate-900 dark:border-amber-800 dark:bg-amber-950 dark:text-white" /></label>} </> : <>
        <p className="m-0 text-slate-700 dark:text-slate-200">设备：<b>{data.device.name}</b>{data.device.version ? ` · ${data.device.version}` : ''}</p>
        {route?.accessAddress && <a className="block truncate text-sky-700 underline dark:text-sky-300" href={route.accessAddress} target="_blank" rel="noreferrer">{route.accessAddress}</a>}
        <label className="block text-xs font-semibold text-slate-600 dark:text-slate-200">本地服务地址<input value={targetURL} onChange={event => setTargetURL(event.target.value)} placeholder="http://127.0.0.1:3000" className="mt-1.5 w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm font-normal text-slate-900 outline-none focus:border-sky-500 dark:border-slate-600 dark:bg-slate-900 dark:text-white" /></label>
        <div className="flex flex-wrap gap-2"><Button disabled={busy || !targetURL} onClick={() => void saveTarget(true)}>保存并启用</Button>{route?.status === 'enabled' && <Button tone="secondary" disabled={busy} onClick={() => void action('pause')}>暂停入口</Button>}{route?.status === 'paused' && <Button tone="secondary" disabled={busy || !route.targetURL} onClick={() => void action('resume')}>恢复入口</Button>}<Button tone="danger" disabled={busy} onClick={() => { if (window.confirm('撤销后公网入口会立即失效，确认继续吗？')) void revoke() }}>撤销设备</Button></div>
      </>}
      {message && <p className="m-0 text-xs text-slate-600 dark:text-slate-300">{message}</p>}
    </div>
  </section>
}
