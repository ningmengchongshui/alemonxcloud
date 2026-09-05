import { useState } from 'react'
import type { Order, RefundQuote } from '@/types/cloud'
import {
  useLazyGetRefundQuoteQuery,
  useRefundOrderMutation
} from '@/services/cloudApi'
import {
  Button,
  Dialog,
  EmptyState,
  FilterTabs,
  InlineAction,
  LoadingState,
  PageHeader,
  StatusBadge
} from '@/components/ui'
import { XCoinAmount } from '@/components/XCoinMark'
import { trackConsoleEvent } from '@/services/telemetry'

const orderStates: Record<string, { label: string; tone: string }> = {
  deploying: {
    label: '部署中',
    tone: 'progress'
  },
  active: {
    label: '已生效',
    tone: 'success'
  },
  expired: {
    label: '已到期',
    tone: 'danger'
  },
  refunded: {
    label: '已退款',
    tone: 'neutral'
  },
  cancelled: { label: '已取消', tone: 'neutral' },
  rejected: { label: '未通过', tone: 'danger' },
  pending_payment: {
    label: '历史待付款',
    tone: 'pending'
  },
  pending_review: {
    label: '历史待处理',
    tone: 'pending'
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
  const [refunding, setRefunding] = useState<Order | null>(null)
  const [refundQuote, setRefundQuote] = useState<RefundQuote | null>(null)
  const [loadRefundQuote, { isFetching: quoteLoading }] =
    useLazyGetRefundQuoteQuery()
  const [submitRefund, { isLoading: refundLoading }] = useRefundOrderMutation()
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
    <section className="page me-page">
      <PageHeader
        title="服务订阅"
        description="在这里查看部署进度和订阅记录；续费请前往对应实例。"
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
        <section
          className="overflow-hidden rounded-xl border border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-800"
          aria-label="订单列表"
        >
          <div className="overflow-x-auto">
            <table className="min-w-[760px] w-full text-left text-xs">
              <thead className="bg-slate-50 text-[10px] font-bold text-slate-500 dark:bg-slate-900 dark:text-slate-300">
                <tr>
                  <th className="px-5 py-3">服务与订单</th>
                  <th className="px-4 py-3">状态</th>
                  <th className="px-4 py-3">金额</th>
                  <th className="px-4 py-3">服务期</th>
                  <th className="px-5 py-3 text-right">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-700">
                {visibleOrders.map(order => {
                  const state = orderStates[order.status] ?? {
                    label: order.status,
                    tone: 'neutral'
                  }
                  const badgeTone =
                    state.tone === 'success'
                      ? 'success'
                      : state.tone === 'danger'
                        ? 'danger'
                        : state.tone === 'progress'
                          ? 'progress'
                          : state.tone === 'pending'
                            ? 'pending'
                            : 'neutral'
                  return (
                    <tr
                      key={order.id}
                      className="align-middle hover:bg-slate-50/70 dark:hover:bg-slate-900/50"
                    >
                      <td className="px-5 py-3.5">
                        <b className="block text-sm text-slate-800 dark:text-white">
                          {order.imageName}
                          <span className="ml-1.5 text-xs font-normal text-slate-500 dark:text-slate-300">
                            · {order.planName}
                          </span>
                        </b>
                        <span className="mt-1 block text-[11px] text-slate-400">
                          {order.imageVersion || '—'} · {order.id.slice(0, 14)}
                        </span>
                      </td>
                      <td className="px-4 py-3.5">
                        <StatusBadge tone={badgeTone}>
                          {state.label}
                        </StatusBadge>
                      </td>
                      <td className="px-4 py-3.5 font-bold text-slate-700 dark:text-slate-100">
                        <XCoinAmount
                          value={(order.amountFen / 100).toFixed(2)}
                        />
                        {order.refundAmountFen ? (
                          <span className="mt-1 block text-[10px] font-normal text-slate-400">
                            已退回 {(order.refundAmountFen / 100).toFixed(2)}
                          </span>
                        ) : null}
                      </td>
                      <td className="px-4 py-3.5 text-[11px] leading-5 text-slate-500 dark:text-slate-300">
                        <span className="block">
                          起：{date(order.serviceStartsAt)}
                        </span>
                        <span className="block">
                          止：{date(order.expiresAt)}
                        </span>
                      </td>
                      <td className="px-5 py-3.5 text-right">
                        {order.status === 'active' && order.serviceStartsAt ? (
                          <InlineAction
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
                          </InlineAction>
                        ) : (
                          <span className="text-[11px] text-slate-400">—</span>
                        )}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </section>
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
                小时服务期；后续续费订单会同步前移。服务结束后将销毁容器资源，数据再保留
                30 天。
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
