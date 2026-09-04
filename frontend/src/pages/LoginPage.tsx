import { useState } from 'react'
import { BrandLogo } from '@/components/BrandLogo'

export function LoginPage({
  error,
  onLogin,
  onDevLogin
}: {
  error: string
  onLogin: () => Promise<void>
  onDevLogin: () => Promise<void>
}) {
  const [submitting, setSubmitting] = useState<'oauth' | 'dev' | null>(null)

  async function submit(method: 'oauth' | 'dev') {
    setSubmitting(method)
    try {
      await (method === 'oauth' ? onLogin() : onDevLogin())
    } finally {
      setSubmitting(null)
    }
  }

  return (
    <main className="grid min-h-screen grid-cols-[minmax(0,1.28fr)_minmax(430px,.72fr)] bg-slate-50 dark:bg-slate-900 max-[850px]:block">
      <section className="relative isolate overflow-hidden bg-gradient-to-br from-slate-950 via-slate-900 to-cyan-950 px-[clamp(44px,8vw,132px)] py-10 text-white max-[850px]:hidden">
        <div className="pointer-events-none absolute inset-0 opacity-25 [background-image:linear-gradient(#b5d4ff0f_1px,transparent_1px),linear-gradient(90deg,#b5d4ff0f_1px,transparent_1px)] [background-size:42px_42px]" aria-hidden="true" />
        <div className="relative z-10 mb-10 flex items-center gap-2.5">
          <BrandLogo />
          <span className="rounded-full border border-blue-200/20 px-2 py-1 text-[10px] font-semibold text-blue-100">云端部署平台</span>
        </div>
        <div className="relative z-10 my-[clamp(80px,18vh,190px)] max-w-2xl">
          <p className="mb-5 inline-block rounded-full bg-emerald-300/10 px-2.5 py-1.5 text-[10px] font-extrabold tracking-widest text-emerald-200">专为 ALEMONX 构建</p>
          <h1 className="mb-5 text-[clamp(43px,4.9vw,74px)] leading-[1.08] font-bold tracking-[-.05em]">
            让 AlemonX，<br />
            <span className="text-emerald-300">快速上线，稳定运行。</span>
          </h1>
          <p className="max-w-xl text-[15px] leading-7 text-slate-300">选择镜像和算力，部署与运行交给 AlemonX Cloud。</p>
          <div className="mt-12 grid grid-cols-3 gap-2.5" aria-label="平台核心价值">
            <article className="rounded-xl border border-blue-100/15 bg-white/5 p-4">
              <strong className="mb-4 block text-[10px] tracking-widest text-emerald-300">01</strong>
              <div><b>快速创建</b><span>选定配置即可部署</span></div>
            </article>
            <article className="rounded-xl border border-blue-100/15 bg-white/5 p-4">
              <strong>02</strong>
              <div><b>独享空间</b><span>实例隔离，资源可控</span></div>
            </article>
            <article className="rounded-xl border border-blue-100/15 bg-white/5 p-4">
              <strong>03</strong>
              <div><b>统一管理</b><span>订单和服务状态清晰</span></div>
            </article>
          </div>
        </div>
        <div className="relative z-10 flex justify-between text-[11px] text-slate-300"><span><i className="mr-2 inline-block size-2 rounded-full bg-emerald-300 shadow-[0_0_0_4px_rgba(110,231,183,.15)]" />服务运行状态正常</span><span>为稳定交付而设计</span></div>
      </section>
      <section className="grid place-items-center px-[clamp(28px,5vw,72px)] py-8 max-[850px]:min-h-screen">
        <div className="w-full max-w-92">
          <div className="mb-20 hidden max-[850px]:flex"><BrandLogo /></div>
          <p className="mb-2 text-[10px] font-extrabold tracking-widest text-blue-600">开始管理你的服务</p>
          <h2 className="mb-2 text-3xl font-bold tracking-tight text-slate-800 dark:text-white">欢迎回来</h2>
          <p className="mb-7 leading-6 text-slate-500 dark:text-slate-300">登录后即可创建实例、查看订单，并持续掌握每一项服务的运行状态。</p>
          <button className="mb-3 h-13 w-full rounded-lg bg-gradient-to-r from-blue-600 to-blue-500 text-sm font-bold text-white shadow-lg shadow-blue-500/25 hover:from-blue-700 hover:to-blue-600 disabled:opacity-60" onClick={() => void submit('oauth')} disabled={submitting !== null}>
            <b>◉</b>{submitting === 'oauth' ? '正在跳转认证中心…' : '使用 BubbleAuth 继续'}
          </button>
          {import.meta.env.DEV && (
            <button className="mt-2 h-13 w-full rounded-lg border border-dashed border-slate-300 bg-white text-[11px] font-bold text-slate-500 hover:bg-slate-50 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-300" onClick={() => void submit('dev')} disabled={submitting !== null}>
              {submitting === 'dev' ? '正在进入开发环境…' : '开发模式：以内置超级管理员进入'}
            </button>
          )}
          {error && (
            <p className="my-3 rounded-lg border border-red-200 bg-red-50 px-3 py-2.5 text-[11px] leading-4 text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-200" role="alert">
              {error}
            </p>
          )}
          <p className="mt-5 text-center text-[11px] leading-5 text-slate-400">由统一认证中心安全保护你的登录会话。</p>
        </div>
      </section>
    </main>
  )
}
