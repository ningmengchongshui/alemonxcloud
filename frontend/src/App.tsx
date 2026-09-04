import { useEffect, useState } from 'react'
import { cloudApi } from '@/services/cloudApi'
import { useAppDispatch, useAppSelector } from '@/store/hooks'
import { setPage } from '@/store/uiSlice'
import type { CurrentUser, Instance } from '@/types/cloud'
import { Shell } from './components/Shell'
import { DashboardPage } from './pages/DashboardPage'
import { LoginPage } from './pages/LoginPage'

export default function App() {
  const dispatch = useAppDispatch()
  const page = useAppSelector(state => state.ui.page)
  const [user, setUser] = useState<CurrentUser | null>(null)
  const [instances, setInstances] = useState<Instance[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  useEffect(() => { void (async () => { try { const p = new URLSearchParams(window.location.search); if (p.get('code') && p.get('state')) { await cloudApi.callback(p.get('code')!, p.get('state')!); window.history.replaceState({}, '', '/') }; const s = await cloudApi.session(); if (s) { setUser(s.user); setInstances(await cloudApi.instances()) } } catch (e) { setError(e instanceof Error ? e.message : '无法恢复登录状态') } finally { setLoading(false) } })() }, [])
  async function login() { setError(''); try { window.location.assign((await cloudApi.authorize(`${window.location.origin}/callback`)).authorizeURL) } catch (e) { setError(e instanceof Error ? e.message : '无法发起登录') } }
  async function devLogin() { setError(''); try { const session = await cloudApi.devLogin(); setUser(session.user); setInstances(await cloudApi.instances()) } catch (e) { setError(e instanceof Error ? e.message : '开发登录不可用') } }
  async function logout() { await cloudApi.logout(); setUser(null); setInstances([]); dispatch(setPage('instances')) }
  if (loading) return <main className="auth"><section className="login"><div className="login-card"><h2>正在恢复登录状态…</h2></div></section></main>
  if (!user) return <LoginPage error={error} onLogin={login} onDevLogin={devLogin} />
  return <Shell user={user} page={page} onPageChange={next => dispatch(setPage(next))} onLogout={logout}><DashboardPage instances={instances} page={page} onCreate={() => dispatch(setPage('create'))} onCreated={item => { setInstances(current => [item, ...current]); dispatch(setPage('instances')) }} /></Shell>
}
