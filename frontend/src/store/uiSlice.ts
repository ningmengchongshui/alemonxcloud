import { createSlice, type PayloadAction } from '@reduxjs/toolkit'
import type { Page } from '@/types/cloud'

export interface WatchedTask {
  id: string
  action: string
}

interface UiState {
  page: Page
  watchedTasks: WatchedTask[]
}
const initialState: UiState = { page: 'instances', watchedTasks: [] }

const uiSlice = createSlice({
  name: 'ui',
  initialState,
  reducers: {
    setPage: (state, action: PayloadAction<Page>) => {
      state.page = action.payload
    },
    watchTask: (state, action: PayloadAction<WatchedTask>) => {
      if (!state.watchedTasks.some(task => task.id === action.payload.id)) {
        state.watchedTasks.push(action.payload)
      }
    },
    clearWatchedTask: (state, action: PayloadAction<string>) => {
      state.watchedTasks = state.watchedTasks.filter(task => task.id !== action.payload)
    }
  }
})

export const { setPage, watchTask, clearWatchedTask } = uiSlice.actions
export default uiSlice.reducer
