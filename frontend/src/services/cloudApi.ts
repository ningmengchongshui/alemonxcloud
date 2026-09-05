import { createApi, fetchBaseQuery } from '@reduxjs/toolkit/query/react'
import type {
  AdminMetrics,
  AuditLog,
  Catalog,
  AdminCatalog,
  CatalogImage,
  CloudUser,
  CurrentUser,
  Instance,
  ImageVersion,
  Node,
  Notification,
  Order,
  Plan,
  RefundQuote,
  Task,
  Wallet,
  WalletEntry,
  Ticket,
  InstanceTaskRecord,
  TicketDetail,
  TicketPriority,
  TicketStatus,
  PriceQuote,
  RechargeContact,
  BenefitProgram,
  PlanPriceTier
} from '@/types/cloud'

interface SessionResponse {
  user?: CurrentUser
}
interface AuthorizeResponse {
  authorizeURL: string
}

export const cloudApi = createApi({
  reducerPath: 'cloudApi',
  baseQuery: fetchBaseQuery({ baseUrl: '/api' }),
  // Console data is operational data rather than static content. A page must
  // revalidate when it is opened again or when the user returns to the tab.
  refetchOnMountOrArgChange: true,
  refetchOnFocus: true,
  refetchOnReconnect: true,
  tagTypes: [
    'Session',
    'Instances',
    'Catalog',
    'Orders',
    'Admin',
    'Wallet',
    'Notifications',
    'Tickets'
  ],
  endpoints: builder => ({
    getSession: builder.query<{ user: CurrentUser } | null, void>({
      query: () => ({
        url: '/oidc/session',
        validateStatus: response =>
          response.status === 200 || response.status === 401
      }),
      transformResponse: (response: SessionResponse) =>
        response.user ? { user: response.user } : null,
      providesTags: ['Session']
    }),
    getInstances: builder.query<Instance[], void>({
      query: () => '/instances',
      providesTags: ['Instances']
    }),
    instanceAction: builder.mutation<
      { task?: Task; message?: string; destroyAt?: string },
      {
        id: string
        action:
          | 'start'
          | 'stop'
          | 'restart'
          | 'destroy'
          | 'destroy-now'
          | 'cancel-destroy'
          | 'archive'
          | 'retry-deploy'
          | 'update'
      }
    >({
      query: ({ id, action }) => ({
        url: `/instances/${id}/${action}`,
        method: 'POST'
      }),
      invalidatesTags: ['Instances', 'Orders']
    }),
    getInstanceLogs: builder.query<
      { lines: string[]; tail: number; truncated: boolean },
      { id: string; tail: number; since?: string }
    >({
      query: ({ id, tail, since }) => ({
        url: `/instances/${id}/logs`,
        params: { tail, ...(since ? { since } : {}) }
      })
    }),
    getCatalog: builder.query<Catalog, void>({
      query: () => '/catalog',
      providesTags: ['Catalog']
    }),
    getWallet: builder.query<Wallet, void>({
      query: () => '/wallet',
      providesTags: ['Wallet']
    }),
    getWalletEntries: builder.query<WalletEntry[], void>({
      query: () => '/wallet/entries',
      providesTags: ['Wallet']
    }),
    getRechargeContact: builder.query<RechargeContact, void>({
      query: () => '/recharge-contact'
    }),
    getAdminRechargeContact: builder.query<RechargeContact, void>({
      query: () => '/admin/settings/recharge-contact',
      providesTags: ['Admin']
    }),
    saveAdminRechargeContact: builder.mutation<
      RechargeContact,
      RechargeContact
    >({
      query: body => ({
        url: '/admin/settings/recharge-contact',
        method: 'PUT',
        body
      }),
      invalidatesTags: ['Admin']
    }),
    purchase: builder.mutation<
      { order: Order; task: Task },
      {
        planId: string
        imageId: string
        imageVersion: string
        months: number
        promoCode?: string
      }
    >({
      query: body => ({ url: '/purchases', method: 'POST', body }),
      invalidatesTags: ['Wallet', 'Orders', 'Instances']
    }),
    quotePurchase: builder.mutation<
      PriceQuote,
      {
        planId: string
        imageId: string
        months: number
        promoCode?: string
      }
    >({ query: body => ({ url: '/purchases/quote', method: 'POST', body }) }),
    getNotifications: builder.query<Notification[], void>({
      query: () => '/notifications',
      providesTags: ['Notifications']
    }),
    readNotification: builder.mutation<void, string>({
      query: id => ({ url: `/notifications/${id}/read`, method: 'POST' }),
      invalidatesTags: ['Notifications']
    }),
    readAllNotifications: builder.mutation<void, void>({
      query: () => ({ url: '/notifications/read-all', method: 'POST' }),
      invalidatesTags: ['Notifications']
    }),
    getInstanceTasks: builder.query<InstanceTaskRecord[], string>({
      query: id => `/instances/${id}/tasks`
    }),
    getTask: builder.query<Task, string>({
      query: id => `/tasks/${id}`
    }),
    getOrders: builder.query<Order[], void>({
      query: () => '/orders',
      providesTags: ['Orders']
    }),
    getTickets: builder.query<Ticket[], void>({
      query: () => '/tickets',
      providesTags: ['Tickets']
    }),
    getTicket: builder.query<TicketDetail, string>({
      query: id => `/tickets/${id}`,
      providesTags: (_result, _error, id) => [{ type: 'Tickets', id }]
    }),
    createTicket: builder.mutation<
      Ticket,
      {
        category: string
        priority: string
        subject: string
        body: string
        instanceId?: string
        orderId?: string
      }
    >({
      query: body => ({ url: '/tickets', method: 'POST', body }),
      invalidatesTags: ['Tickets', 'Notifications']
    }),
    replyTicket: builder.mutation<TicketDetail, { id: string; body: string }>({
      query: ({ id, body }) => ({
        url: `/tickets/${id}/messages`,
        method: 'POST',
        body: { body }
      }),
      invalidatesTags: (_result, _error, arg) => [
        'Tickets',
        { type: 'Tickets', id: arg.id }
      ]
    }),
    reopenTicket: builder.mutation<TicketDetail, string>({
      query: id => ({ url: `/tickets/${id}/reopen`, method: 'POST' }),
      invalidatesTags: (_result, _error, id) => [
        'Tickets',
        'Notifications',
        { type: 'Tickets', id }
      ]
    }),
    renewOrder: builder.mutation<
      { order: Order; task?: Task },
      {
        id: string
        months: number
        promoCode?: string
      }
    >({
      query: ({ id, ...body }) => ({
        url: `/orders/${id}/renew`,
        method: 'POST',
        body
      }),
      invalidatesTags: ['Wallet', 'Orders', 'Instances']
    }),
    quoteRenewal: builder.mutation<
      PriceQuote,
      {
        id: string
        months: number
        promoCode?: string
      }
    >({
      query: ({ id, ...body }) => ({
        url: `/orders/${id}/renew/quote`,
        method: 'POST',
        body
      })
    }),
    getAdminPlanPriceTiers: builder.query<PlanPriceTier[], void>({
      query: () => '/admin/plan-price-tiers',
      providesTags: ['Admin', 'Catalog']
    }),
    saveAdminPlanPriceTier: builder.mutation<PlanPriceTier, PlanPriceTier>({
      query: body => ({
        url: body.id
          ? `/admin/plan-price-tiers/${body.id}`
          : '/admin/plan-price-tiers',
        method: body.id ? 'PUT' : 'POST',
        body
      }),
      invalidatesTags: ['Admin', 'Catalog']
    }),
    getAdminBenefitPrograms: builder.query<BenefitProgram[], void>({
      query: () => '/admin/benefit-programs',
      providesTags: ['Admin']
    }),
    saveAdminBenefitProgram: builder.mutation<BenefitProgram, BenefitProgram>({
      query: body => ({
        url: body.id
          ? `/admin/benefit-programs/${body.id}`
          : '/admin/benefit-programs',
        method: body.id ? 'PUT' : 'POST',
        body
      }),
      invalidatesTags: ['Admin']
    }),
    grantAdminBenefitProgram: builder.mutation<
      { created: number },
      { id: string; ownerIds: string[]; expiresAt?: string }
    >({
      query: ({ id, ...body }) => ({
        url: `/admin/benefit-programs/${id}/grants`,
        method: 'POST',
        body
      }),
      invalidatesTags: ['Admin']
    }),
    getAdminBenefitGrants: builder.query<
      Array<{
        id: string
        ownerId: string
        status: string
        expiresAt?: string
        usedOrderId?: string
        createdAt: string
      }>,
      string
    >({
      query: id => `/admin/benefit-programs/${id}/grants`,
      providesTags: ['Admin']
    }),
    voidAdminBenefitGrant: builder.mutation<
      void,
      { id: string; grantId: string }
    >({
      query: ({ id, grantId }) => ({
        url: `/admin/benefit-programs/${id}/grants/${grantId}/void`,
        method: 'POST'
      }),
      invalidatesTags: ['Admin']
    }),
    getAdminBenefitRedemptions: builder.query<
      Array<{
        id: string
        programId: string
        ownerId: string
        orderId: string
        discountAmountFen: number
        bonusDays: number
        createdAt: string
      }>,
      void
    >({ query: () => '/admin/benefit-redemptions', providesTags: ['Admin'] }),
    getRefundQuote: builder.query<RefundQuote, string>({
      query: id => `/orders/${id}/refund-quote`
    }),
    refundOrder: builder.mutation<
      { quote: RefundQuote; entry: WalletEntry; wallet: Wallet },
      string
    >({
      query: id => ({ url: `/orders/${id}/refund`, method: 'POST' }),
      invalidatesTags: ['Wallet', 'Orders', 'Instances', 'Notifications']
    }),
    getAdminCatalog: builder.query<AdminCatalog, void>({
      query: () => '/admin/catalog',
      providesTags: ['Admin']
    }),
    getAdminOrders: builder.query<Order[], void>({
      query: () => '/admin/orders',
      providesTags: ['Admin']
    }),
    getAdminTickets: builder.query<
      Ticket[],
      { status?: TicketStatus; priority?: TicketPriority } | void
    >({
      query: filters => ({
        url: `/admin/tickets?${new URLSearchParams(filters ?? {}).toString()}`
      }),
      providesTags: ['Tickets', 'Admin']
    }),
    getAdminTicket: builder.query<TicketDetail, string>({
      query: id => `/admin/tickets/${id}`,
      providesTags: (_result, _error, id) => [{ type: 'Tickets', id }]
    }),
    adminReplyTicket: builder.mutation<
      TicketDetail,
      { id: string; body: string }
    >({
      query: ({ id, body }) => ({
        url: `/admin/tickets/${id}/messages`,
        method: 'POST',
        body: { body }
      }),
      invalidatesTags: (_result, _error, arg) => [
        'Tickets',
        'Admin',
        'Notifications',
        { type: 'Tickets', id: arg.id }
      ]
    }),
    adminUpdateTicketStatus: builder.mutation<
      TicketDetail,
      { id: string; status: 'in_progress' | 'closed' }
    >({
      query: ({ id, status }) => ({
        url: `/admin/tickets/${id}/status`,
        method: 'POST',
        body: { status }
      }),
      invalidatesTags: ['Tickets', 'Admin', 'Notifications']
    }),
    adminUpdateTicketPriority: builder.mutation<
      TicketDetail,
      { id: string; priority: TicketPriority }
    >({
      query: ({ id, priority }) => ({
        url: `/admin/tickets/${id}/priority`,
        method: 'POST',
        body: { priority }
      }),
      invalidatesTags: ['Tickets', 'Admin']
    }),
    getAdminNodes: builder.query<import('@/types/cloud').Node[], void>({
      query: () => '/admin/nodes',
      providesTags: ['Admin']
    }),
    getAdminTasks: builder.query<Task[], void>({
      query: () => '/admin/tasks',
      providesTags: ['Admin']
    }),
    getAdminAuditLogs: builder.query<AuditLog[], void>({
      query: () => '/admin/audit-logs',
      providesTags: ['Admin']
    }),
    getAdminMetrics: builder.query<AdminMetrics, void>({
      query: () => '/admin/metrics',
      providesTags: ['Admin']
    }),
    searchAdminUsers: builder.query<CloudUser[], string>({
      query: q => `/admin/users?q=${encodeURIComponent(q)}`,
      providesTags: ['Admin']
    }),
    getAdminWalletEntries: builder.query<WalletEntry[], string>({
      query: id => `/admin/users/${id}/wallet/entries`,
      providesTags: ['Admin']
    }),
    adjustAdminWallet: builder.mutation<
      WalletEntry,
      {
        id: string
        amountFen: number
        direction: 'increase' | 'decrease'
        note: string
      }
    >({
      query: ({ id, ...body }) => ({
        url: `/admin/users/${id}/wallet/adjust`,
        method: 'POST',
        body
      }),
      invalidatesTags: ['Admin', 'Wallet']
    }),
    saveAdminImage: builder.mutation<CatalogImage, CatalogImage>({
      query: body => ({
        url: body.id ? `/admin/images/${body.id}` : '/admin/images',
        method: body.id ? 'PUT' : 'POST',
        body
      }),
      invalidatesTags: ['Admin', 'Catalog']
    }),
    saveAdminImageVersion: builder.mutation<ImageVersion, ImageVersion>({
      query: body => ({
        url: body.id
          ? `/admin/images/${body.imageId}/versions/${body.id}`
          : `/admin/images/${body.imageId}/versions`,
        method: body.id ? 'PUT' : 'POST',
        body
      }),
      invalidatesTags: ['Admin', 'Catalog']
    }),
    pullAdminImageVersion: builder.mutation<{ message: string }, ImageVersion>({
      query: version => ({
        url: `/admin/images/${version.imageId}/versions/${version.id}/pull`,
        method: 'POST'
      }),
      invalidatesTags: ['Admin']
    }),
    deleteAdminImageVersion: builder.mutation<
      void,
      { imageId: string; versionId: string }
    >({
      query: ({ imageId, versionId }) => ({
        url: `/admin/images/${imageId}/versions/${versionId}`,
        method: 'DELETE'
      }),
      invalidatesTags: ['Admin', 'Catalog']
    }),
    publishAdminImageVersion: builder.mutation<
      { version: ImageVersion; message: string },
      { imageId: string; tag: string }
    >({
      query: ({ imageId, tag }) => ({
        url: `/admin/images/${imageId}/versions/publish`,
        method: 'POST',
        body: { tag }
      }),
      invalidatesTags: ['Admin', 'Catalog']
    }),
    saveAdminPlan: builder.mutation<Plan, Plan>({
      query: body => ({
        url: body.id ? `/admin/plans/${body.id}` : '/admin/plans',
        method: body.id ? 'PUT' : 'POST',
        body
      }),
      invalidatesTags: ['Admin', 'Catalog']
    }),
    saveAdminNode: builder.mutation<Node, Node>({
      query: body => ({
        url: body.id ? `/admin/nodes/${body.id}` : '/admin/nodes',
        method: body.id ? 'PUT' : 'POST',
        body
      }),
      invalidatesTags: ['Admin']
    }),
    retryTask: builder.mutation<void, string>({
      query: id => ({ url: `/admin/tasks/${id}/retry`, method: 'POST' }),
      invalidatesTags: ['Admin']
    }),
    resumeReviewTask: builder.mutation<void, string>({
      query: id => ({
        url: `/admin/tasks/${id}/review-resume`,
        method: 'POST'
      }),
      invalidatesTags: ['Admin']
    }),
    discardReviewTask: builder.mutation<void, string>({
      query: id => ({
        url: `/admin/tasks/${id}/review-discard`,
        method: 'POST'
      }),
      invalidatesTags: ['Admin']
    }),
    logout: builder.mutation<void, void>({
      query: () => ({ url: '/logout', method: 'POST' }),
      invalidatesTags: ['Session', 'Instances']
    }),
    authorize: builder.mutation<AuthorizeResponse, string>({
      query: redirectUri => ({
        url: '/oidc/authorize',
        method: 'POST',
        body: { redirect_uri: redirectUri }
      })
    }),
    callback: builder.mutation<
      SessionResponse,
      { code: string; state: string }
    >({
      query: body => ({ url: '/oidc/callback', method: 'POST', body }),
      invalidatesTags: ['Session']
    }),
    devLogin: builder.mutation<SessionResponse, void>({
      query: () => ({ url: '/dev/login', method: 'POST' }),
      invalidatesTags: ['Session']
    })
  })
})

