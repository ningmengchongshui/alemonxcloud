import { useState } from 'react'
import {
  useAdminReplyTicketMutation,
  useAdminUpdateTicketPriorityMutation,
  useAdminUpdateTicketStatusMutation,
  useGetAdminTicketQuery,
  useGetAdminTicketsQuery
} from '@/services/cloudApi'
import {
  Alert,
  Button,
  EmptyState,
  FilterTabs,
  LoadingState,
  PageHeader,
  StatusBadge
} from '@/components/ui'
import { TicketReplyComposer } from '@/components/TicketReplyComposer'
import type { TicketPriority, TicketStatus } from '@/types/cloud'

const stateLabel: Record<TicketStatus, string> = {
  open: '待受理',
  in_progress: '处理中',
  closed: '已关闭'
}
const priorityLabel: Record<TicketPriority, string> = {
  normal: '普通',
  high: '高',
  urgent: '紧急'
}
const categoryLabel: Record<string, string> = {
  instance: '实例与部署',
  billing: '账务与续费',
  account: '账号与访问',
  other: '其他问题'
}

export function AdminTicketsPage() {
  const [status, setStatus] = useState<TicketStatus | 'all'>('all')
  const [priority, setPriority] = useState<TicketPriority | 'all'>('all')
  const [selectedID, setSelectedID] = useState<string>()
  const tickets = useGetAdminTicketsQuery({
    status: status === 'all' ? undefined : status,
    priority: priority === 'all' ? undefined : priority
  })
  if (selectedID)
    return (
      <AdminTicketDetail
        id={selectedID}
        onBack={() => setSelectedID(undefined)}
      />
    )
  return (
    <section className="page super-page">
      <PageHeader
        eyebrow="支持运营"
        title="工单管理"
        description="紧急与高优先级工单优先展示；管理员回复会自动将待受理工单标为处理中。"
        actions={
          <Button
            tone="secondary"
            loading={tickets.isFetching}
            onClick={() => void tickets.refetch()}
          >
            ↻ 刷新
          </Button>
        }
      />
      <div className="mb-4 grid gap-3">
        <FilterTabs
          value={status}
          onChange={setStatus}
          label="工单状态筛选"
          items={[
            { value: 'all', label: '全部' },
            { value: 'open', label: '待受理' },
            { value: 'in_progress', label: '处理中' },
            { value: 'closed', label: '已关闭' }
          ]}
        />
        <FilterTabs
          value={priority}
          onChange={setPriority}
          label="优先级筛选"
          items={[
            { value: 'all', label: '全部优先级' },
            { value: 'urgent', label: '紧急' },
            { value: 'high', label: '高' },
            { value: 'normal', label: '普通' }
          ]}
        />
      </div>
      {tickets.isLoading ? (
        <LoadingState>正在加载工单…</LoadingState>
      ) : (tickets.data ?? []).length === 0 ? (
        <EmptyState
          title="没有匹配工单"
          description="当前筛选条件下没有需要处理的工单。"
        />
      ) : (
        <div className="admin-table-wrap">
          <table>
            <thead>
              <tr>
                <th>优先级</th>
                <th>主题</th>
                <th>用户</th>
                <th>状态</th>
                <th>更新时间</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {tickets.data?.map(ticket => (
                <tr key={ticket.id}>
                  <td>
                    <StatusBadge
                      tone={
                        ticket.priority === 'urgent'
                          ? 'danger'
                          : ticket.priority === 'high'
                            ? 'pending'
                            : 'neutral'
                      }
                    >
                      {priorityLabel[ticket.priority]}
                    </StatusBadge>
                  </td>
                  <td>
                    <b>{ticket.subject}</b>
                    <small>
                      {categoryLabel[ticket.category] ?? ticket.category} ·{' '}
                      {ticket.id.slice(0, 14)}
                    </small>
                  </td>
                  <td>
                    <code>{ticket.ownerId}</code>
                  </td>
                  <td>{stateLabel[ticket.status]}</td>
                  <td>{new Date(ticket.updatedAt).toLocaleString('zh-CN')}</td>
                  <td  className="flex gap-2">
                    <button
                      className="text-button"
                      onClick={() => setSelectedID(ticket.id)}
                    >
                      处理
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}

function AdminTicketDetail({ id, onBack }: { id: string; onBack: () => void }) {
  const detail = useGetAdminTicketQuery(id)
  const [reply, setReply] = useState('')
  const [error, setError] = useState('')
  const [send, { isLoading: sending }] = useAdminReplyTicketMutation()
  const [setStatus, { isLoading: settingStatus }] =
    useAdminUpdateTicketStatusMutation()
  const [setPriority, { isLoading: settingPriority }] =
    useAdminUpdateTicketPriorityMutation()
  if (detail.isLoading)
    return (
      <section className="page super-page">
        <LoadingState>正在加载工单详情…</LoadingState>
      </section>
    )
  if (!detail.data)
    return (
      <section className="page super-page">
        <EmptyState
          title="工单不存在"
          description="请返回列表刷新后重试。"
          action={<Button onClick={onBack}>返回工单列表</Button>}
        />
      </section>
    )
  const { ticket, messages } = detail.data
  return (
    <section className="page super-page">
      <PageHeader
        eyebrow="支持运营"
        title={ticket.subject}
        description={`用户 ${ticket.ownerId} · ${categoryLabel[ticket.category] ?? ticket.category} · ${stateLabel[ticket.status]}`}
        actions={
          <Button tone="secondary" onClick={onBack}>
            ← 返回列表
          </Button>
        }
      />
      {error && <Alert tone="error">{error}</Alert>}
      <div className="mb-4 flex flex-wrap gap-2 text-xs text-slate-600 dark:text-slate-300">
        {ticket.instanceId && (
          <span className="rounded-full bg-slate-100 px-3 py-1.5 dark:bg-slate-800">
            关联实例：{ticket.instanceId.slice(0, 14)}
          </span>
        )}
        {ticket.orderId && (
          <span className="rounded-full bg-slate-100 px-3 py-1.5 dark:bg-slate-800">
            关联订单：{ticket.orderId.slice(0, 14)}
          </span>
        )}
      </div>
      <div className="mb-5 flex flex-wrap gap-2">
        <select
          value={ticket.priority}
          disabled={settingPriority}
          onChange={event =>
            void setPriority({
              id,
              priority: event.target.value as TicketPriority
            })
              .unwrap()
              .catch(value =>
                setError(value?.data?.message ?? '优先级更新失败')
              )
          }
        >
          <option value="normal">普通</option>
          <option value="high">高</option>
          <option value="urgent">紧急</option>
        </select>
        {ticket.status !== 'closed' && (
          <>
            <Button
              tone="secondary"
              loading={settingStatus}
              onClick={() =>
                void setStatus({ id, status: 'in_progress' })
                  .unwrap()
                  .catch(value =>
                    setError(value?.data?.message ?? '状态更新失败')
                  )
              }
            >
              标记处理中
            </Button>
            <Button
              tone="danger"
              loading={settingStatus}
              onClick={() =>
                void setStatus({ id, status: 'closed' })
                  .unwrap()
                  .catch(value => setError(value?.data?.message ?? '关闭失败'))
              }
            >
              关闭工单
            </Button>
          </>
        )}
      </div>
      <div className="grid gap-3">
        {messages.map(message => (
          <article
            key={message.id}
            className={`max-w-3xl rounded-xl border p-4 text-sm ${message.senderRole === 'admin' ? 'border-blue-200 bg-blue-50 dark:border-blue-900 dark:bg-blue-950' : 'border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-800'}`}
          >
            <b>
              {message.senderRole === 'admin'
                ? '管理员'
                : `用户 · ${ticket.ownerId}`}
            </b>
            <p className="mb-2 mt-2 whitespace-pre-wrap leading-6">
              {message.body}
            </p>
            <small className="text-slate-400">
              {new Date(message.createdAt).toLocaleString('zh-CN')}
            </small>
          </article>
        ))}
      </div>
      {ticket.status !== 'closed' && (
        <TicketReplyComposer
          title="回复用户"
          helper="说明处理结果、下一步安排，或请用户补充必要的信息。"
          placeholder="请输入处理结果或需要用户补充的信息"
          value={reply}
          error={error}
          sending={sending}
          onChange={value => {
            setReply(value)
            setError('')
          }}
          onSubmit={() =>
            void send({ id, body: reply })
              .unwrap()
              .then(() => setReply(''))
              .catch(value => setError(value?.data?.message ?? '回复失败'))
          }
        />
      )}
    </section>
  )
}
