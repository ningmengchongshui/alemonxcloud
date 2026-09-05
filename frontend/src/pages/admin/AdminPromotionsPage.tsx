import { Button, EmptyState, PageHeader, StatusBadge } from '@/components/ui'
import {
  useGetAdminCouponRedemptionsQuery,
  useGetAdminCouponsQuery,
  useGetAdminPromotionsQuery,
  useUpdateAdminCouponStatusMutation
} from '@/services/cloudApi'
import type { Promotion } from '@/types/cloud'

const kindLabel = (kind: Promotion['kind']) =>
  ({
    campaign: '活动优惠',
    newcomer: '新人专属',
    first_plan_purchase: '套餐新购优惠'
  })[kind]

export function AdminPromotionsPage({
  onCreate,
  onEdit
}: {
  onCreate?: () => void
  onEdit?: (id: string) => void
}) {
  const promotions = useGetAdminPromotionsQuery()
  const coupons = useGetAdminCouponsQuery()
  const redemptions = useGetAdminCouponRedemptionsQuery()
  const [status] = useUpdateAdminCouponStatusMutation()
  return (
    <section className="page super-page">
      <PageHeader
        eyebrow="营销运营"
        title="营销与优惠"
        description="新人专属、套餐首次选购与普通活动分开管理；活动券由用户领取后进入优惠券包。"
        actions={<Button onClick={onCreate}>＋ 引导式创建活动</Button>}
      />
      {(promotions.data ?? []).length === 0 ? (
        <EmptyState
          title="暂无活动"
          description="使用引导流程创建第一项营销活动。"
        />
      ) : (
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
                    <small className="block">{kindLabel(p.kind)}</small>
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
                    <button
                      className="text-button"
                      onClick={() => onEdit?.(p.id)}
                    >
                      编辑
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
      <h2 className="mt-8 text-lg font-bold">代金券</h2>
      {(coupons.data ?? []).length === 0 ? (
        <EmptyState
          title="暂无代金券"
          description="普通活动发布后可在其编辑页生成可领取代金券。"
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
                  <td>{c.mode === 'general' ? '通用券' : '单次券'}</td>
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
    </section>
  )
}
