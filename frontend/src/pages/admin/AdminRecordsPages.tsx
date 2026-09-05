import { useState } from 'react'
import {
  useAdjustAdminWalletMutation,
  useGetAdminAuditLogsQuery,
  useGetAdminOrdersQuery,
  useGetAdminTasksQuery,
  useGetAdminWalletEntriesQuery,
  useRetryTaskMutation,
  useSearchAdminUsersQuery
} from '@/services/cloudApi'
import { Button, Dialog, PageHeader } from '@/components/ui'
import type { CloudUser } from '@/types/cloud'

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
  return (
    <section className="page super-page">
      <PageHeader
        eyebrow="任务队列"
        title="任务执行记录"
        description="查看部署和生命周期任务；失败任务可以安全重试。"
        actions={
          <Button
            tone="secondary"
            loading={tasks.isFetching}
            onClick={() => void tasks.refetch()}
          >
            ↻ 刷新
          </Button>
        }
      />
      <div className="admin-table-wrap">
        <table>
          <thead>
            <tr>
              <th>动作</th>
              <th>实例</th>
              <th>状态</th>
              <th>尝试次数</th>
              <th>最近错误</th>
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
                <td>{task.lastError || '—'}</td>
                <td>
                  {task.status === 'failed' && (
                    <button
                      className="text-button"
                      onClick={() => void retry(task.id)}
                    >
                      安全重试
                    </button>
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
    if (!Number.isInteger(value) || value <= 0) return
    await adjust({
      id: adjusting.id,
      amountFen: value,
      direction,
      note: note.trim()
    }).unwrap()
    setAdjusting(null)
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
                <td>
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
          description={adjusting.username}
          onClose={() => setAdjusting(null)}
        >
          <div className="space-y-3">
            <label>
              操作
              <select
                value={direction}
                onChange={event =>
                  setDirection(event.target.value as 'increase' | 'decrease')
                }
              >
                <option value="increase">增加余额</option>
                <option value="decrease">扣减余额</option>
              </select>
            </label>
            <label>
              金额（代币）
              <input
                type="number"
                min="0.01"
                step="0.01"
                value={amount}
                onChange={event => setAmount(event.target.value)}
              />
            </label>
            <label>
              运营备注
              <input
                value={note}
                onChange={event => setNote(event.target.value)}
              />
            </label>
            <div className="flex justify-end gap-2">
              <Button tone="secondary" onClick={() => setAdjusting(null)}>
                取消
              </Button>
              <Button
                loading={saving}
                disabled={!amount || !note.trim()}
                onClick={() => void submitAdjust()}
              >
                确认变更
              </Button>
            </div>
          </div>
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
