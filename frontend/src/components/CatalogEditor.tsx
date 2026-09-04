import { useEffect, useState } from 'react'
import { useSaveAdminImageMutation, useSaveAdminPlanMutation } from '@/services/cloudApi'

type EditorMode = 'image' | 'plan' | null

export function CatalogEditor() {
  const [mode, setMode] = useState<EditorMode>(null)
  const [imageName, setImageName] = useState('')
  const [imageRef, setImageRef] = useState('')
  const [planName, setPlanName] = useState('')
  const [cpu, setCPU] = useState(2)
  const [memory, setMemory] = useState(4096)
  const [price, setPrice] = useState(9900)
  const [error, setError] = useState('')
  const [saveImage, { isLoading: imageSaving }] = useSaveAdminImageMutation()
  const [savePlan, { isLoading: planSaving }] = useSaveAdminPlanMutation()

  useEffect(() => {
    const close = (event: KeyboardEvent) => { if (event.key === 'Escape') setMode(null) }
    document.addEventListener('keydown', close)
    return () => document.removeEventListener('keydown', close)
  }, [])
  function openEditor(nextMode: Exclude<EditorMode, null>) { setError(''); setMode(nextMode) }
  async function addImage() { try { setError(''); await saveImage({ id: '', name: imageName, imageRef, imageDigest: '', version: 'latest', enabled: true }).unwrap(); setImageName(''); setImageRef(''); setMode(null) } catch { setError('镜像来源保存失败，请确认镜像地址') } }
  async function addPlan() { try { setError(''); await savePlan({ id: '', name: planName, cpu, memoryMB: memory, monthlyPriceFen: price, enabled: true, sortOrder: 100 }).unwrap(); setPlanName(''); setMode(null) } catch { setError('套餐保存失败') } }

  return <>
    <div className="catalog-add-actions"><button className="subtle-button" onClick={() => openEditor('image')}>＋ 新增镜像来源</button><button className="primary" onClick={() => openEditor('plan')}>＋ 新增计算套餐</button></div>
    {mode && <div className="modal-backdrop" onMouseDown={event => { if (event.target === event.currentTarget) setMode(null) }}><section className="modal-card compact-modal" role="dialog" aria-modal="true" aria-labelledby="catalog-modal-title"><div className="modal-heading"><div><p className="eyebrow">商品管理</p><h2 id="catalog-modal-title">{mode === 'image' ? '新增镜像来源' : '新增计算套餐'}</h2><p>{mode === 'image' ? '配置一个可供用户选择的可信镜像来源。' : '配置用户可购买的 CPU、内存和月度价格。'}</p></div><button className="modal-close" onClick={() => setMode(null)} aria-label="关闭商品弹窗">×</button></div>{mode === 'image' ? <div className="admin-editor-block"><p className="admin-editor-block-title">镜像来源</p><div className="admin-field"><label htmlFor="catalog-name">镜像名称</label><input id="catalog-name" value={imageName} onChange={event => setImageName(event.target.value)} placeholder="AlemonX" /></div><div className="admin-field"><label htmlFor="catalog-ref">镜像地址</label><input id="catalog-ref" value={imageRef} onChange={event => setImageRef(event.target.value)} placeholder="registry.example/alemonx" /></div><div className="modal-actions"><button className="subtle-button" onClick={() => setMode(null)}>取消</button><button className="primary" disabled={imageSaving || !imageName || !imageRef} onClick={() => void addImage()}>{imageSaving ? '正在保存…' : '保存镜像来源'}</button></div></div> : <div className="admin-editor-block"><p className="admin-editor-block-title">计算套餐</p><div className="admin-field"><label htmlFor="catalog-plan">套餐名称</label><input id="catalog-plan" value={planName} onChange={event => setPlanName(event.target.value)} placeholder="标准版" /></div><div className="admin-editor-fields"><div className="admin-field"><label htmlFor="catalog-cpu">CPU 核数</label><input id="catalog-cpu" type="number" min="1" value={cpu} onChange={event => setCPU(Number(event.target.value))} /></div><div className="admin-field"><label htmlFor="catalog-memory">内存 MB</label><input id="catalog-memory" type="number" min="256" value={memory} onChange={event => setMemory(Number(event.target.value))} /></div></div><div className="admin-field"><label htmlFor="catalog-price">月价（分）</label><input id="catalog-price" type="number" min="0" value={price} onChange={event => setPrice(Number(event.target.value))} /></div><div className="modal-actions"><button className="subtle-button" onClick={() => setMode(null)}>取消</button><button className="primary" disabled={planSaving || !planName} onClick={() => void addPlan()}>{planSaving ? '正在保存…' : '保存计算套餐'}</button></div></div>}{error && <p className="login-error" role="alert">{error}</p>}</section></div>}
  </>
}
