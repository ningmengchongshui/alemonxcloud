import { useEffect, useState } from 'react'
import { useDispatch, useSelector } from 'react-redux'
import classNames from 'classnames'
import { cloudApi, useGetTaskQuery } from '@/services/cloudApi'
import type { RootState } from '@/store'
import { clearWatchedTask } from '@/store/uiSlice'
import type { Task } from '@/types/cloud'

function taskLabel(action: string) {
  return (
    {
      create: '创建服务',
      start: '启动',
      stop: '关机',
      restart: '重启',
      reinstall: '重装',
      update: '更新',
      delete: '删除',
      deploy: '部署'
    }[action] ?? '资源操作'
  )
}

export function TaskNotification() {
  const dispatch = useDispatch()
  const watchedTask = useSelector((state: RootState) => state.ui.watchedTask)
  const [result, setResult] = useState<Task | null>(null)
  const { data } = useGetTaskQuery(watchedTask?.id ?? '', {
    skip: !watchedTask,
    pollingInterval: watchedTask ? 2500 : 0
  })

  useEffect(() => {
    if (!data || (data.status !== 'succeeded' && data.status !== 'failed'))
      return
    setResult(data)
    // The initial mutation invalidates data when the task is queued. Refresh
    // again after the worker reaches a terminal state so the instance card
    // immediately reflects the Agent's result without a manual page reload.
    dispatch(
      cloudApi.util.invalidateTags(['Instances', 'Orders', 'Notifications'])
    )
    dispatch(clearWatchedTask())
  }, [data, dispatch])

  if (!result) return null
  const failed = result.status === 'failed'
  const action = taskLabel(result.action)
  return (
    <section
      className={classNames(
        'fixed right-4 top-4 z-60 w-[min(calc(100vw-2rem),23rem)] rounded-xl border p-4 shadow-xl backdrop-blur-sm max-sm:right-3 max-sm:top-3',
        failed
          ? 'border-red-200 bg-white/95 text-slate-800 dark:border-red-900 dark:bg-slate-800/95 dark:text-white'
          : 'border-emerald-200 bg-white/95 text-slate-800 dark:border-emerald-900 dark:bg-slate-800/95 dark:text-white'
      )}
      role="status"
      aria-live="polite"
    >
      <div className="flex items-start gap-3">
        <span
          className={classNames(
            'grid size-8 shrink-0 place-items-center rounded-full text-sm font-bold',
            failed
              ? 'bg-red-50 text-red-600 dark:bg-red-950 dark:text-red-200'
              : 'bg-emerald-50 text-emerald-600 dark:bg-emerald-950 dark:text-emerald-200'
          )}
          aria-hidden="true"
        >
          {failed ? '!' : '✓'}
        </span>
        <div className="min-w-0 flex-1">
          <h2 className="m-0 text-sm font-bold">
            {failed ? `${action}未完成` : `${action}已完成`}
          </h2>
          <p className="mb-0 mt-1 text-xs leading-5 text-slate-500 dark:text-slate-300">
            {failed
              ? result.lastError || '任务未能完成，请稍后重试。'
              : '服务状态已更新。'}
          </p>
        </div>
        <button
          type="button"
          className="grid size-7 shrink-0 place-items-center rounded-md text-slate-400 hover:bg-slate-100 hover:text-slate-600 focus-visible:outline-2 focus-visible:outline-blue-500 dark:hover:bg-slate-700 dark:hover:text-slate-100"
          onClick={() => setResult(null)}
          aria-label="关闭任务提醒"
        >
          ×
        </button>
      </div>
    </section>
  )
}
