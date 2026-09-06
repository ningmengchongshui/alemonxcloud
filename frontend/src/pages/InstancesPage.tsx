import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
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
  useGetCatalogQuery,
  useGetWalletQuery,
  useQuotePlanChangeMutation,
  useSubmitPlanChangeMutation,
  useQuoteRenewalMutation,
  useRenewOrderMutation
} from '@/services/cloudApi'
import { trackConsoleEvent } from '@/services/telemetry'
import { watchTask } from '@/store/uiSlice'
import type {
  Instance,
  Order,
  Plan,
  PlanChangeQuote,
  PriceQuote
} from '@/types/cloud'

const subscriptionMonths = [1, 3, 6, 12]
const renewDiscountLabel = (plan: Plan | undefined, months: number) => {
  const bps = plan?.tierDiscounts?.[months]
  return months > 1 && bps !== undefined && bps < 10000
    ? `${bps / 1000} 折`
    : ''
}

type InstanceAction =
  | 'start'
  | 'stop'
  | 'restart'
  | 'reinstall'
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

function taskLabel(action: string) {
  return (
    {
      'create': '创建中',
      'retry-deploy': '部署中',
      'start': '启动中',
      'stop': '关机中',
      'restart': '重启中',
      'update': '更新中',
      'resize': '套餐变更中',
      'reinstall': '重装中',
      'destroy': '销毁中',
      'purge': '清理中'
    }[action] ?? '处理中'
  )
}

type MoreAction = {
  action: InstanceAction
  label: string
  danger?: boolean
}

