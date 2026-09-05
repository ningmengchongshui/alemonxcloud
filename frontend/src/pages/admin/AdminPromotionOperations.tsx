import { useState } from 'react'
import {
  Alert,
  Button,
  Dialog,
  DialogFooter,
  dialogFieldClass,
  dialogLabelClass,
  EmptyState,
  PageHeader,
  StatusBadge
} from '@/components/ui'
import {
  useGetAdminCouponBatchesQuery,
  useGetAdminCouponRedemptionsQuery,
  useGetAdminCatalogQuery,
  useIssueAdminCouponBatchMutation,
  useSaveAdminCouponBatchMutation,
  useSearchAdminCouponUsersQuery,
  useVoidAdminCouponBatchMutation
} from '@/services/cloudApi'
import type { CouponBatch } from '@/types/cloud'

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
export function AdminCouponsPage() {
  const batches = useGetAdminCouponBatchesQuery()
  const catalog = useGetAdminCatalogQuery()
  const [save, { isLoading }] = useSaveAdminCouponBatchMutation()
  const [voidUnused] = useVoidAdminCouponBatchMutation()
  const [editing, setEditing] = useState<CouponBatch | null>(null)
  const [issuing, setIssuing] = useState<CouponBatch | null>(null)
  const [error, setError] = useState('')
  const missingPlanIDs = (editing?.planIDs ?? []).filter(
    id => !(catalog.data?.plans ?? []).some(plan => plan.id === id)
  )
  const missingImageIDs = (editing?.imageIDs ?? []).filter(
    id => !(catalog.data?.images ?? []).some(image => image.id === id)
  )
  const submit = async () => {
    if (!editing) return
    try {
      await save(editing).unwrap()
      setEditing(null)
    } catch (error: unknown) {
      const message =
        typeof error === 'object' &&
        error !== null &&
        'data' in error &&
        typeof (error as { data?: { message?: unknown } }).data?.message ===
          'string'
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
        <Dialog
          title={editing.id ? '编辑券批次' : '创建券批次'}
          description="设置发放、优惠与适用条件；套餐、镜像和周期留空时表示不限制。"
          className="max-w-3xl"
          onClose={() => {
            setEditing(null)
            setError('')
          }}
        >
          <form
            className="space-y-4"
            onSubmit={event => {
              event.preventDefault()
              void submit()
            }}
          >
            <div className="grid gap-4 sm:grid-cols-2">
              <label className={dialogLabelClass} htmlFor="coupon-batch-name">
                名称
                <input
                  id="coupon-batch-name"
                  className={dialogFieldClass}
                  value={editing.name}
                  onChange={e =>
                    setEditing({ ...editing, name: e.target.value })
                  }
                  placeholder="例如：新用户体验券"
                  data-autofocus
                />
              </label>
              <label
                className={dialogLabelClass}
                htmlFor="coupon-batch-distribution"
              >
                发放方式
                <select
                  id="coupon-batch-distribution"
                  className={dialogFieldClass}
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
              <label
                className={dialogLabelClass}
                htmlFor="coupon-batch-discount-type"
              >
                优惠类型
                <select
                  id="coupon-batch-discount-type"
                  className={dialogFieldClass}
                  value={editing.discountType}
                  onChange={e =>
                    setEditing({
                      ...editing,
                      discountType: e.target
                        .value as CouponBatch['discountType']
                    })
                  }
                >
                  <option value="fixed">固定减免</option>
                  <option value="percent">实付折扣</option>
                </select>
              </label>
              <label
                className={dialogLabelClass}
                htmlFor="coupon-batch-discount-value"
              >
                {editing.discountType === 'fixed'
                  ? '减免金额（分）'
                  : '实付折数（95=95折）'}
                <input
                  id="coupon-batch-discount-value"
                  className={dialogFieldClass}
                  type="number"
                  min="0"
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
              <label
                className={dialogLabelClass}
                htmlFor="coupon-batch-issue-limit"
              >
                总发放量（0不限）
                <input
                  id="coupon-batch-issue-limit"
                  className={dialogFieldClass}
                  type="number"
                  min="0"
                  value={editing.issueLimit}
                  onChange={e =>
                    setEditing({
                      ...editing,
                      issueLimit: Number(e.target.value)
                    })
                  }
                />
              </label>
              <label
                className={dialogLabelClass}
                htmlFor="coupon-batch-user-limit"
              >
                每人限领
                <input
                  id="coupon-batch-user-limit"
                  className={dialogFieldClass}
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
              <label className={dialogLabelClass} htmlFor="coupon-batch-scope">
                适用范围
                <select
                  id="coupon-batch-scope"
                  className={dialogFieldClass}
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
              <label className={dialogLabelClass} htmlFor="coupon-batch-status">
                状态
                <select
                  id="coupon-batch-status"
                  className={dialogFieldClass}
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
            <section className="space-y-4 rounded-lg border border-slate-200 bg-slate-50 p-4 dark:border-slate-700 dark:bg-slate-900">
              <div className="grid gap-4 sm:grid-cols-2">
                <label
                  className={dialogLabelClass}
                  htmlFor="coupon-batch-plans"
                >
                  限定套餐（留空不限）
                  <select
                    id="coupon-batch-plans"
                    multiple
                    className={dialogFieldClass}
                    value={editing.planIDs}
                    onChange={event =>
                      setEditing({
                        ...editing,
                        planIDs: Array.from(
                          event.target.selectedOptions,
                          option => option.value
                        )
                      })
                    }
                  >
                    {missingPlanIDs.map(id => (
                      <option key={id} value={id}>
                        {id} · 当前已下架（可取消选择）
                      </option>
                    ))}
                    {(catalog.data?.plans ?? []).map(plan => (
                      <option key={plan.id} value={plan.id}>
                        {plan.name} · {plan.cpu} 核 / {plan.memoryMB / 1024} GB
                      </option>
                    ))}
                  </select>
                  <small className="mt-1.5 block font-normal text-slate-400">
                    按住 Ctrl 或 Command 可多选。
                  </small>
                </label>
                <label
                  className={dialogLabelClass}
                  htmlFor="coupon-batch-images"
                >
                  限定镜像（留空不限）
                  <select
                    id="coupon-batch-images"
                    multiple
                    className={dialogFieldClass}
                    value={editing.imageIDs}
                    onChange={event =>
                      setEditing({
                        ...editing,
                        imageIDs: Array.from(
                          event.target.selectedOptions,
                          option => option.value
                        )
                      })
                    }
                  >
                    {missingImageIDs.map(id => (
                      <option key={id} value={id}>
                        {id} · 当前已下架（可取消选择）
                      </option>
                    ))}
                    {(catalog.data?.images ?? []).map(image => (
                      <option key={image.id} value={image.id}>
                        {image.name} · {image.version}
                      </option>
                    ))}
                  </select>
                  <small className="mt-1.5 block font-normal text-slate-400">
                    按住 Ctrl 或 Command 可多选。
                  </small>
                </label>
              </div>
              {(editing.planIDs.length > 0 || editing.imageIDs.length > 0) && (
                <div className="flex flex-wrap gap-2">
                  {editing.planIDs.length > 0 && (
                    <Button
                      type="button"
                      tone="ghost"
                      className="min-h-0 px-1 py-0 text-[11px]"
                      onClick={() => setEditing({ ...editing, planIDs: [] })}
                    >
                      清除套餐限制
                    </Button>
                  )}
                  {editing.imageIDs.length > 0 && (
                    <Button
                      type="button"
                      tone="ghost"
                      className="min-h-0 px-1 py-0 text-[11px]"
                      onClick={() => setEditing({ ...editing, imageIDs: [] })}
                    >
                      清除镜像限制
                    </Button>
                  )}
                </div>
              )}
              <div>
                <div className="flex items-center justify-between gap-3">
                  <span className={dialogLabelClass}>限定周期（留空不限）</span>
                  {editing.monthValues.length > 0 && (
                    <Button
                      type="button"
                      tone="ghost"
                      className="min-h-0 px-1 py-0 text-[11px]"
                      onClick={() =>
                        setEditing({ ...editing, monthValues: [] })
                      }
                    >
                      清空选择
                    </Button>
                  )}
                </div>
                <div className="mt-2 grid grid-cols-4 gap-2 sm:grid-cols-6">
                  {Array.from({ length: 12 }, (_, index) =>
                    String(index + 1)
                  ).map(month => {
                    const selected = editing.monthValues.includes(month)
                    return (
                      <button
                        key={month}
                        type="button"
                        aria-pressed={selected}
                        onClick={() =>
                          setEditing({
                            ...editing,
                            monthValues: selected
                              ? editing.monthValues.filter(
                                  value => value !== month
                                )
                              : [...editing.monthValues, month]
                          })
                        }
                        className={`rounded-md border px-2 py-2 text-xs font-bold transition-colors focus-visible:outline-2 focus-visible:outline-blue-500 ${selected ? 'border-blue-600 bg-blue-600 text-white' : 'border-slate-200 bg-white text-slate-600 hover:border-blue-300 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-200'}`}
                      >
                        {month} 个月
                      </button>
                    )
                  })}
                </div>
              </div>
            </section>
            {error && <Alert tone="error">{error}</Alert>}
            <DialogFooter>
              <Button
                type="button"
                tone="secondary"
                onClick={() => setEditing(null)}
              >
                取消
              </Button>
              <Button
                type="submit"
                loading={isLoading}
                disabled={!editing.name.trim()}
              >
                保存
              </Button>
            </DialogFooter>
          </form>
        </Dialog>
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
export function AdminCouponRedemptionsPage() {
  const redemptions = useGetAdminCouponRedemptionsQuery()
  return (
    <section className="page super-page">
      <PageHeader
        eyebrow="营销运营"
        title="优惠核销记录"
        description="已核销记录不可修改，订单快照与钱包流水以此为准。"
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
        className={dialogFieldClass}
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
