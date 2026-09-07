import { useState } from 'react'
import { BrandLogo } from '@/components/BrandLogo'

export function LoginPage({
  onLogin,
  onDevLogin
}: {
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
    <main className="relative grid min-h-screen place-items-center overflow-hidden bg-[#07111f] px-6 py-10 text-white">
      <div className="pointer-events-none absolute inset-0 opacity-30 [background-image:linear-gradient(#dbeafe0c_1px,transparent_1px),linear-gradient(90deg,#dbeafe0c_1px,transparent_1px)] [background-size:48px_48px]" aria-hidden="true" />
      <div className="pointer-events-none absolute left-1/2 top-1/2 size-[min(72vw,780px)] -translate-x-1/2 -translate-y-1/2 rounded-full bg-blue-500/15 blur-[120px]" aria-hidden="true" />
      <section className="relative z-10 w-full max-w-md text-center">
        <div className="mb-11 flex justify-center">
          <BrandLogo />
        </div>
        <p className="mb-5 text-[11px] font-bold tracking-[.2em] text-cyan-200">
          ALEMONX CLOUD
        </p>
        <h1 className="m-0 text-[clamp(36px,5vw,56px)] leading-[1.12] font-semibold tracking-[-.045em] text-balance">
          让服务稳定地
          <br />运行在云端。
        </h1>
        <p className="mx-auto mt-6 max-w-sm text-[15px] leading-7 text-slate-300">
          为 AlemonX 服务提供清晰、可控的运行环境。
        </p>
        <button
          className="mt-10 h-14 w-full rounded-xl bg-gradient-to-r from-blue-500 to-cyan-400 text-sm font-bold text-slate-950 shadow-xl shadow-blue-500/20 transition-all hover:-translate-y-0.5 hover:from-blue-400 hover:to-cyan-300 focus-visible:outline-3 focus-visible:outline-offset-3 focus-visible:outline-cyan-200 disabled:translate-y-0 disabled:opacity-60"
          onClick={() => void submit('oauth')}
          disabled={submitting !== null}
        >
          {submitting === 'oauth' ? '正在进入 X Cloud…' : '进入 X Cloud'} <span aria-hidden="true">→</span>
        </button>
        {import.meta.env.DEV && (
          <button
            className="mt-3 min-h-10 text-[11px] font-bold text-slate-400 hover:text-white focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-cyan-200"
            onClick={() => void submit('dev')}
            disabled={submitting !== null}
          >
            {submitting === 'dev' ? '正在进入开发环境…' : '开发模式进入'}
          </button>
        )}
      </section>
      <footer className="absolute inset-x-0 bottom-6 z-10 text-center text-[10px] font-medium tracking-wide text-slate-500">为稳定的持续交付而设计</footer>
    </main>
  )
}
