import { useCallback, useEffect, useState } from 'react'
import { useDispatch } from 'react-redux'
import { BalanceSettlement } from '@/components/BalanceSettlement'
import {
  useGetWalletQuery,
  usePurchaseMutation,
  useQuotePurchaseMutation
} from '@/services/cloudApi'
import {
  Alert,
  Button,
  InlineAction,
  LoadingState,
  PageHeader
} from '@/components/ui'
import { trackConsoleEvent } from '@/services/telemetry'
import { watchTask } from '@/store/uiSlice'
import type { Catalog, Plan, PriceQuote } from '@/types/cloud'

const money = (fen: number) => `¥${(fen / 100).toFixed(2)}`
const subscriptionMonths = [1, 3, 6, 12]
const discountLabel = (plan: Plan | undefined, months: number) => {
  const bps = plan?.tierDiscounts?.[months]
  return months > 1 && bps !== undefined && bps < 10000
    ? `${bps / 1000} 折`
    : ''
}

function PlanChoice({
  plan,
  selected,
  onSelect
}: {
  plan: Plan
  selected: boolean
  onSelect: () => void
}) {
  return (
    <button
      type="button"
      className={`catalog-choice plan-choice ${selected ? 'selected' : ''}`}
      aria-pressed={selected}
      onClick={onSelect}
    >
      <span className="choice-mark">{selected ? '✓' : ''}</span>
      <span>
        <b>{plan.name}</b>
        <small>
          {plan.cpu} 核 CPU · {plan.memoryMB / 1024} GB 内存 · 共享网络参考{' '}
          {plan.bandwidthMbps} Mbps
        </small>
      </span>
      <em>
        {money(plan.monthlyPriceFen)}
        <small>/月</small>
      </em>
    </button>
  )
}

