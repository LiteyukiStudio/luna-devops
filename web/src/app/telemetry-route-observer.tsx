import { useEffect } from 'react'
import { useLocation } from 'react-router-dom'
import { recordNavigation } from '@/lib/telemetry'

export function TelemetryRouteObserver() {
  const location = useLocation()

  useEffect(() => {
    recordNavigation(location.pathname)
  }, [location.pathname])

  return null
}
