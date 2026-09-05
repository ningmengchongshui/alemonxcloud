import { useState } from 'react'
import { useDispatch } from 'react-redux'
import { ActionDialog } from '@/components/ActionDialog'
import { BalanceSettlement } from '@/components/BalanceSettlement'
import {
  Alert,
  Button,
  Dialog,
  EmptyState,
  LoadingState,
  PageHeader,
  StatusBadge
} from '@/components/ui'
import {
  useInstanceActionMutation,
  useGetWalletQuery,
  useQuoteRenewalMutation,
  useRenewOrderMutation
} from '@/services/cloudApi'
import { trackConsoleEvent } from '@/services/telemetry'
import { watchTask } from '@/store/uiSlice'
import type { Instance, Order, PriceQuote } from '@/types/cloud'

type InstanceAction =
  | 'start'
  | 'stop'
  | 'restart'
  | 'destroy'
  | 'destroy-now'
  | 'cancel-destroy'
  | 'archive'
  | 'retry-deploy'
  | 'update'

function stateFor(status: string) {
  const value = status.toLowerCase()
  if (['running', 'active', 'online'].includes(value))
    return { label: '运行中', tone: 'success' as const }
  if (['creating', 'deploying', 'pending'].includes(value))
    return { label: '部署中', tone: 'progress' as const }
  if (['stopped'].includes(value))
    return { label: '已关机', tone: 'neutral' as const }
  if (value === 'destroy_scheduled')
    return { label: '待销毁', tone: 'pending' as const }
  if (value === 'destroyed')
    return { label: '已销毁', tone: 'neutral' as const }
  if (['failed', 'error', 'deployment_failed'].includes(value))
    return { label: '需要处理', tone: 'danger' as const }
  return { label: status || '状态同步中', tone: 'neutral' as const }
}

function imageName(image: string) {
  const parts = image.split('@')[0].split('/').filter(Boolean)
  return parts[parts.length - 1] || image
}

function actionCopy(action: InstanceAction) {
  return action === 'destroy'
    ? [
        '计划销毁实例',
        '服务会继续保持当前状态，7 天后将销毁容器资源；数据将在资源销毁后保留 30 天。',
        '确认计划销毁'
      ]
    : action === 'destroy-now'
      ? [
          '立即销毁容器',
          '将立即销毁容器和 Compose 运行资源；数据仍保留 30 天。',
          '立即销毁'
        ]
      : action === 'cancel-destroy'
        ? ['取消销毁计划', '实例会继续保持当前运行状态。', '取消销毁计划']
        : action === 'retry-deploy'
          ? ['重试部署', '将重新创建实例运行资源，确定继续吗？', '重试部署']
          : action === 'update'
            ? [
                '更新实例',
                '将短暂重建容器并保留数据目录，服务会暂时不可访问。',
                '确认更新'
              ]
            : action === 'archive'
              ? [
                  '从列表移除',
                  '这只会隐藏实例记录，不会调用节点或删除数据。',
                  '从列表移除'
                ]
              : action === 'restart'
                ? [
                    '重启实例',
                    '将按当前实例配置重新生成运行容器并保留数据目录；不会更新镜像版本。服务会短暂不可访问，确定继续吗？',
                    '确认重启'
                  ]
                : action === 'stop'
                  ? [
                      '关闭实例',
                      '服务将停止运行，但数据和订阅保留。确定继续吗？',
                      '确认关机'
                    ]
                  : ['启动实例', '服务将恢复运行。确定继续吗？', '确认启动']
}

