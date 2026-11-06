import { Alert, Button, LoadingState } from './ui'
import { useGetInstanceStatsQuery } from '@/services/cloudApi'

export function InstanceUsage({
  instanceID,
  running
}: {
  instanceID: string
  running: boolean
}) {
  const { currentData, isLoading, isFetching, isError, refetch } =
    useGetInstanceStatsQuery(instanceID, {
      skip: !running,
      pollingInterval: 15000,
      skipPollingIfUnfocused: true
    })
  return (
    <article className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-700 dark:bg-slate-800">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="m-0 text-base font-bold">当前资源占用</h2>
        <Button
          tone="secondary"
          disabled={!running}
          loading={isFetching}
          onClick={() => void refetch()}
        >
          刷新监控
        </Button>
      </div>
      {!running ? (
        <p className="text-sm text-slate-500 dark:text-slate-400">
          实例未运行，暂无实时占用数据。
        </p>
      ) : (
        <>
          {isLoading && <LoadingState>正在采集资源占用…</LoadingState>}
          {isError && (
            <Alert tone="error">
              监控采集失败，请点击刷新重试。节点需要在线并支持监控采集。
              {currentData ? '下方保留上次采样，数据可能已过期。' : ''}
            </Alert>
          )}
          {currentData && (
            <>
              <dl className="my-5 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
                {[
                  ['CPU 使用率', currentData.cpuPercent],
                  ['内存使用 / 上限', currentData.memoryUsage],
                  ['内存使用率', currentData.memoryPercent],
                  ['累计网络接收 / 发送', currentData.networkIO],
                  ['累计磁盘读取 / 写入', currentData.blockIO],
                  ['进程与线程数', currentData.pids]
                ].map(([label, value]) => (
                  <div key={label}>
                    <dt className="text-xs text-slate-500 dark:text-slate-400">
                      {label}
                    </dt>
                    <dd className="m-0 mt-2 text-lg font-bold">
                      {value || '暂未上报'}
                    </dd>
                  </div>
                ))}
              </dl>
              <p className="text-xs text-slate-500 dark:text-slate-400">
                采样时间：
                {new Date(currentData.sampledAt).toLocaleString('zh-CN')} · 每
                15 秒刷新
              </p>
            </>
          )}
          <p className="mb-0 text-xs leading-6 text-slate-500 dark:text-slate-400">
            CPU 按单核 100% 计，多核可超过
            100%。网络和磁盘为容器累计读写量，并非实时速率。磁盘空间占用暂未采集。
          </p>
        </>
      )}
    </article>
  )
}
