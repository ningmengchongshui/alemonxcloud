import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { Provider } from 'react-redux'
import { PersistGate } from 'redux-persist/integration/react'
import App from './App'
import { ErrorBoundary } from './components/ErrorBoundary'
import './index.css'
import { persistor, store } from './store'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Provider store={store}>
      <PersistGate loading={null} persistor={persistor}>
        <ErrorBoundary><App /></ErrorBoundary>
      </PersistGate>
    </Provider>
  </StrictMode>
)
