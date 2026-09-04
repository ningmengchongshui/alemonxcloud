import { configureStore } from '@reduxjs/toolkit'
import { persistReducer, persistStore } from 'redux-persist'
import uiReducer from './uiSlice'

const storage = {
  getItem: (key: string) => Promise.resolve(window.localStorage.getItem(key)),
  setItem: (key: string, value: string) => Promise.resolve(window.localStorage.setItem(key, value)),
  removeItem: (key: string) => Promise.resolve(window.localStorage.removeItem(key))
}

const persistedUiReducer = persistReducer({ key: 'alemonx-ui', version: 1, storage, whitelist: ['page'] }, uiReducer)

export const store = configureStore({ reducer: { ui: persistedUiReducer } })
export const persistor = persistStore(store)
export type RootState = ReturnType<typeof store.getState>
export type AppDispatch = typeof store.dispatch
