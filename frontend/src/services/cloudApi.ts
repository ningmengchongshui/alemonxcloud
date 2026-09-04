import { createApi, fetchBaseQuery } from '@reduxjs/toolkit/query/react'
import type { CreateInstanceInput, CurrentUser, Instance } from '@/types/cloud'

interface SessionResponse { user?: CurrentUser }
interface AuthorizeResponse { authorizeURL: string }

export const cloudApi = createApi({
  reducerPath: 'cloudApi',
  baseQuery: fetchBaseQuery({ baseUrl: '/api' }),
  tagTypes: ['Session', 'Instances'],
  endpoints: builder => ({
    getSession: builder.query<{ user: CurrentUser } | null, void>({
      query: () => ({ url: '/oidc/session', validateStatus: response => response.status === 200 || response.status === 401 }),
      transformResponse: (response: SessionResponse) => response.user ? { user: response.user } : null,
      providesTags: ['Session']
    }),
    getInstances: builder.query<Instance[], void>({ query: () => '/instances', providesTags: ['Instances'] }),
    createInstance: builder.mutation<Instance, CreateInstanceInput>({ query: body => ({ url: '/instances', method: 'POST', body }), invalidatesTags: ['Instances'] }),
    logout: builder.mutation<void, void>({ query: () => ({ url: '/logout', method: 'POST' }), invalidatesTags: ['Session', 'Instances'] }),
    authorize: builder.mutation<AuthorizeResponse, string>({ query: redirectUri => ({ url: '/oidc/authorize', method: 'POST', body: { redirect_uri: redirectUri } }) }),
    callback: builder.mutation<SessionResponse, { code: string; state: string }>({ query: body => ({ url: '/oidc/callback', method: 'POST', body }), invalidatesTags: ['Session'] }),
    devLogin: builder.mutation<SessionResponse, void>({ query: () => ({ url: '/dev/login', method: 'POST' }), invalidatesTags: ['Session'] })
  })
})

export const { useGetSessionQuery, useGetInstancesQuery, useCreateInstanceMutation, useLogoutMutation, useAuthorizeMutation, useCallbackMutation, useDevLoginMutation } = cloudApi
