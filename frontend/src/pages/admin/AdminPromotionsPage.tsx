import { Button, EmptyState, PageHeader, StatusBadge } from '@/components/ui'
import { useGetAdminPromotionsQuery } from '@/services/cloudApi'
import type { Promotion } from '@/types/cloud'

const kindMeta: Record<
  Promotion['kind'],
  { label: string; description: string; tone: string }
> = {
  campaign: {
    label: '普通活动',
    description: '可用于新购、续费，并可发放领取券。',
    tone: 'border-violet-200 bg-violet-50 text-violet-700 dark:border-violet-900 dark:bg-violet-950 dark:text-violet-200'
  },
  newcomer: {
    label: '新人专属',
    description: '首次成功购买任意套餐时可用。',
    tone: 'border-sky-200 bg-sky-50 text-sky-700 dark:border-sky-900 dark:bg-sky-950 dark:text-sky-200'
  },
  first_plan_purchase: {
    label: '套餐新购',
    description: '每个套餐的首次购买可单独享受。',
    tone: 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900 dark:bg-amber-950 dark:text-amber-200'
  }
}
const legacyNewUserMeta = kindMeta.newcomer
export function AdminPromotionsPage({
  onCreate,
  onEdit
}: {
  onCreate?: () => void
  onEdit?: (id: string) => void
}) {
  const promotions = useGetAdminPromotionsQuery()
  const values = promotions.data ?? []
  return (
    <section className="page super-page">
      <PageHeader
        eyebrow="营销运营"
        title="营销与优惠"
        description="配置活动、发放代金券，并追踪每一笔优惠核销。"
        actions={<Button onClick={onCreate}>＋ 创建活动</Button>}
      />
      <div className="mb-3 flex items-end justify-between">
        <div>
          <h2 className="text-lg font-bold">活动中心</h2>
          <p className="mt-1 text-xs text-slate-500">
            点击活动可调整规则；活动类型决定其资格校验。
          </p>
        </div>
        <span className="text-xs text-slate-500">
          共 {values.length} 个活动
        </span>
      </div>
      {values.length === 0 ? (
        <EmptyState
          title="暂无营销活动"
          description="从“创建活动”开始设置第一项优惠。"
        />
      ) : (
        <section className="grid gap-4 xl:grid-cols-2">
          {values.map(item => {
            const meta = kindMeta[item.kind] ?? legacyNewUserMeta
            return (
              <article
                key={item.id}
                className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-800"
              >
                <div className="flex items-start justify-between gap-4">
                  <div>
                    <span
                      className={`inline-flex rounded-full border px-2 py-1 text-[10px] font-extrabold ${meta.tone}`}
                    >
                      {meta.label}
                    </span>
                    <h3 className="mt-3 text-base font-bold">{item.name}</h3>
                    <p className="mt-1 text-xs text-slate-500">
                      {meta.description}
                    </p>
                  </div>
                  <StatusBadge tone={item.enabled ? 'success' : 'neutral'}>
                    {item.enabled ? '启用中' : '已停用'}
                  </StatusBadge>
                </div>
                <div className="mt-5 grid grid-cols-3 gap-2 rounded-lg bg-slate-50 p-3 text-xs dark:bg-slate-900">
                  <div>
                    <small className="block text-slate-500">优惠</small>
                    <b>
                      {item.discountType === 'fixed'
                        ? `减 ${(item.discountValue / 100).toFixed(2)}`
                        : `${(item.discountValue / 100).toFixed(2)} 折`}
                    </b>
                  </div>
                  <div>
                    <small className="block text-slate-500">范围</small>
                    <b>
                      {item.scope === 'both'
                        ? '新购与续费'
                        : item.scope === 'purchase'
                          ? '仅新购'
                          : '仅续费'}
                    </b>
                  </div>
                  <div>
                    <small className="block text-slate-500">核销</small>
                    <b>
                      {item.usedCount}
                      {item.totalLimit ? ` / ${item.totalLimit}` : ' / 不限'}
                    </b>
                  </div>
                </div>
                <div className="mt-4 flex justify-end">
                  <button
                    className="text-button"
                    onClick={() => onEdit?.(item.id)}
                  >
                    编辑活动
                  </button>
                </div>
              </article>
            )
          })}
        </section>
      )}
    </section>
  )
}
