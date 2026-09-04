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
import { NodeEditor } from '@/components/NodeEditor'
import type { SuperPage } from '@/types/cloud'

const orderLabel: Record<string, string> = { pending_payment: '待支付', pending_review: '待确认', deploying: '部署中', active: '已生效', expired: '已到期', cancelled: '已取消', rejected: '未通过' }

export function AdminPage({ page }: { page: SuperPage }) {
  const catalog = useGetAdminCatalogQuery()
  const auditLogs = useGetAdminAuditLogsQuery()
  const orders = useGetAdminOrdersQuery()
  const nodes = useGetAdminNodesQuery()
  const tasks = useGetAdminTasksQuery()
  const metrics = useGetAdminMetricsQuery()
	const [userQuery, setUserQuery] = useState('')
	const users = useSearchAdminUsersQuery(userQuery)
  const [confirm, { isLoading: confirming }] = useConfirmOrderMutation()
  const [reject] = useRejectOrderMutation()
  const [retry] = useRetryTaskMutation()
  const [saveImage] = useSaveAdminImageMutation()
  const [savePlan] = useSaveAdminPlanMutation()
	const [adjustWallet] = useAdjustAdminWalletMutation()
  const [error, setError] = useState('')

  const pendingOrders = (orders.data ?? []).filter(value => ['pending_payment', 'pending_review'].includes(value.status))
  const failedTasks = (tasks.data ?? []).filter(value => value.status === 'failed')
  const onlineNodes = (nodes.data ?? []).filter(value => Boolean(value.lastHeartbeatAt) && value.enabled)
  const enabledProducts = (catalog.data?.images.filter(value => value.enabled).length ?? 0) + (catalog.data?.plans.filter(value => value.enabled).length ?? 0)

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
    overview: ['平台运营', '超级管理台', '优先处理会影响交付的订单、资源与失败任务。'],
    catalog: ['商品管理', '商品目录', '维护用户可选择的镜像版本和计算套餐。'],
    nodes: ['资源运营', '节点与配额', '查看节点心跳、资源预占并维护节点配置。'],
    orders: ['订单处理', '人工收款订单', '确认款项后，系统将核验资源并自动开始部署。'],
    tasks: ['任务队列', '任务执行记录', '查看执行结果；失败任务可安全重试。'],
    users: ['用户运营', '用户与钱包', '仅已登录过 xCloud 的用户可被充值、扣减或冲正。'],
    audit: ['安全审计', '平台操作记录', '追踪平台范围内的关键管理操作。']
  }
  const [eyebrow, title, description] = heading[page]

  return <section className="page super-page">
    <header className="page-heading super-heading">
      <div><p className="eyebrow">{eyebrow}</p><h1>{title}</h1><p>{description}</p></div>
      <div className="super-health"><span className={failedTasks.length ? 'warning' : 'healthy'}><i />{failedTasks.length ? `${failedTasks.length} 个任务待处理` : '平台运行正常'}</span><small>数据实时同步</small></div>
    </header>
    {page === 'overview' && <><section className="super-metrics" aria-label="平台核心指标">
      <article className="super-highlight"><small>优先处理</small><strong>{pendingOrders.length}</strong><span>笔订单等待人工确认</span></article>
      <article><small>任务积压</small><strong>{metrics.data?.taskBacklog ?? '—'}</strong><span>包含待执行与运行中的任务</span></article>
      <article><small>失败任务</small><strong className={failedTasks.length ? 'attention-text' : ''}>{metrics.data?.taskFailures ?? '—'}</strong><span>失败后可安全重试</span></article>
      <article><small>可用节点</small><strong>{nodes.data ? `${onlineNodes.length}/${nodes.data.length}` : '—'}</strong><span>已启用且已上报心跳</span></article>
    </section>
    {error && <p className="form-error" role="alert">{error}</p>}

    <section className="super-section attention-section">
      <div className="super-section-heading"><div><p className="eyebrow">待处理事项</p><h2>先完成这些，服务才能继续交付</h2></div><span>{pendingOrders.length + failedTasks.length} 项待办</span></div>
      {pendingOrders.length || failedTasks.length ? <div className="attention-grid">
        <div className="attention-list">
          {pendingOrders.slice(0, 4).map(order => <article key={order.id} className="attention-row"><span className="attention-icon payment-icon">¥</span><div><b>确认订单款项</b><p>{order.planName} · ¥{(order.amountFen / 100).toFixed(2)} · {order.ownerId}</p></div><div className="attention-actions"><button className="primary compact-primary" disabled={confirming} onClick={() => void confirmOrder(order.id)}>{confirming ? '处理中…' : '确认收款'}</button><button className="subtle-button danger-action" onClick={() => void rejectOrder(order.id)}>拒绝</button></div></article>)}
          {failedTasks.slice(0, 4).map(task => <article key={task.id} className="attention-row"><span className="attention-icon task-icon">!</span><div><b>重试失败任务</b><p>{task.action} · 实例 {task.instanceId.slice(0, 14)} · 已尝试 {task.attempts} 次</p></div><div className="attention-actions"><button className="subtle-button" onClick={() => void retryTask(task.id)}>安全重试</button></div></article>)}
        </div>
        <aside className="attention-note"><span>运营提示</span><b>确认收款后会先核验可用资源，再开始创建实例。</b><p>这可以避免超售，也避免在资源不足时向用户承诺无法交付的服务。</p></aside>
      </div> : <div className="super-empty"><span>✓</span><div><b>没有需要立即处理的事项</b><p>订单确认、资源调度和任务执行均处于正常状态。</p></div></div>}
    </section></>}

      {page === 'catalog' && <section className="super-section catalog-section">
        <div className="super-section-heading"><div><p className="eyebrow">商品管理</p><h2>可售目录</h2></div><span>{enabledProducts} 个商品在售</span></div>
        <CatalogEditor />
        <div className="admin-table-wrap"><h3>镜像版本</h3><table><thead><tr><th>镜像</th><th>版本</th><th>不可变摘要</th><th>状态</th><th aria-label="操作" /></tr></thead><tbody>{(catalog.data?.images ?? []).map(image => <tr key={image.id}><td><b>{image.name}</b><small>{image.imageRef}</small></td><td>{image.version}</td><td><code>{image.imageDigest || '—'}</code></td><td><span className={`admin-state ${image.enabled ? 'enabled' : 'disabled'}`}>{image.enabled ? '可售' : '已下架'}</span></td><td><button className="text-button" onClick={() => { if (window.confirm(`确定${image.enabled ? '下架' : '启用'}此镜像版本吗？`)) void saveImage({ ...image, enabled: !image.enabled }) }}>{image.enabled ? '下架' : '启用'}</button></td></tr>)}</tbody></table></div>
        <div className="admin-table-wrap"><h3>计算套餐</h3><table><thead><tr><th>套餐</th><th>配置</th><th>月价</th><th>状态</th><th aria-label="操作" /></tr></thead><tbody>{(catalog.data?.plans ?? []).map(plan => <tr key={plan.id}><td><b>{plan.name}</b></td><td>{plan.cpu} 核 / {plan.memoryMB / 1024} GB</td><td>¥{(plan.monthlyPriceFen / 100).toFixed(2)}</td><td><span className={`admin-state ${plan.enabled ? 'enabled' : 'disabled'}`}>{plan.enabled ? '可售' : '已下架'}</span></td><td><button className="text-button" onClick={() => { if (window.confirm(`确定${plan.enabled ? '下架' : '启用'}此套餐吗？`)) void savePlan({ ...plan, enabled: !plan.enabled }) }}>{plan.enabled ? '下架' : '启用'}</button></td></tr>)}</tbody></table></div>
      </section>}
      {page === 'nodes' && <section className="super-section node-section">
        <div className="super-section-heading"><div><p className="eyebrow">资源运营</p><h2>节点与配额</h2></div><span>{onlineNodes.length} 个节点在线</span></div>
        <NodeEditor nodes={nodes.data ?? []} />
        <div className="node-list">{(nodes.data ?? []).map(node => <article key={node.id} className="node-row"><div><b>{node.name}</b><p>{node.agentURL || '尚未配置 Agent 地址'}</p></div><div className="node-capacity"><span>CPU {node.cpuReserved}/{node.cpuTotal}</span><progress max={Math.max(node.cpuTotal, 1)} value={node.cpuReserved} /><span>内存 {Math.round(node.memoryReservedMB / 1024)}/{Math.round(node.memoryTotalMB / 1024)} GB</span><progress max={Math.max(node.memoryTotalMB, 1)} value={node.memoryReservedMB} /></div><span className={`admin-state ${node.enabled && node.lastHeartbeatAt ? 'enabled' : 'disabled'}`}>{node.enabled && node.lastHeartbeatAt ? '在线' : node.enabled ? '等待心跳' : '已停用'}</span></article>)}</div>
      </section>}
      

    {page === 'orders' && <section className="super-section admin-orders-section"><div className="super-section-heading"><div><p className="eyebrow">订单记录</p><h2>全部人工收款订单</h2></div><span>{orders.data?.length ?? 0} 笔订单</span></div><div className="admin-table-wrap"><table><thead><tr><th>订单</th><th>用户</th><th>商品</th><th>金额</th><th>付款备注</th><th>状态</th></tr></thead><tbody>{(orders.data ?? []).map(order => <tr key={order.id}><td><code>{order.id.slice(0, 14)}</code></td><td>{order.ownerId}</td><td><b>{order.planName}</b><small>{order.imageName} · {order.imageVersion}</small></td><td>¥{(order.amountFen / 100).toFixed(2)}</td><td>{order.paymentNote || '—'}</td><td><span className={`admin-state ${['active'].includes(order.status) ? 'enabled' : ['rejected', 'cancelled', 'expired'].includes(order.status) ? 'disabled' : 'pending'}`}>{orderLabel[order.status] || order.status}</span></td></tr>)}</tbody></table></div></section>}
    {page === 'tasks' && <section className="super-section admin-tasks-section"><div className="super-section-heading"><div><p className="eyebrow">任务记录</p><h2>任务队列与执行结果</h2></div><span>{tasks.data?.length ?? 0} 个任务</span></div><div className="admin-table-wrap"><table><thead><tr><th>动作</th><th>实例</th><th>状态</th><th>尝试次数</th><th>最近错误</th><th aria-label="操作" /></tr></thead><tbody>{(tasks.data ?? []).map(task => <tr key={task.id}><td><b>{task.action}</b></td><td><code>{task.instanceId.slice(0, 14)}</code></td><td><span className={`admin-state ${task.status === 'failed' ? 'disabled' : task.status === 'completed' ? 'enabled' : 'pending'}`}>{task.status}</span></td><td>{task.attempts}</td><td>{task.lastError || '—'}</td><td>{task.status === 'failed' ? <button className="text-button" onClick={() => void retryTask(task.id)}>安全重试</button> : '—'}</td></tr>)}</tbody></table></div></section>}
    {page === 'users' && <section className="super-section"><div className="super-section-heading"><div><p className="eyebrow">用户目录</p><h2>钱包运营</h2></div><span>账本不可修改</span></div><label className="admin-search">搜索已登录用户<input value={userQuery} onChange={event=>setUserQuery(event.target.value)} placeholder="用户名、邮箱或 Auth 用户 ID" /></label><div className="admin-table-wrap"><table><thead><tr><th>用户</th><th>邮箱</th><th>余额</th><th>最后登录</th><th aria-label="操作" /></tr></thead><tbody>{(users.data??[]).map(user=><tr key={user.id}><td><b>{user.username}</b><small>{user.id}</small></td><td>{user.email||'—'}</td><td>{(user.balanceFen/100).toFixed(2)} 代币</td><td>{new Date(user.lastLoginAt).toLocaleString('zh-CN')}</td><td><button className="text-button" onClick={()=>{const amount=window.prompt('输入变动代币数（正数充值，负数扣减/冲正）');const note=window.prompt('填写不可变运营备注');const fen=Number(amount)*100;if(!note?.trim()||!Number.isInteger(fen)||fen===0)return;void adjustWallet({id:user.id,amountFen:fen,note:note.trim()})}}>调整余额</button></td></tr>)}</tbody></table></div></section>}
    {page === 'audit' && <section className="super-section"><div className="super-section-heading"><div><p className="eyebrow">安全审计</p><h2>最近平台操作</h2></div><span>{auditLogs.data?.length ?? 0} 条记录</span></div><div className="admin-table-wrap"><table><thead><tr><th>时间</th><th>操作者</th><th>操作</th><th>对象</th></tr></thead><tbody>{(auditLogs.data ?? []).map(item=><tr key={item.id}><td>{new Date(item.createdAt).toLocaleString('zh-CN')}</td><td>{item.actorId}</td><td>{item.action}</td><td>{item.targetType} · {item.targetId.slice(0,14)}</td></tr>)}</tbody></table></div></section>}
  </section>
}
