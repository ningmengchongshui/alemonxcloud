import { useId } from 'react'
import { Button } from '@/components/ui'
import { useSetInstanceHostAccessMutation } from '@/services/cloudApi'
import type { InstanceHostAccessProps } from './InstanceHostAccess.d'

export function InstanceHostAccess({ instance, busy, onSubmitted }: InstanceHostAccessProps) {
  const descriptionID = useId()
  const [setHostAccess, { isLoading }] = useSetInstanceHostAccessMutation()
  const available = ['running', 'stopped'].includes(instance.status)
  const pending = instance.hostAccessEnabled !== instance.hostAccessApplied
  const description = !instance.hostAccessAllowed
    ? '仅限后台白名单用户，请联系管理员。'
    : !available
      ? '运行中或已关机的实例可以修改此配置。'
      : '通过 host.docker.internal 访问宿主机端口。保存后应用配置，运行中的服务可能短暂中断。'

  async function toggle() {
    if (isLoading || busy) return
    try {
      const response = await setHostAccess({
        id: instance.id,
        enabled: !instance.hostAccessEnabled,
        resourceVersion: instance.resourceVersion ?? 1
      }).unwrap()
      if (response.task) onSubmitted(response.task)
    } catch { /* The shared API layer reports the server's permission or task error. */ }
  }

  return (
    <div className="mx-5 mb-3 flex flex-wrap items-center justify-between gap-3 rounded-lg bg-slate-50 px-3 py-2.5 dark:bg-slate-900">
      <div className="min-w-0 flex-1">
        <b className="text-xs text-slate-700 dark:text-slate-100">宿主机访问</b>
        <p id={descriptionID} className="m-0 mt-1 text-xs text-slate-500 dark:text-slate-300">{description}</p>
        {pending && (
          <p role="status" className="m-0 mt-1 text-xs text-amber-700 dark:text-amber-200">
            {busy || isLoading ? '配置应用中，请等待任务完成。' : '配置尚未生效，请查看执行记录；失败时可重试任务。'}
          </p>
        )}
      </div>
      <Button
        tone="secondary"
        role="switch"
        aria-label={`${instance.name} 的宿主机访问`}
        aria-checked={instance.hostAccessEnabled}
        aria-describedby={descriptionID}
        disabled={!instance.hostAccessAllowed || !available || busy}
        loading={isLoading}
        onClick={() => void toggle()}
      >
        {instance.hostAccessEnabled ? '已开启' : '已关闭'}
      </Button>
    </div>
  )
}
