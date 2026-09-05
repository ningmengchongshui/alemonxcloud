import classNames from 'classnames'
import { XCoinAmount } from '@/components/XCoinMark'
import { useGetMyCouponsQuery } from '@/services/cloudApi'
import type { PriceQuote, UserCoupon } from '@/types/cloud'

function couponRule(coupon: UserCoupon) {
  const batch = coupon.batch
  if (!batch) return '规则以订单快照为准'
  return batch.discountType === 'fixed'
    ? `立减 ${(batch.discountValue / 100).toFixed(2)}`
    : `${batch.discountValue / 100} 折`
}

function couponConditions(coupon: UserCoupon) {
  const batch = coupon.batch
  if (!batch) return []
  const scope =
    batch.scope === 'both'
      ? '新购与续费'
      : batch.scope === 'purchase'
        ? '仅新购'
        : '仅续费'
  const conditions = [scope]
  if (batch.minAmountFen > 0)
    conditions.push(`满 ${(batch.minAmountFen / 100).toFixed(2)} 可用`)
  if (batch.planIDs.length)
    conditions.push(`限定套餐：${batch.planIDs.join('、')}`)
  if (batch.imageIDs.length)
    conditions.push(`限定镜像：${batch.imageIDs.join('、')}`)
  if (batch.monthValues.length)
    conditions.push(`限定周期：${batch.monthValues.join('、')} 个月`)
  return conditions
}

export function PriceQuoteSelector({
  quote,
  selectionID,
  payFullPrice,
  onSelect
}: {
  quote?: PriceQuote | null
  selectionID: string
  payFullPrice: boolean
  onSelect: (selectionID: string, payFullPrice: boolean) => void
}) {
  const coupons = useGetMyCouponsQuery()
  if (!quote) {
    return (
      <div className="rounded-lg border border-slate-200 px-3 py-3 text-xs text-slate-500 dark:border-slate-700 dark:text-slate-300">
        正在计算订单金额与可用优惠券…
      </div>
    )
  }
  const applicable = new Map(
    quote.candidates
      .filter(candidate => candidate.kind === 'coupon')
      .map(candidate => [candidate.id, candidate])
  )
  const availableCoupons = (coupons.data ?? []).filter(
    coupon => coupon.status === 'available'
  )
  const selectedID = payFullPrice ? '' : selectionID
  return (
    <section className="overflow-hidden rounded-lg border border-slate-200 dark:border-slate-700">
      <div className="border-b border-slate-100 px-3 py-3 dark:border-slate-700">
        <h3 className="m-0 text-xs font-bold text-slate-800 dark:text-white">
          优惠券与结算
        </h3>
        <p className="mb-0 mt-1 text-[10px] leading-4 text-slate-500 dark:text-slate-300">
          已默认选中当前订单可用且减免最高的优惠券；你可改选其他券或原价支付。
        </p>
      </div>
      <dl className="grid grid-cols-3 divide-x divide-slate-100 border-b border-slate-100 text-[10px] dark:divide-slate-700 dark:border-slate-700">
        <div className="px-3 py-2.5">
          <dt className="text-slate-400">原价</dt>
          <dd className="mt-1 font-bold text-slate-700 dark:text-slate-100">
            <XCoinAmount value={(quote.listAmountFen / 100).toFixed(2)} />
          </dd>
        </div>
        <div className="px-3 py-2.5">
          <dt className="text-slate-400">优惠</dt>
          <dd className="mt-1 font-bold text-emerald-700 dark:text-emerald-200">
            −<XCoinAmount value={(quote.discountAmountFen / 100).toFixed(2)} />
          </dd>
        </div>
        <div className="px-3 py-2.5">
          <dt className="text-slate-400">实付</dt>
          <dd className="mt-1 font-bold text-slate-900 dark:text-white">
            <XCoinAmount value={(quote.amountFen / 100).toFixed(2)} />
          </dd>
        </div>
      </dl>
      <div className="space-y-3 p-3">
        <button
          type="button"
          aria-pressed={!selectedID}
          onClick={() => onSelect('', true)}
          className={classNames(
            'flex w-full items-center justify-between gap-3 rounded-md border px-3 py-2 text-left text-xs transition-colors focus-visible:outline-2 focus-visible:outline-blue-500',
            !selectedID
              ? 'border-blue-500 bg-blue-50 text-blue-800 dark:bg-blue-950 dark:text-blue-100'
              : 'border-slate-200 text-slate-600 hover:border-blue-300 dark:border-slate-700 dark:text-slate-300'
          )}
        >
          <span>不使用优惠</span>
          <span className="font-bold">按原价支付</span>
        </button>
        <section aria-label="选择优惠券">
          <div className="mb-1.5 flex items-baseline justify-between gap-3">
            <h4 className="m-0 text-[11px] font-bold text-slate-700 dark:text-slate-100">
              选择优惠券
            </h4>
            <span className="text-[10px] text-slate-400">
              展示券包全部可用券
            </span>
          </div>
          {coupons.isLoading ? (
            <p className="m-0 text-[11px] text-slate-500">正在载入券包…</p>
          ) : availableCoupons.length === 0 ? (
            <p className="m-0 text-[11px] leading-5 text-slate-500 dark:text-slate-300">
              券包中没有待使用优惠券；本单将按原价结算。
            </p>
          ) : (
            <div className="grid gap-1.5">
              {availableCoupons.map(coupon => {
                const candidate = applicable.get(coupon.id)
                const selected = selectedID === coupon.id
                const usable = Boolean(candidate)
                const conditions = couponConditions(coupon)
                return (
                  <button
                    key={coupon.id}
                    type="button"
                    disabled={!usable}
                    aria-pressed={selected}
                    onClick={() => usable && onSelect(coupon.id, false)}
                    className={classNames(
                      'flex items-center justify-between gap-3 rounded-md border px-3 py-2.5 text-left transition-colors focus-visible:outline-2 focus-visible:outline-blue-500',
                      selected
                        ? 'border-blue-500 bg-blue-50 dark:bg-blue-950'
                        : usable
                          ? 'border-slate-200 hover:border-blue-300 dark:border-slate-700'
                          : 'cursor-not-allowed border-slate-100 bg-slate-50 opacity-60 dark:border-slate-800 dark:bg-slate-900'
                    )}
                  >
                    <span className="min-w-0">
                      <b className="block truncate text-xs text-slate-800 dark:text-white">
                        {coupon.batch?.name ?? '历史优惠券'}
                      </b>
                      <small className="mt-0.5 block text-[10px] leading-4 text-slate-500 dark:text-slate-300">
                        {couponRule(coupon)} ·{' '}
                        {usable ? '适用于当前订单' : '当前订单不满足使用条件'}
                        {conditions.length > 0 && (
                          <span className="mt-0.5 block text-slate-400">
                            {conditions.join(' · ')}
                          </span>
                        )}
                      </small>
                    </span>
                    {candidate && (
                      <span className="shrink-0 text-right text-[10px]">
                        <b className="block text-emerald-700 dark:text-emerald-200">
                          减{' '}
                          <XCoinAmount
                            value={(candidate.discountAmountFen / 100).toFixed(
                              2
                            )}
                          />
                        </b>
                        <small className="mt-0.5 block text-slate-400">
                          实付 {(candidate.payableAmountFen / 100).toFixed(2)}
                        </small>
                      </span>
                    )}
                  </button>
                )
              })}
            </div>
          )}
        </section>
      </div>
    </section>
  )
}
