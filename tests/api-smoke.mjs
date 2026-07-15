import assert from 'node:assert/strict'
import crypto from 'node:crypto'

const apiBase = process.env.API_BASE_URL ?? 'http://127.0.0.1:8080/api/v1'
const healthBase = apiBase.replace(/\/api\/v1\/?$/, '')
const origin = process.env.WEB_BASE_URL ?? process.env.TEST_ORIGIN ?? 'http://127.0.0.1:5173'
const adminEmail = process.env.TEST_ADMIN_EMAIL ?? 'admin@luna.dev'
const adminPassword = process.env.TEST_ADMIN_PASSWORD ?? 'devops'
const runKey = `${Date.now().toString(36).slice(-6)}${crypto.randomBytes(2).toString('hex')}`
const runId = `smk-${runKey}`
const userSlug = `u-${runKey}`
const projectSlug = `p-${runKey}`
const environmentSlug = `e-${runKey}`

let cookie = ''

async function request(path, options = {}) {
  const response = await fetch(`${apiBase}${path}`, {
    ...options,
    headers: {
      'Accept-Language': 'zh-CN',
      'Content-Type': 'application/json',
      Origin: origin,
      ...(cookie ? { Cookie: cookie } : {}),
      ...options.headers,
    },
  })
  const setCookie = response.headers.get('set-cookie')
  if (setCookie)
    cookie = setCookie.split(';')[0]
  const text = await response.text()
  const body = text ? JSON.parse(text) : undefined
  return { body, response }
}

async function ok(path, options) {
  const result = await request(path, options)
  assert(result.response.status >= 200 && result.response.status < 300, `${path} -> ${result.response.status}: ${JSON.stringify(result.body)}`)
  return result.body
}

async function status(path, expected, options) {
  const result = await request(path, options)
  assert.equal(result.response.status, expected, `${path} expected ${expected}, got ${result.response.status}: ${JSON.stringify(result.body)}`)
  return result.body
}

function json(body) {
  return { body: JSON.stringify(body) }
}

function itemsOf(page) {
  return Array.isArray(page) ? page : page.items
}

