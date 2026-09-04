import { useState } from 'react'
import type { Order } from '@/types/cloud'
import { useRenewOrderMutation } from '@/services/cloudApi'
import { ActionDialog } from '@/components/ActionDialog'

const orderStates: Record<string, { label: string; tone: string; hint: string }> = {
  deploying: { label: '部署中', tone: 'progress', hint: '钱包已扣款，系统正在为你准备运行环境。' },
  active: { label: '已生效', tone: 'success', hint: '服务正在运行，可在实例列表中访问。' },
  expired: { label: '已到期', tone: 'danger', hint: '服务已到期，可用钱包续费并自动恢复。' },
  cancelled: { label: '已取消', tone: 'neutral', hint: '此历史订单已取消。' },
  rejected: { label: '未通过', tone: 'danger', hint: '此历史订单未完成。' },
  pending_payment: { label: '历史待付款', tone: 'pending', hint: '人工付款订单流程已停用，请重新使用钱包购买。' },
  pending_review: { label: '历史待处理', tone: 'pending', hint: '人工付款订单流程已停用，请重新使用钱包购买。' }
}

const date = (value?: string) => value ? new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: 'short', day: 'numeric' }).format(new Date(value)) : '—'
const orderStages = ['钱包扣款', '资源部署', '服务生效']
const stageForStatus = (status: string) => status === 'deploying' ? 1 : status === 'active' ? 2 : 0

export function OrdersPage({ orders, loading, onCreate }: { orders: Order[]; loading: boolean; onCreate: () => void }) {
  const [filter, setFilter] = useState<'all' | 'processing' | 'active' | 'closed'>('all')
  const [renewing, setRenewing] = useState<Order | null>(null)
  const [months, setMonths] = useState('1')
  const [renewOrder, { isLoading: renewalLoading }] = useRenewOrderMutation()
  const [renewalError, setRenewalError] = useState('')
  const processing = orders.filter(order => order.status === 'deploying').length
  const visibleOrders = orders.filter(order => {
    if (filter === 'processing') return order.status === 'deploying'
    if (filter === 'active') return order.status === 'active'
    if (filter === 'closed') return ['expired', 'cancelled', 'rejected', 'pending_payment', 'pending_review'].includes(order.status)
    return true
  })

  return <section className="page me-page orders-page">
    <header className="page-heading"><div><p className="eyebrow">订单中心</p><h1>订单记录</h1><p>所有订单均由钱包扣款后自动校验资源并部署。</p></div><button className="primary create-action" onClick={onCreate}><span>＋</span> 创建服务</button></header>
    <section className="orders-overview"><article><small>全部订单</small><strong>{loading ? '—' : orders.length}</strong><span>当前及历史订单</span></article><article><small>部署中</small><strong>{loading ? '—' : processing}</strong><span>等待资源准备完成</span></article><article className="orders-overview-note"><b>钱包购买自动部署</b><span>资源不足时不会扣款或创建订单。</span></article></section>
    {!loading && orders.length > 0 && <div className="order-toolbar"><div><h2>订单记录</h2><span>共 {visibleOrders.length} 笔</span></div><div className="order-filters" role="tablist" aria-label="订单筛选"><button className={filter === 'all' ? 'active' : ''} onClick={() => setFilter('all')} role="tab" aria-selected={filter === 'all'}>全部</button><button className={filter === 'processing' ? 'active' : ''} onClick={() => setFilter('processing')} role="tab" aria-selected={filter === 'processing'}>部署中</button><button className={filter === 'active' ? 'active' : ''} onClick={() => setFilter('active')} role="tab" aria-selected={filter === 'active'}>已生效</button><button className={filter === 'closed' ? 'active' : ''} onClick={() => setFilter('closed')} role="tab" aria-selected={filter === 'closed'}>历史订单</button></div></div>}
    {loading ? <div className="orders-panel loading-panel"><span className="loading-dot" />加载订单中…</div> : orders.length === 0 ? <div className="orders-panel empty-instance"><div className="empty-icon">□</div><h2>暂无订单</h2><p>选择镜像和套餐后，系统会从钱包扣款并自动部署。</p><button className="primary" onClick={onCreate}>创建服务</button></div> : visibleOrders.length === 0 ? <div className="orders-panel empty-instance compact-empty"><h2>暂无匹配订单</h2><p>请切换筛选条件。</p></div> : <section className="orders-list" aria-label="订单列表">{visibleOrders.map(order => {
      const state = orderStates[order.status] ?? { label: order.status, tone: 'neutral', hint: '订单状态正在同步。' }
      return <article className="order-card" key={order.id}><div className="order-card-top"><div><span className={`status-badge ${state.tone}`}><i />{state.label}</span><h2>{order.imageName} <small>· {order.planName}</small></h2><p>订单号 {order.id.slice(0, 14)} · 创建于 {date(order.createdAt)}</p></div><strong className="order-price">¥{(order.amountFen / 100).toFixed(2)}</strong></div><div className="order-progress" aria-label={`订单进度：${state.label}`}>{orderStages.map((stage, index) => <div key={stage} className={index <= stageForStatus(order.status) ? 'done' : ''}><i>{index < stageForStatus(order.status) ? '✓' : index + 1}</i><span>{stage}</span></div>)}</div><div className="order-card-bottom"><p>{state.hint}</p><dl><div><dt>镜像版本</dt><dd>{order.imageVersion || '—'}</dd></div><div><dt>服务到期</dt><dd>{date(order.expiresAt)}</dd></div></dl>{(order.status === 'active' || order.status === 'expired') && <button className="text-button" onClick={() => { setRenewalError(''); setMonths('1'); setRenewing(order) }}>钱包续费</button>}</div></article>
    })}</section>}
    {renewing && <ActionDialog title="钱包续费" description={`将从钱包扣除 ${renewing.planName} 对应的续费金额；余额不足时不会续费。`} confirmLabel="确认续费" inputLabel="续费月数（1–24）" inputValue={months} inputPlaceholder="例如 1" onInputChange={setMonths} busy={renewalLoading} onCancel={() => setRenewing(null)} onConfirm={() => { const value = Number(months); if (!Number.isInteger(value) || value < 1 || value > 24) { setRenewalError('请输入 1 至 24 的整数月数'); return } void renewOrder({ id: renewing.id, months: value }).unwrap().then(() => setRenewing(null)).catch(error => setRenewalError(typeof error?.data?.message === 'string' ? error.data.message : '续费失败，请稍后重试')) }} />}
    {renewalError && <p className="login-error" role="alert">{renewalError}</p>}
  </section>
}
