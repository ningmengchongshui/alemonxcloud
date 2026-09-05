import { useState } from 'react'
import {
  Alert,
  Button,
  Dialog,
  EmptyState,
  PageHeader,
  StatusBadge
} from '@/components/ui'
import {
  useCreateAdminCouponsMutation,
  useGetAdminCouponRedemptionsQuery,
  useGetAdminCouponsQuery,
  useGetAdminCatalogQuery,
  useGetAdminPromotionsQuery,
  useSaveAdminPromotionMutation,
  useUpdateAdminCouponStatusMutation
} from '@/services/cloudApi'
import type { Promotion } from '@/types/cloud'

const formFieldClass =
  'mt-1 block w-full rounded-md border border-slate-300 bg-white px-3 py-2.5 text-sm font-normal text-slate-800 outline-none placeholder:text-slate-400 focus:border-blue-500 dark:border-slate-600 dark:bg-slate-900 dark:text-white'
const formLabelClass =
  'block text-[11px] font-bold text-slate-700 dark:text-slate-100'

const blank = (): Promotion => ({
  id: '',
  name: '',
  kind: 'campaign',
  scope: 'both',
  discountType: 'fixed',
  discountValue: 100,
  minAmountFen: 0,
  maxDiscountFen: 0,
  planIDs: [],
  imageIDs: [],
  monthValues: [],
  totalLimit: 0,
  perUserLimit: 1,
  usedCount: 0,
  enabled: true,
  createdAt: ''
})
export function AdminPromotionsPage() {
  const promotions = useGetAdminPromotionsQuery()
  const catalog = useGetAdminCatalogQuery()
  const coupons = useGetAdminCouponsQuery()
  const redemptions = useGetAdminCouponRedemptionsQuery()
  const [save, { isLoading: saving }] = useSaveAdminPromotionMutation()
  const [createCoupons] = useCreateAdminCouponsMutation()
  const [status] = useUpdateAdminCouponStatusMutation()
  const [editing, setEditing] = useState<Promotion | null>(null)
  const [codes, setCodes] = useState<string[]>([])
  const [couponMode, setCouponMode] = useState<'single' | 'general'>('single')
  const [couponCount, setCouponCount] = useState(1)
  const [couponLimit, setCouponLimit] = useState(100)
  const [couponUserLimit, setCouponUserLimit] = useState(1)
  const [error, setError] = useState('')
  const submit = async () => {
    if (!editing) return
    try {
      await save(editing).unwrap()
      setEditing(null)
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
          : '保存失败'
      setError(message)
    }
  }
  return (
    <section className="page super-page">
      <PageHeader
        eyebrow="营销运营"
        title="营销与优惠"
        description="管理新人、活动优惠及通用或批量单次代金券；订单完成后规则快照不可修改。"
        actions={
          <Button
            onClick={() => {
              setError('')
              setEditing(blank())
            }}
          >
            ＋ 新建活动
          </Button>
        }
      />
      <section className="admin-table-wrap">
        <table>
          <thead>
            <tr>
              <th>活动</th>
              <th>优惠</th>
              <th>适用</th>
              <th>核销</th>
              <th>状态</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {(promotions.data ?? []).map(p => (
              <tr key={p.id}>
                <td>
                  <b>{p.name}</b>
                  <small className="block">
                    {p.kind === 'new_user' ? '新人优惠' : '营销活动'}
                  </small>
                </td>
                <td>
                  {p.discountType === 'fixed'
                    ? `减 ${(p.discountValue / 100).toFixed(2)} XCoin`
                    : `${(p.discountValue / 100).toFixed(2)} 折`}
                </td>
                <td>
                  {p.scope === 'both'
                    ? '新购与续费'
                    : p.scope === 'purchase'
                      ? '仅新购'
                      : '仅续费'}
                </td>
                <td>
                  {p.usedCount}
                  {p.totalLimit ? ` / ${p.totalLimit}` : ' / 不限'}
                </td>
                <td>
                  <StatusBadge tone={p.enabled ? 'success' : 'neutral'}>
                    {p.enabled ? '启用' : '停用'}
                  </StatusBadge>
                </td>
                <td>
                  <button className="text-button" onClick={() => setEditing(p)}>
                    编辑
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
      <h2 className="mt-8 text-lg font-bold">代金券</h2>
      {(coupons.data ?? []).length === 0 ? (
        <EmptyState
          title="暂无代金券"
          description="先创建一个营销活动，再生成通用码或批量单次码。"
        />
      ) : (
        <section className="admin-table-wrap">
          <table>
            <thead>
              <tr>
                <th>券码</th>
                <th>模式</th>
                <th>核销</th>
                <th>状态</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {(coupons.data ?? []).map(c => (
                <tr key={c.id}>
                  <td>{c.codeMask}</td>
                  <td>{c.mode === 'general' ? '通用码' : '单次码'}</td>
                  <td>
                    {c.usedCount} / {c.totalLimit}
                  </td>
                  <td>{c.enabled ? '启用' : '停用'}</td>
                  <td>
                    <button
                      className="text-button"
                      onClick={() =>
                        void status({ id: c.id, enabled: !c.enabled })
                      }
                    >
                      {c.enabled ? '停用' : '启用'}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
      <h2 className="mt-8 text-lg font-bold">最近核销</h2>
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
            {(redemptions.data ?? []).slice(0, 20).map(item => (
              <tr key={item.id}>
                <td>{item.orderId.slice(0, 14)}</td>
                <td>{item.ownerId}</td>
                <td>{(item.discountAmountFen / 100).toFixed(2)} XCoin</td>
                <td>{new Date(item.createdAt).toLocaleString('zh-CN')}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
      {editing && (
        <Dialog
          title={editing.id ? '编辑活动' : '新建活动'}
          description="固定金额单位为分；折扣值为万分比（9500 表示 95 折）。"
          onClose={() => { setEditing(null); setError('') }}
          className="max-w-2xl"
        >
          <form className="space-y-4" onSubmit={event => { event.preventDefault(); void submit() }}>
            <div className="grid grid-cols-2 gap-3 max-[560px]:grid-cols-1">
            <label className={formLabelClass}>
              名称
              <input
                data-autofocus
                className={formFieldClass}
                value={editing.name}
                onChange={e => setEditing({ ...editing, name: e.target.value })}
              />
            </label>
            <label className={formLabelClass}>
              类型
              <select
                className={formFieldClass}
                value={editing.kind}
                onChange={e =>
                  setEditing({
                    ...editing,
                    kind: e.target.value as Promotion['kind']
                  })
                }
              >
                <option value="campaign">活动优惠</option>
                <option value="new_user">新人优惠</option>
              </select>
            </label>
            <label className={formLabelClass}>
              适用范围
              <select
                className={formFieldClass}
                value={editing.scope}
                onChange={e =>
                  setEditing({
                    ...editing,
                    scope: e.target.value as Promotion['scope']
                  })
                }
              >
                <option value="both">新购与续费</option>
                <option value="purchase">仅新购</option>
                <option value="renewal">仅续费</option>
              </select>
            </label>
            <label className={formLabelClass}>
              规则
              <select
                className={formFieldClass}
                value={editing.discountType}
                onChange={e =>
                  setEditing({
                    ...editing,
                    discountType: e.target.value as Promotion['discountType']
                  })
                }
              >
                <option value="fixed">固定减免</option>
                <option value="percent">比例折扣</option>
              </select>
            </label>
            </div>
            <div className="grid grid-cols-3 gap-3 max-[560px]:grid-cols-1">
            <label className={formLabelClass}>
              优惠值
              <input
                className={formFieldClass}
                type="number"
                value={editing.discountValue}
                onChange={e =>
                  setEditing({
                    ...editing,
                    discountValue: Number(e.target.value)
                  })
                }
              />
            </label>
            <label className={formLabelClass}>
              最低消费（分，0 为不限）
              <input
                className={formFieldClass}
                type="number"
                value={editing.minAmountFen}
                onChange={e =>
                  setEditing({
                    ...editing,
                    minAmountFen: Number(e.target.value)
                  })
                }
              />
            </label>
            <label className={formLabelClass}>
              总核销上限（0 为不限）
              <input
                className={formFieldClass}
                type="number"
                value={editing.totalLimit}
                onChange={e =>
                  setEditing({ ...editing, totalLimit: Number(e.target.value) })
                }
              />
            </label></div>
            <div className="grid grid-cols-3 gap-3 max-[560px]:grid-cols-1">
              <label className={formLabelClass}>最高减免（分，0 为不限）<input className={formFieldClass} type="number" value={editing.maxDiscountFen} onChange={e => setEditing({ ...editing, maxDiscountFen: Number(e.target.value) })} /></label>
              <label className={formLabelClass}>每用户活动限额（0 为不限）<input className={formFieldClass} type="number" value={editing.perUserLimit} onChange={e => setEditing({ ...editing, perUserLimit: Number(e.target.value) })} /></label>
              <label className={formLabelClass}>状态<select className={formFieldClass} value={editing.enabled ? 'enabled' : 'disabled'} onChange={e => setEditing({ ...editing, enabled: e.target.value === 'enabled' })}><option value="enabled">启用</option><option value="disabled">停用</option></select></label>
            </div>
            <div className="grid grid-cols-2 gap-3 max-[560px]:grid-cols-1">
              <label className={formLabelClass}>开始时间（留空立即生效）<input className={formFieldClass} type="datetime-local" value={editing.startsAt?.slice(0, 16) ?? ''} onChange={e => setEditing({ ...editing, startsAt: e.target.value ? new Date(e.target.value).toISOString() : undefined })} /></label>
              <label className={formLabelClass}>结束时间（留空不限）<input className={formFieldClass} type="datetime-local" value={editing.endsAt?.slice(0, 16) ?? ''} onChange={e => setEditing({ ...editing, endsAt: e.target.value ? new Date(e.target.value).toISOString() : undefined })} /></label>
            </div>
            <div className="grid grid-cols-2 gap-3 max-[560px]:grid-cols-1">
            <label className={formLabelClass}>
              适用套餐（留空不限）
              <select multiple
                className={formFieldClass}
                value={editing.planIDs}
                onChange={e =>
                  setEditing({
                    ...editing,
                    planIDs: Array.from(e.target.selectedOptions, option => option.value)
                  })
                }
              >{(catalog.data?.plans ?? []).map(plan => <option key={plan.id} value={plan.id}>{plan.name}</option>)}</select>
            </label>
            <label className={formLabelClass}>
              适用镜像（留空不限）
              <select multiple
                className={formFieldClass}
                value={editing.imageIDs}
                onChange={e =>
                  setEditing({
                    ...editing,
                    imageIDs: Array.from(e.target.selectedOptions, option => option.value)
                  })
                }
              >{(catalog.data?.images ?? []).map(image => <option key={image.id} value={image.id}>{image.name} · {image.version}</option>)}</select>
            </label></div>
            <label className={formLabelClass}>
              适用月数（逗号分隔，留空不限）
              <input
                className={formFieldClass}
                value={editing.monthValues.join(',')}
                onChange={e =>
                  setEditing({
                    ...editing,
                    monthValues: e.target.value
                      .split(',')
                      .map(x => x.trim())
                      .filter(Boolean)
                  })
                }
              />
            </label>
            {error && <Alert tone="error">{error}</Alert>}
            <div className="flex justify-end gap-2">
              <Button type="button" tone="secondary" onClick={() => { setEditing(null); setError('') }}>
                取消
              </Button>
              <Button type="submit" loading={saving} disabled={!editing.name.trim()}>保存活动</Button>
            </div>
            {editing.id && editing.kind === 'campaign' && (
              <div className="grid gap-3 rounded-lg border border-slate-200 p-3 dark:border-slate-700">
                <b className="text-xs">生成代金券</b>
                <div className="grid grid-cols-4 gap-2 max-[560px]:grid-cols-2">
                  <label className={formLabelClass}>类型<select className={formFieldClass} value={couponMode} onChange={e => setCouponMode(e.target.value as 'single' | 'general')}><option value="single">批量单次券</option><option value="general">通用码</option></select></label>
                  <label className={formLabelClass}>数量<input className={formFieldClass} type="number" min="1" max="500" disabled={couponMode === 'general'} value={couponCount} onChange={e => setCouponCount(Number(e.target.value))} /></label>
                  <label className={formLabelClass}>总次数<input className={formFieldClass} type="number" min="1" disabled={couponMode === 'single'} value={couponLimit} onChange={e => setCouponLimit(Number(e.target.value))} /></label>
                  <label className={formLabelClass}>每用户次数<input className={formFieldClass} type="number" min="1" disabled={couponMode === 'single'} value={couponUserLimit} onChange={e => setCouponUserLimit(Number(e.target.value))} /></label>
                </div>
                <Button
                  type="button"
                  tone="secondary"
                  onClick={() =>
                    void createCoupons({
                      promotionId: editing.id,
                      mode: couponMode,
                      count: couponMode === 'general' ? 1 : couponCount,
                      totalLimit: couponMode === 'single' ? 1 : couponLimit,
                      perUserLimit: couponMode === 'single' ? 1 : couponUserLimit
                    })
                      .unwrap()
                      .then(r => setCodes(r.coupons.map(x => x.code)))
                  }
                >
                  生成券码
                </Button>
              </div>
            )}
            {codes.length > 0 && <p className="rounded-lg bg-amber-50 p-3 text-xs text-amber-800 dark:bg-amber-950 dark:text-amber-100">请立即复制券码：{codes.join('，')}</p>}
          </form>
        </Dialog>
      )}
    </section>
  )
}