export function CreateServicePage({
  catalog,
  loading,
  hasError,
  onRetry,
  onCreated
}: {
  catalog?: Catalog
  loading: boolean
  hasError: boolean
  onRetry: () => void
  onCreated: () => void
}) {
  const [imageID, setImageID] = useState('')
  const [imageVersion, setImageVersion] = useState('')
  const [planID, setPlanID] = useState('')
  const [months, setMonths] = useState(1)
  const [error, setError] = useState('')
  const [promoCode, setPromoCode] = useState('')
  const [quote, setQuote] = useState<PriceQuote | null>(null)
  const [purchase, { isLoading: saving }] = usePurchaseMutation()
  const [quotePurchase] = useQuotePurchaseMutation()
  const { data: wallet } = useGetWalletQuery()
  const dispatch = useDispatch()
  const images = Array.isArray(catalog?.images) ? catalog.images : []
  const selectedImageID = imageID || images[0]?.id || ''
  const selectedImage = images.find(image => image.id === selectedImageID)
  const imageVersions = Array.isArray(selectedImage?.versions)
    ? selectedImage.versions
    : []
  const preferredVersion =
    imageVersions.find(version => version.tag.toLowerCase() === 'latest') ??
    imageVersions[0]
  const selectedVersion =
    imageVersions.find(version => version.tag === imageVersion) ??
    preferredVersion
  const plans = catalog?.plans ?? []
  const selectedPlanID = planID || plans[0]?.id || ''
  const selectedPlan = plans.find(plan => plan.id === selectedPlanID)
  const payableFen = quote?.amountFen
  const canPurchase =
    Boolean(
      selectedImage && selectedPlan && wallet && payableFen !== undefined
    ) && (wallet?.balanceFen ?? 0) >= (payableFen ?? 0)
  const preview = useCallback(
    (code = promoCode) => {
      if (!selectedImage || !selectedPlan) return
      setError('')
      void quotePurchase({
        planId: selectedPlan.id,
        imageId: selectedImage.id,
        months,
        promoCode: code || undefined
      })
        .unwrap()
        .then(value => {
          setQuote(value)
        })
        .catch(value =>
          setError(
            typeof value?.data?.message === 'string'
              ? value.data.message
              : '暂时无法计算优惠'
          )
        )
    },
    [months, promoCode, quotePurchase, selectedImage, selectedPlan]
  )
  useEffect(() => {
    setQuote(null)
    if (selectedImage && selectedPlan) preview('')
  }, [selectedImage, selectedPlan, months, preview])

  function submit() {
    if (!selectedImage || !selectedPlan) return
    setError('')
    const started = performance.now()
    trackConsoleEvent('create_service', 'me', 'create', { result: 'started' })
    void purchase({
      imageId: selectedImage.id,
      imageVersion: selectedVersion?.tag || 'latest',
      planId: selectedPlan.id,
      months,
      promoCode: promoCode || undefined
    })
      .unwrap()
      .then(value => {
        trackConsoleEvent('create_service', 'me', 'create', {
          result: 'success',
          durationMs: performance.now() - started
        })
        dispatch(watchTask({ id: value.task.id, action: value.task.action }))
        onCreated()
      })
      .catch(value => {
        trackConsoleEvent('create_service', 'me', 'create', {
          result: 'error',
          durationMs: performance.now() - started
        })
        setError(
          typeof value?.data?.message === 'string'
            ? value.data.message
            : '购买未完成：余额或可用资源不足时，系统不会扣款，也不会创建订单。'
        )
      })
  }

  return (
    <section className="page me-page create-page">
      <PageHeader
        title="创建运行环境"
        description="从经过审核的镜像与套餐中选择。系统会先校验钱包余额和可用资源，再扣款并自动部署。"
        actions={
          <div className="step-hint">
            <b>01</b>
            <span>选择资源</span>
            <i />
            <b>02</b>
            <span>确认订单</span>
          </div>
        }
      />
      <div className="create-layout">
        <div className="create-form">
          <section className="selection-section">
            <div className="selection-title">
              <span className="selection-number">1</span>
              <div>
                <h2>选择镜像</h2>
                <p>仅展示经过管理员审核的镜像来源和版本。</p>
              </div>
            </div>
            {loading ? (
              <LoadingState>正在加载可信镜像…</LoadingState>
            ) : hasError ? (
              <Alert tone="error">
                镜像加载失败。
                <InlineAction onClick={onRetry}>重新加载</InlineAction>
              </Alert>
            ) : images.length === 0 ? (
              <Alert tone="error">暂无可售镜像，请联系管理员配置。</Alert>
            ) : (
              <>
                <div className="choice-grid image-grid">
                  {images.map(source => (
                    <button
                      key={source.id}
                      type="button"
                      className={`catalog-choice ${selectedImageID === source.id ? 'selected' : ''}`}
                      aria-pressed={selectedImageID === source.id}
                      onClick={() => {
                        setImageID(source.id)
                        const versions = Array.isArray(source.versions)
                          ? source.versions
                          : []
                        setImageVersion(
                          versions.find(version => version.tag.toLowerCase() === 'latest')
                            ?.tag ?? versions[0]?.tag ?? ''
                        )
                        setError('')
                      }}
                    >
                      <span className="choice-mark">
                        {selectedImageID === source.id ? '✓' : ''}
                      </span>
                      <span>
                        <b>{source.name}</b>
                        <small>请选择可购买版本</small>
                      </span>
                    </button>
                  ))}
                </div>
                <div className="mt-5">
                  <label className="block text-sm font-bold text-slate-800 dark:text-slate-100" htmlFor="image-version">
                    选择镜像版本
                    <span className="ml-2 text-[11px] font-normal text-slate-400">默认推荐 latest</span>
                    <select
                      id="image-version"
                      className="mt-2 block h-11 w-full rounded-lg border border-slate-200 bg-white px-3 text-sm font-medium text-slate-800 outline-none transition focus:border-blue-500 focus:ring-3 focus:ring-blue-100 disabled:cursor-not-allowed disabled:bg-slate-50 dark:border-slate-600 dark:bg-slate-900 dark:text-slate-100 dark:focus:ring-blue-950"
                      value={selectedVersion?.tag ?? ''}
                      disabled={imageVersions.length === 0}
                      onChange={event => {
                        setImageVersion(event.target.value)
                        setError('')
                      }}
                    >
                      {imageVersions.length === 0 ? (
                        <option value="">暂无可购买版本</option>
                      ) : (
                        imageVersions.map(version => (
                          <option key={version.tag} value={version.tag}>
                            {version.tag}{version.tag.toLowerCase() === 'latest' ? '（推荐）' : ''}
                          </option>
                        ))
                      )}
                    </select>
                  </label>
                </div>
              </>
            )}
          </section>
          <section className="selection-section">
            <div className="selection-title">
              <span className="selection-number">2</span>
              <div>
                <h2>选择套餐</h2>
                <p>按需选择 CPU 和内存。</p>
              </div>
            </div>
            {loading ? (
              <LoadingState>正在加载计算套餐…</LoadingState>
            ) : hasError ? (
              <Alert tone="error">
                套餐加载失败。
                <InlineAction onClick={onRetry}>重新加载</InlineAction>
              </Alert>
            ) : plans.length === 0 ? (
              <Alert tone="error">暂无可售套餐，请联系管理员配置。</Alert>
            ) : (
              <div className="choice-grid plan-grid">
                {plans.map(plan => (
                  <PlanChoice
                    key={plan.id}
                    plan={plan}
                    selected={selectedPlanID === plan.id}
                    onSelect={() => {
                      setPlanID(plan.id)
                      setError('')
                    }}
                  />
                ))}
              </div>
            )}
          </section>
          <section className="selection-section compact-section">
            <div className="selection-title">
              <span className="selection-number">3</span>
              <div>
                <h2>选择周期</h2>
              </div>
            </div>
            <div className="period-controls" role="group" aria-label="订阅周期">
              {subscriptionMonths.map(value => (
                <button
                  type="button"
                  key={value}
                  className={months === value ? 'selected' : ''}
                  aria-pressed={months === value}
                  onClick={() => setMonths(value)}
                >
                  <span>{value} 个月</span>
                  {discountLabel(selectedPlan, value) && (
                    <small className="ml-1 text-red-600">
                      {discountLabel(selectedPlan, value)}
                    </small>
                  )}
                </button>
              ))}
            </div>
          </section>
        </div>
        <aside className="order-summary" aria-live="polite">
          <h2>确认你的选择</h2>
          <dl>
            <div>
              <dt>镜像</dt>
              <dd>
                {selectedImage
                  ? `${selectedImage.name} · ${selectedVersion?.tag ?? '请选择版本'}`
                  : '请选择镜像'}
              </dd>
            </div>
            <div>
              <dt>套餐</dt>
              <dd>{selectedPlan?.name || '请选择套餐'}</dd>
            </div>
            <div>
              <dt>资源</dt>
              <dd>
                {selectedPlan
                  ? `${selectedPlan.cpu} 核 CPU · ${selectedPlan.memoryMB / 1024} GB 内存 · 共享网络参考 ${selectedPlan.bandwidthMbps} Mbps`
                  : '—'}
              </dd>
            </div>
            <div>
              <dt>周期</dt>
              <dd>{months} 个月</dd>
            </div>
          </dl>
          <div className="mt-5 border-t border-slate-100 pt-4 text-sm dark:border-slate-700">
            <div className="flex items-center justify-between gap-3">
              <span className="text-[11px] font-semibold text-slate-500 dark:text-slate-300">
                有推广码？
              </span>
              <span className="text-[11px] text-slate-400">可选填写</span>
            </div>
            <div className="mt-2 flex gap-2">
              <input
                className="h-10 min-w-0 flex-1 rounded-lg border border-slate-200 bg-slate-50 px-3 text-xs text-slate-700 shadow-none outline-none placeholder:text-slate-400 focus:border-blue-300 focus-visible:outline-1 focus-visible:outline-offset-1 focus-visible:outline-blue-100 dark:border-slate-600 dark:bg-slate-900 dark:text-slate-100"
                value={promoCode}
                onChange={event => setPromoCode(event.target.value)}
                placeholder="输入推广码"
              />
              <Button
                className="h-10 px-4"
                tone="secondary"
                onClick={() => preview(promoCode)}
              >
                应用
              </Button>
            </div>
            <div className="mt-4 space-y-2 text-xs">
              <div className="flex justify-between gap-3 text-slate-500 dark:text-slate-300">
                <span>
                  套餐价格
                  {quote?.tierMonths ? `（${quote.tierMonths} 个月）` : ''}
                </span>
                <b>{quote ? money(quote.listAmountFen) : '—'}</b>
              </div>
              {quote?.program && (
                <div className="flex justify-between gap-3 text-emerald-700">
                  <span>
                    已自动应用：{quote.program.name}
                    {quote.bonusDays ? ` · 赠送 ${quote.bonusDays} 天` : ''}
                  </span>
                  <b>
                    {quote.discountAmountFen
                      ? `-${money(quote.discountAmountFen)}`
                      : '权益已生效'}
                  </b>
                </div>
              )}
            </div>
          </div>
          <div className="my-4">
            <BalanceSettlement
              balanceFen={wallet?.balanceFen}
              payableFen={payableFen}
            />
          </div>
          {error && <Alert tone="error">{error}</Alert>}
          <Button
            className="w-full"
            loading={saving}
            tone={canPurchase ? 'primary' : 'secondary'}
            disabled={!canPurchase}
            onClick={submit}
          >
            {wallet && payableFen !== undefined && !canPurchase
              ? '余额不足，暂不能购买'
              : '确认购买'}
          </Button>
        </aside>
      </div>
    </section>
  )
}
