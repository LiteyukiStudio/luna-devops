import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { Toaster } from 'sonner'
import App from './App.tsx'
import { PublicConfigProvider } from './app/public-config'
import { SessionProvider } from './app/session'
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
            <SessionProvider>
              <App />
              <Toaster richColors duration={4000} position="top-right" />
            </SessionProvider>
          </BrowserRouter>
        </PublicConfigProvider>
      </ThemeProvider>
    </QueryClientProvider>
  </StrictMode>,
)
