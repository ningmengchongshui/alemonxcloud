import { useEffect, useState } from 'react'
import {
  Alert,
  Button,
  EmptyState,
  LoadingState,
  PageHeader
} from '@/components/ui'
import {
  useGetAdminCatalogQuery,
  useGetAdminPlanPriceTiersQuery,
  useSaveAdminPlanPriceTierMutation
} from '@/services/cloudApi'
import type { PlanPriceTier } from '@/types/cloud'

const periods = [3, 6, 12]
const money = (fen: number) => `¥${(fen / 100).toFixed(2)}`
const keyFor = (planID: string, months: number) => `${planID}:${months}`

export function AdminPlanPriceTiersPage() {
  const catalog = useGetAdminCatalogQuery()
  const tiers = useGetAdminPlanPriceTiersQuery()
  const [save, { isLoading: saving }] = useSaveAdminPlanPriceTierMutation()
  const [drafts, setDrafts] = useState<Record<string, string>>({})
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')
  const plans = catalog.data?.plans ?? []
  const values = tiers.data ?? []

  useEffect(() => {
    const next: Record<string, string> = {}
    for (const tier of tiers.data ?? [])
      next[keyFor(tier.planId, tier.months)] = String(tier.discountBps / 1000)
    setDrafts(next)
  }, [tiers.data])

  function tierFor(planID: string, months: number): PlanPriceTier | undefined {
    return values.find(tier => tier.planId === planID && tier.months === months)
  }
  function saveTier(planID: string, months: number) {
    const key = keyFor(planID, months)
    const raw = drafts[key]?.trim() ?? ''
    const plan = plans.find(item => item.id === planID)
    if (!plan) return
    if (!raw) {
      setError('请输入折扣；留空会使用月价 × 周期的原价。')
      return
    }
    const discount = Number(raw)
    if (!Number.isFinite(discount) || discount < 0 || discount > 10) {
      setError('请输入 0 至 10 之间的折扣值，例如 8.5 表示 8.5 折。')
      return
    }
    setError('')
    setSuccess('')
    const existing = tierFor(planID, months)
    void save({
      id: existing?.id ?? '',
      planId: planID,
      months,
      discountBps: Math.round(discount * 1000),
      enabled: true
    })
      .unwrap()
      .then(() => {
        setSuccess(`${plan.name} ${months} 个月阶梯折扣已保存。`)
        void tiers.refetch()
      })
      .catch(value =>
        setError(value?.data?.message ?? '保存失败，请稍后重试。')
      )
  }

  return (
    <section className="page super-page">
      <PageHeader
        eyebrow="营销运营"
        title="套餐"
        description="设置 3、6、12 个月的折扣值；例如 8.5 表示 8.5 折，未设置时按原价计算。"
        actions={
          <Button
            tone="secondary"
            loading={catalog.isFetching || tiers.isFetching}
            onClick={() => {
              void catalog.refetch()
              void tiers.refetch()
            }}
          >
            ↻ 刷新
          </Button>
        }
      />
      {error && <Alert tone="error">{error}</Alert>}
      {success && <Alert tone="success">{success}</Alert>}
      {catalog.isLoading || tiers.isLoading ? (
        <LoadingState>正在加载套餐与阶梯定价…</LoadingState>
      ) : plans.length === 0 ? (
        <EmptyState
          title="暂无可定价套餐"
          description="先在商品目录创建可售套餐，才能配置阶梯价格。"
        />
      ) : (
        <section className="overflow-hidden rounded-xl border border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-800">
          <div className="grid grid-cols-[minmax(9rem,1fr)_repeat(3,minmax(8rem,auto))] gap-4 border-b border-slate-100 bg-slate-50 px-5 py-3 text-[10px] font-extrabold tracking-wide text-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300">
            <span>套餐</span>
            {periods.map(months => (
              <span key={months}>{months} 个月折扣</span>
            ))}
          </div>
          <div className="divide-y divide-slate-100 dark:divide-slate-700">
            {plans.map(plan => (
              <article
                key={plan.id}
                className="grid gap-3 px-5 py-4 max-[680px]:grid-cols-1 sm:grid-cols-[minmax(9rem,1fr)_repeat(3,minmax(8rem,auto))] sm:items-center sm:gap-4"
              >
                <div>
                  <b className="text-sm text-slate-800 dark:text-white">
                    {plan.name}
                  </b>
                  <p className="mb-0 mt-1 text-[11px] text-slate-500 dark:text-slate-300">
                    月价 {money(plan.monthlyPriceFen)} · {plan.cpu} 核 /{' '}
                    {plan.memoryMB / 1024} GB
                  </p>
                </div>
                {periods.map(months => {
                  const key = keyFor(plan.id, months)
                  const tier = tierFor(plan.id, months)
                  return (
                    <label
                      key={months}
                      className="grid gap-1 text-[10px] text-slate-500 dark:text-slate-300"
                    >
                      <span className="sm:hidden">{months} 个月折扣</span>
                      <div className="flex items-center gap-1.5">
                        <input
                          className="min-w-0 w-24 rounded-md border border-slate-300 bg-white px-2 py-2 text-xs text-slate-800 outline-none placeholder:text-slate-400 focus:border-blue-500 focus:ring-2 focus:ring-blue-100 dark:border-slate-600 dark:bg-slate-900 dark:text-white dark:focus:ring-blue-950"
                          inputMode="decimal"
                          value={drafts[key] ?? ''}
                          placeholder="10"
                          onChange={event =>
                            setDrafts(current => ({
                              ...current,
                              [key]: event.target.value
                            }))
                          }
                        />
                        <span>折</span>
                        <Button
                          tone="secondary"
                          className="min-h-8 px-2 text-[10px]"
                          loading={saving}
                          onClick={() => saveTier(plan.id, months)}
                        >
                          保存
                        </Button>
                      </div>
                      {tier && (
                        <small className="text-emerald-700 dark:text-emerald-200">
                          已启用 · {tier.discountBps / 1000} 折
                        </small>
                      )}
                    </label>
                  )
                })}
              </article>
            ))}
          </div>
        </section>
      )}
    </section>
  )
}