export const {
  useGetSessionQuery,
  useGetInstancesQuery,
  useInstanceActionMutation,
  useGetInstanceLogsQuery,
  useLazyGetInstanceLogsQuery,
  useGetCatalogQuery,
  useGetWalletQuery,
  useGetWalletEntriesQuery,
  useGetRechargeContactQuery,
  useGetAdminRechargeContactQuery,
  useSaveAdminRechargeContactMutation,
  usePurchaseMutation,
  useQuotePurchaseMutation,
  useGetNotificationsQuery,
  useReadNotificationMutation,
  useReadAllNotificationsMutation,
  useGetInstanceTasksQuery,
  useGetTaskQuery,
  useGetOrdersQuery,
  useGetTicketsQuery,
  useGetTicketQuery,
  useCreateTicketMutation,
  useReplyTicketMutation,
  useReopenTicketMutation,
  useRenewOrderMutation,
  useQuoteRenewalMutation,
  useLazyGetRefundQuoteQuery,
  useRefundOrderMutation,
  useGetAdminCatalogQuery,
  useGetAdminPlanPriceTiersQuery,
  useSaveAdminPlanPriceTierMutation,
  useGetAdminBenefitProgramsQuery,
  useSaveAdminBenefitProgramMutation,
  useGrantAdminBenefitProgramMutation,
  useGetAdminBenefitGrantsQuery,
  useVoidAdminBenefitGrantMutation,
  useGetAdminBenefitRedemptionsQuery,
  useGetAdminOrdersQuery,
  useGetAdminTicketsQuery,
  useGetAdminTicketQuery,
  useAdminReplyTicketMutation,
  useAdminUpdateTicketStatusMutation,
  useAdminUpdateTicketPriorityMutation,
  useGetAdminNodesQuery,
  useGetAdminTasksQuery,
  useGetAdminAuditLogsQuery,
  useGetAdminMetricsQuery,
  useSearchAdminUsersQuery,
  useGetAdminWalletEntriesQuery,
  useAdjustAdminWalletMutation,
  useSaveAdminImageMutation,
  useSaveAdminImageVersionMutation,
  usePullAdminImageVersionMutation,
  useDeleteAdminImageVersionMutation,
  usePublishAdminImageVersionMutation,
  useSaveAdminPlanMutation,
  useSaveAdminNodeMutation,
  useRetryTaskMutation,
  useResumeReviewTaskMutation,
  useDiscardReviewTaskMutation,
  useLogoutMutation,
  useAuthorizeMutation,
  useCallbackMutation,
  useDevLoginMutation
} = cloudApi
