import { useEffect, useState } from 'react'
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
import type { Catalog, Plan, PriceQuote } from '@/types/cloud'

const money = (fen: number) => `¥${(fen / 100).toFixed(2)}`

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
          {plan.cpu} 核 CPU · {plan.memoryMB / 1024} GB 内存
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
  const [imageRef, setImageRef] = useState('')
  const [planID, setPlanID] = useState('')
  const [months, setMonths] = useState(1)
  const [error, setError] = useState('')
  const [couponCode, setCouponCode] = useState('')
  const [selectionID, setSelectionID] = useState('')
  const [payFullPrice, setPayFullPrice] = useState(false)
  const [quote, setQuote] = useState<PriceQuote | null>(null)
  const [purchase, { isLoading: saving }] = usePurchaseMutation()
  const [quotePurchase, { isLoading: quoting }] = useQuotePurchaseMutation()
  const { data: wallet } = useGetWalletQuery()
  const images = catalog?.images ?? []
  const imageSources = images
  const selectedRef = imageRef || imageSources[0]?.imageRef || ''
  const sourceImages = images.filter(image => image.imageRef === selectedRef)
  const selectedImageID = imageID || sourceImages[0]?.id || ''
  const selectedImage = images.find(image => image.id === selectedImageID)
  const imageVersions = selectedImage?.versions ?? []
  const selectedVersion =
    imageVersions.find(version => version.tag === imageVersion) ?? imageVersions[0]
  const plans = catalog?.plans ?? []
  const selectedPlanID = planID || plans[0]?.id || ''
  const selectedPlan = plans.find(plan => plan.id === selectedPlanID)
  const total = (selectedPlan?.monthlyPriceFen ?? 0) * months
  function preview(selected = selectionID, code = couponCode, fullPrice = payFullPrice) {
    if (!selectedImage || !selectedPlan) return
    setError('')
    void quotePurchase({
      planId: selectedPlan.id,
      imageId: selectedImage.id,
      months,
      couponCode: code,
      selectionId: selected,
      payFullPrice: fullPrice
    })
      .unwrap()
      .then(value => {
        setQuote(value)
        setSelectionID(value.selectedId ?? '')
        setPayFullPrice(Boolean(value.payFullPrice))
      })
      .catch(value =>
        setError(
          typeof value?.data?.message === 'string'
            ? value.data.message
            : '暂时无法计算优惠'
        )
      )
  }
  useEffect(() => {
    setQuote(null)
    setSelectionID('')
    setPayFullPrice(false)
    if (selectedImage && selectedPlan) preview('', '', false)
  }, [selectedImage?.id, selectedPlan?.id, months])

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
      couponCode: couponCode || undefined,
      selectionId: selectionID || undefined,
      payFullPrice
    })
      .unwrap()
      .then(() => {
        trackConsoleEvent('create_service', 'me', 'create', {
          result: 'success',
          durationMs: performance.now() - started
        })
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
            ) : imageSources.length === 0 ? (
              <Alert tone="error">暂无可售镜像，请联系管理员配置。</Alert>
            ) : (
              <>
                <div className="choice-grid image-grid">
                  {imageSources.map(source => (
                    <button
                      key={source.imageRef}
                      type="button"
                      className={`catalog-choice ${selectedRef === source.imageRef ? 'selected' : ''}`}
                      aria-pressed={selectedRef === source.imageRef}
                      onClick={() => {
                        setImageRef(source.imageRef)
                        setImageID('')
                        setImageVersion('')
                        setError('')
                      }}
                    >
                      <span className="choice-mark">
                        {selectedRef === source.imageRef ? '✓' : ''}
                      </span>
                      <span>
                        <b>{source.name}</b>
                        <small>{source.imageRef}</small>
                      </span>
                    </button>
                  ))}
                </div>
                <label htmlFor="image-version">
                  镜像版本
                  <select
                    id="image-version"
                    value={selectedVersion?.tag ?? ''}
                    onChange={event => {
                      setImageVersion(event.target.value)
                      setError('')
                    }}
                  >
                    {(selectedImage?.versions ?? []).map(version => (
                      <option key={version.id} value={version.tag}>
                        {version.tag}
                      </option>
                    ))}
                  </select>
                </label>
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
                <p>费用将从钱包余额扣除。</p>
              </div>
            </div>
            <div className="period-controls" role="group" aria-label="订阅周期">
              {[1, 3, 6, 12].map(value => (
                <button
                  type="button"
                  key={value}
                  className={months === value ? 'selected' : ''}
                  aria-pressed={months === value}
                  onClick={() => setMonths(value)}
                >
                  {value} 个月
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
                  ? `${selectedPlan.cpu} 核 CPU · ${selectedPlan.memoryMB / 1024} GB 内存`
                  : '—'}
              </dd>
            </div>
            <div>
              <dt>周期</dt>
              <dd>{months} 个月</dd>
            </div>
          </dl>
          <div className="summary-total">
            <span>应付 XCoin</span>
            <strong>
              {selectedPlan
                ? `${((quote?.amountFen ?? total) / 100).toFixed(2)} XCoin`
                : '—'}
            </strong>
          </div>
          <div className="mt-3 grid gap-2 rounded-lg border border-slate-200 p-3 text-xs dark:border-slate-700">
            <label>
              代金券码
              <input
                value={couponCode}
                placeholder="输入券码后试算"
                onChange={e => {
                  setCouponCode(e.target.value)
                  setSelectionID('')
                  setPayFullPrice(false)
                  setQuote(null)
                }}
              />
            </label>
            <Button
              tone="secondary"
              loading={quoting}
              disabled={!selectedImage || !selectedPlan}
              onClick={() => preview()}
            >
              试算优惠
            </Button>
            {quote && (
              <>
                <p className="m-0 text-slate-500">
                  原价 {(quote.listAmountFen / 100).toFixed(2)} XCoin，已优惠{' '}
                  {(quote.discountAmountFen / 100).toFixed(2)} XCoin
                </p>
                {quote.candidates.length > 1 && (
                  <select
                    value={payFullPrice ? '__full__' : selectionID}
                    onChange={e => {
                      const full = e.target.value === '__full__'
                      setPayFullPrice(full)
                      setSelectionID(full ? '' : e.target.value)
                      preview(full ? '' : e.target.value, couponCode, full)
                    }}
                  >
                    <option value="__full__">不使用优惠，按原价购买</option>
                    {quote.candidates.map(item => (
                      <option key={item.id} value={item.id}>
                        {item.name} · 减{' '}
                        {(item.discountAmountFen / 100).toFixed(2)} XCoin
                      </option>
                    ))}
                  </select>
                )}
              </>
            )}
          </div>
          <p className="summary-note">
            当前余额：
            {wallet
              ? `${(wallet.balanceFen / 100).toFixed(2)} XCoin`
              : '同步中'}
          </p>
          {error && <Alert tone="error">{error}</Alert>}
          <Button
            className="w-full"
            loading={saving}
            disabled={!selectedImage || !selectedPlan}
            onClick={submit}
          >
            使用 XCoin 直接购买
          </Button>
          <p className="summary-note">
            系统按“余额校验 → 资源校验 → 扣款 →
            部署”处理；任一步失败均不会扣款。
          </p>
        </aside>
      </div>
    </section>
  )
}
