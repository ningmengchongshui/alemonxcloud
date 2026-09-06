import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { Provider } from 'react-redux'
import App from './App'
import { ErrorBoundary } from './components/ErrorBoundary'
import { ToastViewport } from './components/ToastViewport'
import './index.css'
import { store } from './store'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Provider store={store}>
      <ErrorBoundary>
        <App />
        <ToastViewport />
      </ErrorBoundary>
    </Provider>
  </StrictMode>
)
