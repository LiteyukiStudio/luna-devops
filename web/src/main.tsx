import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { Toaster } from 'sonner'
import App from './App.tsx'
import { AppUpdateChecker } from './app/app-update-checker'
import { PublicConfigProvider } from './app/public-config'
import { SessionProvider } from './app/session'
import { TelemetryRouteObserver } from './app/telemetry-route-observer'
import { ThemeProvider } from './app/theme'
import './index.css'
import './i18n'

const queryClient = new QueryClient()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <PublicConfigProvider>
          <BrowserRouter>
            <TelemetryRouteObserver />
            <SessionProvider>
              <AppUpdateChecker />
              <App />
              <Toaster richColors duration={4000} position="top-right" />
            </SessionProvider>
          </BrowserRouter>
        </PublicConfigProvider>
      </ThemeProvider>
    </QueryClientProvider>
  </StrictMode>,
)
