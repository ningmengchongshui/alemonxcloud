import { useState } from 'react'
import type { Order, RefundQuote } from '@/types/cloud'
import {
  useLazyGetRefundQuoteQuery,
  useRefundOrderMutation,
  useRenewOrderMutation
  ,useQuoteRenewalMutation
} from '@/services/cloudApi'
import { ActionDialog } from '@/components/ActionDialog'
import {
  Button,
  Dialog,
  EmptyState,
  FilterTabs,
  LoadingState,
  PageHeader,
  StatusBadge
} from '@/components/ui'
import { trackConsoleEvent } from '@/services/telemetry'

const orderStates: Record<
  string,
  { label: string; tone: string; hint: string }
> = {
  deploying: {
    label: '部署中',
    tone: 'progress',
    hint: '钱包已扣款，系统正在为你准备运行环境。'
  },
  active: {
    label: '已生效',
    tone: 'success',
    hint: '服务正在运行，可在实例列表中访问。'
  },
  expired: {
    label: '已到期',
    tone: 'danger',
    hint: '服务已到期，可用钱包续费并自动恢复。'
  },
  refunded: {
    label: '已退款',
    tone: 'neutral',
    hint: '退款已退回钱包；未退款的服务期仍按调整后的日期继续。'
  },
  cancelled: { label: '已取消', tone: 'neutral', hint: '此历史订单已取消。' },
  rejected: { label: '未通过', tone: 'danger', hint: '此历史订单未完成。' },
  pending_payment: {
    label: '历史待付款',
    tone: 'pending',
    hint: '人工付款订单流程已停用，请重新使用钱包购买。'
  },
  pending_review: {
    label: '历史待处理',
    tone: 'pending',
    hint: '人工付款订单流程已停用，请重新使用钱包购买。'
  }
}

const date = (value?: string) =>
  value
    ? new Intl.DateTimeFormat('zh-CN', {
        year: 'numeric',
        month: 'short',
        day: 'numeric'
      }).format(new Date(value))
    : '—'
const dateTime = (value?: string) =>
  value
    ? new Intl.DateTimeFormat('zh-CN', {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit'
      }).format(new Date(value))
    : '—'
const orderStages = ['钱包扣款', '资源部署', '服务生效']
const stageForStatus = (status: string) =>
  status === 'deploying' ? 1 : status === 'active' ? 2 : 0

