import { useEffect, useState } from 'react'
import { useAuthorizeMutation, useCallbackMutation, useDevLoginMutation, useGetCatalogQuery, useGetInstancesQuery, useGetOrdersQuery, useGetSessionQuery, useLogoutMutation } from '@/services/cloudApi'
import { Shell } from './components/Shell'
import { DashboardPage } from './pages/DashboardPage'
import { LoginPage } from './pages/LoginPage'
import { OrdersPage } from './pages/OrdersPage'
import { AdminPage } from './pages/AdminPage'
import type { SuperPage } from '@/types/cloud'

type UserPage = 'instances' | 'create' | 'orders'
const userPaths = new Set(['/me', '/me/create', '/me/orders'])
const superPaths: Record<SuperPage, string> = { overview: '/super', catalog: '/super/catalog', nodes: '/super/nodes', orders: '/super/orders', tasks: '/super/tasks', users: '/super/users', audit: '/super/audit' }

function message(error: unknown, fallback: string) { return typeof error === 'object' && error !== null && 'data' in error && typeof error.data === 'object' && error.data !== null && 'message' in error.data && typeof error.data.message === 'string' ? error.data.message : fallback }
function currentPath() { return window.location.pathname.replace(/\/+$/, '') || '/' }
function userPageFor(path: string): UserPage { return path === '/me/create' ? 'create' : path === '/me/orders' ? 'orders' : 'instances' }
function superPageFor(path: string): SuperPage | null { return (Object.entries(superPaths).find(([, value]) => value === path)?.[0] as SuperPage | undefined) ?? null }
function safeReturnPath() {
  const value = window.sessionStorage.getItem('alemonxcloud:return-to')
  window.sessionStorage.removeItem('alemonxcloud:return-to')
  return value === '/super' || (value !== null && userPaths.has(value)) ? value : '/me'
}

export default function App() {
  const [path, setPath] = useState(currentPath)
  const [error, setError] = useState('')
  const { data: session, isLoading } = useGetSessionQuery()
  const isUserArea = userPaths.has(path)
  const { data: instances = [], isLoading: instancesLoading } = useGetInstancesQuery(undefined, { skip: !session || !isUserArea })
  const { data: catalog, isLoading: catalogLoading, isError: catalogError, refetch: refetchCatalog } = useGetCatalogQuery(undefined, { skip: !session || !isUserArea })
  const { data: orders = [], isLoading: ordersLoading } = useGetOrdersQuery(undefined, { skip: !session || !isUserArea })
  const [authorize] = useAuthorizeMutation()
  const [callback] = useCallbackMutation()
  const [devLogin] = useDevLoginMutation()
  const [logout] = useLogoutMutation()

  function navigate(nextPath: string, replace = false) {
    if (currentPath() === nextPath) return
    window.history[replace ? 'replaceState' : 'pushState']({}, '', nextPath)
    setPath(nextPath)
  }

  useEffect(() => {
    const onPopState = () => setPath(currentPath())
    window.addEventListener('popstate', onPopState)
    return () => window.removeEventListener('popstate', onPopState)
  }, [])

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const code = params.get('code')
    const state = params.get('state')
    if (!code || !state) return
    void callback({ code, state }).unwrap().then(() => navigate(safeReturnPath(), true)).catch(value => {
      window.history.replaceState({}, '', '/login')
      setPath('/login')
      setError(message(value, '统一认证失败'))
    })
  }, [callback])

  async function login() {
    setError('')
    window.sessionStorage.setItem('alemonxcloud:return-to', path === '/super' ? '/super' : '/me')
    try {
      window.location.assign((await authorize(`${window.location.origin}/callback`).unwrap()).authorizeURL)
    } catch (value) {
      setError(message(value, '无法发起登录'))
      throw value
    }
  }

  async function loginAsDeveloper() {
    setError('')
    try {
      await devLogin().unwrap()
      navigate(safeReturnPath(), true)
    } catch (value) {
      setError(message(value, '开发登录不可用'))
      throw value
    }
  }

  async function signOut() {
    await logout().unwrap()
    navigate('/login', true)
  }

  if (isLoading) return <main className="auth"><section className="login"><div className="login-card"><h2>正在恢复登录状态…</h2></div></section></main>
  if (!session) return <LoginPage error={error} onLogin={login} onDevLogin={loginAsDeveloper} />
  if (path === '/' || path === '/login' || path === '/callback') { navigate('/me', true); return null }
  const superPage = superPageFor(path)
  if (superPage) {
    if (!session.user.isAdmin) { navigate('/me', true); return null }
    return <Shell user={session.user} area="super" superPage={superPage} onSuperPageChange={next => navigate(superPaths[next])} onGoToMe={() => navigate('/me')} onLogout={signOut}><AdminPage page={superPage} /></Shell>
  }
  if (!userPaths.has(path)) { navigate('/me', true); return null }

  const page = userPageFor(path)
  const routeForPage: Record<UserPage, string> = { instances: '/me', create: '/me/create', orders: '/me/orders' }
  const content = page === 'orders'
    ? <OrdersPage orders={orders} loading={ordersLoading} onCreate={() => navigate('/me/create')} />
    : <DashboardPage instances={instances} loading={instancesLoading} page={page} catalog={catalog} catalogLoading={catalogLoading} catalogError={catalogError} onRetryCatalog={() => void refetchCatalog()} onCreate={() => navigate('/me/create')} onCreated={() => navigate('/me/orders')} onViewOrders={() => navigate('/me/orders')} />
  return <Shell user={session.user} area="me" page={page} onPageChange={next => navigate(routeForPage[next])} onGoToMe={() => navigate('/me')} onGoToSuper={session.user.isAdmin ? () => navigate('/super') : undefined} onLogout={signOut}>{content}</Shell>
}
