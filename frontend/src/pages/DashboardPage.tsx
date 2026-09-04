import { useState } from 'react'
import { useGetWalletQuery, useInstanceActionMutation, useLazyGetInstanceLogsQuery, usePurchaseMutation } from '@/services/cloudApi'
import type { Catalog, Instance, Page, Plan } from '@/types/cloud'

const money = (fen: number) => `¥${(fen / 100).toFixed(2)}`
const instanceState = (status: string) => {
  const normalized = status.toLowerCase()
  if (['running', 'active', 'online', '运行中'].includes(normalized)) return { label: '运行中', tone: 'success' }
  if (['creating', 'deploying', 'pending', '部署中', '等待节点接入'].includes(normalized)) return { label: '部署中', tone: 'pending' }
  if (['failed', 'error', 'stopped', 'expired', '已停止', '已过期'].includes(normalized)) return { label: '需要处理', tone: 'danger' }
  return { label: status || '状态同步中', tone: 'neutral' }
}

export function DashboardPage({
  instances,
  loading,
  page,
  catalog,
	catalogLoading,
	catalogError,
	onRetryCatalog,
  onCreate,
  onCreated,
  onViewOrders
}: {
  instances: Instance[]
  loading: boolean
  page: Page
  catalog?: Catalog
	catalogLoading: boolean
	catalogError: boolean
	onRetryCatalog: () => void
  onCreate: () => void
  onCreated: () => void
  onViewOrders: () => void
}) {
	const [imageID, setImageID] = useState('')
	const [imageRef, setImageRef] = useState('')
	const [imageVersion, setImageVersion] = useState('latest')
  const [planID, setPlanID] = useState('')
  const [months, setMonths] = useState(1)
	const [error, setError] = useState('')
	const [purchase, { isLoading: saving }] = usePurchaseMutation()
	const { data: wallet } = useGetWalletQuery()
  const [operateInstance, { isLoading: operating }] = useInstanceActionMutation()
  const [loadLogs, { isLoading: loadingLogs }] = useLazyGetInstanceLogsQuery()
  const [logs, setLogs] = useState<{ name: string; lines: string[] } | null>(null)
  const images = catalog?.images ?? []
	const imageGroups = Array.from(new Map(images.map(item => [item.imageRef, { name: item.name, imageRef: item.imageRef }])).values())
	const selectedImageRef = imageRef || imageGroups[0]?.imageRef || ''
	const sourceImages = images.filter(item => item.imageRef === selectedImageRef)
	const plans = catalog?.plans ?? []
	const image = imageID || sourceImages[0]?.id || ''
  const plan = planID || plans[0]?.id || ''
  const selectedImage = images.find(value => value.id === image)
  const selectedPlan = plans.find(value => value.id === plan)

  if (page === 'create') {
    const total = selectedPlan ? selectedPlan.monthlyPriceFen * months : 0
    return (
      <section className="page me-page create-page">
        <header className="page-heading">
          <div>
            <p className="eyebrow">创建服务</p>
            <h1>创建运行环境</h1>
            <p>选择镜像和套餐，提交后等待付款确认。</p>
          </div>
          <div className="step-hint"><b>01</b><span>选择资源</span><i /><b>02</b><span>确认订单</span></div>
        </header>
        <div className="create-layout">
          <div className="create-form">
            <section className="selection-section">
              <div className="selection-title"><span className="selection-number">1</span><div><h2>选择镜像</h2><p>仅展示已审核的镜像来源。</p></div></div>
              {catalogLoading ? <div className="catalog-empty">加载镜像中…</div> : catalogError ? <div className="catalog-empty">镜像加载失败。<button className="text-button" onClick={onRetryCatalog}>重试</button></div> : imageGroups.length ? <><div className="choice-grid image-grid">{imageGroups.map(value => <button key={value.imageRef} type="button" className={`catalog-choice ${selectedImageRef === value.imageRef ? 'selected' : ''}`} aria-pressed={selectedImageRef === value.imageRef} onClick={() => { setImageRef(value.imageRef); setImageID(''); setError('') }}><span className="choice-mark">{selectedImageRef === value.imageRef ? '✓' : ''}</span><span><b>{value.name}</b><small>{value.imageRef}</small></span></button>)}</div><label htmlFor="image-version">镜像版本 <small>默认 latest</small></label><input id="image-version" value={imageVersion} maxLength={64} onChange={event=>setImageVersion(event.target.value)} placeholder="latest" /></> : <div className="catalog-empty">暂无可售镜像。</div>}
            </section>
            <section className="selection-section">
              <div className="selection-title"><span className="selection-number">2</span><div><h2>选择套餐</h2><p>按需选择 CPU 和内存。</p></div></div>
              {catalogLoading ? <div className="catalog-empty">加载套餐中…</div> : catalogError ? <div className="catalog-empty">套餐加载失败。<button className="text-button" onClick={onRetryCatalog}>重试</button></div> : plans.length ? <div className="choice-grid plan-grid">
                {plans.map(value => <PlanChoice key={value.id} plan={value} selected={plan === value.id} onSelect={() => { setPlanID(value.id); setError('') }} />)}
              </div> : <div className="catalog-empty">暂无可售套餐。</div>}
            </section>
            <section className="selection-section compact-section">
              <div className="selection-title"><span className="selection-number">3</span><div><h2>选择周期</h2><p>费用将从账户余额扣除。</p></div></div>
              <div className="period-controls" role="group" aria-label="订阅周期">
                {[1, 3, 6, 12].map(value => <button type="button" key={value} className={months === value ? 'selected' : ''} aria-pressed={months === value} onClick={() => setMonths(value)}>{value} 个月</button>)}
              </div>
            </section>
          </div>
          <aside className="order-summary" aria-live="polite">
            <p className="eyebrow">订单预览</p>
            <h2>确认你的选择</h2>
            <dl>
	              <div><dt>镜像</dt><dd>{selectedImage ? `${selectedImage.name} · ${imageVersion || 'latest'}` : '请选择镜像'}</dd></div>
              <div><dt>套餐</dt><dd>{selectedPlan?.name || '请选择套餐'}</dd></div>
              <div><dt>资源</dt><dd>{selectedPlan ? `${selectedPlan.cpu} 核 CPU · ${selectedPlan.memoryMB / 1024} GB 内存` : '—'}</dd></div>
              <div><dt>周期</dt><dd>{months} 个月</dd></div>
            </dl>
            <div className="summary-total"><span>应付代币</span><strong>{selectedPlan ? `${(total / 100).toFixed(2)} 代币` : '—'}</strong></div>
            {error && <p className="form-error" role="alert">{error}</p>}
	            <button className="primary full" disabled={saving || !image || !plan || !imageVersion.trim()} onClick={() => void purchase({ imageId: image, imageVersion: imageVersion.trim(), planId: plan, months }).unwrap().then(onCreated).catch(value => setError(typeof value === 'object' && value !== null && 'data' in value ? '代币余额、镜像版本或节点资源不足，请检查后重试' : '购买失败，请检查网络后重试'))}>
              {saving ? '正在扣款并安排部署…' : '使用代币直接购买'}
            </button>
            <p className="summary-note">购买后立即进入部署队列，可在订单中心追踪状态。</p>
          </aside>
        </div>
      </section>
    )
  }

  const running = instances.filter(item => instanceState(item.status).tone === 'success').length
  const progressing = instances.filter(item => instanceState(item.status).tone === 'pending').length
  return (
    <section className="page me-page dashboard-page">
      <header className="page-heading">
        <div><p className="eyebrow">我的服务</p><h1>实例总览</h1><p>查看实例状态，快速创建新服务。</p></div>
        <button className="primary create-action" onClick={onCreate}><span>＋</span> 创建服务</button>
      </header>
	      <section className="service-overview" aria-label="服务概览">
	        <article className="overview-primary"><span>已运行服务</span><strong>{loading ? '—' : running}<small> / {loading ? '—' : instances.length} 个实例</small></strong><p>{progressing ? `${progressing} 个服务正在部署中` : '所有服务状态已同步'}</p></article>
	        <article className="overview-card"><span className="overview-icon">◈</span><div><small>代币余额</small><b>{wallet ? `${(wallet.balanceFen / 100).toFixed(2)} 代币` : '同步中'}</b><p>1 代币 = ¥1.00</p></div></article>
        <article className="overview-card"><span className="overview-icon">◌</span><div><small>运行空间</small><b>独享实例隔离</b><p>资源与访问路径清晰可控</p></div></article>
        <article className="overview-card"><span className="overview-icon">↗</span><div><small>下一步</small><b>创建新的服务</b><button onClick={onCreate}>选择镜像和套餐 →</button></div></article>
      </section>
      <section className="instance-section">
        <div className="section-heading"><div><h2>实例列表</h2><p>{loading ? '正在同步实例状态…' : `${instances.length} 个实例`}</p></div>{instances.length > 0 && <button className="subtle-button" onClick={onCreate}>创建服务</button>}</div>
        {loading ? <div className="instance-panel loading-panel"><span className="loading-dot" />正在加载实例…</div> : instances.length === 0 ? <div className="instance-panel empty-instance"><div className="empty-icon">＋</div><h2>从第一项服务开始</h2><p>选择经过审核的镜像和合适的算力套餐，创建订单后即可开始部署。</p><button className="primary" onClick={onCreate}>创建服务</button></div> : <div className="instance-list">{instances.map(item => {
          const state = instanceState(item.status)
          const action = state.tone === 'success' ? 'stop' : item.status === 'stopped' ? 'start' : null
          return <article className="instance-row" key={item.id}><div className="instance-name"><span className="instance-avatar">A</span><div><h3>{item.name}</h3><p>{item.image} · {item.version}</p></div></div><div className="instance-resource"><small>配置</small><b>{item.spec}</b></div><div className="instance-state"><small>状态</small><span className={`status-badge ${state.tone}`}><i />{state.label}</span></div><div className="instance-access">{item.ip ? <a href={item.ip} target="_blank" rel="noreferrer">打开服务 <span>↗</span></a> : <span>地址准备中</span>}<button className="text-button" disabled={loadingLogs} onClick={() => void loadLogs(item.id).unwrap().then(value => setLogs({name:item.name,lines:value.lines}))}>日志</button>{action && <button className="text-button" disabled={operating} onClick={() => { if(window.confirm(`确定${action==='stop'?'停止':'启动'}此实例吗？`)) void operateInstance({id:item.id,action}) }}>{action==='stop'?'停止':'启动'}</button>}<button className="text-button" disabled={operating} onClick={() => { if(window.confirm('确定删除实例吗？服务会停止，数据保留 7 天。')) void operateInstance({id:item.id,action:'delete'}) }}>删除</button></div></article>
        })}</div>}
      </section>
      <section className="dashboard-help"><div><b>服务还未出现？</b><span>订单确认后会自动部署。</span></div><button onClick={onViewOrders}>查看订单 →</button></section>
      {logs && <div className="modal-backdrop" onMouseDown={event => { if (event.target === event.currentTarget) setLogs(null) }}><section className="modal-card logs-modal" role="dialog" aria-modal="true" aria-label={`${logs.name} 日志`}><div className="modal-heading"><div><p className="eyebrow">实例日志</p><h2>{logs.name} · 最近日志</h2><p>展示最近一次采集到的运行输出。</p></div><button className="modal-close" onClick={() => setLogs(null)} aria-label="关闭日志弹窗">×</button></div><pre className="instance-logs">{logs.lines.join('\n') || '暂无日志'}</pre></section></div>}
    </section>
  )
}

function PlanChoice({ plan, selected, onSelect }: { plan: Plan; selected: boolean; onSelect: () => void }) {
  return <button type="button" className={`catalog-choice plan-choice ${selected ? 'selected' : ''}`} aria-pressed={selected} onClick={onSelect}><span className="choice-mark">{selected ? '✓' : ''}</span><span><b>{plan.name}</b><small>{plan.cpu} 核 CPU · {plan.memoryMB / 1024} GB 内存</small></span><em>{money(plan.monthlyPriceFen)}<small>/月</small></em></button>
}
