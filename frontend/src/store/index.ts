import { configureStore } from '@reduxjs/toolkit'
import { cloudApi } from '@/services/cloudApi'
import uiReducer from './uiSlice'

export const store = configureStore({
  reducer: { ui: uiReducer, [cloudApi.reducerPath]: cloudApi.reducer },
  middleware: getDefaultMiddleware =>
    getDefaultMiddleware().concat(cloudApi.middleware)
})
export type RootState = ReturnType<typeof store.getState>
export type AppDispatch = typeof store.dispatch
