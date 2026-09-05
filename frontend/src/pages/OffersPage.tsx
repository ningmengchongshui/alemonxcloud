import { useState } from 'react'
import {
  Alert,
  Button,
  Dialog,
  DialogFooter,
  EmptyState,
  FilterTabs,
  PageHeader,
  StatusBadge
} from '@/components/ui'
import { XCoinAmount } from '@/components/XCoinMark'
import {
  useClaimCouponBatchMutation,
  useGetMyCouponsQuery,
  useGetPromotionsQuery,
  useGetPublicCouponBatchesQuery
} from '@/services/cloudApi'
import type { Promotion } from '@/types/cloud'

const money = (fen: number) => `${(fen / 100).toFixed(2)}`
const scopeText = (scope: Promotion['scope']) =>
  scope === 'both' ? '新购与续费' : scope === 'purchase' ? '仅新购' : '仅续费'
const ruleList = (values: string[], fallback: string) =>
  values.length ? values.join('、') : fallback
const couponConditions = (item: {
  batch?: {
    scope: 'purchase' | 'renewal' | 'both'
    minAmountFen: number
    planIDs: string[]
    imageIDs: string[]
    monthValues: string[]
  }
}) => {
  const batch = item.batch
  if (!batch) return '规则以订单快照为准'
  const values = [scopeText(batch.scope)]
  if (batch.minAmountFen > 0)
    values.push(`满 ${money(batch.minAmountFen)} 可用`)
  if (batch.planIDs.length) values.push(`限定套餐：${batch.planIDs.join('、')}`)
  if (batch.imageIDs.length)
    values.push(`限定镜像：${batch.imageIDs.join('、')}`)
  if (batch.monthValues.length)
    values.push(`限定周期：${batch.monthValues.join('、')} 个月`)
  return values.join(' · ')
}