function MoreActionsMenu({
  instanceID,
  actions,
  onSelect
}: {
  instanceID: string
  actions: MoreAction[]
  onSelect: (action: InstanceAction) => void
}) {
  const [open, setOpen] = useState(false)
  const root = useRef<HTMLDivElement>(null)
  const menu = useRef<HTMLDivElement>(null)
  const [position, setPosition] = useState({ top: 0, left: 0 })
  useEffect(() => {
    if (!open) return
    const updatePosition = () => {
      const bounds = root.current
        ?.querySelector('button')
        ?.getBoundingClientRect()
      if (!bounds) return
      setPosition({
        top: bounds.bottom + 6,
        left: Math.max(8, bounds.right - 112)
      })
    }
    const closeOnOutsidePointer = (event: PointerEvent) => {
      const target = event.target as Node
      if (!root.current?.contains(target) && !menu.current?.contains(target)) {
        setOpen(false)
      }
    }
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false)
    }
    updatePosition()
    document.addEventListener('pointerdown', closeOnOutsidePointer)
    document.addEventListener('keydown', closeOnEscape)
    window.addEventListener('resize', updatePosition)
    window.addEventListener('scroll', updatePosition, true)
    return () => {
      document.removeEventListener('pointerdown', closeOnOutsidePointer)
      document.removeEventListener('keydown', closeOnEscape)
      window.removeEventListener('resize', updatePosition)
      window.removeEventListener('scroll', updatePosition, true)
    }
  }, [open])
  if (actions.length === 0) return null
  return (
    <div ref={root} className="relative">
      <Button
        tone="secondary"
        aria-expanded={open}
        aria-controls={`instance-actions-${instanceID}`}
        onClick={() => setOpen(value => !value)}
      >
        更多
      </Button>
      {open &&
        createPortal(
          <div
            ref={menu}
            id={`instance-actions-${instanceID}`}
            role="menu"
            style={{ top: position.top, left: position.left }}
            className="fixed z-[100] min-w-28 overflow-hidden rounded-md border border-slate-200 bg-white py-1 shadow-lg dark:border-slate-600 dark:bg-slate-800"
          >
            {actions.map(item => (
              <button
                key={item.action}
                type="button"
                role="menuitem"
                className={`block w-full px-3 py-2 text-left text-xs font-bold hover:bg-slate-50 dark:hover:bg-slate-700 ${item.danger ? 'text-red-700 dark:text-red-200' : 'text-slate-700 dark:text-slate-100'}`}
                onClick={() => {
                  setOpen(false)
                  onSelect(item.action)
                }}
              >
                {item.label}
              </button>
            ))}
          </div>,
          document.body
        )}
    </div>
  )
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
                : action === 'reinstall'
                  ? [
                      '重装实例',
                      '将永久删除此实例的数据目录和工作区，并按当前镜像重新部署。该操作不可恢复，服务会暂时不可访问。',
                      '确认重装并清空数据'
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
  onOpenTerminal,
  onOpenExecutions
}: {
  instances: Instance[]
  orders: Order[]
  loading: boolean
  onCreate: () => void
  onOpenLogs: (instanceID: string) => void
  onOpenTerminal: (instanceID: string) => void
  onOpenExecutions: (instanceID: string) => void
}) {
  const [error, setError] = useState('')
  const [pending, setPending] = useState<{
    id: string
    action: InstanceAction
  } | null>(null)
  const [submittedTasks, setSubmittedTasks] = useState<
    Record<string, NonNullable<Instance['activeTask']>>
  >({})
  const [renewing, setRenewing] = useState<Order | null>(null)
  const [resizing, setResizing] = useState<Instance | null>(null)
  const [resizePlanID, setResizePlanID] = useState('')
  const [resizeQuote, setResizeQuote] = useState<PlanChangeQuote | null>(null)
  const [resizeError, setResizeError] = useState('')
  const [months, setMonths] = useState('1')
  const [renewPromoCode, setRenewPromoCode] = useState('')
  const [renewQuote, setRenewQuote] = useState<PriceQuote | null>(null)
  const [renewalError, setRenewalError] = useState('')
  const [operate, { isLoading: operating }] = useInstanceActionMutation()
  const { data: catalog } = useGetCatalogQuery()
  const [renewOrder, { isLoading: renewalLoading }] = useRenewOrderMutation()
  const [quoteRenewal] = useQuoteRenewalMutation()
  const [quotePlanChange] = useQuotePlanChangeMutation()
  const [submitPlanChange, { isLoading: resizeLoading }] =
    useSubmitPlanChangeMutation()
  const { data: wallet } = useGetWalletQuery()
  const dispatch = useDispatch()
  useEffect(() => {
    setSubmittedTasks(current => {
      const next = { ...current }
      for (const item of instances) {
        if (next[item.id]) delete next[item.id]
      }
      return next
    })
  }, [instances])
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

  function refreshRenewQuote(
    order: Order,
    promoCode = renewPromoCode,
    quoteMonths = Number(months) || 1
  ) {
    setRenewalError('')
    void quoteRenewal({
      id: order.id,
      months: quoteMonths,
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

  function openResize(item: Instance) {
    setResizing(item)
    setResizeQuote(null)
    setResizeError('')
    const current =
      item.currentPlanId || renewableOrderByInstance.get(item.id)?.planId
    const first = catalog?.plans.find(plan => plan.id !== current)?.id || ''
    setResizePlanID(first)
    if (first)
      void quotePlanChange({ id: item.id, targetPlanId: first })
        .unwrap()
        .then(setResizeQuote)
        .catch((error: { data?: { message?: string } }) =>
          setResizeError(error.data?.message || '套餐报价失败')
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
          const task = response.task
          setSubmittedTasks(current => ({
            ...current,
            [value.id]: {
              id: task.id,
              action: task.action,
              status: task.status
            }
          }))
          dispatch(watchTask({ id: task.id, action: task.action }))
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
            const activeTask = item.activeTask ?? submittedTasks[item.id]
            const planChangeBlocked = ['processing', 'needs_review'].includes(
              item.planChangeStatus || ''
            )
            const state = activeTask
              ? {
                  label: taskLabel(activeTask.action),
                  tone: 'progress' as const
                }
              : runtime === 'missing'
                ? { label: '资源异常', tone: 'danger' as const }
                : stateFor(item.status)
            const canOperate =
              !activeTask &&
              !planChangeBlocked &&
              ![
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
                {item.planChangeStatus === 'needs_review' && (
                  <div className="mx-5 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-900 dark:bg-amber-950 dark:text-amber-100">
                    套餐变更正在核实运行资源，暂不重复操作；资金状态将在核实后更新。
                  </div>
                )}
                {item.planChangeStatus === 'failed' && (
                  <div className="mx-5 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-200">
                    上次套餐变更失败，原套餐和原资源配置已保留。
                  </div>
                )}
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
                    {!item.terminalOnly && (
                      <Button
                        tone="secondary"
                        disabled={!item.ip || runtime !== 'running' || Boolean(activeTask)}
                        onClick={() => window.open(item.ip, '_blank', 'noopener,noreferrer')}
                      >
                        {item.ip ? 'Web服务 ↗' : '服务准备中'}
                      </Button>
                    )}
                    <Button
                      tone="secondary"
                      disabled={runtime !== 'running' || Boolean(activeTask)}
                      onClick={() => onOpenTerminal(item.id)}
                    >
                      {runtime === 'running' ? '终端' : '终端准备中'}
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
                    {runtime === 'stopped' &&
                      lifecycle === 'stopped' &&
                      canOperate && (
                        <Button
                          tone="secondary"
                          disabled={!canOperate}
                          onClick={() =>
                            setPending({ id: item.id, action: 'start' })
                          }
                        >
                          启动
                        </Button>
                      )}
                    {runtime === 'running' &&
                      lifecycle === 'running' &&
                      canOperate && (
                        <>
                          <Button
                            tone="secondary"
                            onClick={() =>
                              setPending({ id: item.id, action: 'stop' })
                            }
                          >
                            关机
                          </Button>
                        </>
                      )}
                    {lifecycle === 'running' &&
                      runtime === 'running' &&
                      canOperate && (
                        <MoreActionsMenu
                          instanceID={item.id}
                          actions={[
                            { action: 'restart', label: '重启' },
                            { action: 'update', label: '更新' },
                            {
                              action: 'reinstall',
                              label: '重装',
                              danger: true
                            },
                            { action: 'destroy', label: '销毁', danger: true }
                          ]}
                          onSelect={action =>
                            setPending({ id: item.id, action })
                          }
                        />
                      )}
                    {['running', 'stopped'].includes(lifecycle) &&
                      canOperate && (
                        <Button
                          tone="secondary"
                          onClick={() => openResize(item)}
                        >
                          变更套餐
                        </Button>
                      )}
                    {lifecycle === 'stopped' &&
                      runtime === 'stopped' &&
                      canOperate && (
                        <MoreActionsMenu
                          instanceID={item.id}
                          actions={[
                            {
                              action: 'reinstall',
                              label: '重装',
                              danger: true
                            },
                            { action: 'destroy', label: '销毁', danger: true }
                          ]}
                          onSelect={action =>
                            setPending({ id: item.id, action })
                          }
                        />
                      )}
                    {renewalOrder &&
                      ['running', 'stopped'].includes(lifecycle) &&
                      item.destroyReason !== 'refund' && (
                        <Button
                          tone={
                            renewalOrder.status === 'expired'
                              ? 'primary'
                              : 'secondary'
                          }
                          disabled={Boolean(activeTask)}
                          onClick={() => openRenewal(renewalOrder)}
                        >
                          续费
                        </Button>
                      )}
                    {lifecycle === 'destroy_scheduled' && !activeTask && (
                      <>
                        {item.destroyReason === 'manual' && (
                          <Button
                            tone="secondary"
                            disabled={Boolean(activeTask)}
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
                          disabled={Boolean(activeTask)}
                          onClick={() =>
                            setPending({ id: item.id, action: 'destroy-now' })
                          }
                        >
                          立即销毁
                        </Button>
                      </>
                    )}
                    {lifecycle === 'destroyed' && !activeTask && (
                      <Button
                        tone="secondary"
                        disabled={Boolean(activeTask)}
                        onClick={() =>
                          setPending({ id: item.id, action: 'archive' })
                        }
                      >
                        移除
                      </Button>
                    )}
                    {lifecycle === 'deployment_failed' && !activeTask && (
                      <Button
                        tone="secondary"
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
          danger={['destroy', 'destroy-now', 'reinstall'].includes(
            pending.action
          )}
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
          <fieldset className="grid gap-2 text-sm font-medium">
            <legend>续费周期</legend>
            <div className="flex flex-wrap gap-2">
              {subscriptionMonths.map(value => (
                <Button
                  key={value}
                  tone={Number(months) === value ? 'primary' : 'secondary'}
                  onClick={() => {
                    setMonths(String(value))
                    refreshRenewQuote(renewing, renewPromoCode, value)
                  }}
                >
                  <span>{value} 个月</span>
                  {renewDiscountLabel(
                    catalog?.plans.find(plan => plan.id === renewing.planId),
                    value
                  ) && (
                    <small className="ml-1 text-red-600">
                      {renewDiscountLabel(
                        catalog?.plans.find(
                          plan => plan.id === renewing.planId
                        ),
                        value
                      )}
                    </small>
                  )}
                </Button>
              ))}
            </div>
          </fieldset>
          <div className="mt-4 border-t border-slate-100 pt-4 dark:border-slate-700">
            <div className="flex items-center justify-between gap-3">
              <span className="text-[11px] font-semibold text-slate-500 dark:text-slate-300">
                有推广码？
              </span>
              <span className="text-[11px] text-slate-400">可选填写</span>
            </div>
            <div className="mt-2 flex gap-2">
              <input
                className="h-10 min-w-0 flex-1 rounded-lg border border-slate-200 bg-slate-50 px-3 text-xs text-slate-700 shadow-none outline-none placeholder:text-slate-400 focus:border-blue-300 focus-visible:outline-1 focus-visible:outline-offset-1 focus-visible:outline-blue-100 dark:border-slate-600 dark:bg-slate-900 dark:text-slate-100"
                value={renewPromoCode}
                onChange={event => setRenewPromoCode(event.target.value)}
                placeholder="输入推广码"
              />
              <Button
                className="h-10 px-4"
                tone="secondary"
                onClick={() => refreshRenewQuote(renewing, renewPromoCode)}
              >
                应用
              </Button>
            </div>
            <div className="mt-4 space-y-2 text-xs">
              <div className="flex justify-between gap-3 text-slate-500 dark:text-slate-300">
                <span>
                  套餐价格
                  {renewQuote?.tierMonths
                    ? `（${renewQuote.tierMonths} 个月）`
                    : ''}
                </span>
                <b>
                  {renewQuote
                    ? `¥${(renewQuote.listAmountFen / 100).toFixed(2)}`
                    : '—'}
                </b>
              </div>
              {renewQuote?.program && (
                <div className="flex justify-between gap-3 text-emerald-700">
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
                if (!subscriptionMonths.includes(value)) {
                  setRenewalError('请选择 1、3、6 或 12 个月')
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
      {resizing && (
        <Dialog
          onClose={() => setResizing(null)}
          eyebrow="实例套餐"
          title={`变更套餐 · ${imageName(resizing.image)}`}
        >
          <label className="grid gap-2 text-xs font-bold text-slate-600 dark:text-slate-200">
            目标套餐
            <select
              className="h-11 rounded-lg border border-slate-200 px-3 text-sm"
              value={resizePlanID}
              onChange={event => {
                const value = event.target.value
                setResizePlanID(value)
                setResizeQuote(null)
                setResizeError('')
                void quotePlanChange({ id: resizing.id, targetPlanId: value })
                  .unwrap()
                  .then(setResizeQuote)
                  .catch((error: { data?: { message?: string } }) =>
                    setResizeError(error.data?.message || '套餐报价失败')
                  )
              }}
            >
              <option value="">请选择套餐</option>
              {(catalog?.plans || []).map(plan => (
                <option key={plan.id} value={plan.id}>
                  {plan.name} · {plan.cpu} 核 / {plan.memoryMB / 1024} GB
                </option>
              ))}
            </select>
          </label>
          {resizeQuote && (
            <div className="mt-4 rounded-lg bg-slate-50 p-3 text-xs dark:bg-slate-900">
              <div className="flex justify-between">
                <span>当前套餐</span>
                <b>{resizeQuote.currentPlanName}</b>
              </div>
              <div className="mt-2 flex justify-between">
                <span>变更后</span>
                <b>
                  {resizeQuote.targetPlanName} · {resizeQuote.targetCpu} 核 /{' '}
                  {resizeQuote.targetMemoryMB / 1024} GB
                </b>
              </div>
              <div className="mt-2 flex justify-between">
                <span>
                  {resizeQuote.chargeFen
                    ? '需补差价'
                    : resizeQuote.refundFen
                      ? '退回钱包'
                      : '本次应付'}
                </span>
                <b className={resizeQuote.refundFen ? 'text-emerald-600' : ''}>
                  ¥
                  {(
                    (resizeQuote.chargeFen || resizeQuote.refundFen) / 100
                  ).toFixed(2)}
                </b>
              </div>
              <p className="mb-0 mt-2 text-slate-500">{resizeQuote.summary}</p>
            </div>
          )}
          {resizeError && <Alert tone="error">{resizeError}</Alert>}
          <div className="mt-5 flex justify-end gap-2">
            <Button tone="secondary" onClick={() => setResizing(null)}>
              取消
            </Button>
            <Button
              loading={resizeLoading}
              disabled={!resizeQuote}
              onClick={() => {
                if (!resizeQuote) return
                void submitPlanChange({
                  id: resizing.id,
                  targetPlanId: resizeQuote.targetPlanId,
                  currentPlanId: resizeQuote.currentPlanId,
                  quoteExpiresAt: resizeQuote.expiresAt
                })
                  .unwrap()
                  .then(response => {
                    setSubmittedTasks(current => ({
                      ...current,
                      [resizing.id]: {
                        id: response.task.id,
                        action: response.task.action,
                        status: response.task.status
                      }
                    }))
                    dispatch(
                      watchTask({
                        id: response.task.id,
                        action: response.task.action
                      })
                    )
                    setResizing(null)
                  })
                  .catch((error: { data?: { message?: string } }) =>
                    setResizeError(error.data?.message || '套餐变更失败')
                  )
              }}
            >
              确认变更
            </Button>
          </div>
        </Dialog>
      )}
    </section>
  )
}
