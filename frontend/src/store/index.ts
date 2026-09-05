import { configureStore } from '@reduxjs/toolkit'
import { setupListeners } from '@reduxjs/toolkit/query'
import { cloudApi } from '@/services/cloudApi'
import uiReducer from './uiSlice'

export const store = configureStore({
  reducer: { ui: uiReducer, [cloudApi.reducerPath]: cloudApi.reducer },
  middleware: getDefaultMiddleware =>
    getDefaultMiddleware().concat(cloudApi.middleware)
})

// Enables RTK Query's focus and reconnect revalidation configured by cloudApi.
setupListeners(store.dispatch)
export type RootState = ReturnType<typeof store.getState>
export type AppDispatch = typeof store.dispatch
