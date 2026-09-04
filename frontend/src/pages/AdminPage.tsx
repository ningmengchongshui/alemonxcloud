import { useState } from 'react'
import {
  useGetAdminCatalogQuery,
  useGetAdminAuditLogsQuery,
  useGetAdminMetricsQuery,
  useGetAdminNodesQuery,
  useGetAdminOrdersQuery,
  useGetAdminTasksQuery,
  useGetAdminWalletEntriesQuery,
  useSearchAdminUsersQuery,
  useAdjustAdminWalletMutation,
  useRetryTaskMutation,
  useSaveAdminImageMutation,
  useSaveAdminPlanMutation
} from '@/services/cloudApi'
import { CatalogEditor } from '@/components/CatalogEditor'
import { NodeConfigButton, NodeEditor } from '@/components/NodeEditor'
import { ActionDialog } from '@/components/ActionDialog'
import type { CloudUser, SuperPage } from '@/types/cloud'

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
  const [retry] = useRetryTaskMutation()
  const [saveImage] = useSaveAdminImageMutation()
  const [savePlan] = useSaveAdminPlanMutation()
  const [adjustWallet, { isLoading: adjustingWallet }] = useAdjustAdminWalletMutation()
  const [adjustingUser, setAdjustingUser] = useState<CloudUser | null>(null)
  const [adjustAmount, setAdjustAmount] = useState('')
  const [adjustDirection, setAdjustDirection] = useState<'increase' | 'decrease'>('increase')
  const [adjustNote, setAdjustNote] = useState('')
  const [walletHistoryUser, setWalletHistoryUser] = useState<CloudUser | null>(null)
  const walletHistory = useGetAdminWalletEntriesQuery(walletHistoryUser?.id ?? '', { skip: !walletHistoryUser })
  const [error, setError] = useState('')
  const [dialog, setDialog] = useState<{ kind: 'toggle-image' | 'toggle-plan'; id: string; title: string; description: string; enabled?: boolean } | null>(null)
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

  const deployingOrders = (orders.data ?? []).filter(value => value.status === 'deploying')
  const failedTasks = (tasks.data ?? []).filter(value => value.status === 'failed')
  const onlineNodes = (nodes.data ?? []).filter(value => Boolean(value.lastHeartbeatAt) && value.enabled)
  const enabledProducts = (catalog.data?.images.filter(value => value.enabled).length ?? 0) + (catalog.data?.plans.filter(value => value.enabled).length ?? 0)
  const nodeCPU = (nodes.data ?? []).reduce((total, node) => total + node.cpuTotal, 0)
  const nodeMemoryMB = (nodes.data ?? []).reduce((total, node) => total + node.memoryTotalMB, 0)

  async function retryTask(id: string) {
    setError('')
    try { await retry(id).unwrap() } catch { setError('任务重试失败，请稍后重试') }
  }

  function openWalletAdjust(user: CloudUser) {
    setError('')
    setAdjustingUser(user)
    setAdjustAmount('')
    setAdjustDirection('increase')
    setAdjustNote('')
  }

  async function submitWalletAdjust() {
    if (!adjustingUser || !adjustNote.trim()) return
    const amount = Number(adjustAmount)
    const amountFen = Math.round(Math.abs(amount) * 100) * (adjustDirection === 'increase' ? 1 : -1)
    if (!Number.isFinite(amount) || !Number.isInteger(amountFen) || amountFen === 0) {
      setError('请输入非零的有效金额，最多保留两位小数')
      return
    }
    setError('')
    try {
      await adjustWallet({ id: adjustingUser.id, amountFen: Math.abs(amountFen), direction: adjustDirection, note: adjustNote.trim() }).unwrap()
      setAdjustingUser(null)
    } catch {
      setError('调整余额失败，请检查余额和运营备注后重试')
    }
  }

  const heading: Record<SuperPage, [string, string, string]> = {
    overview: ['平台运营', '超级管理台', '监控自动订单、资源和失败任务。'],
    catalog: ['商品管理', '商品目录', '管理镜像和计算套餐。'],
    nodes: ['资源运营', '节点管理', '管理节点接入与运行状态。'],
    orders: ['订单记录', '自动购买订单', '钱包扣款后自动部署。'],
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
      <article className="super-highlight"><small>自动部署</small><strong>{deployingOrders.length}</strong><span>笔订单正在等待资源准备</span></article>
      <article><small>任务积压</small><strong>{metrics.data?.taskBacklog ?? '—'}</strong><span>包含待执行与运行中的任务</span></article>
      <article><small>失败任务</small><strong className={failedTasks.length ? 'attention-text' : ''}>{metrics.data?.taskFailures ?? '—'}</strong><span>失败后可安全重试</span></article>
      <article><small>可用节点</small><strong>{nodes.data ? `${onlineNodes.length}/${nodes.data.length}` : '—'}</strong><span>已启用且已上报心跳</span></article>
    </section>
    {error && <p className="form-error" role="alert">{error}</p>}

    <section className="super-section attention-section">
      <div className="super-section-heading"><div><p className="eyebrow">自动交付</p><h2>部署与任务</h2></div><span>{deployingOrders.length + failedTasks.length} 项</span></div>
      {deployingOrders.length || failedTasks.length ? <div className="attention-grid">
        <div className="attention-list">
          {deployingOrders.slice(0, 4).map(order => <article key={order.id} className="attention-row"><span className="attention-icon payment-icon">✓</span><div><b>自动部署中</b><p>{order.planName} · ¥{(order.amountFen / 100).toFixed(2)} · {order.ownerId}</p></div><div className="attention-actions"><span className="admin-state pending">等待任务完成</span></div></article>)}
          {failedTasks.slice(0, 4).map(task => <article key={task.id} className="attention-row"><span className="attention-icon task-icon">!</span><div><b>重试失败任务</b><p>{task.action} · 实例 {task.instanceId.slice(0, 14)} · 已尝试 {task.attempts} 次</p></div><div className="attention-actions"><button className="subtle-button" onClick={() => void retryTask(task.id)}>安全重试</button></div></article>)}
        </div>
        <aside className="attention-note"><span>自动购买</span><b>系统先校验余额和资源，再扣款并开始部署。</b><p>资源不足时不会扣款，也不会创建订单。</p></aside>
      </div> : <div className="super-empty"><span>✓</span><div><b>暂无待办</b><p>订单、资源和任务运行正常。</p></div></div>}
    </section></>}

      {page === 'catalog' && <section className="super-section catalog-section">
        <div className="super-section-heading"><div><p className="eyebrow">商品管理</p><h2>可售目录</h2></div><div className="section-heading-actions"><span>{enabledProducts} 个商品在售</span><CatalogEditor /></div></div>
        <div className="admin-table-wrap"><h3>镜像版本</h3><table><thead><tr><th>镜像</th><th>版本</th><th>不可变摘要</th><th>状态</th><th aria-label="操作" /></tr></thead><tbody>{(catalog.data?.images ?? []).map(image => <tr key={image.id}><td><b>{image.name}</b><small>{image.imageRef}</small></td><td>{image.version}</td><td><code>{image.imageDigest || '—'}</code></td><td><span className={`admin-state ${image.enabled ? 'enabled' : 'disabled'}`}>{image.enabled ? '可售' : '已下架'}</span></td><td><button className="text-button" onClick={() => setDialog({ kind: 'toggle-image', id: image.id, title: image.enabled ? '下架镜像版本' : '启用镜像版本', description: `确定${image.enabled ? '下架' : '启用'} ${image.name} ${image.version} 吗？`, enabled: image.enabled })}>{image.enabled ? '下架' : '启用'}</button></td></tr>)}</tbody></table></div>
        <div className="admin-table-wrap"><h3>计算套餐</h3><table><thead><tr><th>套餐</th><th>配置</th><th>月价</th><th>状态</th><th aria-label="操作" /></tr></thead><tbody>{(catalog.data?.plans ?? []).map(plan => <tr key={plan.id}><td><b>{plan.name}</b></td><td>{plan.cpu} 核 / {plan.memoryMB / 1024} GB</td><td>¥{(plan.monthlyPriceFen / 100).toFixed(2)}</td><td><span className={`admin-state ${plan.enabled ? 'enabled' : 'disabled'}`}>{plan.enabled ? '可售' : '已下架'}</span></td><td><button className="text-button" onClick={() => setDialog({ kind: 'toggle-plan', id: plan.id, title: plan.enabled ? '下架计算套餐' : '启用计算套餐', description: `确定${plan.enabled ? '下架' : '启用'} ${plan.name} 吗？`, enabled: plan.enabled })}>{plan.enabled ? '下架' : '启用'}</button></td></tr>)}</tbody></table></div>
      </section>}
      {page === 'nodes' && <section className="super-section node-section">
        <div className="super-section-heading"><div><p className="eyebrow">资源运营</p><h2>节点管理</h2></div><div className="section-heading-actions"><span>{onlineNodes.length} 个节点在线</span><NodeEditor /></div></div>
        <section className="node-summary" aria-label="节点资源汇总"><article><small>节点总数</small><strong>{nodes.data?.length ?? '—'}</strong></article><article><small>在线节点</small><strong>{onlineNodes.length}</strong></article><article><small>可调度 CPU</small><strong>{nodeCPU || '—'}</strong></article><article><small>可调度内存</small><strong>{nodeMemoryMB ? `${Math.round(nodeMemoryMB / 1024)} GB` : '—'}</strong></article></section><p className="node-intro">新实例只会调度到已启用且已确认容量的节点。</p><div className="node-list">{(nodes.data ?? []).map(node => <article key={node.id} className="node-row"><div><b>{node.name}</b><p>{node.agentURL || '未配置 Agent 地址'}</p></div><div className="node-details"><span>{node.cpuTotal ? `可调度 ${node.cpuTotal} 核 / ${Math.round(node.memoryTotalMB / 1024)} GB` : '待确认可调度容量'}</span><span>{node.managedContainerCount ?? 0} 个托管容器</span><span>{disk(node.diskAvailableBytes)}</span><span>{node.dockerVersion ? `Docker ${node.dockerVersion}` : 'Docker 信息同步中'}</span></div><div className="node-actions"><span className={`admin-state ${node.enabled && node.lastHeartbeatAt ? 'enabled' : 'disabled'}`}>{node.enabled && node.lastHeartbeatAt ? '在线' : node.enabled ? '等待心跳' : '未启用'}</span><NodeConfigButton node={node} /></div></article>)}</div>
      </section>}
      

    {page === 'orders' && <section className="super-section admin-orders-section"><div className="super-section-heading"><div><p className="eyebrow">订单记录</p><h2>全部自动购买订单</h2></div><span>{orders.data?.length ?? 0} 笔订单</span></div><div className="admin-table-wrap"><table><thead><tr><th>订单</th><th>用户</th><th>商品</th><th>金额</th><th>状态</th></tr></thead><tbody>{(orders.data ?? []).map(order => <tr key={order.id}><td><code>{order.id.slice(0, 14)}</code></td><td>{order.ownerId}</td><td><b>{order.planName}</b><small>{order.imageName} · {order.imageVersion}</small></td><td>¥{(order.amountFen / 100).toFixed(2)}</td><td><span className={`admin-state ${['active'].includes(order.status) ? 'enabled' : ['rejected', 'cancelled', 'expired'].includes(order.status) ? 'disabled' : 'pending'}`}>{orderLabel[order.status] || order.status}</span></td></tr>)}</tbody></table></div></section>}
    {page === 'tasks' && <section className="super-section admin-tasks-section"><div className="super-section-heading"><div><p className="eyebrow">任务记录</p><h2>任务队列与执行结果</h2></div><span>{tasks.data?.length ?? 0} 个任务</span></div><div className="admin-table-wrap"><table><thead><tr><th>动作</th><th>实例</th><th>状态</th><th>尝试次数</th><th>最近错误</th><th aria-label="操作" /></tr></thead><tbody>{(tasks.data ?? []).map(task => <tr key={task.id}><td><b>{task.action}</b></td><td><code>{task.instanceId.slice(0, 14)}</code></td><td><span className={`admin-state ${task.status === 'failed' ? 'disabled' : task.status === 'completed' ? 'enabled' : 'pending'}`}>{task.status}</span></td><td>{task.attempts}</td><td>{task.lastError || '—'}</td><td>{task.status === 'failed' ? <button className="text-button" onClick={() => void retryTask(task.id)}>安全重试</button> : '—'}</td></tr>)}</tbody></table></div></section>}
    {page === 'users' && <section className="super-section"><div className="super-section-heading"><div><p className="eyebrow">用户目录</p></div><span>账本不可修改</span></div><input value={userQuery} onChange={event=>setUserQuery(event.target.value)} placeholder="用户名、邮箱或 Auth 用户 ID" /><div className="admin-table-wrap"><table><thead><tr><th>用户</th><th>邮箱</th><th>余额</th><th>最后登录</th><th aria-label="操作" /></tr></thead><tbody>{(users.data??[]).map(user=><tr key={user.id}><td><b>{user.username}</b><small>{user.id}</small></td><td>{user.email||'—'}</td><td>{(user.balanceFen/100).toFixed(2)} 代币</td><td>{new Date(user.lastLoginAt).toLocaleString('zh-CN')}</td><td><button className="text-button" onClick={() => setWalletHistoryUser(user)}>查看流水</button><button className="text-button" onClick={() => openWalletAdjust(user)}>变更余额</button></td></tr>)}</tbody></table></div></section>}
    {dialog && <ActionDialog title={dialog.title} description={dialog.description} confirmLabel={dialog.enabled ? '确认下架' : '确认启用'} danger={Boolean(dialog.enabled)} onCancel={() => setDialog(null)} onConfirm={() => { if (dialog.kind === 'toggle-image') { const image = catalog.data?.images.find(value => value.id === dialog.id); if (image) void saveImage({ ...image, enabled: !image.enabled }).unwrap().then(() => setDialog(null)) } else { const plan = catalog.data?.plans.find(value => value.id === dialog.id); if (plan) void savePlan({ ...plan, enabled: !plan.enabled }).unwrap().then(() => setDialog(null)) } }} />}
    {adjustingUser && <div className="modal-backdrop" onMouseDown={event => { if (event.target === event.currentTarget) setAdjustingUser(null) }}><section className="modal-card compact-modal" role="dialog" aria-modal="true" aria-labelledby="wallet-adjust-title"><div className="modal-heading"><div><p className="eyebrow">用户与钱包</p><h2 id="wallet-adjust-title">变更余额</h2><p>{adjustingUser.username} · 当前余额 {(adjustingUser.balanceFen / 100).toFixed(2)} 代币</p></div><button className="modal-close" onClick={() => setAdjustingUser(null)} aria-label="关闭余额变更弹窗">×</button></div><div className="admin-editor-block"><div className="admin-field"><label htmlFor="wallet-adjust-direction">操作类型</label><select id="wallet-adjust-direction" value={adjustDirection} onChange={event => setAdjustDirection(event.target.value as 'increase' | 'decrease')}><option value="increase">增加余额（充值）</option><option value="decrease">扣减余额</option></select></div><div className="admin-field"><label htmlFor="wallet-adjust-amount">{adjustDirection === 'increase' ? '增加' : '扣减'}金额（代币）</label><input id="wallet-adjust-amount" type="number" min="0.01" step="0.01" value={adjustAmount} onChange={event => setAdjustAmount(event.target.value)} placeholder="请输入金额，不需要填写正负号" autoFocus /></div><div className="admin-field"><label htmlFor="wallet-adjust-note">操作原因</label><input id="wallet-adjust-note" value={adjustNote} onChange={event => setAdjustNote(event.target.value)} placeholder="每笔余额变更都必须填写原因" /></div><div className="modal-actions"><button className="subtle-button" onClick={() => setAdjustingUser(null)}>取消</button><button className="primary" disabled={adjustingWallet || !adjustAmount || !adjustNote.trim()} onClick={() => void submitWalletAdjust()}>{adjustingWallet ? '提交中…' : `确认${adjustDirection === 'increase' ? '增加' : '扣减'}`}</button></div></div></section></div>}
    {walletHistoryUser && <div className="modal-backdrop" onMouseDown={event => { if (event.target === event.currentTarget) setWalletHistoryUser(null) }}><section className="modal-card compact-modal" role="dialog" aria-modal="true" aria-labelledby="wallet-history-title"><div className="modal-heading"><div><p className="eyebrow">用户与钱包</p><h2 id="wallet-history-title">充值与消费流水</h2><p>{walletHistoryUser.username} · 当前余额 {(walletHistoryUser.balanceFen / 100).toFixed(2)} 代币</p></div><button className="modal-close" onClick={() => setWalletHistoryUser(null)} aria-label="关闭钱包流水弹窗">×</button></div>{walletHistory.isLoading ? <p>正在加载流水…</p> : <div className="wallet-entry-list">{(walletHistory.data ?? []).map(entry => <article className="wallet-entry-row" key={entry.id}><div><b>{entry.type === 'purchase' ? '服务购买' : entry.type === 'renewal' ? '服务续费' : entry.type === 'manual_credit' ? '管理员充值' : entry.type === 'manual_debit' ? '管理员扣减' : '余额变动'}</b><p>{entry.note}</p></div><strong className={entry.amountFen >= 0 ? 'wallet-credit' : 'wallet-debit'}>{entry.amountFen >= 0 ? '+' : ''}{(entry.amountFen / 100).toFixed(2)} 代币</strong><small>{new Date(entry.createdAt).toLocaleString('zh-CN')} · 变动后余额 {(entry.balanceAfterFen / 100).toFixed(2)} 代币{entry.actorId ? ` · 操作人 ${entry.actorId}` : ''}</small></article>)}</div>}</section></div>}
    {page === 'audit' && <section className="super-section"><div className="super-section-heading"><div><p className="eyebrow">安全审计</p><h2>最近平台操作</h2></div><span>{auditLogs.data?.length ?? 0} 条记录</span></div><div className="admin-table-wrap"><table><thead><tr><th>时间</th><th>操作者</th><th>操作</th><th>对象</th></tr></thead><tbody>{(auditLogs.data ?? []).map(item=><tr key={item.id}><td>{new Date(item.createdAt).toLocaleString('zh-CN')}</td><td>{item.actorId}</td><td>{item.action}</td><td>{item.targetType} · {item.targetId.slice(0,14)}</td></tr>)}</tbody></table></div></section>}
  </section>
}
