import { useState } from 'react'
import { useCancelOrderMutation, useRenewOrderMutation, useSubmitPaymentMutation } from '@/services/cloudApi'
import type { Order } from '@/types/cloud'

const orderStates: Record<string, { label: string; tone: string; hint: string }> = {
  pending_payment: { label: '待支付', tone: 'pending', hint: '请完成付款并填写付款备注，便于我们核验。' },
  pending_review: { label: '待确认', tone: 'pending', hint: '款项正在核验，确认后会自动开始部署。' },
  deploying: { label: '部署中', tone: 'progress', hint: '资源已预占，服务正在准备运行环境。' },
  active: { label: '已生效', tone: 'success', hint: '服务正在运行，可在实例列表中访问。' },
  expired: { label: '已到期', tone: 'danger', hint: '续费后可继续使用对应的服务。' },
  cancelled: { label: '已取消', tone: 'neutral', hint: '此订单已取消，不会继续处理。' },
  rejected: { label: '未通过', tone: 'danger', hint: '订单未能通过核验，请重新创建订单。' }
}

const date = (value?: string) => value ? new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: 'short', day: 'numeric' }).format(new Date(value)) : '—'
const orderStages = ['订单创建', '款项确认', '资源部署', '服务生效']
const stageForStatus = (status: string) => status === 'pending_payment' ? 0 : status === 'pending_review' ? 1 : status === 'deploying' ? 2 : status === 'active' ? 3 : 0

