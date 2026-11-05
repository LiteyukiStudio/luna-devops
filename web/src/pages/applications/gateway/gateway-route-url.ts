import type { GatewayRoute } from '@/api'

/**
 * 路由的可访问地址。优先使用后端回读的 accessUrl，否则按 host/path/TLS 推导；
 * 只返回 http(s) 协议的地址，其它值视为不可点击。
 */
export function gatewayRouteAccessUrl(route: GatewayRoute) {
  const candidate = route.accessUrl?.trim() || derivedRouteUrl(route)
  if (!candidate)
    return ''
  try {
    const url = new URL(candidate)
    return url.protocol === 'http:' || url.protocol === 'https:' ? candidate : ''
  }
  catch {
    return ''
  }
}

/** 应用的主访问地址：启用且就绪的路由优先，其次是任一启用的路由。 */
export function primaryGatewayRoute(routes: GatewayRoute[]) {
  const usable = routes.filter(route => route.enabled && route.deleteStatus !== 'deleting' && gatewayRouteAccessUrl(route))
  return usable.find(route => route.status === 'ready') ?? usable[0]
}

/** 用于展示的地址文本，去掉协议前缀以节省列宽。 */
export function displayAccessUrl(url: string) {
  return url.replace(/^https?:\/\//, '').replace(/\/$/, '')
}

function derivedRouteUrl(route: GatewayRoute) {
  const host = route.host?.trim()
  if (!host)
    return ''
  const protocol = route.tlsMode === 'http-only' ? 'http' : 'https'
  const path = route.path?.startsWith('/') ? route.path : `/${route.path || ''}`
  return `${protocol}://${host}${path === '/' ? '' : path}`
}
