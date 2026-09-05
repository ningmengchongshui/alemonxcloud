import { useState } from 'react'
import {
  useCreateTicketMutation,
  useGetInstancesQuery,
  useGetOrdersQuery,
  useGetTicketQuery,
  useGetTicketsQuery,
  useReopenTicketMutation,
  useReplyTicketMutation
} from '@/services/cloudApi'
import {
  Alert,
  Button,
  Dialog,
  EmptyState,
  LoadingState,
  PageHeader,
  StatusBadge
} from '@/components/ui'
import { TicketReplyComposer } from '@/components/TicketReplyComposer'
import type { Ticket, TicketCategory, TicketPriority } from '@/types/cloud'

const categoryLabels: Record<TicketCategory, string> = {
  instance: '实例与部署',
  billing: '账务与续费',
  account: '账号与访问',
  other: '其他问题'
}
const priorityLabels: Record<TicketPriority, string> = {
  normal: '普通',
  high: '高',
  urgent: '紧急'
}
const statusLabels = { open: '待受理', in_progress: '处理中', closed: '已关闭' }
const fieldClass =
  'mt-1.5 block w-full rounded-md border border-slate-300 bg-white px-3 py-2.5 text-sm font-normal text-slate-800 outline-none placeholder:text-slate-400 focus:border-blue-500 dark:border-slate-600 dark:bg-slate-900 dark:text-white'
const tone = (status: Ticket['status']) =>
  status === 'closed'
    ? 'neutral'
    : status === 'in_progress'
      ? 'progress'
      : 'pending'

