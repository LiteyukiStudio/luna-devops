import { createReadStream, existsSync, statSync } from 'node:fs'
import { createServer } from 'node:http'
import { extname, join, normalize, resolve, sep } from 'node:path'
import process from 'node:process'

const distRoot = resolve(process.argv[2] ?? 'dist')
const port = Number(process.argv[3] ?? 4173)

createServer((request, response) => {
  const requestUrl = new URL(request.url ?? '/', `http://127.0.0.1:${port}`)
  if (requestUrl.pathname.startsWith('/api/v1/')) {
    serveMockAPI(requestUrl.pathname, response)
    return
  }

  const requestPath = normalize(decodeURIComponent(requestUrl.pathname)).replace(/^[/\\]+/, '')
  let target = join(distRoot, requestPath || 'index.html')
  if (target !== distRoot && !target.startsWith(`${distRoot}${sep}`)) {
    response.statusCode = 404
    response.end('Not found')
    return
  }
  if (!existsSync(target) || !statSync(target).isFile()) {
    const indexTarget = join(distRoot, 'index.html')
    if (!existsSync(indexTarget)) {
      response.statusCode = 404
      response.end('Not found')
      return
    }
    target = indexTarget
  }

  const acceptEncoding = request.headers['accept-encoding'] ?? ''
  const brotliTarget = `${target}.br`
  const gzipTarget = `${target}.gz`
  response.setHeader('Cache-Control', 'no-store')
  response.setHeader('Content-Type', contentType(target))
  response.setHeader('Timing-Allow-Origin', '*')
  if (acceptEncoding.includes('br') && existsSync(brotliTarget)) {
    target = brotliTarget
    response.setHeader('Content-Encoding', 'br')
    response.setHeader('Vary', 'Accept-Encoding')
  }
  else if (acceptEncoding.includes('gzip') && existsSync(gzipTarget)) {
    target = gzipTarget
    response.setHeader('Content-Encoding', 'gzip')
    response.setHeader('Vary', 'Accept-Encoding')
  }
  response.setHeader('Content-Length', statSync(target).size)
  createReadStream(target).pipe(response)
}).listen(port, '127.0.0.1', () => {
  console.log(`Benchmark server ready at http://127.0.0.1:${port}`)
})

function serveMockAPI(pathname, response) {
  response.setHeader('Cache-Control', 'no-store')
  response.setHeader('Content-Type', 'application/json; charset=utf-8')
  if (pathname === '/api/v1/public/configs') {
    response.end('{}')
    return
  }
  if (pathname === '/api/v1/auth/bootstrap') {
    response.end('{"initialized":true}')
    return
  }
  if (pathname === '/api/v1/auth/providers') {
    response.end('[]')
    return
  }
  if (pathname === '/api/v1/auth/registration') {
    response.end('{"enabled":false}')
    return
  }
  if (pathname === '/api/v1/users/me') {
    response.statusCode = 401
    response.end('{"code":"auth.unauthenticated","error":"Authentication required"}')
    return
  }
  response.statusCode = 404
  response.end('{"code":"request.not_found","error":"Not found"}')
}

function contentType(file) {
  return {
    '.css': 'text/css; charset=utf-8',
    '.html': 'text/html; charset=utf-8',
    '.ico': 'image/x-icon',
    '.js': 'text/javascript; charset=utf-8',
    '.json': 'application/json; charset=utf-8',
    '.png': 'image/png',
    '.svg': 'image/svg+xml',
    '.webp': 'image/webp',
  }[extname(file)] ?? 'application/octet-stream'
}
