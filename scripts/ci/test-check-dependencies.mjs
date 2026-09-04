#!/usr/bin/env node

import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { createServer } from 'node:http'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const rootDir = fileURLToPath(new URL('../..', import.meta.url))
const checkScriptPath = path.join(rootDir, 'scripts/ci/check-dependencies.sh')
const agentDir = path.join(rootDir, 'luna-agent')

const checkScript = await readFile(checkScriptPath, 'utf8')
const auditLines = checkScript
  .split(/\r?\n/u)
  .filter(line => /^pnpm .* audit /u.test(line))

assert.equal(auditLines.length, 3, 'the dependency gate must audit all three pnpm projects')
assert.equal(
  checkScript.includes('--ignore-registry-errors'),
  false,
  'registry failures must never be converted into successful audits',
)
assert.equal(
  checkScript.includes('--ignore-unfixable'),
  false,
  'the dependency gate must never add broad vulnerability exemptions',
)
assert.equal(
  /(?:^|\s)--ignore(?:=|\s|$)/mu.test(checkScript),
  false,
  'audit exemptions must be declared in pnpm-workspace.yaml, not added by the CI command',
)
assert.equal(
  auditLines.filter(line => line.includes('"${PNPM_AUDIT_NETWORK_ARGS[@]}"')).length,
  3,
  'all pnpm audits must share the bounded network retry policy',
)

for (const flag of [
  '--fetch-retries=5',
  '--fetch-retry-factor=2',
  '--fetch-retry-mintimeout=5000',
  '--fetch-retry-maxtimeout=20000',
  '--fetch-timeout=30000',
]) {
  assert.ok(checkScript.includes(flag), `missing dependency audit network setting: ${flag}`)
}

for (const workspacePath of [
  path.join(rootDir, 'web/pnpm-workspace.yaml'),
  path.join(rootDir, 'docs/pnpm-workspace.yaml'),
]) {
  const workspace = await readFile(workspacePath, 'utf8')
  assert.match(
    workspace,
    /auditConfig:\s*\n\s+ignoreGhsas:\s*\n\s+- GHSA-qwww-vcr4-c8h2/u,
    `missing the reviewed React Router audit exemption in ${workspacePath}`,
  )
}

function runAudit(registry, fetchRetries) {
  const args = [
    '--dir',
    agentDir,
    'audit',
    '--prod',
    '--audit-level=high',
    '--json',
    `--registry=${registry}`,
    `--fetch-retries=${fetchRetries}`,
    '--fetch-retry-factor=2',
    '--fetch-retry-mintimeout=10',
    '--fetch-retry-maxtimeout=20',
    '--fetch-timeout=1000',
  ]

  return new Promise((resolve, reject) => {
    const child = spawn('pnpm', args, {
      cwd: rootDir,
      env: {
        ...process.env,
        CI: 'true',
        NO_COLOR: '1',
      },
      stdio: ['ignore', 'pipe', 'pipe'],
    })
    let stdout = ''
    let stderr = ''

    child.stdout.setEncoding('utf8')
    child.stderr.setEncoding('utf8')
    child.stdout.on('data', chunk => {
      stdout += chunk
    })
    child.stderr.on('data', chunk => {
      stderr += chunk
    })
    child.once('error', reject)
    child.once('close', (code, signal) => {
      resolve({ code, signal, stderr, stdout })
    })
  })
}

function sendJson(response, status, body) {
  response.writeHead(status, {
    connection: 'close',
    'content-type': 'application/json',
  })
  response.end(JSON.stringify(body))
}

async function withAuditRegistry(respond, run) {
  const observed = {
    methods: [],
    paths: [],
    requests: 0,
  }
  const server = createServer((request, response) => {
    request.resume()
    request.once('end', () => {
      observed.requests += 1
      observed.methods.push(request.method)
      observed.paths.push(request.url)
      respond(request, response, observed)
    })
  })

  await new Promise((resolve, reject) => {
    server.once('error', reject)
    server.listen(0, '127.0.0.1', resolve)
  })

  const address = server.address()
  assert.ok(address && typeof address === 'object')
  const registry = `http://127.0.0.1:${address.port}/`

  try {
    await run(registry, observed)
  } finally {
    server.closeAllConnections?.()
    await new Promise((resolve, reject) => {
      server.close(error => error ? reject(error) : resolve())
    })
  }
}

function assertAuditRequests(observed) {
  assert.deepEqual(new Set(observed.methods), new Set(['POST']))
  assert.deepEqual(
    new Set(observed.paths),
    new Set(['/-/npm/v1/security/advisories/bulk']),
  )
}

await withAuditRegistry((_request, response, observed) => {
  if (observed.requests < 3) {
    sendJson(response, 503, { error: 'temporary registry failure' })
    return
  }
  sendJson(response, 200, {})
}, async (registry, observed) => {
  const result = await runAudit(registry, 2)
  assert.equal(result.code, 0, `flaky registry audit failed:\n${result.stdout}\n${result.stderr}`)
  assert.equal(observed.requests, 3, 'pnpm must retry transient registry failures')
  assertAuditRequests(observed)
  assert.deepEqual(JSON.parse(result.stdout).advisories, {})
})

await withAuditRegistry((_request, response) => {
  sendJson(response, 200, {
    '@fastify/otel': [{
      cwe: ['CWE-000'],
      id: 999998,
      severity: 'high',
      title: 'fixture high advisory',
      url: 'https://github.com/advisories/GHSA-1111-2222-3333',
      vulnerable_versions: '*',
    }],
  })
}, async (registry, observed) => {
  const result = await runAudit(registry, 5)
  assert.equal(result.code, 1, `high advisory did not fail the audit:\n${result.stdout}\n${result.stderr}`)
  assert.equal(observed.requests, 1, 'a completed vulnerable audit must not be retried')
  assertAuditRequests(observed)
  assert.ok(JSON.parse(result.stdout).advisories['999998'])
})

await withAuditRegistry((_request, response) => {
  sendJson(response, 503, { error: 'persistent registry failure' })
}, async (registry, observed) => {
  const result = await runAudit(registry, 2)
  assert.equal(result.code, 1, `persistent registry failure passed the audit:\n${result.stdout}\n${result.stderr}`)
  assert.equal(observed.requests, 3, 'pnpm must stop after the bounded retry count')
  assertAuditRequests(observed)
  assert.equal(JSON.parse(result.stdout).error.code, 'ERR_PNPM_AUDIT_BAD_RESPONSE')
})

process.stdout.write('Dependency audit policy tests passed.\n')