export function InstancesPage({
  instances,
  orders,
  loading,
  onCreate,
  onOpenLogs,
  onOpenExecutions
}: {
  instances: Instance[]
  orders: Order[]
  loading: boolean
  onCreate: () => void
  onOpenLogs: (instanceID: string) => void
  onOpenExecutions: (instanceID: string) => void
}) {
  const [error, setError] = useState('')
  const [pending, setPending] = useState<{
    id: string
    action: InstanceAction
  } | null>(null)
  const [renewing, setRenewing] = useState<Order | null>(null)
  const [months, setMonths] = useState('1')
  const [renewPromoCode, setRenewPromoCode] = useState('')
  const [renewQuote, setRenewQuote] = useState<PriceQuote | null>(null)
  const [renewalError, setRenewalError] = useState('')
  const [operate, { isLoading: operating }] = useInstanceActionMutation()
  const [renewOrder, { isLoading: renewalLoading }] = useRenewOrderMutation()
  const [quoteRenewal] = useQuoteRenewalMutation()
  const { data: wallet } = useGetWalletQuery()
  const dispatch = useDispatch()
  const sorted = [...instances].sort(
    (left, right) =>
      new Date(right.createdAt).getTime() - new Date(left.createdAt).getTime()
  )
  const renewPayableFen = renewQuote?.amountFen
  const canRenew =
    Boolean(wallet && renewPayableFen !== undefined) &&
    (wallet?.balanceFen ?? 0) >= (renewPayableFen ?? 0)
  const renewableOrderByInstance = new Map<string, Order>()
  for (const order of orders) {
    if (!order.instanceId || !['active', 'expired'].includes(order.status))
      continue
    const current = renewableOrderByInstance.get(order.instanceId)
    if (
      !current ||
      new Date(order.createdAt).getTime() >
        new Date(current.createdAt).getTime()
    )
      renewableOrderByInstance.set(order.instanceId, order)
  }

  function refreshRenewQuote(order: Order, promoCode = renewPromoCode) {
    setRenewalError('')
    void quoteRenewal({
      id: order.id,
      months: Number(months) || 1,
      promoCode: promoCode || undefined
    })
      .unwrap()
      .then(value => {
        setRenewQuote(value)
      })
      .catch(error =>
        setRenewalError(
          typeof error?.data?.message === 'string'
            ? error.data.message
            : '优惠试算失败'
        )
      )
  }

  function openRenewal(order: Order) {
    setMonths('1')
    setRenewPromoCode('')
    setRenewQuote(null)
    setRenewalError('')
    setRenewing(order)
    void quoteRenewal({ id: order.id, months: 1 })
      .unwrap()
      .then(value => {
        setRenewQuote(value)
      })
      .catch(error =>
        setRenewalError(
          typeof error?.data?.message === 'string'
            ? error.data.message
            : '优惠试算失败'
        )
      )
  }

  function confirmAction() {
    if (!pending) return
    const value = pending
    const started = performance.now()
    trackConsoleEvent('instance_action', 'me', 'instances', {
      action: value.action,
      result: 'started'
    })
    void operate(value)
      .unwrap()
      .then(response => {
        trackConsoleEvent('instance_action', 'me', 'instances', {
          action: value.action,
          result: 'success',
          durationMs: performance.now() - started
        })
        if (response.task) {
          dispatch(
            watchTask({ id: response.task.id, action: response.task.action })
          )
        }
        setPending(null)
      })
      .catch(() => {
        trackConsoleEvent('instance_action', 'me', 'instances', {
          action: value.action,
          result: 'error',
          durationMs: performance.now() - started
        })
        setError('实例操作未完成，请稍后重试。')
        setPending(null)
      })
  }

  return (
    <section className="page me-page">
      <PageHeader
        title="运行环境"
        description="管理服务运行状态、销毁计划和数据保留期。"
        actions={
          <Button onClick={onCreate}>
            <span aria-hidden="true">＋</span> 创建服务
          </Button>
        }
      />
      {error && <Alert tone="error">{error}</Alert>}
      {loading ? (
        <LoadingState>正在同步实例状态…</LoadingState>
      ) : sorted.length === 0 ? (
        <EmptyState
          title="还没有实例"
          description="从可信镜像和可售套餐创建第一个服务。"
          action={
            <Button onClick={onCreate}>
              <span aria-hidden="true">＋</span> 创建服务
            </Button>
          }
        />
      ) : (
        <div className="grid gap-4">
          {sorted.map(item => {
            const renewalOrder = renewableOrderByInstance.get(item.id)
            const lifecycle = item.status.toLowerCase()
            const runtime = item.runtimeStatus?.toLowerCase() || lifecycle
            const state =
              runtime === 'missing'
                ? { label: '资源异常', tone: 'danger' as const }
                : stateFor(item.status)
            const canOperate = ![
              'deploying',
              'creating',
              'pending',
              'deployment_failed',
              'destroyed',
              'purged'
            ].includes(lifecycle)
            const destroyDate = item.destroyAt
              ? new Date(item.destroyAt).toLocaleString('zh-CN')
              : '同步中'
            const purgeDate = item.purgeAt
              ? new Date(item.purgeAt).toLocaleString('zh-CN')
              : '同步中'
            const destroyLabel =
              item.destroyReason === 'refund'
                ? '退款后计划销毁'
                : item.destroyReason === 'expired'
                  ? '到期后计划销毁'
                  : '已计划销毁'
            return (
              <article
                key={item.id}
                className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm transition-shadow hover:shadow-md dark:border-slate-700 dark:bg-slate-800"
              >
                <div className="flex items-start justify-between gap-5 px-5 py-4 max-[640px]:flex-col max-[640px]:gap-3">
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2.5">
                      <h2
                        className="m-0 truncate text-base font-bold text-slate-900 dark:text-white"
                        title={`${imageName(item.image)} ｜ ${item.id}`}
                      >
                        {imageName(item.image)} ｜ {item.id.slice(0, 12)}
                      </h2>
                      <StatusBadge tone={state.tone}>{state.label}</StatusBadge>
                    </div>
                    <p
                      className="mb-0 mt-1.5 truncate text-xs text-slate-500 dark:text-slate-300"
                      title={`${item.image}:${item.version}`}
                    >
                      版本 {item.version}
                      {item.containerName
                        ? ` · 域址 ${item.containerName}`
                        : ''}
                    </p>
                  </div>
                  <div className="shrink-0 rounded-lg bg-slate-50 px-3 py-2 text-right dark:bg-slate-900 max-[640px]:text-left">
                    <span className="block text-[10px] font-bold text-slate-400">
                      资源规格
                    </span>
                    <b className="mt-0.5 block text-xs text-slate-700 dark:text-slate-100">
                      {item.spec}
                    </b>
                  </div>
                </div>
                {lifecycle === 'destroy_scheduled' && (
                  <div className="mx-5 flex items-center gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-900 dark:bg-amber-950 dark:text-amber-100 max-[640px]:items-start">
                    <span className="mt-0.5 font-bold" aria-hidden="true">
                      !
                    </span>
                    <span>
                      <b>{destroyLabel}</b>，容器预计于 {destroyDate} 销毁。
                    </span>
                  </div>
                )}
                {lifecycle === 'destroyed' && (
                  <div className="mx-5 flex items-center gap-2 rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-600 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200">
                    <span aria-hidden="true">○</span>
                    <span>容器已销毁，数据预计于 {purgeDate} 物理清除。</span>
                  </div>
                )}
                <div className="flex items-center justify-between gap-4 border-t border-slate-100 px-5 py-3.5 dark:border-slate-700 max-[760px]:items-start max-[760px]:flex-col">
                  <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-xs">
                    <span className="text-slate-500 dark:text-slate-300">
                      创建于 {new Date(item.createdAt).toLocaleString('zh-CN')}
                    </span>
                  </div>
                  <div className="flex flex-wrap items-center justify-end gap-2 max-[760px]:w-full">
                    <Button
                      tone="secondary"
                      disabled={!item.ip}
                      onClick={() =>
                        window.open(item.ip, '_blank', 'noopener,noreferrer')
                      }
                    >
                      {item.ip ? '打开服务 ↗' : '服务准备中'}
                    </Button>
                    {lifecycle !== 'destroyed' && lifecycle !== 'purged' && (
                      <Button
                        tone="secondary"
                        onClick={() => onOpenLogs(item.id)}
                      >
                        日志
                      </Button>
                    )}
                    <Button
                      tone="secondary"
                      onClick={() => onOpenExecutions(item.id)}
                    >
                      执行记录
                    </Button>
                    {runtime === 'stopped' && canOperate && (
                      <Button
                        tone="secondary"
                        disabled={!canOperate || operating}
                        onClick={() =>
                          setPending({ id: item.id, action: 'start' })
                        }
                      >
                        启动
                      </Button>
                    )}
                    {runtime === 'running' && canOperate && (
                      <>
                        <Button
                          tone="secondary"
                          disabled={!canOperate || operating}
                          onClick={() =>
                            setPending({ id: item.id, action: 'restart' })
                          }
                        >
                          重启
                        </Button>
                        <Button
                          tone="secondary"
                          disabled={!canOperate || operating}
                          onClick={() =>
                            setPending({ id: item.id, action: 'stop' })
                          }
                        >
                          关机
                        </Button>
                        <Button
                          tone="secondary"
                          disabled={operating}
                          onClick={() =>
                            setPending({ id: item.id, action: 'update' })
                          }
                        >
                          更新
                        </Button>
                      </>
                    )}
                    {renewalOrder &&
                      lifecycle !== 'destroyed' &&
                      lifecycle !== 'purged' &&
                      item.destroyReason !== 'refund' && (
                        <Button
                          tone={
                            renewalOrder.status === 'expired'
                              ? 'primary'
                              : 'secondary'
                          }
                          disabled={operating}
                          onClick={() => openRenewal(renewalOrder)}
                        >
                          续费
                        </Button>
                      )}
                    {(lifecycle === 'running' || lifecycle === 'stopped') && (
                      <Button
                        tone="danger"
                        disabled={!canOperate || operating}
                        onClick={() =>
                          setPending({ id: item.id, action: 'destroy' })
                        }
                      >
                        销毁
                      </Button>
                    )}
                    {lifecycle === 'destroy_scheduled' && (
                      <>
                        {item.destroyReason === 'manual' && (
                          <Button
                            tone="secondary"
                            disabled={operating}
                            onClick={() =>
                              setPending({
                                id: item.id,
                                action: 'cancel-destroy'
                              })
                            }
                          >
                            取消销毁
                          </Button>
                        )}
                        <Button
                          tone="danger"
                          disabled={operating}
                          onClick={() =>
                            setPending({ id: item.id, action: 'destroy-now' })
                          }
                        >
                          立即销毁
                        </Button>
                      </>
                    )}
                    {lifecycle === 'destroyed' && (
                      <Button
                        tone="secondary"
                        disabled={operating}
                        onClick={() =>
                          setPending({ id: item.id, action: 'archive' })
                        }
                      >
                        移除
                      </Button>
                    )}
                    {lifecycle === 'deployment_failed' && (
                      <Button
                        tone="secondary"
                        disabled={operating}
                        onClick={() =>
                          setPending({ id: item.id, action: 'retry-deploy' })
                        }
                      >
                        重试部署
                      </Button>
                    )}
                  </div>
                </div>
              </article>
            )
          })}
        </div>
      )}
      {pending && (
        <ActionDialog
          title={actionCopy(pending.action)[0]}
          description={actionCopy(pending.action)[1]}
          confirmLabel={actionCopy(pending.action)[2]}
          danger={
            pending.action === 'destroy' || pending.action === 'destroy-now'
          }
          busy={operating}
          onCancel={() => setPending(null)}
          onConfirm={confirmAction}
        />
      )}
      {renewing && (
        <Dialog
          eyebrow="实例续费"
          title={`续费 ${renewing.imageName}`}
          description={`为当前实例续费 ${renewing.planName}；余额不足时不会续费。`}
          onClose={() => setRenewing(null)}
        >
          <label className="grid gap-1.5 text-sm font-medium">
            续费月数（1–24）
            <input
              type="number"
              min="1"
              max="24"
              value={months}
              onChange={event => setMonths(event.target.value)}
              onBlur={() => refreshRenewQuote(renewing)}
            />
          </label>
          <div className="mt-4">
            <div className="rounded-xl border border-slate-200 p-4 text-sm">
              <label className="grid gap-1.5 font-medium">
                推广码（可选）
                <div className="flex gap-2">
                  <input
                    value={renewPromoCode}
                    onChange={event => setRenewPromoCode(event.target.value)}
                    placeholder="有推广码再输入"
                  />
                  <Button
                    tone="secondary"
                    onClick={() => refreshRenewQuote(renewing, renewPromoCode)}
                  >
                    应用
                  </Button>
                </div>
              </label>
              <div className="mt-3 flex justify-between">
                <span>
                  套餐价格
                  {renewQuote?.tierMonths
                    ? `（${renewQuote.tierMonths} 个月阶梯价）`
                    : ''}
                </span>
                <b>
                  {renewQuote
                    ? `¥${(renewQuote.listAmountFen / 100).toFixed(2)}`
                    : '—'}
                </b>
              </div>
              {renewQuote?.program && (
                <div className="mt-2 flex justify-between text-emerald-700">
                  <span>
                    已自动应用：{renewQuote.program.name}
                    {renewQuote.bonusDays
                      ? ` · 赠送 ${renewQuote.bonusDays} 天`
                      : ''}
                  </span>
                  <b>
                    {renewQuote.discountAmountFen
                      ? `-¥${(renewQuote.discountAmountFen / 100).toFixed(2)}`
                      : '权益已生效'}
                  </b>
                </div>
              )}
            </div>
          </div>
          <div className="mt-4">
            <BalanceSettlement
              balanceFen={wallet?.balanceFen}
              payableFen={renewPayableFen}
            />
          </div>
          {renewalError && <Alert tone="error">{renewalError}</Alert>}
          <div className="mt-5 flex justify-end gap-2">
            <Button tone="secondary" onClick={() => setRenewing(null)}>
              取消
            </Button>
            <Button
              loading={renewalLoading}
              tone={canRenew ? 'primary' : 'secondary'}
              disabled={!canRenew}
              onClick={() => {
                const value = Number(months)
                if (!Number.isInteger(value) || value < 1 || value > 24) {
                  setRenewalError('请输入 1 至 24 的整数月数')
                  return
                }
                const started = performance.now()
                trackConsoleEvent('renew_order', 'me', 'instances', {
                  result: 'started'
                })
                void renewOrder({
                  id: renewing.id,
                  months: value,
                  promoCode: renewPromoCode || undefined
                })
                  .unwrap()
                  .then(() => {
                    trackConsoleEvent('renew_order', 'me', 'instances', {
                      result: 'success',
                      durationMs: performance.now() - started
                    })
                    setRenewing(null)
                  })
                  .catch(error => {
                    trackConsoleEvent('renew_order', 'me', 'instances', {
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
              {wallet && renewPayableFen !== undefined && !canRenew
                ? '余额不足，暂不能续费'
                : '确认续费'}
            </Button>
          </div>
        </Dialog>
      )}
    </section>
  )
}