export function OffersPage() {
  const [tab, setTab] = useState<'activities' | 'public' | 'wallet'>(
    'activities'
  )
  const [activePromotion, setActivePromotion] = useState<Promotion | null>(null)
  const batches = useGetPublicCouponBatchesQuery()
  const wallet = useGetMyCouponsQuery()
  const promotions = useGetPromotionsQuery()
  const [claim] = useClaimCouponBatchMutation()
  const [error, setError] = useState('')
  return (
    <section className="page">
      <PageHeader
        eyebrow="费用中心"
        title="优惠中心"
        description="查看平台活动、领取优惠券，并在结算时主动选择要使用的一张券。"
      />
      <div className="mb-5">
        <FilterTabs
          value={tab}
          onChange={setTab}
          label="优惠中心分类"
          items={[
            { value: 'activities', label: '活动' },
            { value: 'public', label: '可领取优惠券' },
            { value: 'wallet', label: '我的优惠券' }
          ]}
        />
      </div>
      {error && <Alert tone="error">{error}</Alert>}
      {tab === 'activities' ? (
        (promotions.data ?? []).length === 0 ? (
          <EmptyState
            title="暂无进行中的活动"
            description="后续活动将在这里公布具体规则与适用项目。"
          />
        ) : (
          <section className="grid gap-4 md:grid-cols-2">
            {(promotions.data ?? []).map(item => (
              <article
                key={item.id}
                className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-800"
              >
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <StatusBadge tone="success">进行中</StatusBadge>
                    <h2 className="mb-0 mt-3 text-lg font-bold">{item.name}</h2>
                  </div>
                  <span className="shrink-0 text-sm font-bold text-emerald-700 dark:text-emerald-200">
                    {item.discountType === 'fixed' ? (
                      <>
                        立减 <XCoinAmount value={money(item.discountValue)} />
                      </>
                    ) : (
                      `${item.discountValue / 100} 折`
                    )}
                  </span>
                </div>
                <p className="mb-0 mt-3 text-xs leading-5 text-slate-500 dark:text-slate-300">
                  {scopeText(item.scope)} ·{' '}
                  {item.minAmountFen ? (
                    <>
                      满 <XCoinAmount value={money(item.minAmountFen)} /> 可用
                    </>
                  ) : (
                    '无最低消费门槛'
                  )}
                </p>
                <Button
                  tone="secondary"
                  className="mt-4"
                  onClick={() => setActivePromotion(item)}
                >
                  查看活动规则
                </Button>
              </article>
            ))}
          </section>
        )
      ) : tab === 'public' ? (
        (batches.data ?? []).length === 0 ? (
          <EmptyState
            title="暂无可领取代金券"
            description="新的公开代金券将在这里展示。"
          />
        ) : (
          <section className="grid gap-4 md:grid-cols-2">
            {(batches.data ?? []).map(item => (
              <article
                key={item.id}
                className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-800"
              >
                <StatusBadge tone="success">可领取</StatusBadge>
                <h2 className="mt-3 text-lg font-bold">{item.name}</h2>
                <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                  {item.discountType === 'fixed'
                    ? `立减 ${money(item.discountValue)}`
                    : `${item.discountValue / 100} 折`}
                  {item.minAmountFen
                    ? ` · 满 ${money(item.minAmountFen)} 可用`
                    : ''}
                </p>
                <p className="mt-2 text-xs text-slate-500">
                  {item.scope === 'both'
                    ? '新购与续费可用'
                    : item.scope === 'purchase'
                      ? '仅新购可用'
                      : '仅续费可用'}{' '}
                  · 每人限领 {item.perUserLimit} 张
                </p>
                <Button
                  className="mt-4"
                  onClick={() =>
                    void claim(item.id)
                      .unwrap()
                      .then(() => {
                        setTab('wallet')
                        setError('')
                      })
                      .catch(e =>
                        setError(
                          typeof e?.data?.message === 'string'
                            ? e.data.message
                            : '领取失败'
                        )
                      )
                  }
                >
                  领取到我的券包
                </Button>
              </article>
            ))}
          </section>
        )
      ) : (wallet.data ?? []).length === 0 ? (
        <EmptyState
          title="你的券包为空"
          description="领取公开券或等待管理员定向发券后，会在这里显示。"
        />
      ) : (
        <section className="grid gap-4 md:grid-cols-2">
          {(wallet.data ?? []).map(item => {
            const batch = item.batch
            return (
              <article
                key={item.id}
                className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-700 dark:bg-slate-800"
              >
                <div className="flex justify-between gap-3">
                  <div>
                    <h2 className="text-base font-bold">
                      {batch?.name ?? '历史代金券'}
                    </h2>
                    <p className="mt-2 text-sm">
                      {batch
                        ? batch.discountType === 'fixed'
                          ? `立减 ${money(batch.discountValue)}`
                          : `${batch.discountValue / 100} 折`
                        : '规则以订单快照为准'}
                    </p>
                  </div>
                  <StatusBadge
                    tone={item.status === 'available' ? 'success' : 'neutral'}
                  >
                    {item.status === 'available'
                      ? '待使用'
                      : item.status === 'used'
                        ? '已使用'
                        : item.status === 'expired'
                          ? '已过期'
                          : '已作废'}
                  </StatusBadge>
                </div>
                <p className="mt-4 text-xs text-slate-500">
                  {item.expiresAt
                    ? `有效至 ${new Date(item.expiresAt).toLocaleString('zh-CN')}`
                    : '长期有效'}{' '}
                  ·{' '}
                  {item.issueSource === 'targeted'
                    ? '管理员发放'
                    : item.issueSource === 'public'
                      ? '公开领取'
                      : '历史迁移'}
                </p>
                <p className="mb-0 mt-2 text-[11px] leading-5 text-slate-400 dark:text-slate-300">
                  使用条件：{couponConditions(item)}
                </p>
              </article>
            )
          })}
        </section>
      )}
      {activePromotion && (
        <Dialog
          eyebrow="平台活动"
          title={activePromotion.name}
          description="活动优惠不会自动套用；符合条件时，请在结算页主动选择可用优惠券。"
          onClose={() => setActivePromotion(null)}
        >
          <dl className="grid gap-3 text-xs">
            <div className="flex justify-between gap-4">
              <dt className="text-slate-500">优惠内容</dt>
              <dd className="m-0 text-right font-bold">
                {activePromotion.discountType === 'fixed' ? (
                  <>
                    立减{' '}
                    <XCoinAmount value={money(activePromotion.discountValue)} />
                  </>
                ) : (
                  `${activePromotion.discountValue / 100} 折`
                )}
              </dd>
            </div>
            <div className="flex justify-between gap-4">
              <dt className="text-slate-500">适用订单</dt>
              <dd className="m-0 text-right font-bold">
                {scopeText(activePromotion.scope)}
              </dd>
            </div>
            <div className="flex justify-between gap-4">
              <dt className="text-slate-500">涉及套餐</dt>
              <dd className="m-0 text-right font-bold">
                {ruleList(activePromotion.planIDs, '全部套餐')}
              </dd>
            </div>
            <div className="flex justify-between gap-4">
              <dt className="text-slate-500">涉及镜像</dt>
              <dd className="m-0 text-right font-bold">
                {ruleList(activePromotion.imageIDs, '全部镜像')}
              </dd>
            </div>
            <div className="flex justify-between gap-4">
              <dt className="text-slate-500">适用周期</dt>
              <dd className="m-0 text-right font-bold">
                {ruleList(
                  activePromotion.monthValues.map(value => `${value} 个月`),
                  '全部周期'
                )}
              </dd>
            </div>
            <div className="flex justify-between gap-4">
              <dt className="text-slate-500">活动时间</dt>
              <dd className="m-0 text-right font-bold">
                {activePromotion.startsAt
                  ? new Date(activePromotion.startsAt).toLocaleString('zh-CN')
                  : '即日起'}{' '}
                至{' '}
                {activePromotion.endsAt
                  ? new Date(activePromotion.endsAt).toLocaleString('zh-CN')
                  : '长期有效'}
              </dd>
            </div>
          </dl>
          <DialogFooter>
            <Button tone="secondary" onClick={() => setActivePromotion(null)}>
              关闭
            </Button>
          </DialogFooter>
        </Dialog>
      )}
    </section>
  )
}