export function TicketsPage({
  selectedID,
  onSelect
}: {
  selectedID?: string
  onSelect: (id?: string) => void
}) {
  const tickets = useGetTicketsQuery()
  const instances = useGetInstancesQuery()
  const orders = useGetOrdersQuery()
  const [creating, setCreating] = useState(false)
  const [category, setCategory] = useState<TicketCategory>('instance')
  const [priority, setPriority] = useState<TicketPriority>('normal')
  const [subject, setSubject] = useState('')
  const [body, setBody] = useState('')
  const [instanceID, setInstanceID] = useState('')
  const [orderID, setOrderID] = useState('')
  const [error, setError] = useState('')
  const [createTicket, { isLoading: saving }] = useCreateTicketMutation()
  const selected = selectedID ? (
    <TicketConversation id={selectedID} onBack={() => onSelect()} />
  ) : null
  async function submit() {
    setError('')
    try {
      const ticket = await createTicket({
        category,
        priority,
        subject,
        body,
        instanceId: instanceID || undefined,
        orderId: orderID || undefined
      }).unwrap()
      setCreating(false)
      setSubject('')
      setBody('')
      setInstanceID('')
      setOrderID('')
      onSelect(ticket.id)
    } catch (value: unknown) {
      const message =
        typeof value === 'object' &&
        value !== null &&
        'data' in value &&
        typeof value.data === 'object' &&
        value.data !== null &&
        'message' in value.data &&
        typeof value.data.message === 'string'
          ? value.data.message
          : '工单提交失败，请稍后重试'
      setError(message)
    }
  }
  if (selected) return selected
  return (
    <section className="page me-page">
      <PageHeader
        eyebrow="支持中心"
        title="工单支持"
        description="提交服务、账务或账号问题；管理员回复会通过站内通知提醒你。"
        actions={<Button onClick={() => setCreating(true)}>＋ 新建工单</Button>}
      />
      <p className="mb-4 text-xs text-slate-500 dark:text-slate-300">
        紧急工单会优先展示给管理员；请仅在服务完全不可用或数据风险时选择“紧急”。
      </p>
      {tickets.isLoading ? (
        <LoadingState>正在加载工单…</LoadingState>
      ) : (tickets.data ?? []).length === 0 ? (
        <EmptyState
          title="暂无工单"
          description="遇到实例、账务或账号问题时，可以提交工单获取支持。"
        />
      ) : (
        <div className="grid gap-3">
          {tickets.data?.map(ticket => (
            <button
              type="button"
              key={ticket.id}
              onClick={() => onSelect(ticket.id)}
              className="rounded-xl border border-slate-200 bg-white p-4 text-left transition hover:border-blue-300 dark:border-slate-700 dark:bg-slate-800"
            >
              <div className="flex items-start justify-between gap-3">
                <div>
                  <div className="mb-2 flex flex-wrap gap-2">
                    <StatusBadge tone={tone(ticket.status)}>
                      {statusLabels[ticket.status]}
                    </StatusBadge>
                    <StatusBadge
                      tone={
                        ticket.priority === 'urgent'
                          ? 'danger'
                          : ticket.priority === 'high'
                            ? 'pending'
                            : 'neutral'
                      }
                    >
                      {priorityLabels[ticket.priority]}
                    </StatusBadge>
                  </div>
                  <b>{ticket.subject}</b>
                  <p className="mb-0 mt-1 text-xs text-slate-500">
                    {categoryLabels[ticket.category]} · 更新于{' '}
                    {new Date(ticket.updatedAt).toLocaleString('zh-CN')}
                  </p>
                </div>
                <span className="text-blue-700 dark:text-blue-200">查看 →</span>
              </div>
            </button>
          ))}
        </div>
      )}
      {creating && (
        <Dialog
          title="新建工单"
          description="请尽量提供可复现的信息，便于管理员快速处理。"
          onClose={() => {
            setCreating(false)
            setError('')
          }}
          className="max-w-2xl"
        >
          <form
            className="space-y-4"
            onSubmit={event => {
              event.preventDefault()
              void submit()
            }}
          >
            <div className="grid grid-cols-2 gap-3 max-[560px]:grid-cols-1">
              <label
                className="block text-[11px] font-bold text-slate-700 dark:text-slate-100"
                htmlFor="ticket-category"
              >
                问题分类
                <select
                  id="ticket-category"
                  className={fieldClass}
                  value={category}
                  onChange={event =>
                    setCategory(event.target.value as TicketCategory)
                  }
                >
                  {Object.entries(categoryLabels).map(([value, label]) => (
                    <option value={value} key={value}>
                      {label}
                    </option>
                  ))}
                </select>
              </label>
              <label
                className="block text-[11px] font-bold text-slate-700 dark:text-slate-100"
                htmlFor="ticket-priority"
              >
                优先级
                <select
                  id="ticket-priority"
                  className={fieldClass}
                  value={priority}
                  onChange={event =>
                    setPriority(event.target.value as TicketPriority)
                  }
                >
                  {Object.entries(priorityLabels).map(([value, label]) => (
                    <option value={value} key={value}>
                      {label}
                    </option>
                  ))}
                </select>
              </label>
            </div>
            <label
              className="block text-[11px] font-bold text-slate-700 dark:text-slate-100"
              htmlFor="ticket-subject"
            >
              <span className="flex items-center justify-between">
                <span>主题</span>
                <small className="text-slate-400">{subject.length}/160</small>
              </span>
              <input
                id="ticket-subject"
                data-autofocus
                className={fieldClass}
                maxLength={160}
                value={subject}
                onChange={event => setSubject(event.target.value)}
                placeholder="简要描述你遇到的问题"
              />
            </label>
            <div className="grid grid-cols-2 gap-3 max-[560px]:grid-cols-1">
              <label
                className="block text-[11px] font-bold text-slate-700 dark:text-slate-100"
                htmlFor="ticket-instance"
              >
                关联实例（可选）
                <select
                  id="ticket-instance"
                  className={fieldClass}
                  value={instanceID}
                  onChange={event => setInstanceID(event.target.value)}
                >
                  <option value="">不关联实例</option>
                  {(instances.data ?? []).map(item => (
                    <option value={item.id} key={item.id}>
                      {item.name} · {item.id.slice(0, 12)}
                    </option>
                  ))}
                </select>
              </label>
              <label
                className="block text-[11px] font-bold text-slate-700 dark:text-slate-100"
                htmlFor="ticket-order"
              >
                关联订单（可选）
                <select
                  id="ticket-order"
                  className={fieldClass}
                  value={orderID}
                  onChange={event => setOrderID(event.target.value)}
                >
                  <option value="">不关联订单</option>
                  {(orders.data ?? []).map(item => (
                    <option value={item.id} key={item.id}>
                      {item.imageName} · {item.id.slice(0, 12)}
                    </option>
                  ))}
                </select>
              </label>
            </div>
            <label
              className="block text-[11px] font-bold text-slate-700 dark:text-slate-100"
              htmlFor="ticket-body"
            >
              <span className="flex items-center justify-between">
                <span>问题描述</span>
                <small className="text-slate-400">{body.length}/4000</small>
              </span>
              <textarea
                id="ticket-body"
                className={fieldClass}
                rows={6}
                maxLength={4000}
                value={body}
                onChange={event => setBody(event.target.value)}
                placeholder="请描述现象、发生时间及已尝试的操作"
              />
            </label>
            {error && <Alert tone="error">{error}</Alert>}
            <div className="flex justify-end gap-2">
              <Button
                type="button"
                tone="secondary"
                onClick={() => {
                  setCreating(false)
                  setError('')
                }}
              >
                取消
              </Button>
              <Button
                type="submit"
                loading={saving}
                disabled={!subject.trim() || !body.trim()}
              >
                提交工单
              </Button>
            </div>
          </form>
        </Dialog>
      )}
    </section>
  )
}

