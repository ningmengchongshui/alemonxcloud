import { createSlice, type PayloadAction } from '@reduxjs/toolkit'
import type { Page } from '@/types/cloud'

export interface WatchedTask {
  id: string
  action: string
}

interface UiState {
  page: Page
  watchedTask: WatchedTask | null
}
const initialState: UiState = { page: 'instances', watchedTask: null }

const uiSlice = createSlice({
  name: 'ui',
  initialState,
  reducers: {
    setPage: (state, action: PayloadAction<Page>) => {
      state.page = action.payload
    },
    watchTask: (state, action: PayloadAction<WatchedTask>) => {
      state.watchedTask = action.payload
    },
    clearWatchedTask: state => {
      state.watchedTask = null
    }
  }
})

export const { setPage, watchTask, clearWatchedTask } = uiSlice.actions
export default uiSlice.reducer
