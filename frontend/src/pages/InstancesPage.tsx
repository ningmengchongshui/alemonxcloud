import { useState } from 'react'
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
import type { Instance } from '@/types/cloud'

type InstanceAction = 'start' | 'stop' | 'restart' | 'delete'

function stateFor(status: string) {
  const value = status.toLowerCase()
  if (['running', 'active', 'online'].includes(value))
    return { label: '运行中', tone: 'success' as const }
  if (['creating', 'deploying', 'pending'].includes(value))
    return { label: '部署中', tone: 'progress' as const }
  if (['stopped'].includes(value))
    return { label: '已关机', tone: 'neutral' as const }
  if (['failed', 'error', 'expired', 'retention'].includes(value))
    return { label: '需要处理', tone: 'danger' as const }
  return { label: status || '状态同步中', tone: 'neutral' as const }
}

function actionCopy(action: InstanceAction) {
  return action === 'delete'
    ? ['删除实例', '服务会停止，数据保留 7 天。确定继续吗？', '确认删除']
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
      .then(() => {
        trackConsoleEvent('instance_action', 'me', 'instances', {
          action: value.action,
          result: 'success',
          durationMs: performance.now() - started
        })
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
        description="查看每个服务的运行状态，并执行启动、关机、重启、日志查看和删除操作。"
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
            const canOperate = ![
              'deploying',
              'creating',
              'pending',
              'retention'
            ].includes(item.status.toLowerCase())
            return (
              <article
                key={item.id}
                className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-800"
              >
                <div className="flex flex-wrap items-start justify-between gap-4">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <h2 className="m-0 text-base font-bold text-slate-900 dark:text-white">
                        {item.name}
                      </h2>
                      <StatusBadge tone={state.tone}>{state.label}</StatusBadge>
                    </div>
                    <p className="mt-2 break-all text-xs text-slate-500 dark:text-slate-300">
                      {item.image} · {item.version}
                    </p>
                  </div>
                  <div className="text-right text-xs">
                    <span className="block text-slate-400">资源规格</span>
                    <b className="mt-1 block text-slate-700 dark:text-slate-100">
                      {item.spec}
                    </b>
                  </div>
                </div>
                <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-slate-100 pt-4 dark:border-slate-700">
                  <div className="min-w-0 text-xs">
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
                    <span className="mx-2 text-slate-300">·</span>
                    <span className="text-slate-500 dark:text-slate-300">
                      创建于 {new Date(item.createdAt).toLocaleString('zh-CN')}
                    </span>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Button
                      tone="secondary"
                      onClick={() => onOpenLogs(item.id)}
                    >
                      查看日志
                    </Button>
                    {item.status.toLowerCase() === 'stopped' && (
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
                    {item.status.toLowerCase() === 'running' && (
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
                    <Button
                      tone="danger"
                      disabled={!canOperate || operating}
                      onClick={() =>
                        setPending({ id: item.id, action: 'delete' })
                      }
                    >
                      删除
                    </Button>
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
          danger={pending.action === 'delete'}
          busy={operating}
          onCancel={() => setPending(null)}
          onConfirm={confirmAction}
        />
      )}
    </section>
  )
}
