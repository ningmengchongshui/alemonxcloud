import { useState } from 'react'
import {
  useAdjustAdminWalletMutation,
  useGetAdminAuditLogsQuery,
  useGetAdminOrdersQuery,
  useGetAdminTasksQuery,
  useGetAdminWalletEntriesQuery,
  useDiscardReviewTaskMutation,
  useDiscardAllAdminTasksMutation,
  useRetryTaskMutation,
  useResumeReviewTaskMutation,
  useSearchAdminUsersQuery
} from '@/services/cloudApi'
import {
  Alert,
  Button,
  Dialog,
  DialogFooter,
  dialogFieldClass,
  dialogLabelClass,
  PageHeader
} from '@/components/ui'
import type { CloudUser } from '@/types/cloud'

const dangerousTaskAction = (action: string) =>
  [
    'stop',
    'update',
    'restart',
    'reinstall',
    'destroy',
    'purge',
    'retry-deploy'
  ].includes(action)

export function AdminOrdersPage() {
  const orders = useGetAdminOrdersQuery()
  return (
    <section className="page super-page">
      <PageHeader
        eyebrow="订单记录"
        title="自动购买订单"
        description="钱包扣款后自动校验资源并进入部署。"
        actions={
          <Button
            tone="secondary"
            loading={orders.isFetching}
            onClick={() => void orders.refetch()}
          >
            ↻ 刷新
          </Button>
        }
      />
      <div className="admin-table-wrap">
        <table>
          <thead>
            <tr>
              <th>订单</th>
              <th>用户</th>
              <th>商品</th>
              <th>金额</th>
              <th>状态</th>
            </tr>
          </thead>
          <tbody>
            {(orders.data ?? []).map(order => (
              <tr key={order.id}>
                <td>
                  <code>{order.id.slice(0, 14)}</code>
                </td>
                <td>{order.ownerId}</td>
                <td>
                  <b>{order.planName}</b>
                  <small>
                    {order.imageName} · {order.imageVersion}
                  </small>
                </td>
                <td>¥{(order.amountFen / 100).toFixed(2)}</td>
                <td>{order.status}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  )
}

export function AdminTasksPage() {
  const tasks = useGetAdminTasksQuery()
  const [retry] = useRetryTaskMutation()
  const [resume] = useResumeReviewTaskMutation()
  const [discard] = useDiscardReviewTaskMutation()
  const [discardAll, discardAllState] = useDiscardAllAdminTasksMutation()
  const abnormalCount = (tasks.data ?? []).filter(task =>
    ['failed', 'needs_review'].includes(task.status)
  ).length

  async function discardAbnormalTasks() {
    if (
      !window.confirm(
        `确认一键作废全部 ${abnormalCount} 个失败或待复核任务吗？任务记录会保留，但不会再执行。`
      )
    )
      return
    await discardAll()
    await tasks.refetch()
  }
  return (
    <section className="page super-page">
      <PageHeader
        eyebrow="任务队列"
        title="任务执行记录"
        description="危险的过期生命周期任务会进入待复核，确认前不会再次操作容器。"
        actions={
          <div className="flex gap-2">
            {abnormalCount > 0 && (
              <Button
                tone="danger"
                loading={discardAllState.isLoading}
                onClick={() => void discardAbnormalTasks()}
              >
                一键作废异常任务（{abnormalCount}）
              </Button>
            )}
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
      <div className="admin-table-wrap">
        <table>
          <thead>
            <tr>
              <th>动作</th>
              <th>实例</th>
              <th>状态</th>
              <th>尝试</th>
              <th>ERROR</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {(tasks.data ?? []).map(task => (
              <tr key={task.id}>
                <td>{task.action}</td>
                <td>
                  <code>{task.instanceId.slice(0, 14)}</code>
                </td>
                <td>{task.status}</td>
                <td>{task.attempts}</td>
                <td>
                  {task.lastError || '—'}
                  {task.recoveryCount ? (
                    <small>已恢复 {task.recoveryCount} 次</small>
                  ) : null}
                </td>
                <td className="flex gap-2">
                  {task.status === 'failed' && (
                    <button
                      className="text-button"
                      onClick={() => void retry(task.id)}
                    >
                      {dangerousTaskAction(task.action)
                        ? '转入复核'
                        : '重新执行'}
                    </button>
                  )}
                  {task.status === 'needs_review' && (
                    <>
                      <button
                        className="text-button"
                        onClick={() => {
                          if (
                            window.confirm(
                              `确认恢复 ${task.action} 任务吗？这会再次对实例 ${task.instanceId.slice(0, 14)} 执行生命周期操作。`
                            )
                          ) {
                            void resume(task.id)
                          }
                        }}
                      >
                        确认恢复
                      </button>
                      <button
                        className="text-button text-danger"
                        onClick={() => void discard(task.id)}
                      >
                        作废任务
                      </button>
                    </>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  )
}

export function AdminUsersPage({
  onOpenWalletHistory
}: {
  onOpenWalletHistory?: (user: CloudUser) => void
}) {
  const [query, setQuery] = useState('')
  const users = useSearchAdminUsersQuery(query)
  const [selected, setSelected] = useState<CloudUser | null>(null)
  const [adjusting, setAdjusting] = useState<CloudUser | null>(null)
  const [amount, setAmount] = useState('')
  const [note, setNote] = useState('')
  const [adjustError, setAdjustError] = useState('')
  const [direction, setDirection] = useState<'increase' | 'decrease'>(
    'increase'
  )
  const entries = useGetAdminWalletEntriesQuery(selected?.id ?? '', {
    skip: !selected
  })
  const [adjust, { isLoading: saving }] = useAdjustAdminWalletMutation()
  async function submitAdjust() {
    if (!adjusting || !note.trim()) return
    const value = Math.round(Number(amount) * 100)
    if (!Number.isInteger(value) || value <= 0) {
      setAdjustError('请输入大于 0 的有效金额。')
      return
    }
    try {
      await adjust({
        id: adjusting.id,
        amountFen: value,
        direction,
        note: note.trim()
      }).unwrap()
      setAdjusting(null)
    } catch {
      setAdjustError('余额变更未完成，请稍后重试。')
    }
  }
  return (
    <section className="page super-page">
      <PageHeader
        eyebrow="用户运营"
        title="用户与钱包"
        description="仅可为已经登录过 xCloud 的用户管理余额；账本流水不可修改。"
      />
      <div className="mb-4 flex justify-end">
        <input
          value={query}
          onChange={event => setQuery(event.target.value)}
          placeholder="用户名、邮箱或ID"
        />
      </div>
      <div className="admin-table-wrap">
        <table>
          <thead>
            <tr>
              <th>用户</th>
              <th>邮箱</th>
              <th>余额</th>
              <th>最后登录</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {(users.data ?? []).map(user => (
              <tr key={user.id}>
                <td>
                  <b>{user.username}</b>
                  <small>{user.id}</small>
                </td>
                <td>{user.email || '—'}</td>
                <td>{(user.balanceFen / 100).toFixed(2)} 代币</td>
                <td>{new Date(user.lastLoginAt).toLocaleString('zh-CN')}</td>
                <td className="flex gap-2">
                  <button
                    className="text-button"
                    onClick={() =>
                      onOpenWalletHistory
                        ? onOpenWalletHistory(user)
                        : setSelected(user)
                    }
                  >
                    查看流水
                  </button>
                  <button
                    className="text-button"
                    onClick={() => {
                      setAdjusting(user)
                      setAmount('')
                      setNote('')
                      setAdjustError('')
                      setDirection('increase')
                    }}
                  >
                    变更余额
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {selected && (
        <Dialog
          eyebrow="用户与钱包"
          title={`${selected.username} 的账本流水`}
          description={`当前余额 ${(selected.balanceFen / 100).toFixed(2)} 代币`}
          onClose={() => setSelected(null)}
        >
          {entries.isLoading ? (
            <p>正在加载流水…</p>
          ) : (
            <div className="space-y-3">
              {(entries.data ?? []).map(entry => (
                <article
                  key={entry.id}
                  className="rounded-lg border border-slate-200 p-3 text-xs dark:border-slate-700"
                >
                  <b>
                    {entry.amountFen >= 0 ? '+' : ''}
                    {(entry.amountFen / 100).toFixed(2)} 代币
                  </b>
                  <p>{entry.note || '—'}</p>
                  <small>
                    {new Date(entry.createdAt).toLocaleString('zh-CN')}
                  </small>
                </article>
              ))}
            </div>
          )}
        </Dialog>
      )}
      {adjusting && (
        <Dialog
          eyebrow="用户与钱包"
          title="变更余额"
          description={`为 ${adjusting.username} 调整钱包余额。此操作会记入账本，无法直接修改或删除。`}
          onClose={() => {
            setAdjusting(null)
            setAdjustError('')
          }}
        >
          <form
            className="space-y-4"
            onSubmit={event => {
              event.preventDefault()
              void submitAdjust()
            }}
          >
            <div className="rounded-lg bg-slate-50 px-3 py-2.5 text-xs dark:bg-slate-900">
              <span className="text-slate-500 dark:text-slate-300">
                当前余额
              </span>
              <b className="ml-2 text-slate-800 dark:text-white">
                {(adjusting.balanceFen / 100).toFixed(2)} XCoin
              </b>
            </div>
            <label
              className={dialogLabelClass}
              htmlFor="wallet-adjust-direction"
            >
              变更方式
              <select
                id="wallet-adjust-direction"
                className={dialogFieldClass}
                value={direction}
                onChange={event =>
                  setDirection(event.target.value as 'increase' | 'decrease')
                }
              >
                <option value="increase">增加余额</option>
                <option value="decrease">扣减余额</option>
              </select>
            </label>
            <label className={dialogLabelClass} htmlFor="wallet-adjust-amount">
              金额（XCoin）
              <input
                id="wallet-adjust-amount"
                className={dialogFieldClass}
                type="number"
                min="0.01"
                step="0.01"
                value={amount}
                onChange={event => {
                  setAmount(event.target.value)
                  setAdjustError('')
                }}
                placeholder="例如 100.00"
                data-autofocus
              />
            </label>
            <label className={dialogLabelClass} htmlFor="wallet-adjust-note">
              运营备注
              <textarea
                id="wallet-adjust-note"
                className={dialogFieldClass}
                rows={3}
                maxLength={240}
                value={note}
                onChange={event => {
                  setNote(event.target.value)
                  setAdjustError('')
                }}
                placeholder="请说明本次变更的原因，便于审计追溯"
              />
            </label>
            {adjustError && <Alert tone="error">{adjustError}</Alert>}
            <DialogFooter>
              <Button
                type="button"
                tone="secondary"
                onClick={() => setAdjusting(null)}
              >
                取消
              </Button>
              <Button
                type="submit"
                loading={saving}
                disabled={!amount || !note.trim()}
              >
                确认变更
              </Button>
            </DialogFooter>
          </form>
        </Dialog>
      )}
    </section>
  )
}

export function AdminAuditPage() {
  const audit = useGetAdminAuditLogsQuery()
  return (
    <section className="page super-page">
      <PageHeader
        eyebrow="安全审计"
        title="平台操作记录"
        description="记录管理员配置、余额和实例生命周期操作。"
        actions={
          <Button
            tone="secondary"
            loading={audit.isFetching}
            onClick={() => void audit.refetch()}
          >
            ↻ 刷新
          </Button>
        }
      />
      <div className="admin-table-wrap">
        <table>
          <thead>
            <tr>
              <th>时间</th>
              <th>操作者</th>
              <th>操作</th>
              <th>对象</th>
            </tr>
          </thead>
          <tbody>
            {(audit.data ?? []).map(item => (
              <tr key={item.id}>
                <td>{new Date(item.createdAt).toLocaleString('zh-CN')}</td>
                <td>{item.actorId}</td>
                <td>{item.action}</td>
                <td>
                  {item.targetType} · {item.targetId.slice(0, 14)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  )
}