async function main() {
  const health = await fetch(`${healthBase}/healthz`)
  assert.equal(health.status, 200, 'healthz should be healthy')

  await ok('/auth/bootstrap')
  await ok('/auth/login', { method: 'POST', ...json({ email: adminEmail, password: adminPassword }) })
  const me = await ok('/users/me')
  assert.equal(me.email, adminEmail)

  await ok('/users/me', { method: 'PUT', ...json({ name: me.name || 'Platform Admin', language: 'zh-CN', avatarUrl: me.avatarUrl ?? '' }) })
  await ok('/public/configs', { method: 'POST', ...json({ keys: ['site.title', 'site.logo'] }) })
  await ok('/configs/definitions')
  await ok('/configs', { method: 'PUT', ...json({ values: { 'site.title': `Luna DevOps ${runId}` } }) })

  await ok('/auth/providers')
  const providers = await ok('/auth/providers?includeDisabled=true')
  assert(Array.isArray(providers))
  const admission = await ok('/auth/admission-policy')
  await ok('/auth/admission-policy', { method: 'PUT', ...json({
    allowLocalLogin: admission.allowLocalLogin,
    allowOidcLogin: admission.allowOidcLogin,
    allowedEmailDomains: admission.allowedEmailDomains ?? [],
    allowedOidcGroups: admission.allowedOidcGroups ?? [],
    invitedEmails: admission.invitedEmails ?? [],
    defaultRole: admission.defaultRole ?? 'user',
  }) })

  const userEmail = `${userSlug}@example.com`
  const user = await ok('/users', { method: 'POST', ...json({ email: userEmail, name: userSlug, password: 'devops-pass', role: 'user', language: 'zh-CN', disabled: false }) })
  const defaultProjects = itemsOf(await ok(`/projects?scope=all&page=1&pageSize=10&search=${encodeURIComponent(userSlug)}&sortBy=createdAt&sortOrder=desc`))
  const defaultProject = defaultProjects.find(item => item.slug === userSlug)
  assert(defaultProject, 'new user should receive a default project space')
  await ok('/users?page=1&pageSize=10&sortBy=email&sortOrder=asc')
  await ok(`/users/${user.id}`, { method: 'PUT', ...json({ email: userEmail, name: `${userSlug}-updated`, role: 'user', language: 'zh-CN', disabled: false }) })

  const token = await ok('/access-tokens', { method: 'POST', ...json({ name: runId, scope: 'project:read', expiresInDays: 7 }) })
  assert.equal(typeof token.accessToken, 'string')
  assert(token.accessToken.length >= 32)
  await ok('/access-tokens?page=1&pageSize=10')
  await ok(`/access-tokens/${token.token.id}`, { method: 'DELETE' })

  const project = await ok('/projects', { method: 'POST', ...json({ slug: projectSlug, name: projectSlug, description: 'smoke project' }) })
  await ok('/projects?page=1&pageSize=10&sortBy=createdAt&sortOrder=desc')
  await ok(`/projects/${project.id}`)
  await ok(`/projects/${project.id}`, { method: 'PUT', ...json({ slug: project.slug, name: `${runId} project`, description: 'updated smoke project' }) })
  await ok('/projects/pins')
  await ok(`/projects/${project.id}/pin`, { method: 'PUT' })
  await ok(`/projects/${project.id}/pin`, { method: 'DELETE' })

  await ok(`/projects/${project.id}/members`)
  const member = await ok(`/projects/${project.id}/members`, { method: 'POST', ...json({ email: userEmail, role: 'viewer' }) })
  await ok(`/projects/${project.id}/members/${member.id}`, { method: 'PUT', ...json({ role: 'developer' }) })

  const app = await ok(`/projects/${project.id}/applications`, { method: 'POST', ...json({
    slug: runId,
    name: runId,
    sourceType: 'repository',
    repositoryUrl: 'https://example.com/luna/demo.git',
    imageReference: '',
    dockerfilePath: 'Dockerfile',
    buildContext: '.',
    servicePort: 8080,
  }) })
  await ok(`/projects/${project.id}/applications`)
  await ok(`/projects/${project.id}/applications/${app.id}`)
  await ok(`/projects/${project.id}/applications/${app.id}`, { method: 'PUT', ...json({ ...app, name: `${runId} app`, servicePort: 8081 }) })

  const registry = await ok('/registries', { method: 'POST', ...json({
    name: runId,
    provider: 'harbor',
    endpoint: 'https://registry-1.docker.io',
    namespace: runId,
    scope: 'project',
    ownerRef: '',
    projectIds: [project.id],
    isDefault: true,
    capabilities: ['pull', 'push'],
  }) })
  await ok('/registries')
  await ok(`/projects/${project.id}/registries/default`)
  const credential = await ok(`/registries/${registry.id}/credentials`, { method: 'POST', ...json({ name: runId, username: runId, token: 'dummy-token', scope: 'push-pull', accessScope: 'registry' }) })
  await ok(`/registries/${registry.id}/credentials`)
  const registryTest = await request(`/registries/${registry.id}/test`, { method: 'POST' })
  assert(registryTest.response.status >= 200 && registryTest.response.status < 500, `registry test unexpected ${registryTest.response.status}`)
  const image = await ok('/container-images', { method: 'POST', ...json({ projectId: project.id, applicationId: app.id, registryId: registry.id, repository: 'smoke/app', tag: runId, sourceType: 'manual-image', scanStatus: 'unknown' }) })
  assert(image.imageRef.includes('smoke/app'))
  await ok(`/container-images?projectId=${encodeURIComponent(project.id)}`)

  const cluster = await ok('/runtime/clusters', { method: 'POST', ...json({
    name: runId,
    type: 'kubernetes',
    endpoint: 'https://cluster.example.com',
    kubeconfig: 'apiVersion: v1\nkind: Config\n',
    isDefault: true,
  }) })
  await ok('/runtime/clusters')
  const clusterTest = await request(`/runtime/clusters/${cluster.id}/test`, { method: 'POST' })
  assert(clusterTest.response.status >= 200 && clusterTest.response.status < 500, `cluster test unexpected ${clusterTest.response.status}`)
  await ok(`/runtime/clusters/${cluster.id}`, { method: 'PUT', ...json({ ...cluster, name: `${runId} cluster`, kubeconfig: '' }) })

  const environment = await ok(`/projects/${project.id}/environments`, { method: 'POST', ...json({
    name: 'Development',
    slug: environmentSlug,
    stage: 'dev',
    clusterId: cluster.id,
    namespace: `${project.slug}-dev`,
    replicas: 1,
    cpuRequest: '100m',
    memoryRequest: '128Mi',
    envVars: '{}',
    configRefs: '',
    secretRefs: '',
  }) })
  await ok(`/projects/${project.id}/environments`)
  await ok(`/projects/${project.id}/environments/${environment.id}`, { method: 'PUT', ...json({ ...environment, replicas: 2 }) })

  const gitProvider = await ok('/git/providers', { method: 'POST', ...json({
    type: 'gitea',
    name: runId,
    baseUrl: 'https://gitea.example.com',
    scope: 'project',
    ownerRef: '',
    projectIds: [project.id],
    authType: 'pat',
    clientId: '',
    clientSecret: '',
    enabled: true,
  }) })
  await ok(`/git/providers?projectId=${encodeURIComponent(project.id)}`)
  await ok(`/git/providers/${gitProvider.id}`, { method: 'PUT', ...json({ ...gitProvider, name: `${runId} git`, clientSecret: '' }) })
  await status(`/git/providers/${gitProvider.id}/oauth/start`, 400)

  const gitAccount = await ok('/git/accounts', { method: 'POST', ...json({
    providerId: gitProvider.id,
    scope: 'project',
    ownerRef: '',
    projectIds: [project.id],
    externalUserId: runId,
    username: runId,
    avatarUrl: '',
    scopes: ['repo'],
    accessToken: 'dummy-token',
    refreshToken: 'dummy-refresh',
    status: 'connected',
  }) })
  await ok(`/git/accounts?projectId=${encodeURIComponent(project.id)}`)
  const repoList = await request(`/git/accounts/${gitAccount.id}/repositories?page=1&pageSize=10&search=demo`)
  assert(repoList.response.status >= 200 && repoList.response.status < 600)
  const fileRead = await request(`/git/accounts/${gitAccount.id}/repositories/luna/demo/file?path=Dockerfile`)
  assert(fileRead.response.status >= 200 && fileRead.response.status < 600)

  const binding = await ok(`/projects/${project.id}/repository-bindings`, { method: 'POST', ...json({
    applicationId: app.id,
    gitAccountId: gitAccount.id,
    owner: 'luna',
    repo: 'demo',
    cloneUrl: 'https://gitea.example.com/luna/demo.git',
    defaultBranch: 'main',
    webhookStatus: 'pending',
  }) })
  await ok(`/projects/${project.id}/repository-bindings`)
  const webhookCreate = await request(`/projects/${project.id}/repository-bindings/${binding.id}/webhook`, { method: 'POST' })
  assert(webhookCreate.response.status >= 200 && webhookCreate.response.status < 600)
  await status(`/git/webhooks/${binding.id}`, 401, { method: 'POST', ...json({ after: 'abc123' }) })

  const target = await ok(`/projects/${project.id}/applications/${app.id}/deployment-targets`, { method: 'POST', ...json({
    name: 'Development',
    environmentId: environment.id,
    servicePort: 8080,
    sourceType: 'repository',
    repositoryBindingId: binding.id,
    dockerfilePath: 'Dockerfile',
    buildContext: '.',
    targetRegistryId: registry.id,
    targetRepository: 'smoke/app',
    targetTag: runId,
    buildHooksEnabled: true,
    autoDeploy: true,
    branchPattern: 'main',
    tagPattern: 'v*',
    concurrencyPolicy: 'queue',
    envVars: '{}',
    configRefs: '',
    secretRefs: '',
  }) })
  await ok(`/projects/${project.id}/applications/${app.id}/deployment-targets`)

  const buildRun = await ok(`/projects/${project.id}/build-runs/trigger`, { method: 'POST', ...json({
    applicationId: app.id,
    deploymentTargetId: target.id,
    triggerType: 'manual',
    sourceBranch: 'main',
    targetRegistryId: registry.id,
    targetRepository: 'smoke/app',
    targetTag: runId,
  }) })
  await ok(`/projects/${project.id}/build-runs`)
  await ok(`/projects/${project.id}/build-runs/${buildRun.id}`)
  await ok(`/projects/${project.id}/build-jobs?buildRunId=${encodeURIComponent(buildRun.id)}`)

  const release = await ok(`/projects/${project.id}/releases`, { method: 'POST', ...json({
    applicationId: app.id,
    environmentId: environment.id,
    deploymentTargetId: target.id,
    imageRef: image.imageRef,
    type: 'deploy',
    status: 'pending',
    revision: 1,
    message: 'smoke deploy',
  }) })
  await ok(`/projects/${project.id}/releases`)
  const rollback = await request(`/projects/${project.id}/releases/${release.id}/rollback`, { method: 'POST' })
  assert(rollback.response.status >= 200 && rollback.response.status < 500, `rollback unexpected ${rollback.response.status}`)

  const route = await ok(`/projects/${project.id}/gateway-routes`, { method: 'POST', ...json({
    applicationId: app.id,
    environmentId: environment.id,
    deploymentTargetId: target.id,
    host: `${runId}.example.test`,
    path: '/',
    servicePort: 8080,
    tlsMode: 'http-only',
    dnsStatus: 'pending',
    status: 'pending',
    isDefault: true,
  }) })
  await ok(`/projects/${project.id}/gateway-routes`)
  await ok(`/projects/${project.id}/gateway-routes/check-domain?host=${encodeURIComponent(`${runId}-other.example.test`)}`)
  const routeUpdate = await request(`/projects/${project.id}/gateway-routes/${route.id}`, { method: 'PUT', ...json({ ...route, path: '/app' }) })
  assert(routeUpdate.response.status >= 200 && routeUpdate.response.status < 600, `gateway route update unexpected ${routeUpdate.response.status}`)

  await ok(`/projects/${project.id}/gateway-routes/${route.id}`, { method: 'DELETE' })
  await ok(`/projects/${project.id}/applications/${app.id}/deployment-targets/${target.id}`, { method: 'DELETE' })
  await ok(`/projects/${project.id}/repository-bindings/${binding.id}`, { method: 'DELETE' })
  await ok(`/git/accounts/${gitAccount.id}`, { method: 'DELETE' })
  await ok(`/git/providers/${gitProvider.id}`, { method: 'DELETE' })

  await ok(`/registries/${registry.id}/credentials/${credential.id}`, { method: 'DELETE' })
  await ok(`/registries/${registry.id}`, { method: 'DELETE' })
  await ok(`/projects/${project.id}/applications/${app.id}`, { method: 'DELETE' })
  await ok(`/projects/${project.id}/members/${member.id}`, { method: 'DELETE' })
  await ok(`/projects/${project.id}`, { method: 'DELETE' })
  await ok(`/projects/${defaultProject.id}`, { method: 'DELETE' })
  await ok('/auth/logout', { method: 'POST' })

  console.log(`API smoke passed: ${runId}`)
}

main().catch((error) => {
  console.error(error)
  process.exit(1)
})
