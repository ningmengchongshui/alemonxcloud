import { createApi, fetchBaseQuery } from '@reduxjs/toolkit/query/react'
import type { AdminMetrics, AuditLog, Catalog, CatalogImage, CloudUser, CreateInstanceInput, CurrentUser, Instance, Node, Notification, Order, Plan, Task, Wallet, WalletEntry } from '@/types/cloud'

interface SessionResponse { user?: CurrentUser }
interface AuthorizeResponse { authorizeURL: string }

export const cloudApi = createApi({
  reducerPath: 'cloudApi',
  baseQuery: fetchBaseQuery({ baseUrl: '/api' }),
  tagTypes: ['Session', 'Instances', 'Catalog', 'Orders', 'Admin', 'Wallet', 'Notifications'],
  endpoints: builder => ({
    getSession: builder.query<{ user: CurrentUser } | null, void>({
      query: () => ({ url: '/oidc/session', validateStatus: response => response.status === 200 || response.status === 401 }),
      transformResponse: (response: SessionResponse) => response.user ? { user: response.user } : null,
      providesTags: ['Session']
    }),
    getInstances: builder.query<Instance[], void>({ query: () => '/instances', providesTags: ['Instances'] }),
    createInstance: builder.mutation<Instance, CreateInstanceInput>({ query: body => ({ url: '/instances', method: 'POST', body }), invalidatesTags: ['Instances'] }),
    instanceAction: builder.mutation<void, { id: string; action: 'start' | 'stop' | 'delete' }>({ query: ({ id, action }) => ({ url: action === 'delete' ? `/instances/${id}` : `/instances/${id}/${action}`, method: action === 'delete' ? 'DELETE' : 'POST' }), invalidatesTags: ['Instances'] }),
    getInstanceLogs: builder.query<{ lines: string[] }, string>({ query: id => `/instances/${id}/logs` }),
    getCatalog: builder.query<Catalog, void>({ query: () => '/catalog', providesTags: ['Catalog'] }),
    getWallet: builder.query<Wallet, void>({ query: () => '/wallet', providesTags: ['Wallet'] }),
    getWalletEntries: builder.query<WalletEntry[], void>({ query: () => '/wallet/entries', providesTags: ['Wallet'] }),
    purchase: builder.mutation<{ order: Order; task: Task }, { planId: string; imageId: string; imageVersion: string; months: number }>({ query: body => ({ url: '/purchases', method: 'POST', body }), invalidatesTags: ['Wallet', 'Orders', 'Instances'] }),
    getNotifications: builder.query<Notification[], void>({ query: () => '/notifications', providesTags: ['Notifications'] }),
    readNotification: builder.mutation<void, string>({ query: id => ({ url: `/notifications/${id}/read`, method: 'POST' }), invalidatesTags: ['Notifications'] }),
    readAllNotifications: builder.mutation<void, void>({ query: () => ({ url: '/notifications/read-all', method: 'POST' }), invalidatesTags: ['Notifications'] }),
    getInstanceTasks: builder.query<Array<{ task: Task }>, string>({ query: id => `/instances/${id}/tasks` }),
    getOrders: builder.query<Order[], void>({ query: () => '/orders', providesTags: ['Orders'] }),
    createOrder: builder.mutation<Order, { planId: string; imageId: string; months: number; paymentNote: string }>({ query: body => ({ url: '/orders', method: 'POST', body }), invalidatesTags: ['Orders'] }),
    cancelOrder: builder.mutation<void, string>({ query: id => ({ url: `/orders/${id}/cancel`, method: 'POST' }), invalidatesTags: ['Orders'] }),
    submitPayment: builder.mutation<void, { id: string; reference: string }>({ query: ({ id, reference }) => ({ url: `/orders/${id}/payment`, method: 'POST', body: { reference } }), invalidatesTags: ['Orders'] }),
    renewOrder: builder.mutation<Order, { id: string; months: number }>({ query: ({ id, months }) => ({ url: `/orders/${id}/renew`, method: 'POST', body: { months } }), invalidatesTags: ['Orders'] }),
    getAdminCatalog: builder.query<Catalog, void>({ query: () => '/admin/catalog', providesTags: ['Admin'] }),
    getAdminOrders: builder.query<Order[], void>({ query: () => '/admin/orders', providesTags: ['Admin'] }),
    getAdminNodes: builder.query<import('@/types/cloud').Node[], void>({ query: () => '/admin/nodes', providesTags: ['Admin'] }),
    getAdminTasks: builder.query<Task[], void>({ query: () => '/admin/tasks', providesTags: ['Admin'] }),
    getAdminAuditLogs: builder.query<AuditLog[], void>({ query: () => '/admin/audit-logs', providesTags: ['Admin'] }),
    getAdminMetrics: builder.query<AdminMetrics, void>({ query: () => '/admin/metrics', providesTags: ['Admin'] }),
    searchAdminUsers: builder.query<CloudUser[], string>({ query: q => `/admin/users?q=${encodeURIComponent(q)}`, providesTags: ['Admin'] }),
    adjustAdminWallet: builder.mutation<WalletEntry, { id: string; amountFen: number; note: string }>({ query: ({ id, ...body }) => ({ url: `/admin/users/${id}/wallet/adjust`, method: 'POST', body }), invalidatesTags: ['Admin', 'Wallet'] }),
    saveAdminImage: builder.mutation<CatalogImage, CatalogImage>({ query: body => ({ url: body.id ? `/admin/images/${body.id}` : '/admin/images', method: body.id ? 'PUT' : 'POST', body }), invalidatesTags: ['Admin', 'Catalog'] }),
    saveAdminPlan: builder.mutation<Plan, Plan>({ query: body => ({ url: body.id ? `/admin/plans/${body.id}` : '/admin/plans', method: body.id ? 'PUT' : 'POST', body }), invalidatesTags: ['Admin', 'Catalog'] }),
    saveAdminNode: builder.mutation<Node, Node>({ query: body => ({ url: body.id ? `/admin/nodes/${body.id}` : '/admin/nodes', method: body.id ? 'PUT' : 'POST', body }), invalidatesTags: ['Admin'] }),
    confirmOrder: builder.mutation<void, string>({ query: id => ({ url: `/admin/orders/${id}/confirm`, method: 'POST' }), invalidatesTags: ['Admin', 'Orders', 'Instances'] }),
    rejectOrder: builder.mutation<void, { id: string; reason: string }>({ query: ({ id, reason }) => ({ url: `/admin/orders/${id}/reject`, method: 'POST', body: { reason } }), invalidatesTags: ['Admin'] }),
    retryTask: builder.mutation<void, string>({ query: id => ({ url: `/admin/tasks/${id}/retry`, method: 'POST' }), invalidatesTags: ['Admin'] }),
    logout: builder.mutation<void, void>({ query: () => ({ url: '/logout', method: 'POST' }), invalidatesTags: ['Session', 'Instances'] }),
    authorize: builder.mutation<AuthorizeResponse, string>({ query: redirectUri => ({ url: '/oidc/authorize', method: 'POST', body: { redirect_uri: redirectUri } }) }),
    callback: builder.mutation<SessionResponse, { code: string; state: string }>({ query: body => ({ url: '/oidc/callback', method: 'POST', body }), invalidatesTags: ['Session'] }),
    devLogin: builder.mutation<SessionResponse, void>({ query: () => ({ url: '/dev/login', method: 'POST' }), invalidatesTags: ['Session'] })
  })
})

export const { useGetSessionQuery, useGetInstancesQuery, useCreateInstanceMutation, useInstanceActionMutation, useLazyGetInstanceLogsQuery, useGetCatalogQuery, useGetWalletQuery, useGetWalletEntriesQuery, usePurchaseMutation, useGetNotificationsQuery, useReadNotificationMutation, useReadAllNotificationsMutation, useGetInstanceTasksQuery, useGetOrdersQuery, useCreateOrderMutation, useCancelOrderMutation, useSubmitPaymentMutation, useRenewOrderMutation, useGetAdminCatalogQuery, useGetAdminOrdersQuery, useGetAdminNodesQuery, useGetAdminTasksQuery, useGetAdminAuditLogsQuery, useGetAdminMetricsQuery, useSearchAdminUsersQuery, useAdjustAdminWalletMutation, useSaveAdminImageMutation, useSaveAdminPlanMutation, useSaveAdminNodeMutation, useConfirmOrderMutation, useRejectOrderMutation, useRetryTaskMutation, useLogoutMutation, useAuthorizeMutation, useCallbackMutation, useDevLoginMutation } = cloudApi
