import { useState } from 'react'
import {
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
  useGetAdminPromotionsQuery,
  useSaveAdminPromotionMutation,
  useUpdateAdminCouponStatusMutation
} from '@/services/cloudApi'
import type { Promotion } from '@/types/cloud'

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
  const coupons = useGetAdminCouponsQuery()
  const redemptions = useGetAdminCouponRedemptionsQuery()
  const [save] = useSaveAdminPromotionMutation()
  const [createCoupons] = useCreateAdminCouponsMutation()
  const [status] = useUpdateAdminCouponStatusMutation()
  const [editing, setEditing] = useState<Promotion | null>(null)
  const [codes, setCodes] = useState<string[]>([])
  const [error, setError] = useState('')
  const submit = async () => {
    if (!editing) return
    try {
      await save(editing).unwrap()
      setEditing(null)
    } catch (e: any) {
      setError(e?.data?.message ?? '保存失败')
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
          eyebrow="优惠活动"
          title={editing.id ? '编辑活动' : '新建活动'}
          description="固定金额单位为分；折扣值为万分比（9500 表示 95 折）。"
          onClose={() => setEditing(null)}
        >
          <div className="grid gap-3">
            <label>
              名称
              <input
                value={editing.name}
                onChange={e => setEditing({ ...editing, name: e.target.value })}
              />
            </label>
            <label>
              类型
              <select
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
            <label>
              适用范围
              <select
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
            <label>
              规则
              <select
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
            <label>
              优惠值
              <input
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
            <label>
              最低消费（分，0 为不限）
              <input
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
            <label>
              总核销上限（0 为不限）
              <input
                type="number"
                value={editing.totalLimit}
                onChange={e =>
                  setEditing({ ...editing, totalLimit: Number(e.target.value) })
                }
              />
            </label>
            <label>
              适用套餐 ID（逗号分隔，留空不限）
              <input
                value={editing.planIDs.join(',')}
                onChange={e =>
                  setEditing({
                    ...editing,
                    planIDs: e.target.value
                      .split(',')
                      .map(x => x.trim())
                      .filter(Boolean)
                  })
                }
              />
            </label>
            <label>
              适用镜像 ID（逗号分隔，留空不限）
              <input
                value={editing.imageIDs.join(',')}
                onChange={e =>
                  setEditing({
                    ...editing,
                    imageIDs: e.target.value
                      .split(',')
                      .map(x => x.trim())
                      .filter(Boolean)
                  })
                }
              />
            </label>
            <label>
              适用月数（逗号分隔，留空不限）
              <input
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
            {error && <p className="login-error">{error}</p>}
            <div className="flex justify-end gap-2">
              <Button tone="secondary" onClick={() => setEditing(null)}>
                取消
              </Button>
              <Button onClick={() => void submit()}>保存活动</Button>
            </div>
            {editing.id && editing.kind === 'campaign' && (
              <div className="flex gap-2">
                <Button
                  tone="secondary"
                  onClick={() =>
                    void createCoupons({
                      promotionId: editing.id,
                      mode: 'single',
                      count: 1
                    })
                      .unwrap()
                      .then(r => setCodes(r.coupons.map(x => x.code)))
                  }
                >
                  生成单次券
                </Button>
                <Button
                  tone="secondary"
                  onClick={() =>
                    void createCoupons({
                      promotionId: editing.id,
                      mode: 'general',
                      count: 1,
                      totalLimit: 100
                    })
                      .unwrap()
                      .then(r => setCodes(r.coupons.map(x => x.code)))
                  }
                >
                  生成通用码
                </Button>
              </div>
            )}
            {codes.length > 0 && <p>请立即复制券码：{codes.join('，')}</p>}
          </div>
        </Dialog>
      )}
    </section>
  )
}
