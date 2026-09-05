import { useMemo, useState } from 'react'
import {
  Alert,
  Button,
  EmptyState,
  LoadingState,
  PageHeader,
  StatusBadge
} from '@/components/ui'
import { useGetInstanceTasksQuery } from '@/services/cloudApi'
import type { Instance } from '@/types/cloud'

function taskTone(status: string) {
  if (status === 'succeeded') return 'success' as const
  if (status === 'failed' || status === 'needs_review') return 'danger' as const
  if (status === 'pending' || status === 'running') return 'progress' as const
  return 'neutral' as const
}

function taskLabel(status: string) {
  return status === 'succeeded'
    ? '已完成'
    : status === 'needs_review'
      ? '待人工复核'
      : status === 'pending'
        ? '等待执行'
        : status === 'running'
          ? '执行中'
          : status === 'failed'
            ? '失败'
            : status
}

export function InstanceExecutionPage({
  instanceID,
  instance,
  onBack
}: {
  instanceID: string
  instance?: Instance
  onBack: () => void
}) {
  const [onlyProblems, setOnlyProblems] = useState(false)
  const tasks = useGetInstanceTasksQuery(instanceID, {
    pollingInterval: 5000,
    refetchOnFocus: true
  })
  const records = useMemo(() => tasks.data ?? [], [tasks.data])
  const visible = records.filter(
    ({ task }) =>
      !onlyProblems || ['failed', 'needs_review'].includes(task.status)
  )
  const riskCount = records.filter(({ task }) =>
    ['failed', 'needs_review'].includes(task.status)
  ).length

  return (
    <section className="page me-page">
      <PageHeader
        title="实例执行记录"
        description={
          instance
            ? `${instance.name} · 自动规则、用户操作和失败恢复均在此留痕。`
            : `实例 ${instanceID}`
        }
        actions={
          <div className="flex gap-2">
            <Button tone="secondary" onClick={onBack}>
              返回实例
            </Button>
            <Button
              tone="secondary"
              loading={tasks.isFetching}
              onClick={() => void tasks.refetch()}
            >
              ↻ 刷新
            </Button>
          </div>
        }
      />
      {riskCount > 0 && (
        <Alert tone="error">
          发现 {riskCount}{' '}
          条需要关注的执行记录。失败或“待人工复核”不会被静默忽略。
        </Alert>
      )}
      <div className="mb-4 flex rounded-xl border border-slate-200 bg-white p-3 shadow-sm dark:border-slate-700 dark:bg-slate-800">
        <button
          className={`rounded-md px-3 py-1.5 text-sm ${onlyProblems ? 'bg-rose-600 text-white' : 'bg-slate-100 text-slate-700 dark:bg-slate-700 dark:text-slate-100'}`}
          onClick={() => setOnlyProblems(value => !value)}
        >
          {onlyProblems ? '仅显示异常' : '显示全部记录'}
          {riskCount ? ` · ${riskCount} 条异常` : ''}
        </button>
      </div>
      {tasks.isLoading ? (
        <LoadingState>正在读取执行记录…</LoadingState>
      ) : tasks.isError ? (
        <Alert tone="error">执行记录加载失败，请稍后刷新。</Alert>
      ) : visible.length === 0 ? (
        <EmptyState
          title="暂无异常记录"
          description="当前没有失败或待复核的任务。"
        />
      ) : (
        <div className="space-y-3">
          {visible.map(({ task, events }) => (
            <article
              key={task.id}
              className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm dark:border-slate-700 dark:bg-slate-800"
            >
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <div className="flex items-center gap-2">
                    <h2 className="m-0 text-base font-bold">{task.action}</h2>
                    <StatusBadge tone={taskTone(task.status)}>
                      {taskLabel(task.status)}
                    </StatusBadge>
                  </div>
                  <p className="mb-0 mt-1 text-xs text-slate-500 dark:text-slate-300">
                    任务 {task.id.slice(0, 14)} · 创建于{' '}
                    {new Date(task.createdAt).toLocaleString('zh-CN')} · 已尝试{' '}
                    {task.attempts} 次
                  </p>
                </div>
                {task.recoveryCount ? (
                  <span className="rounded-full bg-amber-50 px-2 py-1 text-xs text-amber-800 dark:bg-amber-950 dark:text-amber-100">
                    租约恢复 {task.recoveryCount} 次
                  </span>
                ) : null}
              </div>
              {task.lastError && (
                <div className="mt-3 rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-800 dark:border-rose-900 dark:bg-rose-950 dark:text-rose-100">
                  {task.lastError}
                </div>
              )}
              <ol className="mb-0 mt-4 space-y-3 border-l-2 border-slate-200 pl-4 dark:border-slate-600">
                {events.map(event => (
                  <li key={event.id} className="relative">
                    <span className="absolute -left-[21px] top-1.5 h-2.5 w-2.5 rounded-full bg-blue-500" />
                    <p className="mb-0 text-sm font-medium">{event.event}</p>
                    <p className="mb-0 text-sm text-slate-600 dark:text-slate-200">
                      {event.detail || '无补充说明'}
                    </p>
                    <time className="text-xs text-slate-500">
                      {new Date(event.createdAt).toLocaleString('zh-CN')}
                    </time>
                  </li>
                ))}
              </ol>
            </article>
          ))}
        </div>
      )}
      <p className="mt-3 text-xs text-slate-500 dark:text-slate-300">
        页面每 5
        秒刷新一次。容器运行输出请在“日志”页查看；这里记录控制面与生命周期任务的决策和结果。
      </p>
    </section>
  )
}
