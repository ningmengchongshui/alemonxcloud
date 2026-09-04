import { useState } from 'react'
import {
  useConfirmOrderMutation,
  useGetAdminCatalogQuery,
  useGetAdminAuditLogsQuery,
  useGetAdminMetricsQuery,
  useGetAdminNodesQuery,
  useGetAdminOrdersQuery,
  useGetAdminTasksQuery,
  useSearchAdminUsersQuery,
  useAdjustAdminWalletMutation,
  useRejectOrderMutation,
  useRetryTaskMutation,
  useSaveAdminImageMutation,
  useSaveAdminPlanMutation
} from '@/services/cloudApi'
import { CatalogEditor } from '@/components/CatalogEditor'
import { NodeConfigButton, NodeEditor } from '@/components/NodeEditor'
import type { SuperPage } from '@/types/cloud'

const orderLabel: Record<string, string> = { pending_payment: '待支付', pending_review: '待确认', deploying: '部署中', active: '已生效', expired: '已到期', cancelled: '已取消', rejected: '未通过' }
const disk = (bytes?: number) => bytes ? `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB 可用` : '磁盘信息同步中'

export function AdminPage({ page }: { page: SuperPage }) {
  const isOverview = page === 'overview'
  const catalog = useGetAdminCatalogQuery(undefined, { skip: !isOverview && page !== 'catalog' })
  const auditLogs = useGetAdminAuditLogsQuery(undefined, { skip: page !== 'audit' })
  const orders = useGetAdminOrdersQuery(undefined, { skip: !isOverview && page !== 'orders' })
  const nodes = useGetAdminNodesQuery(undefined, { skip: !isOverview && page !== 'nodes' })
  const tasks = useGetAdminTasksQuery(undefined, { skip: !isOverview && page !== 'tasks' })
  const metrics = useGetAdminMetricsQuery(undefined, { skip: !isOverview })
	const [userQuery, setUserQuery] = useState('')
	const users = useSearchAdminUsersQuery(userQuery)
  const [confirm, { isLoading: confirming }] = useConfirmOrderMutation()
  const [reject] = useRejectOrderMutation()
  const [retry] = useRetryTaskMutation()
  const [saveImage] = useSaveAdminImageMutation()
  const [savePlan] = useSaveAdminPlanMutation()
	const [adjustWallet] = useAdjustAdminWalletMutation()
  const [error, setError] = useState('')
  const refreshing = catalog.isFetching || auditLogs.isFetching || orders.isFetching || nodes.isFetching || tasks.isFetching || metrics.isFetching || users.isFetching

  async function refresh() {
    const activeQueries = page === 'overview'
      ? [catalog.refetch(), orders.refetch(), nodes.refetch(), tasks.refetch(), metrics.refetch()]
      : page === 'catalog' ? [catalog.refetch()]
        : page === 'orders' ? [orders.refetch()]
          : page === 'nodes' ? [nodes.refetch()]
            : page === 'tasks' ? [tasks.refetch()]
              : page === 'users' ? [users.refetch()]
                : [auditLogs.refetch()]
    await Promise.all(activeQueries)
  }

  const pendingOrders = (orders.data ?? []).filter(value => ['pending_payment', 'pending_review'].includes(value.status))
  const failedTasks = (tasks.data ?? []).filter(value => value.status === 'failed')
  const onlineNodes = (nodes.data ?? []).filter(value => Boolean(value.lastHeartbeatAt) && value.enabled)
  const enabledProducts = (catalog.data?.images.filter(value => value.enabled).length ?? 0) + (catalog.data?.plans.filter(value => value.enabled).length ?? 0)
  const nodeCPU = (nodes.data ?? []).reduce((total, node) => total + node.cpuTotal, 0)
  const nodeMemoryMB = (nodes.data ?? []).reduce((total, node) => total + node.memoryTotalMB, 0)

  async function confirmOrder(id: string) {
    if (!window.confirm('确认已收到款项并开始部署？')) return
    setError('')
    try { await confirm(id).unwrap() } catch { setError('确认收款失败，请刷新后重试') }
  }

  async function rejectOrder(id: string) {
    const reason = window.prompt('请输入拒绝原因')
    if (!reason?.trim()) return
    setError('')
    try { await reject({ id, reason: reason.trim() }).unwrap() } catch { setError('拒绝订单失败，请稍后重试') }
  }

  async function retryTask(id: string) {
    setError('')
    try { await retry(id).unwrap() } catch { setError('任务重试失败，请稍后重试') }
  }

  const heading: Record<SuperPage, [string, string, string]> = {
    overview: ['平台运营', '超级管理台', '处理订单、资源和失败任务。'],
    catalog: ['商品管理', '商品目录', '管理镜像和计算套餐。'],
    nodes: ['资源运营', '节点管理', '管理节点接入与运行状态。'],
    orders: ['订单处理', '人工收款订单', '确认款项后自动部署。'],
    tasks: ['任务队列', '任务执行记录', '查看结果，重试失败任务。'],
    users: ['用户运营', '用户与钱包', '管理用户余额和账本。'],
    audit: ['安全审计', '平台操作记录', '查看关键管理操作。']
  }
  const [eyebrow, title, description] = heading[page]

  return <section className="page super-page">
    <header className="page-heading super-heading">
      <div><p className="eyebrow">{eyebrow}</p><h1>{title}</h1><p>{description}</p></div>
      <div className="super-heading-tools"><div className="super-health"><span className={failedTasks.length ? 'warning' : 'healthy'}><i />{failedTasks.length ? `${failedTasks.length} 个任务待处理` : '平台运行正常'}</span><small>数据实时同步</small></div><button className="subtle-button refresh-button" onClick={() => void refresh()} disabled={refreshing} aria-label="刷新当前页面数据"><span aria-hidden="true">↻</span>{refreshing ? '刷新中…' : '刷新数据'}</button></div>
    </header>
    {page === 'overview' && <><section className="super-metrics" aria-label="平台核心指标">
      <article className="super-highlight"><small>优先处理</small><strong>{pendingOrders.length}</strong><span>笔订单等待人工确认</span></article>
      <article><small>任务积压</small><strong>{metrics.data?.taskBacklog ?? '—'}</strong><span>包含待执行与运行中的任务</span></article>
      <article><small>失败任务</small><strong className={failedTasks.length ? 'attention-text' : ''}>{metrics.data?.taskFailures ?? '—'}</strong><span>失败后可安全重试</span></article>
      <article><small>可用节点</small><strong>{nodes.data ? `${onlineNodes.length}/${nodes.data.length}` : '—'}</strong><span>已启用且已上报心跳</span></article>
    </section>
    {error && <p className="form-error" role="alert">{error}</p>}

    <section className="super-section attention-section">
      <div className="super-section-heading"><div><p className="eyebrow">待处理事项</p><h2>待办事项</h2></div><span>{pendingOrders.length + failedTasks.length} 项待办</span></div>
      {pendingOrders.length || failedTasks.length ? <div className="attention-grid">
        <div className="attention-list">
          {pendingOrders.slice(0, 4).map(order => <article key={order.id} className="attention-row"><span className="attention-icon payment-icon">¥</span><div><b>确认订单款项</b><p>{order.planName} · ¥{(order.amountFen / 100).toFixed(2)} · {order.ownerId}</p></div><div className="attention-actions"><button className="primary compact-primary" disabled={confirming} onClick={() => void confirmOrder(order.id)}>{confirming ? '处理中…' : '确认收款'}</button><button className="subtle-button danger-action" onClick={() => void rejectOrder(order.id)}>拒绝</button></div></article>)}
          {failedTasks.slice(0, 4).map(task => <article key={task.id} className="attention-row"><span className="attention-icon task-icon">!</span><div><b>重试失败任务</b><p>{task.action} · 实例 {task.instanceId.slice(0, 14)} · 已尝试 {task.attempts} 次</p></div><div className="attention-actions"><button className="subtle-button" onClick={() => void retryTask(task.id)}>安全重试</button></div></article>)}
        </div>
        <aside className="attention-note"><span>操作提示</span><b>确认收款后会先核验资源，再开始部署。</b><p>避免资源不足导致交付失败。</p></aside>
      </div> : <div className="super-empty"><span>✓</span><div><b>暂无待办</b><p>订单、资源和任务运行正常。</p></div></div>}
    </section></>}

      {page === 'catalog' && <section className="super-section catalog-section">
        <div className="super-section-heading"><div><p className="eyebrow">商品管理</p><h2>可售目录</h2></div><div className="section-heading-actions"><span>{enabledProducts} 个商品在售</span><CatalogEditor /></div></div>
        <div className="admin-table-wrap"><h3>镜像版本</h3><table><thead><tr><th>镜像</th><th>版本</th><th>不可变摘要</th><th>状态</th><th aria-label="操作" /></tr></thead><tbody>{(catalog.data?.images ?? []).map(image => <tr key={image.id}><td><b>{image.name}</b><small>{image.imageRef}</small></td><td>{image.version}</td><td><code>{image.imageDigest || '—'}</code></td><td><span className={`admin-state ${image.enabled ? 'enabled' : 'disabled'}`}>{image.enabled ? '可售' : '已下架'}</span></td><td><button className="text-button" onClick={() => { if (window.confirm(`确定${image.enabled ? '下架' : '启用'}此镜像版本吗？`)) void saveImage({ ...image, enabled: !image.enabled }) }}>{image.enabled ? '下架' : '启用'}</button></td></tr>)}</tbody></table></div>
        <div className="admin-table-wrap"><h3>计算套餐</h3><table><thead><tr><th>套餐</th><th>配置</th><th>月价</th><th>状态</th><th aria-label="操作" /></tr></thead><tbody>{(catalog.data?.plans ?? []).map(plan => <tr key={plan.id}><td><b>{plan.name}</b></td><td>{plan.cpu} 核 / {plan.memoryMB / 1024} GB</td><td>¥{(plan.monthlyPriceFen / 100).toFixed(2)}</td><td><span className={`admin-state ${plan.enabled ? 'enabled' : 'disabled'}`}>{plan.enabled ? '可售' : '已下架'}</span></td><td><button className="text-button" onClick={() => { if (window.confirm(`确定${plan.enabled ? '下架' : '启用'}此套餐吗？`)) void savePlan({ ...plan, enabled: !plan.enabled }) }}>{plan.enabled ? '下架' : '启用'}</button></td></tr>)}</tbody></table></div>
      </section>}
      {page === 'nodes' && <section className="super-section node-section">
        <div className="super-section-heading"><div><p className="eyebrow">资源运营</p><h2>节点管理</h2></div><div className="section-heading-actions"><span>{onlineNodes.length} 个节点在线</span><NodeEditor /></div></div>
        <section className="node-summary" aria-label="节点资源汇总"><article><small>节点总数</small><strong>{nodes.data?.length ?? '—'}</strong></article><article><small>在线节点</small><strong>{onlineNodes.length}</strong></article><article><small>可调度 CPU</small><strong>{nodeCPU || '—'}</strong></article><article><small>可调度内存</small><strong>{nodeMemoryMB ? `${Math.round(nodeMemoryMB / 1024)} GB` : '—'}</strong></article></section><p className="node-intro">新实例只会调度到已启用且已确认容量的节点。</p><div className="node-list">{(nodes.data ?? []).map(node => <article key={node.id} className="node-row"><div><b>{node.name}</b><p>{node.agentURL || '未配置 Agent 地址'}</p></div><div className="node-details"><span>{node.cpuTotal ? `可调度 ${node.cpuTotal} 核 / ${Math.round(node.memoryTotalMB / 1024)} GB` : '待确认可调度容量'}</span><span>{node.managedContainerCount ?? 0} 个托管容器</span><span>{disk(node.diskAvailableBytes)}</span><span>{node.dockerVersion ? `Docker ${node.dockerVersion}` : 'Docker 信息同步中'}</span></div><div className="node-actions"><span className={`admin-state ${node.enabled && node.lastHeartbeatAt ? 'enabled' : 'disabled'}`}>{node.enabled && node.lastHeartbeatAt ? '在线' : node.enabled ? '等待心跳' : '未启用'}</span><NodeConfigButton node={node} /></div></article>)}</div>
      </section>}
      

    {page === 'orders' && <section className="super-section admin-orders-section"><div className="super-section-heading"><div><p className="eyebrow">订单记录</p><h2>全部人工收款订单</h2></div><span>{orders.data?.length ?? 0} 笔订单</span></div><div className="admin-table-wrap"><table><thead><tr><th>订单</th><th>用户</th><th>商品</th><th>金额</th><th>付款备注</th><th>状态</th></tr></thead><tbody>{(orders.data ?? []).map(order => <tr key={order.id}><td><code>{order.id.slice(0, 14)}</code></td><td>{order.ownerId}</td><td><b>{order.planName}</b><small>{order.imageName} · {order.imageVersion}</small></td><td>¥{(order.amountFen / 100).toFixed(2)}</td><td>{order.paymentNote || '—'}</td><td><span className={`admin-state ${['active'].includes(order.status) ? 'enabled' : ['rejected', 'cancelled', 'expired'].includes(order.status) ? 'disabled' : 'pending'}`}>{orderLabel[order.status] || order.status}</span></td></tr>)}</tbody></table></div></section>}
    {page === 'tasks' && <section className="super-section admin-tasks-section"><div className="super-section-heading"><div><p className="eyebrow">任务记录</p><h2>任务队列与执行结果</h2></div><span>{tasks.data?.length ?? 0} 个任务</span></div><div className="admin-table-wrap"><table><thead><tr><th>动作</th><th>实例</th><th>状态</th><th>尝试次数</th><th>最近错误</th><th aria-label="操作" /></tr></thead><tbody>{(tasks.data ?? []).map(task => <tr key={task.id}><td><b>{task.action}</b></td><td><code>{task.instanceId.slice(0, 14)}</code></td><td><span className={`admin-state ${task.status === 'failed' ? 'disabled' : task.status === 'completed' ? 'enabled' : 'pending'}`}>{task.status}</span></td><td>{task.attempts}</td><td>{task.lastError || '—'}</td><td>{task.status === 'failed' ? <button className="text-button" onClick={() => void retryTask(task.id)}>安全重试</button> : '—'}</td></tr>)}</tbody></table></div></section>}
    {page === 'users' && <section className="super-section"><div className="super-section-heading"><div><p className="eyebrow">用户目录</p></div><span>账本不可修改</span></div><input value={userQuery} onChange={event=>setUserQuery(event.target.value)} placeholder="用户名、邮箱或 Auth 用户 ID" /><div className="admin-table-wrap"><table><thead><tr><th>用户</th><th>邮箱</th><th>余额</th><th>最后登录</th><th aria-label="操作" /></tr></thead><tbody>{(users.data??[]).map(user=><tr key={user.id}><td><b>{user.username}</b><small>{user.id}</small></td><td>{user.email||'—'}</td><td>{(user.balanceFen/100).toFixed(2)} 代币</td><td>{new Date(user.lastLoginAt).toLocaleString('zh-CN')}</td><td><button className="text-button" onClick={()=>{const amount=window.prompt('输入变动代币数（正数充值，负数扣减/冲正）');const note=window.prompt('填写不可变运营备注');const fen=Number(amount)*100;if(!note?.trim()||!Number.isInteger(fen)||fen===0)return;void adjustWallet({id:user.id,amountFen:fen,note:note.trim()})}}>调整余额</button></td></tr>)}</tbody></table></div></section>}
    {page === 'audit' && <section className="super-section"><div className="super-section-heading"><div><p className="eyebrow">安全审计</p><h2>最近平台操作</h2></div><span>{auditLogs.data?.length ?? 0} 条记录</span></div><div className="admin-table-wrap"><table><thead><tr><th>时间</th><th>操作者</th><th>操作</th><th>对象</th></tr></thead><tbody>{(auditLogs.data ?? []).map(item=><tr key={item.id}><td>{new Date(item.createdAt).toLocaleString('zh-CN')}</td><td>{item.actorId}</td><td>{item.action}</td><td>{item.targetType} · {item.targetId.slice(0,14)}</td></tr>)}</tbody></table></div></section>}
  </section>
}
