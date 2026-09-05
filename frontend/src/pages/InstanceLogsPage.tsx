import { useState } from 'react'
import { Alert, Button, LoadingState, PageHeader } from '@/components/ui'
import { useGetInstanceLogsQuery } from '@/services/cloudApi'
import type { Instance } from '@/types/cloud'

export function InstanceLogsPage({
  instanceID,
  instance,
  onBack
}: {
  instanceID: string
  instance?: Instance
  onBack: () => void
}) {
  const { data, isLoading, isError } = useGetInstanceLogsQuery(instanceID)
  const [copied, setCopied] = useState(false)
  const lines = data?.lines ?? []

  async function copyLogs() {
    try {
      await navigator.clipboard?.writeText(lines.join('\n'))
      setCopied(true)
    } catch {
      setCopied(false)
    }
  }

  return (
    <section className="page me-page">
      <PageHeader
        title="实例日志"
        description={
          instance
            ? `${instance.name} · ${instance.image} · ${instance.version}`
            : `实例 ${instanceID}`
        }
        actions={
          <div className="flex gap-2">
            <Button tone="secondary" onClick={onBack}>
              返回实例
            </Button>
            <Button
              tone="secondary"
              disabled={!lines.length}
              onClick={() => void copyLogs()}
            >
              复制日志
            </Button>
          </div>
        }
      />
      {copied && <Alert tone="success">日志已复制到剪贴板。</Alert>}
      {isLoading ? (
        <LoadingState>正在加载最近日志…</LoadingState>
      ) : isError ? (
        <Alert tone="error">日志加载失败，请返回实例列表后重试。</Alert>
      ) : (
        <pre className="max-h-[calc(100vh-14rem)] overflow-auto rounded-xl bg-slate-950 p-5 text-[11px] leading-5 text-blue-100">
          {lines.join('\n') || '暂无日志'}
        </pre>
      )}
    </section>
  )
}
