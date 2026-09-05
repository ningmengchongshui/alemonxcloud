import { NodeConfigButton, NodeEditor } from '@/components/NodeEditor'
import { useGetAdminNodesQuery } from '@/services/cloudApi'
import { Button, PageHeader } from '@/components/ui'

export function AdminNodesPage() {
  const nodes = useGetAdminNodesQuery()
  const online = (nodes.data ?? []).filter(
    node => node.enabled && node.lastHeartbeatAt
  )
  const cpu = (nodes.data ?? []).reduce(
    (total, node) => total + node.cpuTotal,
    0
  )
  const memory = (nodes.data ?? []).reduce(
    (total, node) => total + node.memoryTotalMB,
    0
  )
  return (
    <section className="page super-page">
      <PageHeader
        eyebrow="资源运营"
        title="节点管理"
        description="新实例只会调度至已启用且已确认容量的健康节点。"
        actions={
          <Button
            tone="secondary"
            loading={nodes.isFetching}
            onClick={() => void nodes.refetch()}
          >
            ↻ 刷新
          </Button>
        }
      />
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <span className="text-xs text-slate-500">
          {online.length} 个在线 · 可调度 {cpu} 核 · {Math.round(memory / 1024)}{' '}
          GB
        </span>
        <NodeEditor />
      </div>
      <div className="node-list">
        {(nodes.data ?? []).map(node => (
          <article key={node.id} className="node-row">
            <div>
              <b>{node.name}</b>
              <p>{node.agentURL}</p>
            </div>
            <div className="node-details">
              <span>
                {node.cpuTotal} 核 / {Math.round(node.memoryTotalMB / 1024)} GB
              </span>
              <span>{node.managedContainerCount ?? 0} 个托管容器</span>
              <span>
                {node.dockerVersion
                  ? `Docker ${node.dockerVersion}`
                  : 'Docker 信息同步中'}
              </span>
              <span>
                {node.agentCompatibility === 'compatible'
                  ? `Agent ${node.agentVersion ?? '未知'} · 已兼容`
                  : node.agentCompatibility === 'outdated'
                    ? `Agent ${node.agentVersion ?? '未知'} · 需要升级`
                    : 'Agent 协议未知 · 建议升级'}
              </span>
            </div>
            <div className="node-actions">
              <span>
                {node.enabled && node.lastHeartbeatAt
                  ? '在线'
                  : node.enabled
                    ? '等待心跳'
                    : '未启用'}
              </span>
              {node.agentCompatibility !== 'compatible' && (
                <span className="text-amber-700 dark:text-amber-200">
                  镜像预拉取等扩展功能暂不下发
                </span>
              )}
              <NodeConfigButton node={node} />
            </div>
          </article>
        ))}
      </div>
    </section>
  )
}
