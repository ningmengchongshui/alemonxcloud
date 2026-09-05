import { useState } from 'react'
import { useDispatch } from 'react-redux'
import { ActionDialog } from '@/components/ActionDialog'
import {
  Alert,
  Button,
  EmptyState,
  LoadingState,
  PageHeader,
  StatusBadge
} from '@/components/ui'
import { useInstanceActionMutation } from '@/services/cloudApi'
import { trackConsoleEvent } from '@/services/telemetry'
import { watchTask } from '@/store/uiSlice'
import type { Instance } from '@/types/cloud'

type InstanceAction =
  | 'start'
  | 'stop'
  | 'restart'
  | 'destroy'
  | 'destroy-now'
  | 'cancel-destroy'
  | 'archive'

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
  if (['failed', 'error'].includes(value))
    return { label: '需要处理', tone: 'danger' as const }
  return { label: status || '状态同步中', tone: 'neutral' as const }
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
        : action === 'archive'
          ? [
              '从列表移除',
              '这只会隐藏实例记录，不会调用节点或删除数据。',
              '从列表移除'
            ]
          : action === 'restart'
            ? ['重启实例', '服务会短暂不可访问，确定继续吗？', '确认重启']
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
  loading,
  onCreate,
  onOpenLogs
}: {
  instances: Instance[]
  loading: boolean
  onCreate: () => void
  onOpenLogs: (instanceID: string) => void
}) {
  const [error, setError] = useState('')
  const [pending, setPending] = useState<{
    id: string
    action: InstanceAction
  } | null>(null)
  const [operate, { isLoading: operating }] = useInstanceActionMutation()
  const dispatch = useDispatch()
  const sorted = [...instances].sort(
    (left, right) =>
      new Date(right.createdAt).getTime() - new Date(left.createdAt).getTime()
  )

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
            const state = stateFor(item.status)
            const lifecycle = item.status.toLowerCase()
            const runtime = item.runtimeStatus?.toLowerCase() || lifecycle
            const canOperate = ![
              'deploying',
              'creating',
              'pending',
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
                      <h2 className="m-0 text-base font-bold text-slate-900 dark:text-white">
                        {item.name}
                      </h2>
                      <StatusBadge tone={state.tone}>{state.label}</StatusBadge>
                    </div>
                    <p
                      className="mb-0 mt-1.5 truncate text-xs text-slate-500 dark:text-slate-300"
                      title={`${item.image}:${item.version}`}
                    >
                      {item.image} · {item.version}
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
                <div className="mt-4 flex items-center justify-between gap-4 border-t border-slate-100 px-5 py-3.5 dark:border-slate-700 max-[760px]:items-start max-[760px]:flex-col">
                  <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-xs">
                    {item.ip ? (
                      <a
                        className="font-bold text-blue-700 hover:underline dark:text-blue-200"
                        href={item.ip}
                        target="_blank"
                        rel="noreferrer"
                      >
                        打开服务 ↗
                      </a>
                    ) : (
                      <span className="text-slate-400">访问地址准备中</span>
                    )}
                    <span className="text-slate-500 dark:text-slate-300">
                      创建于 {new Date(item.createdAt).toLocaleString('zh-CN')}
                    </span>
                  </div>
                  <div className="flex flex-wrap items-center gap-2 max-[760px]:w-full">
                    {lifecycle !== 'destroyed' && lifecycle !== 'purged' && (
                      <Button
                        tone="secondary"
                        onClick={() => onOpenLogs(item.id)}
                      >
                        查看日志
                      </Button>
                    )}
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
                      </>
                    )}
                    {(lifecycle === 'running' || lifecycle === 'stopped') && (
                      <Button
                        tone="danger"
                        disabled={!canOperate || operating}
                        onClick={() =>
                          setPending({ id: item.id, action: 'destroy' })
                        }
                      >
                        计划销毁
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
                        从列表移除
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
    </section>
  )
}
