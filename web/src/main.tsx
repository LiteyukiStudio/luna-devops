import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { createBrowserRouter, RouterProvider } from 'react-router-dom'
import { Toaster } from 'sonner'
import App from './App.tsx'
import { AppUpdateChecker } from './app/app-update-checker'
import { PublicConfigProvider } from './app/public-config'
import { SessionProvider } from './app/session'
import { TelemetryRouteObserver } from './app/telemetry-route-observer'
import { ThemeProvider } from './app/theme'
import { i18nextReady } from './i18n'
import './index.css'

const queryClient = new QueryClient()
// 数据路由只承载一个通配路由，页面路由仍集中在 App.tsx；数据路由是 useBlocker 拦截未保存离开的前提。
const router = createBrowserRouter([{
  path: '*',
  element: (
    <>
      <TelemetryRouteObserver />
      <SessionProvider>
        <AppUpdateChecker />
        <App />
        <Toaster richColors duration={4000} position="top-right" />
      </SessionProvider>
    </>
  ),
}])

void i18nextReady.then(() => {
  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <ThemeProvider>
          <PublicConfigProvider>
            <RouterProvider router={router} />
          </PublicConfigProvider>
        </ThemeProvider>
      </QueryClientProvider>
    </StrictMode>,
  )
})
