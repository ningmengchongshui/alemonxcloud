import { useEffect, useState } from 'react'
import { useAuthorizeMutation, useCallbackMutation, useDevLoginMutation, useGetInstancesQuery, useGetSessionQuery, useLogoutMutation } from '@/services/cloudApi'
import { useAppDispatch, useAppSelector } from '@/store/hooks'
import { setPage } from '@/store/uiSlice'
import { Shell } from './components/Shell'
import { DashboardPage } from './pages/DashboardPage'
import { LoginPage } from './pages/LoginPage'

function message(error: unknown, fallback: string) { return typeof error === 'object' && error !== null && 'data' in error && typeof error.data === 'object' && error.data !== null && 'message' in error.data && typeof error.data.message === 'string' ? error.data.message : fallback }

export default function App() {
  const dispatch = useAppDispatch(); const page = useAppSelector(state => state.ui.page)
  const [error, setError] = useState(''); const { data: session, isLoading } = useGetSessionQuery()
  const { data: instances = [], isLoading: instancesLoading } = useGetInstancesQuery(undefined, { skip: !session })
  const [authorize] = useAuthorizeMutation(); const [callback] = useCallbackMutation(); const [devLogin] = useDevLoginMutation(); const [logout] = useLogoutMutation()
  useEffect(() => { const params = new URLSearchParams(window.location.search); const code = params.get('code'); const state = params.get('state'); if (!code || !state) return; void callback({ code, state }).unwrap().then(() => window.history.replaceState({}, '', '/')).catch(value => setError(message(value, '统一认证失败'))) }, [callback])
  async function login() { setError(''); try { window.location.assign((await authorize(`${window.location.origin}/callback`).unwrap()).authorizeURL) } catch (value) { setError(message(value, '无法发起登录')) } }
  async function loginAsDeveloper() { setError(''); try { await devLogin().unwrap() } catch (value) { setError(message(value, '开发登录不可用')) } }
  async function signOut() { await logout().unwrap(); dispatch(setPage('instances')) }
  if (isLoading) return <main className="auth"><section className="login"><div className="login-card"><h2>正在恢复登录状态…</h2></div></section></main>
  if (!session) return <LoginPage error={error} onLogin={login} onDevLogin={loginAsDeveloper} />
  return <Shell user={session.user} page={page} onPageChange={next => dispatch(setPage(next))} onLogout={signOut}><DashboardPage instances={instances} loading={instancesLoading} page={page} onCreate={() => dispatch(setPage('create'))} onCreated={() => dispatch(setPage('instances'))} /></Shell>
}
