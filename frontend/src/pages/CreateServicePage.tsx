import { useCallback, useEffect, useState } from 'react'
import { useDispatch } from 'react-redux'
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
          {plan.cpu} 核 CPU · {plan.memoryMB / 1024} GB 内存 · 最高共享{' '}
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
  const [selectionID, setSelectionID] = useState('')
  const [payFullPrice, setPayFullPrice] = useState(false)
  const [quote, setQuote] = useState<PriceQuote | null>(null)
  const [purchase, { isLoading: saving }] = usePurchaseMutation()
  const [quotePurchase] = useQuotePurchaseMutation()
  const { data: wallet } = useGetWalletQuery()
  const dispatch = useDispatch()
  const images = catalog?.images ?? []
  const selectedImageID = imageID || images[0]?.id || ''
  const selectedImage = images.find(image => image.id === selectedImageID)
  const imageVersions = selectedImage?.versions ?? []
  const selectedVersion =
    imageVersions.find(version => version.tag === imageVersion) ??
    imageVersions[0]
  const plans = catalog?.plans ?? []
  const selectedPlanID = planID || plans[0]?.id || ''
  const selectedPlan = plans.find(plan => plan.id === selectedPlanID)
  const total = (selectedPlan?.monthlyPriceFen ?? 0) * months
  const preview = useCallback(
    (selected: string, fullPrice: boolean) => {
      if (!selectedImage || !selectedPlan) return
      setError('')
      void quotePurchase({
        planId: selectedPlan.id,
        imageId: selectedImage.id,
        months,
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
    },
    [months, quotePurchase, selectedImage, selectedPlan]
  )
  useEffect(() => {
    setQuote(null)
    setSelectionID('')
    setPayFullPrice(false)
    if (selectedImage && selectedPlan) preview('', false)
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
      selectionId: selectionID || undefined,
      payFullPrice
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
                        setImageVersion('')
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
                  <p className="mb-2 text-sm font-bold">选择镜像版本</p>
                  <div className="choice-grid">
                    {imageVersions.map(version => (
                      <button
                        key={version.tag}
                        type="button"
                        className={`catalog-choice ${selectedVersion?.tag === version.tag ? 'selected' : ''}`}
                        aria-pressed={selectedVersion?.tag === version.tag}
                        onClick={() => {
                          setImageVersion(version.tag)
                          setError('')
                        }}
                      >
                        <span className="choice-mark">{selectedVersion?.tag === version.tag ? '✓' : ''}</span>
                        <span><b>{version.tag}</b></span>
                      </button>
                    ))}
                  </div>
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
                  ? `${selectedPlan.cpu} 核 CPU · ${selectedPlan.memoryMB / 1024} GB 内存 · 最高共享 ${selectedPlan.bandwidthMbps} Mbps`
                  : '—'}
              </dd>
            </div>
            <div>
              <dt>周期</dt>
              <dd>{months} 个月</dd>
            </div>
          </dl>
          <div className="summary-total">
            <span>应付</span>
            <strong>
              {selectedPlan
                ? `${((quote?.amountFen ?? total) / 100).toFixed(2)} XCoin`
                : '—'}
            </strong>
          </div>
          <div className="my-3 grid gap-2 rounded-lg border border-slate-200 p-3 text-xs dark:border-slate-700">
            {quote && (
              <>
                <div className="flex flex-col  items-center justify-between">
                  <div>
                    余额：
                    {wallet
                      ? `${(wallet.balanceFen / 100).toFixed(2)} XCoin`
                      : '同步中'}
                  </div>

                  <div>原价 {(quote.listAmountFen / 100).toFixed(2)} XCoin</div>
                  <div>
                    已优惠 {(quote.discountAmountFen / 100).toFixed(2)} XCoin
                  </div>
                </div>
                <div className="mt-2 flex items-center justify-between">
                  {quote.candidates.length > 1 && (
                    <select
                      value={payFullPrice ? '__full__' : selectionID}
                      onChange={e => {
                        const full = e.target.value === '__full__'
                        setPayFullPrice(full)
                        setSelectionID(full ? '' : e.target.value)
                        preview(full ? '' : e.target.value, full)
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
                </div>
              </>
            )}
          </div>
          {error && <Alert tone="error">{error}</Alert>}
          <Button
            className="w-full"
            loading={saving}
            disabled={!selectedImage || !selectedPlan}
            onClick={submit}
          >
            购买
          </Button>
        </aside>
      </div>
    </section>
  )
}
