import type { CreateInstanceInput, CurrentUser, Instance } from '@/types/cloud'

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, init)
  const body = await response.json().catch(() => ({})) as T & { message?: string }
  if (!response.ok) throw new Error(body.message ?? '请求失败，请稍后重试')
  return body
}

export const cloudApi = {
  session: async () => {
    const response = await fetch('/api/oidc/session')
    if (response.status === 401) return null
    const body = await response.json().catch(() => ({})) as { user?: CurrentUser; message?: string }
    if (!response.ok || !body.user) throw new Error(body.message ?? '无法检查登录状态')
    return { user: body.user }
  },
  instances: () => request<Instance[]>('/api/instances'),
  createInstance: (input: CreateInstanceInput) => request<Instance>('/api/instances', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) }),
  logout: () => fetch('/api/logout', { method: 'POST' }),
  authorize: (redirectUri: string) => request<{ authorizeURL: string }>('/api/oidc/authorize', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ redirect_uri: redirectUri }) }),
  callback: (code: string, state: string) => request<{ user: CurrentUser }>('/api/oidc/callback', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ code, state }) }),
  devLogin: () => request<{ user: CurrentUser }>('/api/dev/login', { method: 'POST' })
}
