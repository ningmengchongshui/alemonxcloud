import { createSlice, type PayloadAction } from '@reduxjs/toolkit'
import type { Page } from '@/types/cloud'

interface UiState {
  page: Page
}
const initialState: UiState = { page: 'instances' }

const uiSlice = createSlice({
  name: 'ui',
  initialState,
  reducers: {
    setPage: (state, action: PayloadAction<Page>) => {
      state.page = action.payload
    }
  }
})

export const { setPage } = uiSlice.actions
export default uiSlice.reducer