function TicketConversation({
  id,
  onBack
}: {
  id: string
  onBack: () => void
}) {
  const detail = useGetTicketQuery(id)
  const [reply, setReply] = useState('')
  const [error, setError] = useState('')
  const [send, { isLoading: sending }] = useReplyTicketMutation()
  const [reopen, { isLoading: reopening }] = useReopenTicketMutation()
  if (detail.isLoading)
    return (
      <section className="page me-page">
        <LoadingState>正在加载工单详情…</LoadingState>
      </section>
    )
  if (!detail.data)
    return (
      <section className="page me-page">
        <EmptyState
          title="工单不存在"
          description="它可能已被删除，或你没有访问权限。"
          action={<Button onClick={onBack}>返回工单列表</Button>}
        />
      </section>
    )
  const { ticket, messages } = detail.data
  return (
    <section className="page me-page">
      <PageHeader
        eyebrow="支持中心"
        title={ticket.subject}
        description={`${categoryLabels[ticket.category]} · ${priorityLabels[ticket.priority]} · ${statusLabels[ticket.status]}`}
        actions={
          <Button tone="secondary" onClick={onBack}>
            ← 返回列表
          </Button>
        }
      />
      <div className="mb-5 flex flex-wrap gap-2 text-xs text-slate-600 dark:text-slate-300">
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
        {!ticket.instanceId && !ticket.orderId && (
          <span className="rounded-full bg-slate-100 px-3 py-1.5 dark:bg-slate-800">
            未关联具体资源
          </span>
        )}
      </div>
      <div className="grid gap-3">
        {messages.map(message => (
          <article
            key={message.id}
            className={`max-w-3xl rounded-xl border p-4 text-sm ${message.senderRole === 'admin' ? 'border-blue-200 bg-blue-50 dark:border-blue-900 dark:bg-blue-950' : 'border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-800'}`}
          >
            <b>{message.senderRole === 'admin' ? '管理员' : '我'}</b>
            <p className="mb-2 mt-2 whitespace-pre-wrap leading-6">
              {message.body}
            </p>
            <small className="text-slate-400">
              {new Date(message.createdAt).toLocaleString('zh-CN')}
            </small>
          </article>
        ))}
      </div>
      {ticket.status === 'closed' ? (
        <div className="mt-5">
          <Button
            loading={reopening}
            onClick={() =>
              void reopen(id)
                .unwrap()
                .catch(value =>
                  setError(value?.data?.message ?? '重新打开失败')
                )
            }
          >
            重新打开工单
          </Button>
        </div>
      ) : (
        <TicketReplyComposer
          title="补充回复"
          helper="补充现象、操作结果或继续回复管理员，便于快速处理。"
          placeholder="请补充问题信息或回复管理员"
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
