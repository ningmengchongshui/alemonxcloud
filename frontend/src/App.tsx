import { useEffect, useState } from 'react'
import {
  useAuthorizeMutation,
  useCallbackMutation,
  useDevLoginMutation,
  useGetCatalogQuery,
  useGetInstancesQuery,
  useGetOrdersQuery,
  useGetSessionQuery,
  useLogoutMutation
} from '@/services/cloudApi'
import { Shell } from './components/Shell'
import { CreateServicePage } from './pages/CreateServicePage'
import { InstancesPage } from './pages/InstancesPage'
import { InstanceLogsPage } from './pages/InstanceLogsPage'
import { InstanceExecutionPage } from './pages/InstanceExecutionPage'
import { NotificationsPage } from './pages/NotificationsPage'
import { TicketsPage } from './pages/TicketsPage'
import { WalletPage } from './pages/WalletPage'
import { WalletHistoryPage } from './pages/WalletHistoryPage'
import { UserOverviewPage } from './pages/UserOverviewPage'
import { LoginPage } from './pages/LoginPage'
import { OrdersPage } from './pages/OrdersPage'
import { AdminPage } from './pages/AdminPage'
import type { Page, SuperPage } from '@/types/cloud'

const userPaths = new Set([
  '/me',
  '/me/instances',
  '/me/create',
  '/me/orders',
  '/me/wallet',
  '/me/notifications',
  '/me/tickets'
])
const superPaths: Record<SuperPage, string> = {
  'overview': '/super',
  'catalog': '/super/catalog',
  'images': '/super/images',
  'nodes': '/super/nodes',
  'orders': '/super/orders',
  'tasks': '/super/tasks',
  'users': '/super/users',
  'audit': '/super/audit',
  'tickets': '/super/tickets',
  'benefits': '/super/benefits',
  'benefit-redemptions': '/super/benefit-redemptions',
  'price-tiers': '/super/price-tiers',
  'settings': '/super/settings'
}

function message(error: unknown, fallback: string) {
  return typeof error === 'object' &&
    error !== null &&
    'data' in error &&
    typeof error.data === 'object' &&
    error.data !== null &&
    'message' in error.data &&
    typeof error.data.message === 'string'
    ? error.data.message
    : fallback
}
function currentPath() {
  return window.location.pathname.replace(/\/+$/, '') || '/'
}
function userPageFor(path: string): Page {
  if (path === '/me/instances') return 'instances'
  if (path === '/me/create') return 'create'
  if (path === '/me/orders') return 'orders'
  if (path === '/me/wallet') return 'wallet'
  if (path === '/me/notifications') return 'notifications'
  if (path === '/me/tickets') return 'tickets'
  return 'overview'
}
function superPageFor(path: string): SuperPage | null {
  return (
    (Object.entries(superPaths).find(([, value]) => value === path)?.[0] as
      SuperPage | undefined) ?? null
  )
}
function walletHistoryUserID(path: string) {
  const match = path.match(/^\/super\/users\/([^/]+)\/wallet$/)
  if (!match) return null
  try {
    return decodeURIComponent(match[1])
  } catch {
    return null
  }
}
function instanceLogID(path: string) {
  const match = path.match(/^\/me\/instances\/([^/]+)\/logs$/)
  if (!match) return null
  try {
    return decodeURIComponent(match[1])
  } catch {
    return null
  }
}
function instanceExecutionID(path: string) {
  const match = path.match(/^\/me\/instances\/([^/]+)\/executions$/)
  if (!match) return null
  try {
    return decodeURIComponent(match[1])
  } catch {
    return null
  }
}
function ticketID(path: string) {
  const match = path.match(/^\/me\/tickets\/([^/]+)$/)
  if (!match) return null
  try {
    return decodeURIComponent(match[1])
  } catch {
    return null
  }
}
function safeReturnPath() {
  const value = window.sessionStorage.getItem('alemonxcloud:return-to')
  window.sessionStorage.removeItem('alemonxcloud:return-to')
  return value === '/super' || (value !== null && userPaths.has(value))
    ? value
    : '/me'
}

