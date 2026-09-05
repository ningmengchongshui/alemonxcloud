import { useState } from 'react'
import {
  Alert,
  Button,
  EmptyState,
  PageHeader,
  StatusBadge
} from '@/components/ui'
import {
  useGetAdminCouponBatchesQuery,
  useGetAdminCouponRedemptionsQuery,
  useIssueAdminCouponBatchMutation,
  useSaveAdminCouponBatchMutation,
  useSearchAdminCouponUsersQuery,
  useVoidAdminCouponBatchMutation
} from '@/services/cloudApi'
import type { CouponBatch } from '@/types/cloud'

const field =
  'mt-1 block w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm dark:border-slate-600 dark:bg-slate-900'
const blank = (): CouponBatch => ({
  id: '',
  name: '',
  status: 'paused',
  distributionMode: 'public',
  discountType: 'fixed',
  discountValue: 100,
  minAmountFen: 0,
  maxDiscountFen: 0,
  scope: 'both',
  planIDs: [],
  imageIDs: [],
  monthValues: [],
  issueLimit: 100,
  perUserLimit: 1,
  issuedCount: 0,
  createdAt: ''
})
export function AdminCouponsPage({ onBack }: { onBack: () => void }) {
  const batches = useGetAdminCouponBatchesQuery()
  const [save, { isLoading }] = useSaveAdminCouponBatchMutation()
  const [voidUnused] = useVoidAdminCouponBatchMutation()
  const [editing, setEditing] = useState<CouponBatch | null>(null)
  const [issuing, setIssuing] = useState<CouponBatch | null>(null)
  const [error, setError] = useState('')
  const submit = async () => {
    if (!editing) return
    try {
      await save(editing).unwrap()
      setEditing(null)
    } catch (error: unknown) {
      const message =
        typeof error === 'object' && error !== null && 'data' in error &&
        typeof (error as { data?: { message?: unknown } }).data?.message === 'string'
          ? (error as { data: { message: string } }).data.message
          : '保存失败'
      setError(message)
    }
  }
  return (
    <section className="page super-page">
      <PageHeader
        eyebrow="营销运营"
        title="代金券批次"
        description="券批次独立定义规则与库存；公开领取或定向发放后，用户得到独立单次代金券。"
        actions={
          <>
            <Button tone="secondary" onClick={onBack}>
              返回
            </Button>
            <Button
              onClick={() => {
                setError('')
                setEditing(blank())
              }}
            >
              ＋ 创建券批次
            </Button>
          </>
        }
      />
      {issuing && (
        <section className="mb-6 max-w-xl rounded-xl border border-violet-200 bg-violet-50 p-5 dark:border-violet-900 dark:bg-violet-950">
          <div className="flex justify-between">
            <h2 className="text-lg font-bold">
              向用户定向发券：{issuing.name}
            </h2>
            <button className="text-button" onClick={() => setIssuing(null)}>
              关闭
            </button>
          </div>
          <AdminCouponIssuePage batch={issuing} />
        </section>
      )}
      {editing && (
        <section className="mb-6 max-w-3xl rounded-xl border border-blue-200 bg-blue-50 p-5 dark:border-blue-900 dark:bg-blue-950">
          <h2 className="text-lg font-bold">
            {editing.id ? '编辑券批次' : '创建券批次'}
          </h2>
          <div className="mt-4 grid gap-3 sm:grid-cols-2">
            <label>
              名称
              <input
                className={field}
                value={editing.name}
                onChange={e => setEditing({ ...editing, name: e.target.value })}
              />
            </label>
            <label>
              发放方式
              <select
                className={field}
                value={editing.distributionMode}
                onChange={e =>
                  setEditing({
                    ...editing,
                    distributionMode: e.target
                      .value as CouponBatch['distributionMode']
                  })
                }
              >
                <option value="public">公开领取</option>
                <option value="targeted">定向发放</option>
              </select>
            </label>
            <label>
              优惠类型
              <select
                className={field}
                value={editing.discountType}
                onChange={e =>
                  setEditing({
                    ...editing,
                    discountType: e.target.value as CouponBatch['discountType']
                  })
                }
              >
                <option value="fixed">固定减免</option>
                <option value="percent">实付折扣</option>
              </select>
            </label>
            <label>
              {editing.discountType === 'fixed'
                ? '减免金额（分）'
                : '实付折数（95=95折）'}
              <input
                className={field}
                type="number"
                value={
                  editing.discountType === 'percent'
                    ? editing.discountValue / 100
                    : editing.discountValue
                }
                onChange={e =>
                  setEditing({
                    ...editing,
                    discountValue:
                      editing.discountType === 'percent'
                        ? Number(e.target.value) * 100
                        : Number(e.target.value)
                  })
                }
              />
            </label>
            <label>
              总发放量（0不限）
              <input
                className={field}
                type="number"
                value={editing.issueLimit}
                onChange={e =>
                  setEditing({ ...editing, issueLimit: Number(e.target.value) })
                }
              />
            </label>
            <label>
              每人限领
              <input
                className={field}
                type="number"
                min="1"
                value={editing.perUserLimit}
                onChange={e =>
                  setEditing({
                    ...editing,
                    perUserLimit: Number(e.target.value)
                  })
                }
              />
            </label>
            <label>
              适用范围
              <select
                className={field}
                value={editing.scope}
                onChange={e =>
                  setEditing({
                    ...editing,
                    scope: e.target.value as CouponBatch['scope']
                  })
                }
              >
                <option value="both">新购与续费</option>
                <option value="purchase">仅新购</option>
                <option value="renewal">仅续费</option>
              </select>
            </label>
            <label>
              状态
              <select
                className={field}
                value={editing.status}
                onChange={e =>
                  setEditing({
                    ...editing,
                    status: e.target.value as CouponBatch['status']
                  })
                }
              >
                <option value="paused">暂停</option>
                <option value="active">启用</option>
              </select>
            </label>
          </div>
          {error && <Alert tone="error">{error}</Alert>}
          <div className="mt-4 flex gap-2">
            <Button tone="secondary" onClick={() => setEditing(null)}>
              取消
            </Button>
            <Button loading={isLoading} onClick={() => void submit()}>
              保存批次
            </Button>
          </div>
        </section>
      )}
      {(batches.data ?? []).length === 0 ? (
        <EmptyState
          title="暂无券批次"
          description="创建批次后，选择公开领取或定向发放。"
        />
      ) : (
        <section className="admin-table-wrap">
          <table>
            <thead>
              <tr>
                <th>批次</th>
                <th>发放方式</th>
                <th>优惠</th>
                <th>库存</th>
                <th>状态</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {(batches.data ?? []).map(b => (
                <tr key={b.id}>
                  <td>
                    <b>{b.name}</b>
                  </td>
                  <td>
                    {b.distributionMode === 'public' ? '公开领取' : '定向发放'}
                  </td>
                  <td>
                    {b.discountType === 'fixed'
                      ? `减 ${(b.discountValue / 100).toFixed(2)}`
                      : `${b.discountValue / 100} 折`}
                  </td>
                  <td>
                    {b.issuedCount}
                    {b.issueLimit ? ` / ${b.issueLimit}` : ' / 不限'}
                  </td>
                  <td>
                    <StatusBadge
                      tone={b.status === 'active' ? 'success' : 'neutral'}
                    >
                      {b.status === 'active' ? '启用' : '暂停'}
                    </StatusBadge>
                  </td>
                  <td>
                    {b.distributionMode === 'targeted' && (
                      <button
                        className="text-button"
                        onClick={() => setIssuing(b)}
                      >
                        定向发放
                      </button>
                    )}
                    <button
                      className="ml-3 text-button"
                      onClick={() => setEditing(b)}
                    >
                      编辑
                    </button>
                    <button
                      className="ml-3 text-button text-red-600"
                      onClick={() => void voidUnused(b.id)}
                    >
                      作废未使用券
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
    </section>
  )
}
export function AdminCouponRedemptionsPage({ onBack }: { onBack: () => void }) {
  const redemptions = useGetAdminCouponRedemptionsQuery()
  return (
    <section className="page super-page">
      <PageHeader
        eyebrow="营销运营"
        title="优惠核销记录"
        description="已核销记录不可修改，订单快照与钱包流水以此为准。"
        actions={
          <Button tone="secondary" onClick={onBack}>
            返回
          </Button>
        }
      />
      {(redemptions.data ?? []).length === 0 ? (
        <EmptyState
          title="暂无核销记录"
          description="用户成功下单后会在这里显示。"
        />
      ) : (
        <section className="admin-table-wrap">
          <table>
            <thead>
              <tr>
                <th>订单</th>
                <th>用户</th>
                <th>优惠金额</th>
                <th>时间</th>
              </tr>
            </thead>
            <tbody>
              {(redemptions.data ?? []).map(x => (
                <tr key={x.id}>
                  <td>{x.orderId}</td>
                  <td>{x.ownerId}</td>
                  <td>{(x.discountAmountFen / 100).toFixed(2)} XCoin</td>
                  <td>{new Date(x.createdAt).toLocaleString('zh-CN')}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
    </section>
  )
}
export function AdminCouponIssuePage({ batch }: { batch: CouponBatch }) {
  const [q, setQ] = useState('')
  const users = useSearchAdminCouponUsersQuery(q, { skip: q.length < 2 })
  const [selected, setSelected] = useState<string[]>([])
  const [issue] = useIssueAdminCouponBatchMutation()
  return (
    <section>
      <input
        className={field}
        placeholder="搜索用户名、邮箱或 ID"
        value={q}
        onChange={e => setQ(e.target.value)}
      />
      {(users.data ?? []).map(u => (
        <label key={u.id} className="mt-2 flex gap-2 text-sm">
          <input
            type="checkbox"
            checked={selected.includes(u.id)}
            onChange={() =>
              setSelected(
                selected.includes(u.id)
                  ? selected.filter(id => id !== u.id)
                  : [...selected, u.id]
              )
            }
          />
          {u.username} · {u.email}
        </label>
      ))}
      <Button
        className="mt-3"
        onClick={() => void issue({ id: batch.id, ownerIds: selected })}
      >
        向已选用户发券
      </Button>
    </section>
  )
}