export function OrdersPage({
  orders,
  loading,
  onCreate
}: {
  orders: Order[]
  loading: boolean
  onCreate: () => void
}) {
  const [filter, setFilter] = useState<
    'all' | 'processing' | 'active' | 'closed'
  >('all')
  const [renewing, setRenewing] = useState<Order | null>(null)
  const [refunding, setRefunding] = useState<Order | null>(null)
  const [refundQuote, setRefundQuote] = useState<RefundQuote | null>(null)
  const [months, setMonths] = useState('1')
  const [renewCoupon, setRenewCoupon] = useState('')
  const [renewSelection, setRenewSelection] = useState('')
  const [renewFullPrice, setRenewFullPrice] = useState(false)
  const [renewQuote, setRenewQuote] = useState<import('@/types/cloud').PriceQuote | null>(null)
  const [renewOrder, { isLoading: renewalLoading }] = useRenewOrderMutation()
  const [quoteRenewal, { isLoading: renewalQuoting }] = useQuoteRenewalMutation()
  const refreshRenewQuote = (order: Order, code = renewCoupon, selection = renewSelection, full = renewFullPrice) => void quoteRenewal({ id: order.id, months: Number(months) || 1, couponCode: code || undefined, selectionId: selection || undefined, payFullPrice: full }).unwrap().then(value => { setRenewQuote(value); setRenewSelection(value.selectedId ?? ''); setRenewFullPrice(Boolean(value.payFullPrice)) }).catch(error => setRenewalError(typeof error?.data?.message === 'string' ? error.data.message : '优惠试算失败'))
  const [loadRefundQuote, { isFetching: quoteLoading }] =
    useLazyGetRefundQuoteQuery()
  const [submitRefund, { isLoading: refundLoading }] = useRefundOrderMutation()
  const [renewalError, setRenewalError] = useState('')
  const [refundError, setRefundError] = useState('')
  const processing = orders.filter(order => order.status === 'deploying').length
  const visibleOrders = orders.filter(order => {
    if (filter === 'processing') return order.status === 'deploying'
    if (filter === 'active')
      return order.status === 'active' || order.status === 'refunded'
    if (filter === 'closed')
      return [
        'expired',
        'cancelled',
        'rejected',
        'pending_payment',
        'pending_review'
      ].includes(order.status)
    return true
  })

  return (
    <section className="page me-page orders-page">
      <PageHeader
        title="服务订阅"
        description="在这里查看部署进度、到期时间，并处理续费。"
        actions={
          <Button onClick={onCreate}>
            <span aria-hidden="true">＋</span> 创建服务
          </Button>
        }
      />
      {!loading && orders.length > 0 && (
        <div className="mb-4 flex items-center justify-between gap-4 border-y border-slate-200 py-3 dark:border-slate-700 max-[700px]:flex-col max-[700px]:items-start">
          <p className="m-0 text-xs text-slate-500 dark:text-slate-300">
            共 {visibleOrders.length} 笔
            {processing > 0 ? `，${processing} 笔正在部署` : ''}
          </p>
          <FilterTabs
            value={filter}
            label="订单筛选"
            onChange={next => {
              setFilter(next)
              trackConsoleEvent('order_filter', 'me', 'orders', {
                action: next,
                result: 'success'
              })
            }}
            items={[
              { value: 'all', label: '全部' },
              { value: 'processing', label: '部署中' },
              { value: 'active', label: '已生效' },
              { value: 'closed', label: '历史订单' }
            ]}
          />
        </div>
      )}
      {loading ? (
        <LoadingState>正在加载订单…</LoadingState>
      ) : orders.length === 0 ? (
        <EmptyState
          title="暂无订单"
          description="选择镜像和套餐后，系统会从钱包扣款并自动部署。"
          action={
            <Button onClick={onCreate}>
              <span aria-hidden="true">＋</span> 创建服务
            </Button>
          }
        />
      ) : visibleOrders.length === 0 ? (
        <EmptyState
          title="暂无匹配订单"
          description="请切换筛选条件，查看其他订单状态。"
        />
      ) : (
        <section className="orders-list" aria-label="订单列表">
          {visibleOrders.map(order => {
            const state = orderStates[order.status] ?? {
              label: order.status,
              tone: 'neutral',
              hint: '订单状态正在同步。'
            }
            return (
              <article className="order-card" key={order.id}>
                <div className="order-card-top">
                  <div>
                    <StatusBadge
                      tone={
                        state.tone === 'success'
                          ? 'success'
                          : state.tone === 'danger'
                            ? 'danger'
                            : state.tone === 'progress'
                              ? 'progress'
                              : state.tone === 'pending'
                                ? 'pending'
                                : 'neutral'
                      }
                    >
                      {state.label}
                    </StatusBadge>
                    <h2>
                      {order.imageName} <small>· {order.planName}</small>
                    </h2>
                    <p>
                      订单号 {order.id.slice(0, 14)} · 创建于{' '}
                      {date(order.createdAt)}
                    </p>
                  </div>
                  <strong className="order-price">
                    ¥{(order.amountFen / 100).toFixed(2)}
                  </strong>
                </div>
                <div
                  className="order-progress"
                  aria-label={`订单进度：${state.label}`}
                >
                  {orderStages.map((stage, index) => (
                    <div
                      key={stage}
                      className={
                        index <= stageForStatus(order.status) ? 'done' : ''
                      }
                    >
                      <i>
                        {index < stageForStatus(order.status) ? '✓' : index + 1}
                      </i>
                      <span>{stage}</span>
                    </div>
                  ))}
                </div>
                <div className="order-card-bottom">
                  <p>{state.hint}</p>
                  <dl>
                    <div>
                      <dt>镜像版本</dt>
                      <dd>{order.imageVersion || '—'}</dd>
                    </div>
                    <div>
                      <dt>服务开始</dt>
                      <dd>{date(order.serviceStartsAt)}</dd>
                    </div>
                    <div>
                      <dt>服务到期</dt>
                      <dd>{date(order.expiresAt)}</dd>
                    </div>
                    {order.refundAmountFen ? (
                      <div>
                        <dt>退款金额</dt>
                        <dd>
                          {(order.refundAmountFen / 100).toFixed(2)} XCoin
                        </dd>
                      </div>
                    ) : null}
                  </dl>
                  <div className="flex flex-wrap gap-2">
                    {(order.status === 'active' ||
                      order.status === 'expired' ||
                      order.status === 'refunded') && (
                      <Button
                        tone={
                          order.status === 'expired' ? 'primary' : 'secondary'
                        }
                        onClick={() => {
                          setRenewalError('')
                          setMonths('1')
                          setRenewCoupon('')
                          setRenewSelection('')
                          setRenewFullPrice(false)
                          setRenewQuote(null)
                          setRenewing(order)
                          void quoteRenewal({ id: order.id, months: 1 }).unwrap().then(setRenewQuote)
                        }}
                      >
                        钱包续费
                      </Button>
                    )}
                    {order.status === 'active' && order.serviceStartsAt && (
                      <Button
                        tone="secondary"
                        onClick={() => {
                          setRefundError('')
                          setRefundQuote(null)
                          setRefunding(order)
                          void loadRefundQuote(order.id)
                            .unwrap()
                            .then(quote => {
                              setRefundQuote(quote)
                              if (!quote.eligible)
                                setRefundError(
                                  quote.reason ?? '该订单暂不可退款'
                                )
                            })
                            .catch(error => {
                              setRefundError(
                                typeof error?.data?.message === 'string'
                                  ? error.data.message
                                  : '暂时无法获取退款试算，请稍后重试'
                              )
                            })
                        }}
                      >
                        申请退款
                      </Button>
                    )}
                  </div>
                </div>
              </article>
            )
          })}
        </section>
      )}
      {renewing && (
        <ActionDialog
          title="钱包续费"
          description={`将从钱包扣除 ${renewing.planName} 对应的续费金额；余额不足时不会续费。`}
          confirmLabel="确认续费"
          inputLabel="续费月数（1–24）"
          inputValue={months}
          inputPlaceholder="例如 1"
          onInputChange={setMonths}
          secondaryInputLabel="代金券码（可选）"
          secondaryInputValue={renewCoupon}
          secondaryInputPlaceholder="输入可用于续费的券码"
          onSecondaryInputChange={setRenewCoupon}
          busy={renewalLoading}
          onCancel={() => setRenewing(null)}
          onConfirm={() => {
            const value = Number(months)
            if (!Number.isInteger(value) || value < 1 || value > 24) {
              setRenewalError('请输入 1 至 24 的整数月数')
              return
            }
            const started = performance.now()
            trackConsoleEvent('renew_order', 'me', 'orders', {
              result: 'started'
            })
            void renewOrder({
              id: renewing.id,
              months: value,
              couponCode: renewCoupon || undefined
              ,selectionId: renewSelection || undefined, payFullPrice: renewFullPrice
            })
              .unwrap()
              .then(() => {
                trackConsoleEvent('renew_order', 'me', 'orders', {
                  result: 'success',
                  durationMs: performance.now() - started
                })
                setRenewing(null)
              })
              .catch(error => {
                trackConsoleEvent('renew_order', 'me', 'orders', {
                  result: 'error',
                  durationMs: performance.now() - started
                })
                setRenewalError(
                  typeof error?.data?.message === 'string'
                    ? error.data.message
                    : '续费失败，请稍后重试'
                )
              })
          }}
        >
          <div className="mt-3 grid gap-2 rounded-lg border border-slate-200 p-3 text-xs dark:border-slate-700">
            <Button type="button" tone="secondary" loading={renewalQuoting} onClick={() => refreshRenewQuote(renewing)}>试算续费优惠</Button>
            {renewQuote && <><p className="m-0">原价 {(renewQuote.listAmountFen / 100).toFixed(2)} XCoin，实付 {(renewQuote.amountFen / 100).toFixed(2)} XCoin</p>{renewQuote.candidates.length > 0 && <select value={renewFullPrice ? '__full__' : renewSelection} onChange={event => { const full = event.target.value === '__full__'; setRenewFullPrice(full); setRenewSelection(full ? '' : event.target.value); refreshRenewQuote(renewing, renewCoupon, full ? '' : event.target.value, full) }}><option value="__full__">不使用优惠，按原价购买</option>{renewQuote.candidates.map(item => <option key={item.id} value={item.id}>{item.name} · 减 {(item.discountAmountFen / 100).toFixed(2)} XCoin</option>)}</select>}</>}
          </div>
        </ActionDialog>
      )}
      {renewalError && (
        <p className="login-error" role="alert">
          {renewalError}
        </p>
      )}
      {refunding && (
        <Dialog
          eyebrow="订单退款"
          title="确认退款"
          description="退款将退回 XCoin 钱包，不会立即停止当前实例。"
          onClose={() => setRefunding(null)}
        >
          {quoteLoading ? (
            <LoadingState>正在计算可退款金额…</LoadingState>
          ) : refundError ? (
            <div className="space-y-4">
              <p className="login-error" role="alert">
                {refundError}
              </p>
              <div className="flex justify-end gap-2">
                <Button tone="secondary" onClick={() => setRefunding(null)}>
                  关闭
                </Button>
                <Button
                  onClick={() => {
                    setRefundError('')
                    void loadRefundQuote(refunding.id)
                      .unwrap()
                      .then(quote => {
                        setRefundQuote(quote)
                        if (!quote.eligible)
                          setRefundError(quote.reason ?? '该订单暂不可退款')
                      })
                      .catch(error =>
                        setRefundError(
                          typeof error?.data?.message === 'string'
                            ? error.data.message
                            : '暂时无法获取退款试算，请稍后重试'
                        )
                      )
                  }}
                >
                  重新试算
                </Button>
              </div>
            </div>
          ) : refundQuote ? (
            <div className="space-y-4">
              <dl className="grid gap-3 rounded-xl border border-slate-200 p-4 text-sm dark:border-slate-700 sm:grid-cols-2">
                <div>
                  <dt className="text-slate-500 dark:text-slate-300">
                    退回钱包
                  </dt>
                  <dd className="mt-1 font-semibold text-emerald-700 dark:text-emerald-300">
                    {((refundQuote.refundAmountFen ?? 0) / 100).toFixed(2)}{' '}
                    XCoin
                  </dd>
                </div>
                <div>
                  <dt className="text-slate-500 dark:text-slate-300">
                    预扣服务期
                  </dt>
                  <dd className="mt-1 font-semibold">
                    {refundQuote.prepaidDays} 天
                  </dd>
                </div>
                <div>
                  <dt className="text-slate-500 dark:text-slate-300">
                    服务可用至
                  </dt>
                  <dd className="mt-1 font-semibold">
                    {dateTime(refundQuote.serviceEndsAt)}
                  </dd>
                </div>
                <div>
                  <dt className="text-slate-500 dark:text-slate-300">
                    预计清理数据
                  </dt>
                  <dd className="mt-1 font-semibold">
                    {dateTime(refundQuote.dataPurgeAt)}
                  </dd>
                </div>
              </dl>
              <p className="text-xs leading-5 text-slate-500 dark:text-slate-300">
                本订单将扣减 {refundQuote.refundableDays} 个完整 24
                小时服务期；后续续费订单会同步前移。实例停止后数据保留 30 天。
              </p>
              <div className="flex justify-end gap-2">
                <Button tone="secondary" onClick={() => setRefunding(null)}>
                  取消
                </Button>
                <Button
                  tone="danger"
                  loading={refundLoading}
                  onClick={() => {
                    void submitRefund(refunding.id)
                      .unwrap()
                      .then(() => setRefunding(null))
                      .catch(error =>
                        setRefundError(
                          typeof error?.data?.message === 'string'
                            ? error.data.message
                            : '退款失败，请稍后重试'
                        )
                      )
                  }}
                >
                  确认退款
                </Button>
              </div>
            </div>
          ) : null}
        </Dialog>
      )}
    </section>
  )
}
