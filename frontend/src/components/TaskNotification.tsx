import { useEffect, useRef } from 'react'
import { useDispatch, useSelector } from 'react-redux'
import { cloudApi, useGetTaskQuery } from '@/services/cloudApi'
import { toast } from '@/services/toast'
import type { RootState } from '@/store'
import { clearWatchedTask, type WatchedTask } from '@/store/uiSlice'

const labels: Record<string, string> = {
  create: '创建服务', start: '启动', stop: '关机', restart: '重启',
  reinstall: '重装', update: '更新', delete: '删除', deploy: '部署',
  destroy: '销毁', 'retry-deploy': '重新部署'
}

function TaskWatcher({ task }: { task: WatchedTask }) {
  const dispatch = useDispatch()
  const notified = useRef(false)
  const { data } = useGetTaskQuery(task.id, { pollingInterval: 2500 })
  useEffect(() => {
    if (!data || notified.current || !['succeeded', 'failed'].includes(data.status)) return
    notified.current = true
    const action = labels[data.action] ?? '资源操作'
    if (data.status === 'failed') toast.error(`${action}未完成`, '请在执行记录中查看原因并重试。')
    else toast.success(`${action}已完成`, '服务状态已更新。')
    dispatch(cloudApi.util.invalidateTags(['Instances', 'Orders', 'Notifications']))
    dispatch(clearWatchedTask(task.id))
  }, [data, dispatch, task.id])
  return null
}

export function TaskNotification() {
  const tasks = useSelector((state: RootState) => state.ui.watchedTasks)
  return <>{tasks.map(task => <TaskWatcher key={task.id} task={task} />)}</>
}
