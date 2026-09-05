import { useEffect, useMemo, useRef, useState } from 'react'
import { Alert, Button, LoadingState, PageHeader } from '@/components/ui'
import { useGetInstanceLogsQuery } from '@/services/cloudApi'
import type { Instance } from '@/types/cloud'

type LogLevel = 'error' | 'warn' | 'info' | 'debug' | 'other'
type LogFilter = 'all' | 'error'

function classify(line: string): LogLevel {
  const value = line.toLowerCase()
  if (/\b(fatal|panic|error|exception|failed|fail)\b/.test(value))
    return 'error'
  if (/\b(warn|warning|deprecated)\b/.test(value)) return 'warn'
  if (/\b(debug|trace|verbose)\b/.test(value)) return 'debug'
  if (/\b(info|notice|started|listening|ready)\b/.test(value)) return 'info'
  return 'other'
}

function splitLogLine(line: string) {
  const match = line.match(/^(\d{4}-\d\d-\d\dT[^\s]+)\s+(.*)$/)
  return match ? { timestamp: match[1], message: match[2] } : { message: line }
}

export function InstanceLogsPage({
  instanceID,
  instance,
  onBack
}: {
  instanceID: string
  instance?: Instance
  onBack: () => void
}) {
  const [live, setLive] = useState(true)
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<LogFilter>('all')
  const [copied, setCopied] = useState(false)
  const viewport = useRef<HTMLDivElement>(null)
  const { data, isLoading, isFetching, isError, refetch } =
    useGetInstanceLogsQuery(
      { id: instanceID, tail: 300 },
      { pollingInterval: live ? 5000 : 0, refetchOnFocus: true }
    )
  const lines = useMemo(() => data?.lines ?? [], [data?.lines])
  const counts = useMemo(
    () =>
      lines.reduce<Record<LogLevel, number>>(
        (value, line) => {
          value[classify(line)]++
          return value
        },
        { error: 0, warn: 0, info: 0, debug: 0, other: 0 }
      ),
    [lines]
  )
  const visibleLines = useMemo(() => {
    const needle = query.trim().toLowerCase()
    return lines.filter(
      line =>
        (filter !== 'error' || classify(line) === 'error') &&
        (!needle || line.toLowerCase().includes(needle))
    )
  }, [filter, lines, query])

  useEffect(() => {
    if (live && viewport.current)
      viewport.current.scrollTop = viewport.current.scrollHeight
  }, [live, visibleLines])

  async function copyLogs() {
    try {
      await navigator.clipboard?.writeText(visibleLines.join('\n'))
      setCopied(true)
    } catch {
      setCopied(false)
    }
  }
  function downloadLogs() {
    const url = URL.createObjectURL(
      new Blob([visibleLines.join('\n') + (visibleLines.length ? '\n' : '')], {
        type: 'text/plain'
      })
    )
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `xcloud-${instanceID.slice(0, 12)}-logs.txt`
    anchor.click()
    URL.revokeObjectURL(url)
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
          <div className="flex flex-wrap justify-end gap-2">
            <Button tone="secondary" onClick={onBack}>
              返回实例
            </Button>
            <Button
              tone={live ? 'primary' : 'secondary'}
              onClick={() => setLive(value => !value)}
            >
              {live ? '● 实时' : '○ 暂停'}
            </Button>
            <Button
              tone="secondary"
              loading={isFetching}
              onClick={() => void refetch()}
            >
              ↻ 刷新
            </Button>
            <Button
              tone="secondary"
              disabled={!visibleLines.length}
              onClick={() => void copyLogs()}
            >
              复制
            </Button>
            <Button
              tone="secondary"
              disabled={!visibleLines.length}
              onClick={downloadLogs}
            >
              下载
            </Button>
          </div>
        }
      />
      {copied && <Alert tone="success">已复制当前筛选后的日志。</Alert>}
      {data?.truncated && (
        <Alert tone="info">
          日志内容已按安全上限截断；可缩小时间范围或筛选关键词。
        </Alert>
      )}
      <div className="mb-3 flex flex-wrap gap-2 rounded-xl border border-slate-200 bg-white p-3 shadow-sm dark:border-slate-700 dark:bg-slate-800">
        <input
          aria-label="搜索日志"
          value={query}
          onChange={event => setQuery(event.target.value)}
          placeholder="搜索错误、请求 ID 或关键字"
        />
        <button
          className={`rounded-md px-3 py-1 text-sm ${filter === 'all' ? 'bg-blue-600 text-white' : 'bg-slate-100 text-slate-700 dark:bg-slate-700 dark:text-slate-100'}`}
          onClick={() => setFilter('all')}
        >
          全部 {lines.length}
        </button>
        <button
          className={`rounded-md px-3 py-1 text-sm ${filter === 'error' ? 'bg-rose-600 text-white' : 'bg-rose-50 text-rose-700 dark:bg-rose-950 dark:text-rose-100'}`}
          onClick={() => setFilter('error')}
        >
          仅错误 {counts.error}
        </button>
      </div>
      {isLoading ? (
        <LoadingState>正在加载容器日志…</LoadingState>
      ) : isError ? (
        <Alert tone="error">
          日志加载失败。实例若正在更新或节点暂时离线，请稍后刷新；生命周期错误可到“执行记录”查看。
        </Alert>
      ) : (
        <div
          ref={viewport}
          className="max-h-[calc(100vh-19rem)] overflow-auto rounded-xl border border-slate-800 bg-slate-950 p-3 font-mono text-[11px] leading-5 text-slate-200"
        >
          {visibleLines.length === 0 ? (
            <p className="m-2 text-slate-400">没有匹配的日志。</p>
          ) : (
            visibleLines.map((line, index) => {
              const value = splitLogLine(line),
                kind = classify(line)
              return (
                <div
                  key={`${index}-${line.slice(0, 24)}`}
                  className="grid grid-cols-[auto_1fr] gap-x-3 px-1 hover:bg-white/5"
                >
                  <time className="select-none text-slate-500">
                    {value.timestamp ?? '—'}
                  </time>
                  <span
                    className={
                      kind === 'error'
                        ? 'text-rose-300'
                        : kind === 'warn'
                          ? 'text-amber-300'
                          : kind === 'debug'
                            ? 'text-slate-500'
                            : 'text-slate-200'
                    }
                  >
                    {value.message}
                  </span>
                </div>
              )
            })
          )}
        </div>
      )}
      <p className="mt-2 text-xs text-slate-500 dark:text-slate-300">
        {live ? '每 5 秒自动拉取最新日志；' : '实时刷新已暂停；'} 当前显示{' '}
        {visibleLines.length} / {lines.length} 行。容器日志仅保留节点 Docker
        可提供的最近内容。
      </p>
    </section>
  )
}
