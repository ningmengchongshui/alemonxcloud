import { useState } from 'react'
import {
  Alert,
  Button,
  Dialog,
  DialogFooter,
  EmptyState,
  PageHeader,
  StatusBadge,
  dialogFieldClass,
  dialogLabelClass
} from '@/components/ui'
import {
  useGetAdminBenefitGrantsQuery,
  useGetAdminBenefitProgramsQuery,
  useGetAdminCatalogQuery,
  useGrantAdminBenefitProgramMutation,
  useSaveAdminBenefitProgramMutation,
  useVoidAdminBenefitGrantMutation
} from '@/services/cloudApi'
import type { BenefitProgram } from '@/types/cloud'

const fresh = (): BenefitProgram => ({
  id: '',
  name: '',
  goal: 'first_purchase',
  status: 'draft',
  triggerType: 'automatic',
  orderScope: 'purchase',
  benefitType: 'fixed_discount',
  benefitValue: 0,
  minAmountFen: 0,
  planIds: [],
  monthValues: [],
  audienceType: 'all',
  perUserLimit: 0,
  totalLimit: 0,
  usedCount: 0,
  cashBudgetFen: 0,
  cashSpentFen: 0,
  grantDaysLimit: 0,
  grantDaysUsed: 0,
  priority: 0,
  codeTotalLimit: 0,
  codePerUserLimit: 0
})

const money = (value: number) => `¥${(value / 100).toFixed(2)}`
const split = (value: string) =>
  value
    .split(/[\s,]+/)
    .map(item => item.trim())
    .filter(Boolean)
const subscriptionMonths = [1, 3, 6, 12]
const toRFC3339 = (value?: string) => {
  if (!value) return undefined
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? undefined : parsed.toISOString()
}
const toLocalInput = (value?: string) => {
  if (!value) return ''
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return ''
  const offset = parsed.getTimezoneOffset() * 60_000
  return new Date(parsed.getTime() - offset).toISOString().slice(0, 16)
}
const validNonNegativeInteger = (value: number) =>
  Number.isInteger(value) && Number.isFinite(value) && value >= 0
const statusLabel: Record<BenefitProgram['status'], string> = {
  draft: '草稿',
  scheduled: '待生效',
  active: '已启用',
  paused: '已暂停',
  ended: '已结束'
}
const statusTone = (status: BenefitProgram['status']) => {
  if (status === 'active') return 'success' as const
  if (status === 'scheduled') return 'progress' as const
  if (status === 'paused') return 'pending' as const
  return 'neutral' as const
}
const errorMessage = (value: unknown, fallback: string) => {
  if (typeof value === 'object' && value && 'data' in value) {
    const data = value.data
    if (
      typeof data === 'object' &&
      data &&
      'message' in data &&
      typeof data.message === 'string'
    )
      return data.message
  }
  return fallback
}

