import { useEffect, useState } from 'react'
import { Alert, Button, PageHeader } from '@/components/ui'
import {
  useCreateAdminCouponsMutation,
  useGetAdminCatalogQuery,
  useGetAdminPromotionsQuery,
  useSaveAdminPromotionMutation
} from '@/services/cloudApi'
import type { Promotion } from '@/types/cloud'

const input =
  'mt-1 block w-full rounded-md border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-800 outline-none focus:border-blue-500 dark:border-slate-600 dark:bg-slate-900 dark:text-white'
const label = 'block text-[11px] font-bold text-slate-700 dark:text-slate-100'
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
const steps = [
  '选择类型',
  '适用范围与商品',
  '优惠规则',
  '名额与时间',
  '预览发布'
]

export function AdminPromotionEditor({
  promotionID,
  onBack
}: {
  promotionID?: string
  onBack: () => void
}) {
  const all = useGetAdminPromotionsQuery()
  const catalog = useGetAdminCatalogQuery()
  const [save, { isLoading }] = useSaveAdminPromotionMutation()
  const [createCoupons, { isLoading: creatingCoupons }] =
    useCreateAdminCouponsMutation()
  const [value, setValue] = useState<Promotion>(blank)
  const [step, setStep] = useState(0)
  const [error, setError] = useState('')
  const [couponMode, setCouponMode] = useState<'single' | 'general'>('general')
  const [couponCount, setCouponCount] = useState(1)
  const [couponLimit, setCouponLimit] = useState(100)
  const [couponUserLimit, setCouponUserLimit] = useState(1)
  const [codes, setCodes] = useState<string[]>([])
  useEffect(() => {
    if (promotionID && all.data) {
      const found = all.data.find(x => x.id === promotionID)
      if (found) setValue(found)
    }
  }, [promotionID, all.data])
  const special =
    value.kind === 'newcomer' || value.kind === 'first_plan_purchase'
  function chooseKind(kind: Promotion['kind']) {
    setValue(v => ({
      ...v,
      kind,
      scope: kind === 'campaign' ? v.scope : 'purchase',
      planIDs: kind === 'newcomer' ? [] : v.planIDs
    }))
  }
  function validate() {
    if (step === 0 && !value.name.trim()) return '请填写活动名称'
    if (
      step === 1 &&
      value.kind === 'first_plan_purchase' &&
      value.planIDs.length === 0
    )
      return '套餐新购优惠至少选择一个套餐'
    if (step === 2 && value.discountValue <= 0) return '请填写有效优惠值'
    return ''
  }
  function next() {
    const issue = validate()
    if (issue) return setError(issue)
    setError('')
    setStep(x => Math.min(4, x + 1))
  }
  async function submit() {
    const issue = validate()
    if (issue) return setError(issue)
    try {
      await save(value).unwrap()
      onBack()
    } catch (e: unknown) {
      setError(
        typeof e === 'object' &&
          e !== null &&
          'data' in e &&
          typeof e.data === 'object' &&
          e.data !== null &&
          'message' in e.data &&
          typeof e.data.message === 'string'
          ? e.data.message
          : '保存失败'
      )
    }
  }
  return (
    <section className="page super-page">
      <PageHeader
        eyebrow="营销运营"
        title={promotionID ? '编辑活动' : '创建活动'}
        description="按步骤配置，发布后订单会保存不可变的优惠规则快照。"
        actions={
          <Button tone="secondary" onClick={onBack}>
            返回活动列表
          </Button>
        }
      />
      <ol className="mb-6 grid grid-cols-5 gap-2 text-center text-[11px] font-bold">
        {steps.map((name, i) => (
          <li
            key={name}
            className={`rounded-md px-2 py-2 ${i === step ? 'bg-blue-600 text-white' : i < step ? 'bg-blue-100 text-blue-700 dark:bg-blue-950' : 'bg-slate-100 text-slate-500 dark:bg-slate-800'}`}
          >
            {i + 1}. {name}
          </li>
        ))}
      </ol>
      <section className="max-w-3xl rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-700 dark:bg-slate-800">
        {step === 0 && (
          <div className="space-y-4">
            <label className={label}>
              活动名称
              <input
                data-autofocus
                className={input}
                value={value.name}
                onChange={e => setValue({ ...value, name: e.target.value })}
              />
            </label>
            <div className="grid gap-3 sm:grid-cols-3">
              {(
                [
                  {
                    kind: 'campaign',
                    title: '普通活动',
                    note: '可用于新购、续费，也可发放领取券。'
                  },
                  {
                    kind: 'newcomer',
                    title: '新人专属',
                    note: '从未成功购买任意套餐时可用。'
                  },
                  {
                    kind: 'first_plan_purchase',
                    title: '套餐新购优惠',
                    note: '首次购买指定套餐时可用。'
                  }
                ] as const
              ).map(item => (
                <button
                  type="button"
                  key={item.kind}
                  onClick={() => chooseKind(item.kind)}
                  className={`rounded-lg border p-4 text-left ${value.kind === item.kind ? 'border-blue-500 bg-blue-50 dark:bg-blue-950' : 'border-slate-200 dark:border-slate-700'}`}
                >
                  <b>{item.title}</b>
                  <small className="mt-2 block leading-5">{item.note}</small>
                </button>
              ))}
            </div>
          </div>
        )}
        {step === 1 && (
          <div className="space-y-4">
            <label className={label}>
              适用范围
              <select
                className={input}
                disabled={special}
                value={value.scope}
                onChange={e =>
                  setValue({
                    ...value,
                    scope: e.target.value as Promotion['scope']
                  })
                }
              >
                <option value="both">新购与续费</option>
                <option value="purchase">仅新购</option>
                <option value="renewal">仅续费</option>
              </select>
            </label>
            {special && (
              <Alert tone="info">
                {value.kind === 'newcomer'
                  ? '新人专属固定仅适用于新购。'
                  : '套餐新购优惠固定仅适用于新购；每个用户、每个套餐仅首次购买可用。'}
              </Alert>
            )}
            <div className="grid gap-4 sm:grid-cols-2">
              <label className={label}>
                适用套餐
                {value.kind === 'first_plan_purchase'
                  ? '（必选）'
                  : '（留空不限）'}
                <select
                  multiple
                  className={input}
                  value={value.planIDs}
                  onChange={e =>
                    setValue({
                      ...value,
                      planIDs: Array.from(
                        e.target.selectedOptions,
                        x => x.value
                      )
                    })
                  }
                >
                  {(catalog.data?.plans ?? []).map(p => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
                </select>
              </label>
              <label className={label}>
                适用镜像（留空不限）
                <select
                  multiple
                  className={input}
                  value={value.imageIDs}
                  onChange={e =>
                    setValue({
                      ...value,
                      imageIDs: Array.from(
                        e.target.selectedOptions,
                        x => x.value
                      )
                    })
                  }
                >
                  {(catalog.data?.images ?? []).map(i => (
                    <option key={i.id} value={i.id}>
                      {i.name} · {i.version}
                    </option>
                  ))}
                </select>
              </label>
            </div>
            <div>
              <div className="flex items-center justify-between">
                <span className={label}>适用月数（不选即不限）</span>
                {value.monthValues.length > 0 && (
                  <button
                    type="button"
                    className="text-button text-xs"
                    onClick={() => setValue({ ...value, monthValues: [] })}
                  >
                    清空选择
                  </button>
                )}
              </div>
              <div className="mt-2 grid grid-cols-4 gap-2 sm:grid-cols-6">
                {Array.from({ length: 12 }, (_, i) => String(i + 1)).map(
                  month => {
                    const selected = value.monthValues.includes(month)
                    return (
                      <button
                        type="button"
                        key={month}
                        aria-pressed={selected}
                        onClick={() =>
                          setValue({
                            ...value,
                            monthValues: selected
                              ? value.monthValues.filter(x => x !== month)
                              : [...value.monthValues, month]
                          })
                        }
                        className={`rounded-md border px-2 py-2 text-xs font-bold ${selected ? 'border-blue-600 bg-blue-600 text-white' : 'border-slate-200 text-slate-600 hover:border-blue-300 dark:border-slate-600 dark:text-slate-200'}`}
                      >
                        {month} 个月
                      </button>
                    )
                  }
                )}
              </div>
            </div>
          </div>
        )}
        {step === 2 && (
          <div className="grid gap-4 sm:grid-cols-2">
            <label className={label}>
              规则
              <select
                className={input}
                value={value.discountType}
                onChange={e =>
                  setValue({
                    ...value,
                    discountType: e.target.value as Promotion['discountType']
                  })
                }
              >
                <option value="fixed">固定减免</option>
                <option value="percent">用户实付折扣</option>
              </select>
            </label>
            <label className={label}>
              {value.discountType === 'percent'
                ? '用户实付折数（95 = 95 折）'
                : '减免金额（分）'}
              <input
                type="number"
                className={input}
                value={
                  value.discountType === 'percent'
                    ? value.discountValue / 100
                    : value.discountValue
                }
                onChange={e =>
                  setValue({
                    ...value,
                    discountValue:
                      value.discountType === 'percent'
                        ? Math.round(Number(e.target.value) * 100)
                        : Number(e.target.value)
                  })
                }
              />
            </label>
            <label className={label}>
              最低消费（分，0 为不限）
              <input
                type="number"
                className={input}
                value={value.minAmountFen}
                onChange={e =>
                  setValue({ ...value, minAmountFen: Number(e.target.value) })
                }
              />
            </label>
            <label className={label}>
              最高减免（分，0 为不限）
              <input
                type="number"
                className={input}
                value={value.maxDiscountFen}
                onChange={e =>
                  setValue({ ...value, maxDiscountFen: Number(e.target.value) })
                }
              />
            </label>
          </div>
        )}
        {step === 3 && (
          <div className="grid gap-4 sm:grid-cols-2">
            <label className={label}>
              总核销上限（0 为不限）
              <input
                type="number"
                className={input}
                value={value.totalLimit}
                onChange={e =>
                  setValue({ ...value, totalLimit: Number(e.target.value) })
                }
              />
            </label>
            <label className={label}>
              每用户限额（0 为不限）
              <input
                type="number"
                className={input}
                value={value.perUserLimit}
                onChange={e =>
                  setValue({ ...value, perUserLimit: Number(e.target.value) })
                }
              />
            </label>
            <label className={label}>
              开始时间
              <input
                type="datetime-local"
                className={input}
                value={value.startsAt?.slice(0, 16) ?? ''}
                onChange={e =>
                  setValue({
                    ...value,
                    startsAt: e.target.value
                      ? new Date(e.target.value).toISOString()
                      : undefined
                  })
                }
              />
            </label>
            <label className={label}>
              结束时间
              <input
                type="datetime-local"
                className={input}
                value={value.endsAt?.slice(0, 16) ?? ''}
                onChange={e =>
                  setValue({
                    ...value,
                    endsAt: e.target.value
                      ? new Date(e.target.value).toISOString()
                      : undefined
                  })
                }
              />
            </label>
            <label className={label}>
              状态
              <select
                className={input}
                value={value.enabled ? 'yes' : 'no'}
                onChange={e =>
                  setValue({ ...value, enabled: e.target.value === 'yes' })
                }
              >
                <option value="yes">发布并启用</option>
                <option value="no">保存为停用</option>
              </select>
            </label>
            {value.kind === 'campaign' && (
              <Alert tone="info">
                普通活动发布后，管理员可生成用户可领取的券包券。
              </Alert>
            )}
          </div>
        )}
        {step === 4 && (
          <div className="space-y-3">
            <h2 className="text-lg font-bold">确认发布</h2>
            <dl className="grid grid-cols-2 gap-3 text-sm">
              <div>
                <dt>类型</dt>
                <dd className="font-bold">
                  {value.kind === 'newcomer'
                    ? '新人专属'
                    : value.kind === 'first_plan_purchase'
                      ? '套餐新购优惠'
                      : '普通活动'}
                </dd>
              </div>
              <div>
                <dt>范围</dt>
                <dd className="font-bold">
                  {value.scope === 'both'
                    ? '新购与续费'
                    : value.scope === 'purchase'
                      ? '仅新购'
                      : '仅续费'}
                </dd>
              </div>
              <div>
                <dt>优惠</dt>
                <dd className="font-bold">
                  {value.discountType === 'percent'
                    ? `${value.discountValue / 100} 折`
                    : `减 ${value.discountValue} 分`}
                </dd>
              </div>
              <div>
                <dt>套餐</dt>
                <dd className="font-bold">
                  {value.planIDs.length
                    ? `${value.planIDs.length} 个已选`
                    : '不限'}
                </dd>
              </div>
            </dl>
            {promotionID && value.kind === 'campaign' && (
              <div className="mt-5 rounded-lg border border-slate-200 p-4 dark:border-slate-700">
                <b className="text-sm">生成可领取代金券</b>
                <p className="mt-1 text-xs text-slate-500">
                  券码创建后由用户在活动页领取，并自动进入优惠券包。
                </p>
                <div className="mt-3 grid gap-3 sm:grid-cols-4">
                  <label className={label}>
                    类型
                    <select
                      className={input}
                      value={couponMode}
                      onChange={e =>
                        setCouponMode(e.target.value as 'single' | 'general')
                      }
                    >
                      <option value="general">通用券</option>
                      <option value="single">批量单次券</option>
                    </select>
                  </label>
                  <label className={label}>
                    数量
                    <input
                      className={input}
                      type="number"
                      min="1"
                      max="500"
                      disabled={couponMode === 'general'}
                      value={couponCount}
                      onChange={e => setCouponCount(Number(e.target.value))}
                    />
                  </label>
                  <label className={label}>
                    总次数
                    <input
                      className={input}
                      type="number"
                      min="1"
                      disabled={couponMode === 'single'}
                      value={couponLimit}
                      onChange={e => setCouponLimit(Number(e.target.value))}
                    />
                  </label>
                  <label className={label}>
                    每用户次数
                    <input
                      className={input}
                      type="number"
                      min="1"
                      disabled={couponMode === 'single'}
                      value={couponUserLimit}
                      onChange={e => setCouponUserLimit(Number(e.target.value))}
                    />
                  </label>
                </div>
                <Button
                  className="mt-3"
                  tone="secondary"
                  loading={creatingCoupons}
                  onClick={() =>
                    void createCoupons({
                      promotionId: promotionID,
                      mode: couponMode,
                      count: couponMode === 'general' ? 1 : couponCount,
                      totalLimit: couponMode === 'single' ? 1 : couponLimit,
                      perUserLimit:
                        couponMode === 'single' ? 1 : couponUserLimit
                    })
                      .unwrap()
                      .then(r => setCodes(r.coupons.map(x => x.code)))
                      .catch(() => setError('生成券码失败'))
                  }
                >
                  生成券码
                </Button>
                {codes.length > 0 && (
                  <Alert tone="success">
                    请立即复制券码：{codes.join('，')}。系统不会保存原始券码。
                  </Alert>
                )}
              </div>
            )}
          </div>
        )}
        {error && <Alert tone="error">{error}</Alert>}
        <div className="mt-6 flex justify-between">
          <Button
            tone="secondary"
            onClick={() => (step === 0 ? onBack() : setStep(step - 1))}
          >
            上一步
          </Button>
          {step < 4 ? (
            <Button onClick={next}>下一步</Button>
          ) : (
            <Button loading={isLoading} onClick={() => void submit()}>
              确认发布
            </Button>
          )}
        </div>
      </section>
    </section>
  )
}
