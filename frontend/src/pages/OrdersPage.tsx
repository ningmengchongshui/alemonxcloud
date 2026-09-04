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

export function OrdersPage({ orders, loading, onCreate }: { orders: Order[]; loading: boolean; onCreate: () => void }) {
  const [cancel, { isLoading: cancelling }] = useCancelOrderMutation()
  const [renew, { isLoading: renewing }] = useRenewOrderMutation()
  const [submitPayment, { isLoading: submittingPayment }] = useSubmitPaymentMutation()
  const [error, setError] = useState('')
  const pending = orders.filter(order => ['pending_payment', 'pending_review', 'deploying'].includes(order.status)).length

  async function cancelOrder(order: Order) {
    if (!window.confirm('确定取消此待支付订单吗？')) return
    setError('')
    try { await cancel(order.id).unwrap() } catch { setError('取消订单失败，请稍后重试') }
  }

  async function renewOrder(order: Order) {
    setError('')
    try { await renew({ id: order.id, months: 1 }).unwrap() } catch { setError('创建续费订单失败，请稍后重试') }
  }
  async function providePayment(order: Order) { const reference=window.prompt('请输入付款流水号或转账参考号'); if(!reference)return;setError('');try{await submitPayment({id:order.id,reference}).unwrap()}catch{setError('付款信息提交失败，请检查流水号后重试')} }

  return <section className="page me-page orders-page">
    <header className="page-heading">
      <div><p className="eyebrow">订单中心</p><h1>管理每一项服务订阅</h1><p>从付款确认到服务生效，所有状态和后续操作都集中在这里。</p></div>
      <button className="primary create-action" onClick={onCreate}><span>＋</span> 创建服务</button>
    </header>
    <section className="orders-overview"><article><small>全部订单</small><strong>{loading ? '—' : orders.length}</strong><span>包含当前与历史订单</span></article><article><small>处理中</small><strong>{loading ? '—' : pending}</strong><span>付款确认或部署进行中</span></article><article className="orders-overview-note"><b>订单会在确认付款后自动开始部署</b><span>无需重复提交，状态变更会在这里同步。</span></article></section>
    {error && <p className="form-error" role="alert">{error}</p>}
    {loading ? <div className="orders-panel loading-panel"><span className="loading-dot" />正在加载订单…</div> : orders.length === 0 ? <div className="orders-panel empty-instance"><div className="empty-icon">□</div><h2>还没有服务订单</h2><p>选择镜像与套餐后创建第一笔订单，我们会在确认付款后自动开始部署。</p><button className="primary" onClick={onCreate}>创建服务</button></div> : <section className="orders-list" aria-label="订单列表">{orders.map(order => {
      const state = orderStates[order.status] ?? { label: order.status, tone: 'neutral', hint: '订单状态正在同步。' }
      const canCancel = ['pending_payment', 'pending_review'].includes(order.status)
      const canSubmitPayment = order.status === 'pending_payment'
      const canRenew = ['active', 'expired'].includes(order.status)
      return <article className="order-card" key={order.id}><div className="order-card-top"><div><span className={`status-badge ${state.tone}`}><i />{state.label}</span><h2>{order.imageName} <small>· {order.planName}</small></h2><p>订单号 {order.id.slice(0, 14)} · 创建于 {date(order.createdAt)}</p></div><strong className="order-price">¥{(order.amountFen / 100).toFixed(2)}</strong></div><div className="order-card-bottom"><p>{state.hint}</p><dl><div><dt>镜像版本</dt><dd>{order.imageVersion || '—'}</dd></div><div><dt>服务到期</dt><dd>{date(order.expiresAt)}</dd></div></dl><div className="order-actions">{canSubmitPayment && <button className="primary compact-primary" disabled={submittingPayment} onClick={() => void providePayment(order)}>{submittingPayment ? '正在提交…' : '提交付款流水号'}</button>}{canCancel && <button className="subtle-button danger-action" disabled={cancelling} onClick={() => void cancelOrder(order)}>{cancelling ? '正在取消…' : '取消订单'}</button>}{canRenew && <button className="primary compact-primary" disabled={renewing} onClick={() => void renewOrder(order)}>{renewing ? '正在创建…' : '续费 1 个月'}</button>}</div></div></article>
    })}</section>}
  </section>
}