export function AdminBenefitsPage() {
  const { data: programs = [], refetch } = useGetAdminBenefitProgramsQuery()
  const { data: catalog } = useGetAdminCatalogQuery()
  const [save, { isLoading: isSaving }] = useSaveAdminBenefitProgramMutation()
  const [grant, { isLoading: isGranting }] =
    useGrantAdminBenefitProgramMutation()
  const [voidGrant, { isLoading: isVoiding }] =
    useVoidAdminBenefitGrantMutation()
  const [editing, setEditing] = useState<BenefitProgram | null>(null)
  const [step, setStep] = useState(1)
  const [grantProgram, setGrantProgram] = useState<BenefitProgram | null>(null)
  const [userIDs, setUserIDs] = useState('')
  const [expiresAt, setExpiresAt] = useState('')
  const [message, setMessage] = useState('')
  const grants = useGetAdminBenefitGrantsQuery(grantProgram?.id ?? '', {
    skip: !grantProgram
  })
  const plans = catalog?.plans ?? []
  const [filter, setFilter] = useState<'all' | BenefitProgram['triggerType']>(
    'all'
  )
  const visiblePrograms =
    filter === 'all'
      ? programs
      : programs.filter(program => program.triggerType === filter)

  const closeEditor = () => {
    setEditing(null)
    setMessage('')
  }
  const updateEditing = (patch: Partial<BenefitProgram>) =>
    setEditing(current => (current ? { ...current, ...patch } : current))
  const submit = () => {
    if (!editing || !editing.name.trim()) {
      setMessage('请填写权益方案名称。')
      return
    }
    const numericFields = [
      editing.minAmountFen,
      editing.perUserLimit,
      editing.totalLimit,
      editing.cashBudgetFen,
      editing.grantDaysLimit,
      editing.priority,
      editing.codeTotalLimit ?? 0,
      editing.codePerUserLimit ?? 0
    ]
    if (!numericFields.every(validNonNegativeInteger)) {
      setMessage('次数、预算、优先级等只能填写非负整数。')
      return
    }
    if (!Number.isInteger(editing.benefitValue) || editing.benefitValue <= 0) {
      setMessage('权益值必须是大于 0 的整数。')
      return
    }
    if (
      editing.benefitType === 'percent_discount' &&
      editing.benefitValue > 10000
    ) {
      setMessage('折扣权益值不能大于 10000。')
      return
    }
    if (
      editing.monthValues.some(month => !subscriptionMonths.includes(month))
    ) {
      setMessage('限定月数只能选择 1、3、6 或 12 个月。')
      return
    }
    if (
      editing.triggerType === 'promo_code' &&
      !editing.id &&
      !editing.code?.trim()
    ) {
      setMessage('请填写推广码。')
      return
    }
    const startsAt = toRFC3339(editing.startsAt)
    const endsAt = toRFC3339(editing.endsAt)
    if ((editing.startsAt && !startsAt) || (editing.endsAt && !endsAt)) {
      setMessage('开始或结束时间格式不正确。')
      return
    }
    if (startsAt && endsAt && new Date(endsAt) <= new Date(startsAt)) {
      setMessage('结束时间必须晚于开始时间。')
      return
    }
    const payload: BenefitProgram = {
      id: editing.id,
      name: editing.name.trim(),
      goal: editing.goal,
      status: editing.status,
      triggerType: editing.triggerType,
      orderScope: editing.orderScope,
      benefitType: editing.benefitType,
      benefitValue: editing.benefitValue,
      minAmountFen: editing.minAmountFen,
      planIds: editing.planIds,
      monthValues: editing.monthValues,
      audienceType: editing.audienceType,
      startsAt,
      endsAt,
      perUserLimit: editing.perUserLimit,
      totalLimit: editing.totalLimit,
      usedCount: editing.usedCount,
      cashBudgetFen: editing.cashBudgetFen,
      cashSpentFen: editing.cashSpentFen,
      grantDaysLimit: editing.grantDaysLimit,
      grantDaysUsed: editing.grantDaysUsed,
      priority: editing.priority,
      channelLabel: editing.channelLabel?.trim() || undefined,
      code:
        editing.triggerType === 'promo_code'
          ? editing.code?.trim() || undefined
          : undefined,
      codeMask: editing.codeMask,
      codeTotalLimit: editing.codeTotalLimit ?? 0,
      codePerUserLimit: editing.codePerUserLimit ?? 0
    }
    setMessage('')
    void save(payload)
      .unwrap()
      .then(() => {
        closeEditor()
        void refetch()
      })
      .catch(error => setMessage(errorMessage(error, '保存失败，请稍后重试。')))
  }
  const togglePlan = (planID: string) =>
    updateEditing({
      planIds: editing?.planIds.includes(planID)
        ? editing.planIds.filter(id => id !== planID)
        : [...(editing?.planIds ?? []), planID]
    })
  const toggleMonth = (month: number) =>
    updateEditing({
      monthValues: editing?.monthValues.includes(month)
        ? editing.monthValues.filter(value => value !== month)
        : [...(editing?.monthValues ?? []), month]
    })
  const submitGrant = () => {
    if (!grantProgram || split(userIDs).length === 0) {
      setMessage('请至少填写一个用户 ID。')
      return
    }
    setMessage('')
    void grant({
      id: grantProgram.id,
      ownerIds: split(userIDs),
      expiresAt: expiresAt || undefined
    })
      .unwrap()
      .then(() => {
        setUserIDs('')
        void grants.refetch()
      })
      .catch(error => setMessage(errorMessage(error, '发放失败，请稍后重试。')))
  }

  return (
    <section className="page super-page">
      <PageHeader
        eyebrow="营销运营"
        title="商业权益方案"
        description="统一管理自动权益、推广码和定向权益；系统在结算时自动匹配一个最优方案。"
        actions={
          <Button
            onClick={() => {
              setEditing(fresh())
              setStep(1)
              setMessage('')
            }}
          >
            新建权益方案
          </Button>
        }
      />

      <section className="overflow-hidden rounded-xl border border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-800">
        <div className="flex items-center justify-between gap-4 border-b border-slate-100 px-5 py-4 dark:border-slate-700 max-[560px]:items-start">
          <div>
            <h2 className="m-0 text-sm font-bold text-slate-800 dark:text-white">
              方案列表
            </h2>
            <p className="mt-1 text-[11px] text-slate-500 dark:text-slate-300">
              按优先级与适用条件自动参与结算。
            </p>
          </div>
          <div className="flex flex-wrap gap-2 border-b border-slate-100 px-5 py-3 dark:border-slate-700">
            {(
              [
                ['all', '全部'],
                ['automatic', '自动权益'],
                ['promo_code', '推广码'],
                ['targeted', '定向权益']
              ] as const
            ).map(([value, label]) => (
              <button
                key={value}
                type="button"
                onClick={() => setFilter(value)}
                className={`rounded-md border px-3 py-1.5 text-xs font-bold ${filter === value ? 'border-blue-600 bg-blue-600 text-white' : 'border-slate-200 text-slate-600 dark:border-slate-600 dark:text-slate-200'}`}
              >
                {label}
              </button>
            ))}
          </div>
          <span className="shrink-0 text-[11px] font-bold text-slate-500 dark:text-slate-300">
            共 {visiblePrograms.length} 个
          </span>
        </div>
        {visiblePrograms.length === 0 ? (
          <EmptyState
            title="还没有权益方案"
            description="创建首购、多月购买、续费挽回、渠道推广或定向权益方案。"
            action={
              <Button
                onClick={() => {
                  setEditing(fresh())
                  setStep(1)
                }}
              >
                新建方案
              </Button>
            }
          />
        ) : (
          <div className="divide-y divide-slate-100 dark:divide-slate-700">
            {visiblePrograms.map(program => (
              <article
                key={program.id}
                className="flex items-center justify-between gap-5 px-5 py-4 max-[760px]:items-start max-[760px]:flex-col"
              >
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <h3 className="m-0 text-sm font-bold text-slate-800 dark:text-white">
                      {program.name}
                    </h3>
                    <StatusBadge tone={statusTone(program.status)}>
                      {statusLabel[program.status]}
                    </StatusBadge>
                  </div>
                  <p className="mb-0 mt-1.5 text-[11px] leading-5 text-slate-500 dark:text-slate-300">
                    {program.triggerType === 'automatic'
                      ? '自动匹配'
                      : program.triggerType === 'promo_code'
                        ? '推广码'
                        : '定向发放'}{' '}
                    ·{' '}
                    {program.orderScope === 'both'
                      ? '新购和续费'
                      : program.orderScope === 'purchase'
                        ? '仅新购'
                        : '仅续费'}{' '}
                    · 已使用 {program.usedCount}
                    {program.totalLimit ? ` / ${program.totalLimit}` : ''}
                  </p>
                  {program.triggerType === 'promo_code' && (
                    <p className="mb-0 mt-0.5 text-[11px] text-slate-400 dark:text-slate-400">
                      推广码：
                      <b className="text-slate-600 dark:text-slate-200">
                        {program.codeMask || '未配置'}
                      </b>{' '}
                      · 渠道：{program.channelLabel || '未标记'} · 每人{' '}
                      {program.codePerUserLimit || '不限'} 次
                    </p>
                  )}
                  <p className="mb-0 mt-0.5 text-[11px] text-slate-400 dark:text-slate-400">
                    预算 {money(program.cashSpentFen)} /{' '}
                    {program.cashBudgetFen
                      ? money(program.cashBudgetFen)
                      : '不限'}{' '}
                    · 赠送天数 {program.grantDaysUsed} /{' '}
                    {program.grantDaysLimit || '不限'}
                  </p>
                </div>
                <div className="flex shrink-0 flex-wrap gap-2 max-[760px]:w-full">
                  <Button
                    tone="secondary"
                    onClick={() => {
                      setEditing({ ...program, code: '' })
                      setStep(1)
                      setMessage('')
                    }}
                  >
                    编辑
                  </Button>
                  {program.triggerType === 'targeted' && (
                    <Button
                      tone="secondary"
                      onClick={() => {
                        setGrantProgram(program)
                        setMessage('')
                      }}
                    >
                      定向发放
                    </Button>
                  )}
                  <Button
                    tone="secondary"
                    loading={isSaving}
                    onClick={() =>
                      void save({
                        ...program,
                        status:
                          program.status === 'active' ? 'paused' : 'active'
                      })
                        .unwrap()
                        .then(() => void refetch())
                    }
                  >
                    {program.status === 'active' ? '暂停' : '启用'}
                  </Button>
                </div>
              </article>
            ))}
          </div>
        )}
      </section>

      {editing && (
        <Dialog
          className="max-w-2xl"
          eyebrow={`配置进度 ${step} / 4`}
          title="商业权益方案"
          description="按步骤配置，保存后不会修改已生成订单的权益快照。"
          onClose={closeEditor}
        >
          <div className="grid gap-4 pb-1">
            {step === 1 && (
              <>
                <label className={dialogLabelClass}>
                  方案名称
                  <input
                    data-autofocus
                    className={dialogFieldClass}
                    value={editing.name}
                    placeholder="例如：首购限时立减"
                    onChange={event =>
                      updateEditing({ name: event.target.value })
                    }
                  />
                </label>
                <div className="grid gap-4 sm:grid-cols-2">
                  <label className={dialogLabelClass}>
                    运营目标
                    <select
                      className={dialogFieldClass}
                      value={editing.goal}
                      onChange={event =>
                        updateEditing({
                          goal: event.target.value as BenefitProgram['goal']
                        })
                      }
                    >
                      <option value="first_purchase">首购转化</option>
                      <option value="multi_month">多月购买</option>
                      <option value="renewal_recovery">续费挽回</option>
                      <option value="channel">渠道推广</option>
                    </select>
                  </label>
                  <label className={dialogLabelClass}>
                    适用订单
                    <select
                      className={dialogFieldClass}
                      value={editing.orderScope}
                      onChange={event =>
                        updateEditing({
                          orderScope: event.target
                            .value as BenefitProgram['orderScope']
                        })
                      }
                    >
                      <option value="purchase">仅新购</option>
                      <option value="renewal">仅续费</option>
                      <option value="both">新购和续费</option>
                    </select>
                  </label>
                </div>
              </>
            )}
            {step === 2 && (
              <>
                <label className={dialogLabelClass}>
                  触发方式
                  <select
                    className={dialogFieldClass}
                    value={editing.triggerType}
                    onChange={event =>
                      updateEditing({
                        triggerType: event.target
                          .value as BenefitProgram['triggerType']
                      })
                    }
                  >
                    <option value="automatic">自动匹配</option>
                    <option value="promo_code">推广码</option>
                    <option value="targeted">定向发放</option>
                  </select>
                </label>
                {editing.triggerType === 'promo_code' && (
                  <label className={dialogLabelClass}>
                    唯一推广码
                    <input
                      className={dialogFieldClass}
                      value={editing.code ?? ''}
                      placeholder={editing.codeMask ?? 'PARTNER2026'}
                      onChange={event =>
                        updateEditing({ code: event.target.value })
                      }
                    />
                  </label>
                )}
                <label className={dialogLabelClass}>
                  适用人群
                  <select
                    className={dialogFieldClass}
                    value={editing.audienceType}
                    onChange={event =>
                      updateEditing({ audienceType: event.target.value })
                    }
                  >
                    <option value="all">所有用户</option>
                    <option value="first_paid">首次付费用户</option>
                    <option value="first_plan">首次购买此套餐</option>
                    <option value="expiring">实例即将到期</option>
                    <option value="lapsed">实例已到期</option>
                    <option value="targeted">指定用户</option>
                  </select>
                </label>
              </>
            )}
            {step === 3 && (
              <>
                <div className="grid gap-4 sm:grid-cols-2">
                  <label className={dialogLabelClass}>
                    权益类型
                    <select
                      className={dialogFieldClass}
                      value={editing.benefitType}
                      onChange={event =>
                        updateEditing({
                          benefitType: event.target
                            .value as BenefitProgram['benefitType']
                        })
                      }
                    >
                      <option value="fixed_discount">立减金额（分）</option>
                      <option value="percent_discount">折扣（万分比）</option>
                      <option value="bonus_days">赠送天数</option>
                    </select>
                  </label>
                  <label className={dialogLabelClass}>
                    权益值
                    <input
                      className={dialogFieldClass}
                      type="number"
                      min="0"
                      value={editing.benefitValue}
                      onChange={event =>
                        updateEditing({
                          benefitValue: Number(event.target.value)
                        })
                      }
                    />
                  </label>
                </div>
                <fieldset>
                  <legend className={dialogLabelClass}>
                    限定套餐{' '}
                    <span className="font-normal text-slate-400">
                      不选表示全部套餐
                    </span>
                  </legend>
                  <div className="mt-2 grid gap-2 sm:grid-cols-2">
                    {plans.map(plan => (
                      <label
                        key={plan.id}
                        className="flex cursor-pointer items-center gap-2 rounded-md border border-slate-200 px-3 py-2 text-xs text-slate-700 dark:border-slate-600 dark:text-slate-100"
                      >
                        <input
                          type="checkbox"
                          checked={editing.planIds.includes(plan.id)}
                          onChange={() => togglePlan(plan.id)}
                        />
                        {plan.name}
                      </label>
                    ))}
                  </div>
                </fieldset>
                <fieldset>
                  <legend className={dialogLabelClass}>
                    限定购买月数{' '}
                    <span className="font-normal text-slate-400">
                      不选表示全部周期
                    </span>
                  </legend>
                  <div className="mt-2 flex flex-wrap gap-2">
                    {subscriptionMonths.map(month => (
                      <button
                        key={month}
                        type="button"
                        onClick={() => toggleMonth(month)}
                        className={`rounded-md border px-2.5 py-1.5 text-[11px] font-bold ${editing.monthValues.includes(month) ? 'border-blue-600 bg-blue-600 text-white' : 'border-slate-200 text-slate-600 hover:border-blue-300 dark:border-slate-600 dark:text-slate-200'}`}
                      >
                        {month} 月
                      </button>
                    ))}
                  </div>
                </fieldset>
              </>
            )}
            {step === 4 && (
              <>
                <div className="grid gap-4 sm:grid-cols-2">
                  <label className={dialogLabelClass}>
                    开始时间
                    <input
                      className={dialogFieldClass}
                      type="datetime-local"
                      value={toLocalInput(editing.startsAt)}
                      onChange={event =>
                        updateEditing({
                          startsAt: event.target.value || undefined
                        })
                      }
                    />
                  </label>
                  <label className={dialogLabelClass}>
                    结束时间
                    <input
                      className={dialogFieldClass}
                      type="datetime-local"
                      value={toLocalInput(editing.endsAt)}
                      onChange={event =>
                        updateEditing({
                          endsAt: event.target.value || undefined
                        })
                      }
                    />
                  </label>
                </div>
                <div className="grid gap-4 sm:grid-cols-2">
                  <label className={dialogLabelClass}>
                    最低消费（分）
                    <input
                      className={dialogFieldClass}
                      type="number"
                      min="0"
                      value={editing.minAmountFen}
                      onChange={event =>
                        updateEditing({
                          minAmountFen: Number(event.target.value)
                        })
                      }
                    />
                  </label>
                  <label className={dialogLabelClass}>
                    优先级
                    <input
                      className={dialogFieldClass}
                      type="number"
                      min="0"
                      value={editing.priority}
                      onChange={event =>
                        updateEditing({ priority: Number(event.target.value) })
                      }
                    />
                  </label>
                  <label className={dialogLabelClass}>
                    每用户最多使用次数
                    <input
                      className={dialogFieldClass}
                      type="number"
                      min="0"
                      value={editing.perUserLimit}
                      onChange={event =>
                        updateEditing({
                          perUserLimit: Number(event.target.value)
                        })
                      }
                    />
                  </label>
                  <label className={dialogLabelClass}>
                    总使用次数
                    <input
                      className={dialogFieldClass}
                      type="number"
                      min="0"
                      value={editing.totalLimit}
                      onChange={event =>
                        updateEditing({
                          totalLimit: Number(event.target.value)
                        })
                      }
                    />
                  </label>
                  <label className={dialogLabelClass}>
                    现金预算（分）
                    <input
                      className={dialogFieldClass}
                      type="number"
                      min="0"
                      value={editing.cashBudgetFen}
                      onChange={event =>
                        updateEditing({
                          cashBudgetFen: Number(event.target.value)
                        })
                      }
                    />
                  </label>
                  <label className={dialogLabelClass}>
                    赠送天数总量
                    <input
                      className={dialogFieldClass}
                      type="number"
                      min="0"
                      value={editing.grantDaysLimit}
                      onChange={event =>
                        updateEditing({
                          grantDaysLimit: Number(event.target.value)
                        })
                      }
                    />
                  </label>
                </div>
                <label className={dialogLabelClass}>
                  渠道标识{' '}
                  <span className="font-normal text-slate-400">可选</span>
                  <input
                    className={dialogFieldClass}
                    value={editing.channelLabel ?? ''}
                    placeholder="例如：partner-2026"
                    onChange={event =>
                      updateEditing({ channelLabel: event.target.value })
                    }
                  />
                </label>
              </>
            )}
            {message && <Alert tone="error">{message}</Alert>}
          </div>
          <DialogFooter>
            <Button tone="secondary" onClick={closeEditor}>
              取消
            </Button>
            {step > 1 && (
              <Button
                tone="secondary"
                onClick={() => setStep(current => current - 1)}
              >
                上一步
              </Button>
            )}
            {step < 4 ? (
              <Button onClick={() => setStep(current => current + 1)}>
                下一步
              </Button>
            ) : (
              <Button loading={isSaving} onClick={submit}>
                保存方案
              </Button>
            )}
          </DialogFooter>
        </Dialog>
      )}

      {grantProgram && (
        <Dialog
          eyebrow="定向权益"
          title={grantProgram.name}
          description="用户无需领取，在结算时自动命中；此处可追踪和作废已发放记录。"
          onClose={() => {
            setGrantProgram(null)
            setMessage('')
          }}
        >
          <div className="grid gap-4">
            <label className={dialogLabelClass}>
              用户 ID{' '}
              <span className="font-normal text-slate-400">
                支持逗号或换行分隔
              </span>
              <textarea
                data-autofocus
                className={`${dialogFieldClass} min-h-28 resize-y`}
                value={userIDs}
                placeholder="填写一个或多个用户 ID"
                onChange={event => setUserIDs(event.target.value)}
              />
            </label>
            <label className={dialogLabelClass}>
              有效至 <span className="font-normal text-slate-400">可选</span>
              <input
                className={dialogFieldClass}
                type="datetime-local"
                value={expiresAt}
                onChange={event => setExpiresAt(event.target.value)}
              />
            </label>
            {message && <Alert tone="error">{message}</Alert>}
            <div className="overflow-hidden rounded-lg border border-slate-200 dark:border-slate-700">
              <div className="border-b border-slate-100 bg-slate-50 px-3 py-2 text-[10px] font-bold text-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300">
                已发放记录
              </div>
              <div className="max-h-48 overflow-y-auto divide-y divide-slate-100 dark:divide-slate-700">
                {(grants.data ?? []).length === 0 ? (
                  <p className="m-0 px-3 py-4 text-center text-xs text-slate-500">
                    暂无发放记录
                  </p>
                ) : (
                  (grants.data ?? []).map(item => (
                    <div
                      key={item.id}
                      className="flex items-center justify-between gap-3 px-3 py-2.5 text-xs"
                    >
                      <span className="min-w-0 truncate text-slate-700 dark:text-slate-100">
                        {item.ownerId}
                        <small className="ml-2 text-slate-400">
                          {item.status}
                        </small>
                      </span>
                      {item.status === 'available' && (
                        <Button
                          tone="secondary"
                          className="min-h-8 px-2.5 text-[10px]"
                          loading={isVoiding}
                          onClick={() =>
                            void voidGrant({
                              id: grantProgram.id,
                              grantId: item.id
                            })
                              .unwrap()
                              .then(() => void grants.refetch())
                              .catch(error =>
                                setMessage(
                                  errorMessage(error, '作废失败，请稍后重试。')
                                )
                              )
                          }
                        >
                          作废
                        </Button>
                      )}
                    </div>
                  ))
                )}
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button tone="secondary" onClick={() => setGrantProgram(null)}>
              取消
            </Button>
            <Button
              loading={isGranting}
              disabled={split(userIDs).length === 0}
              onClick={submitGrant}
            >
              发放权益
            </Button>
          </DialogFooter>
        </Dialog>
      )}
    </section>
  )
}
