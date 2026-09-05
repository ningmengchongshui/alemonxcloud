import { useState } from 'react'
import {
  Alert,
  Button,
  EmptyState,
  PageHeader,
  StatusBadge
} from '@/components/ui'
import {
  useClaimCouponBatchMutation,
  useGetMyCouponsQuery,
  useGetPublicCouponBatchesQuery
} from '@/services/cloudApi'

const money = (fen: number) => `${(fen / 100).toFixed(2)} XCoin`
export function OffersPage() {
  const [tab, setTab] = useState<'public' | 'wallet'>('public')
  const batches = useGetPublicCouponBatchesQuery()
  const wallet = useGetMyCouponsQuery()
  const [claim] = useClaimCouponBatchMutation()
  const [error, setError] = useState('')
  return (
    <section className="page">
      <PageHeader
        eyebrow="费用中心"
        title="优惠中心"
        description="领取公开代金券，或查看自己账户中的待使用优惠券。"
      />
      <div className="mb-5 flex gap-2 border-b border-slate-200 dark:border-slate-700">
        <button
          className={`px-4 py-3 text-sm font-bold ${tab === 'public' ? 'border-b-2 border-blue-600 text-blue-700' : 'text-slate-500'}`}
          onClick={() => setTab('public')}
        >
          可领取
        </button>
        <button
          className={`px-4 py-3 text-sm font-bold ${tab === 'wallet' ? 'border-b-2 border-blue-600 text-blue-700' : 'text-slate-500'}`}
          onClick={() => setTab('wallet')}
        >
          我的优惠券
        </button>
      </div>
      {error && <Alert tone="error">{error}</Alert>}
      {tab === 'public' ? (
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
              </article>
            )
          })}
        </section>
      )}
    </section>
  )
}
