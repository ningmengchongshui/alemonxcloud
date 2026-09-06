import { useEffect, useState } from 'react'
import { type ToastMessage, toast } from '@/services/toast'

const toneStyles = {
  error: 'border-red-200 bg-white text-slate-800 dark:border-red-900 dark:bg-slate-800 dark:text-white',
  success: 'border-emerald-200 bg-white text-slate-800 dark:border-emerald-900 dark:bg-slate-800 dark:text-white',
  info: 'border-blue-200 bg-white text-slate-800 dark:border-blue-900 dark:bg-slate-800 dark:text-white'
}
const icons = { error: '!', success: '✓', info: 'i' }
const iconStyles = { error: 'bg-red-600', success: 'bg-emerald-600', info: 'bg-blue-600' }

export function ToastViewport() {
  const [items, setItems] = useState<ToastMessage[]>([])
  useEffect(() => toast.subscribe(item => {
    setItems(current => [...current.filter(value => value.title !== item.title), item].slice(-4))
    window.setTimeout(() => setItems(current => current.filter(value => value.id !== item.id)), 5000)
  }), [])
  if (items.length === 0) return null
  return (
    <div className="pointer-events-none fixed right-4 top-4 z-[120] grid w-[min(24rem,calc(100vw-2rem))] gap-2" aria-live="assertive" aria-atomic="true">
      {items.map(item => <div key={item.id} className={`pointer-events-auto flex gap-3 rounded-lg border p-3 shadow-xl shadow-slate-950/10 ${toneStyles[item.tone]}`} role={item.tone === 'error' ? 'alert' : 'status'}>
        <span className={`grid size-5 shrink-0 place-items-center rounded-full text-[11px] font-extrabold text-white ${iconStyles[item.tone]}`} aria-hidden="true">{icons[item.tone]}</span>
        <div className="min-w-0 flex-1"><p className="m-0 text-xs font-bold">{item.title}</p>{item.detail && <p className="mb-0 mt-1 text-[11px] leading-4 text-slate-500 dark:text-slate-300">{item.detail}</p>}</div>
        <button type="button" className="-mr-1 -mt-1 grid size-6 place-items-center rounded text-slate-400 hover:bg-slate-100 hover:text-slate-700 dark:hover:bg-slate-700 dark:hover:text-white" aria-label="关闭提示" onClick={() => setItems(current => current.filter(value => value.id !== item.id))}>×</button>
      </div>)}
    </div>
  )
}