export function OrdersPage({ orders, loading, onCreate }: { orders: Order[]; loading: boolean; onCreate: () => void }) {
  const [cancel, { isLoading: cancelling }] = useCancelOrderMutation()
  const [renew, { isLoading: renewing }] = useRenewOrderMutation()
  const [submitPayment, { isLoading: submittingPayment }] = useSubmitPaymentMutation()
  const [error, setError] = useState('')
  const [paymentOrder, setPaymentOrder] = useState<Order | null>(null)
  const [paymentReference, setPaymentReference] = useState('')
  const [filter, setFilter] = useState<'all' | 'processing' | 'active' | 'closed'>('all')
  const pending = orders.filter(order => ['pending_payment', 'pending_review', 'deploying'].includes(order.status)).length
  const visibleOrders = orders.filter(order => {
    if (filter === 'processing') return ['pending_payment', 'pending_review', 'deploying'].includes(order.status)
    if (filter === 'active') return order.status === 'active'
    if (filter === 'closed') return ['expired', 'cancelled', 'rejected'].includes(order.status)
    return true
  })

  async function cancelOrder(order: Order) {
    if (!window.confirm('确定取消此待支付订单吗？')) return
    setError('')
    try { await cancel(order.id).unwrap() } catch { setError('取消订单失败，请稍后重试') }
  }

  async function renewOrder(order: Order) {
    setError('')
    try { await renew({ id: order.id, months: 1 }).unwrap() } catch { setError('创建续费订单失败，请稍后重试') }
  }
  async function providePayment() { if (!paymentOrder || !paymentReference.trim()) return; setError(''); try { await submitPayment({id:paymentOrder.id,reference:paymentReference.trim()}).unwrap(); setPaymentOrder(null); setPaymentReference('') } catch { setError('付款信息提交失败，请检查流水号后重试') } }

  return <section className="page me-page orders-page">
    <header className="page-heading">
      <div><p className="eyebrow">订单中心</p><h1>订单管理</h1><p>查看订单状态并处理后续操作。</p></div>
      <button className="primary create-action" onClick={onCreate}><span>＋</span> 创建服务</button>
    </header>
    <section className="orders-overview"><article><small>全部订单</small><strong>{loading ? '—' : orders.length}</strong><span>当前及历史订单</span></article><article><small>处理中</small><strong>{loading ? '—' : pending}</strong><span>待确认或部署中</span></article><article className="orders-overview-note"><b>付款确认后自动部署</b><span>状态会自动更新。</span></article></section>
    {error && <p className="form-error" role="alert">{error}</p>}
    {!loading && orders.length > 0 && <div className="order-toolbar"><div><h2>订单记录</h2><span>共 {visibleOrders.length} 笔</span></div><div className="order-filters" role="tablist" aria-label="订单筛选"><button className={filter === 'all' ? 'active' : ''} onClick={() => setFilter('all')} role="tab" aria-selected={filter === 'all'}>全部</button><button className={filter === 'processing' ? 'active' : ''} onClick={() => setFilter('processing')} role="tab" aria-selected={filter === 'processing'}>处理中</button><button className={filter === 'active' ? 'active' : ''} onClick={() => setFilter('active')} role="tab" aria-selected={filter === 'active'}>已生效</button><button className={filter === 'closed' ? 'active' : ''} onClick={() => setFilter('closed')} role="tab" aria-selected={filter === 'closed'}>已结束</button></div></div>}
    {loading ? <div className="orders-panel loading-panel"><span className="loading-dot" />加载订单中…</div> : orders.length === 0 ? <div className="orders-panel empty-instance"><div className="empty-icon">□</div><h2>暂无订单</h2><p>选择镜像和套餐，创建第一笔服务订单。</p><button className="primary" onClick={onCreate}>创建服务</button></div> : visibleOrders.length === 0 ? <div className="orders-panel empty-instance compact-empty"><h2>暂无匹配订单</h2><p>请切换筛选条件。</p></div> : <section className="orders-list" aria-label="订单列表">{visibleOrders.map(order => {
      const state = orderStates[order.status] ?? { label: order.status, tone: 'neutral', hint: '订单状态正在同步。' }
      const canCancel = ['pending_payment', 'pending_review'].includes(order.status)
      const canSubmitPayment = order.status === 'pending_payment'
      const canRenew = ['active', 'expired'].includes(order.status)
      return <article className="order-card" key={order.id}><div className="order-card-top"><div><span className={`status-badge ${state.tone}`}><i />{state.label}</span><h2>{order.imageName} <small>· {order.planName}</small></h2><p>订单号 {order.id.slice(0, 14)} · 创建于 {date(order.createdAt)}</p></div><strong className="order-price">¥{(order.amountFen / 100).toFixed(2)}</strong></div><div className="order-progress" aria-label={`订单进度：${state.label}`}>{orderStages.map((stage, index) => <div key={stage} className={index <= stageForStatus(order.status) ? 'done' : ''}><i>{index < stageForStatus(order.status) ? '✓' : index + 1}</i><span>{stage}</span></div>)}</div><div className="order-card-bottom"><p>{state.hint}</p><dl><div><dt>镜像版本</dt><dd>{order.imageVersion || '—'}</dd></div><div><dt>服务到期</dt><dd>{date(order.expiresAt)}</dd></div></dl><div className="order-actions">{canSubmitPayment && <button className="primary compact-primary" disabled={submittingPayment} onClick={() => { setPaymentOrder(order); setPaymentReference('') }}>{submittingPayment ? '正在提交…' : '提交付款流水号'}</button>}{canCancel && <button className="subtle-button danger-action" disabled={cancelling} onClick={() => void cancelOrder(order)}>{cancelling ? '正在取消…' : '取消订单'}</button>}{canRenew && <button className="primary compact-primary" disabled={renewing} onClick={() => void renewOrder(order)}>{renewing ? '正在创建…' : '续费 1 个月'}</button>}</div></div></article>
    })}</section>}
    {paymentOrder&&<div className="modal-backdrop" onMouseDown={event => { if (event.target === event.currentTarget) setPaymentOrder(null) }}><section className="modal-card compact-modal" role="dialog" aria-modal="true" aria-labelledby="payment-modal-title"><div className="modal-heading"><div><p className="eyebrow">订单付款</p><h2 id="payment-modal-title">提交付款流水号</h2><p>订单 {paymentOrder.id.slice(0, 14)} · ¥{(paymentOrder.amountFen / 100).toFixed(2)}</p></div><button className="modal-close" onClick={() => setPaymentOrder(null)} aria-label="关闭付款弹窗">×</button></div><label className="modal-field" htmlFor="payment-reference">付款流水号或转账参考号<input id="payment-reference" autoFocus value={paymentReference} onChange={event => setPaymentReference(event.target.value)} placeholder="请输入付款凭证中的参考号" /></label><div className="modal-actions"><button className="subtle-button" onClick={() => setPaymentOrder(null)}>取消</button><button className="primary" disabled={submittingPayment||!paymentReference.trim()} onClick={() => void providePayment()}>{submittingPayment?'正在提交…':'提交付款信息'}</button></div></section></div>}
  </section>
}