export default function App() {
  const [path, setPath] = useState(currentPath)
  const [error, setError] = useState('')
  const { data: session, isLoading } = useGetSessionQuery()
  const logInstanceID = instanceLogID(path)
  const executionInstanceID = instanceExecutionID(path)
  const selectedTicketID = ticketID(path)
  const isUserArea =
    userPaths.has(path) ||
    Boolean(logInstanceID) ||
    Boolean(executionInstanceID) ||
    Boolean(selectedTicketID)
  const {
    data: instances = [],
    isLoading: instancesLoading,
    refetch: refetchInstances
  } = useGetInstancesQuery(undefined, {
    skip: !session || !isUserArea,
    pollingInterval: session && isUserArea ? 15000 : 0
  })
  const hasActiveInstanceTask = instances.some(instance =>
    Boolean(instance.activeTask)
  )
  useEffect(() => {
    if (!session || !isUserArea || !hasActiveInstanceTask) return
    const interval = window.setInterval(() => {
      void refetchInstances()
    }, 2500)
    return () => window.clearInterval(interval)
  }, [hasActiveInstanceTask, isUserArea, refetchInstances, session])
  const {
    data: catalog,
    isLoading: catalogLoading,
    isError: catalogError,
    refetch: refetchCatalog
  } = useGetCatalogQuery(undefined, { skip: !session || !isUserArea })
  const {
    data: orders = [],
    isLoading: ordersLoading,
    refetch: refetchOrders
  } = useGetOrdersQuery(undefined, {
    skip: !session || !isUserArea,
    pollingInterval: session && isUserArea ? 15000 : 0
  })
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
    void callback({ code, state })
      .unwrap()
      .then(() => navigate(safeReturnPath(), true))
      .catch(value => {
        window.history.replaceState({}, '', '/login')
        setPath('/login')
        setError(message(value, '统一认证失败'))
      })
  }, [callback])

  useEffect(() => {
    if (!session || !isUserArea) return
    // These queries live above the page switch and otherwise remain subscribed
    // while a user moves between /me routes. Revalidate on every route entry
    // instead of showing a previous route's cached snapshot.
    void refetchInstances()
    void refetchCatalog()
    void refetchOrders()
  }, [
    isUserArea,
    path,
    refetchCatalog,
    refetchInstances,
    refetchOrders,
    session
  ])

  async function login() {
    setError('')
    window.sessionStorage.setItem(
      'alemonxcloud:return-to',
      path === '/super' ? '/super' : '/me'
    )
    try {
      window.location.assign(
        (await authorize(`${window.location.origin}/callback`).unwrap())
          .authorizeURL
      )
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

  if (isLoading)
    return (
      <main className="auth">
        <section className="login">
          <div className="login-card">
            <h2>正在恢复登录状态…</h2>
          </div>
        </section>
      </main>
    )
  if (!session)
    return (
      <LoginPage error={error} onLogin={login} onDevLogin={loginAsDeveloper} />
    )
  if (path === '/' || path === '/login' || path === '/callback') {
    navigate('/me', true)
    return null
  }
  const walletUserID = walletHistoryUserID(path)
  const superPage = superPageFor(path)
  if (superPage || walletUserID) {
    if (!session.user.isAdmin) {
      navigate('/me', true)
      return null
    }
    return (
      <Shell
        user={session.user}
        area="super"
        superPage={superPage ?? 'benefits'}
        onSuperPageChange={next => navigate(superPaths[next])}
        onGoToMe={() => navigate('/me')}
        onLogout={signOut}
      >
        {walletUserID ? (
          <WalletHistoryPage
            userID={walletUserID}
            onBack={() => navigate('/super/users')}
          />
        ) : (
          <AdminPage
            page={superPage!}
            onOpenWalletHistory={user =>
              navigate(`/super/users/${encodeURIComponent(user.id)}/wallet`)
            }
          />
        )}
      </Shell>
    )
  }
  if (logInstanceID) {
    return (
      <Shell
        user={session.user}
        area="me"
        page="instances"
        onPageChange={next =>
          navigate(
            {
              overview: '/me',
              instances: '/me/instances',
              create: '/me/create',
              orders: '/me/orders',
              wallet: '/me/wallet',
              notifications: '/me/notifications',
              tickets: '/me/tickets'
            }[next]
          )
        }
        onGoToMe={() => navigate('/me')}
        onGoToSuper={
          session.user.isAdmin ? () => navigate('/super') : undefined
        }
        onLogout={signOut}
      >
        <InstanceLogsPage
          instanceID={logInstanceID}
          instance={instances.find(item => item.id === logInstanceID)}
          onBack={() => navigate('/me/instances')}
        />
      </Shell>
    )
  }
  if (executionInstanceID) {
    return (
      <Shell
        user={session.user}
        area="me"
        page="instances"
        onPageChange={next =>
          navigate(
            {
              overview: '/me',
              instances: '/me/instances',
              create: '/me/create',
              orders: '/me/orders',
              wallet: '/me/wallet',
              notifications: '/me/notifications',
              tickets: '/me/tickets'
            }[next]
          )
        }
        onGoToMe={() => navigate('/me')}
        onGoToSuper={
          session.user.isAdmin ? () => navigate('/super') : undefined
        }
        onLogout={signOut}
      >
        <InstanceExecutionPage
          instanceID={executionInstanceID}
          instance={instances.find(item => item.id === executionInstanceID)}
          onBack={() => navigate('/me/instances')}
        />
      </Shell>
    )
  }
  if (selectedTicketID) {
    return (
      <Shell
        user={session.user}
        area="me"
        page="tickets"
        onPageChange={next =>
          navigate(
            {
              overview: '/me',
              instances: '/me/instances',
              create: '/me/create',
              orders: '/me/orders',
              wallet: '/me/wallet',
              notifications: '/me/notifications',
              tickets: '/me/tickets'
            }[next]
          )
        }
        onGoToMe={() => navigate('/me')}
        onGoToSuper={
          session.user.isAdmin ? () => navigate('/super') : undefined
        }
        onLogout={signOut}
      >
        <TicketsPage
          selectedID={selectedTicketID}
          onSelect={id =>
            navigate(
              id ? `/me/tickets/${encodeURIComponent(id)}` : '/me/tickets'
            )
          }
        />
      </Shell>
    )
  }
  if (!userPaths.has(path)) {
    navigate('/me', true)
    return null
  }

  const page = userPageFor(path)
  const routeForPage: Record<Page, string> = {
    overview: '/me',
    instances: '/me/instances',
    create: '/me/create',
    orders: '/me/orders',
    wallet: '/me/wallet',
    notifications: '/me/notifications',
    tickets: '/me/tickets'
  }
  const content =
    page === 'orders' ? (
      <OrdersPage
        orders={orders}
        loading={ordersLoading}
        onCreate={() => navigate('/me/create')}
      />
    ) : page === 'instances' ? (
      <InstancesPage
        instances={instances}
        orders={orders}
        loading={instancesLoading}
        onCreate={() => navigate('/me/create')}
        onOpenLogs={instanceID =>
          navigate(`/me/instances/${encodeURIComponent(instanceID)}/logs`)
        }
        onOpenExecutions={instanceID =>
          navigate(`/me/instances/${encodeURIComponent(instanceID)}/executions`)
        }
      />
    ) : page === 'wallet' ? (
      <WalletPage />
    ) : page === 'notifications' ? (
      <NotificationsPage
        onOpenTicket={id => navigate(`/me/tickets/${encodeURIComponent(id)}`)}
      />
    ) : page === 'tickets' ? (
      <TicketsPage
        onSelect={id =>
          navigate(id ? `/me/tickets/${encodeURIComponent(id)}` : '/me/tickets')
        }
      />
    ) : page === 'create' ? (
      <CreateServicePage
        catalog={catalog}
        loading={catalogLoading}
        hasError={catalogError}
        onRetry={() => void refetchCatalog()}
        onCreated={() => navigate('/me/orders')}
      />
    ) : (
      <UserOverviewPage
        instances={instances}
        loading={instancesLoading}
        onCreate={() => navigate('/me/create')}
        onInstances={() => navigate('/me/instances')}
        onWallet={() => navigate('/me/wallet')}
      />
    )
  return (
    <Shell
      user={session.user}
      area="me"
      page={page}
      onPageChange={next => navigate(routeForPage[next])}
      onGoToMe={() => navigate('/me')}
      onGoToSuper={session.user.isAdmin ? () => navigate('/super') : undefined}
      onLogout={signOut}
    >
      {content}
    </Shell>
  )
}
