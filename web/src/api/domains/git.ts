import type { GitAccount, GitBranch, GitContentItem, GitFileContent, GitProvider, GitRepository, GitRepositoryBuildOptions, PaginatedResponse, PaginationParams } from '../types'
import { optionalProjectQuery, paginationWithProjectQuery, request } from '../core'

export const gitApi = {
  listGitProviders: (projectId?: string) =>
    request<GitProvider[]>(`/git/providers${optionalProjectQuery(projectId)}`),
  listGitProvidersPage: (params: PaginationParams & { projectId?: string }) =>
    request<PaginatedResponse<GitProvider>>(`/git/providers?${paginationWithProjectQuery(params)}`),
  createGitProvider: (payload: Omit<GitProvider, 'id' | 'createdAt' | 'clientSecretSet'> & { scope?: GitProvider['scope'], ownerRef?: string, clientSecret?: string }) =>
    request<GitProvider>('/git/providers', { method: 'POST', body: JSON.stringify(payload) }),
  updateGitProvider: (providerId: string, payload: Omit<GitProvider, 'id' | 'createdAt' | 'clientSecretSet'> & { scope?: GitProvider['scope'], ownerRef?: string, clientSecret?: string }) =>
    request<GitProvider>(`/git/providers/${providerId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteGitProvider: (providerId: string) =>
    request<void>(`/git/providers/${providerId}`, { method: 'DELETE' }),
  listGitAccounts: (projectId?: string) =>
    request<GitAccount[]>(`/git/accounts${optionalProjectQuery(projectId)}`),
  listGitAccountsPage: (params: PaginationParams & { projectId?: string }) =>
    request<PaginatedResponse<GitAccount>>(`/git/accounts?${paginationWithProjectQuery(params)}`),
  createGitAccount: (payload: Omit<GitAccount, 'id' | 'userId' | 'scopes' | 'createdAt' | 'accessTokenSet' | 'refreshTokenSet'> & { scope?: GitAccount['scope'], ownerRef?: string, scopes: string[], accessToken?: string, refreshToken?: string }) =>
    request<GitAccount>('/git/accounts', { method: 'POST', body: JSON.stringify(payload) }),
  updateGitAccount: (accountId: string, payload: Omit<GitAccount, 'id' | 'userId' | 'scopes' | 'createdAt' | 'accessTokenSet' | 'refreshTokenSet'> & { scope?: GitAccount['scope'], ownerRef?: string, scopes: string[], accessToken?: string, refreshToken?: string }) =>
    request<GitAccount>(`/git/accounts/${accountId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteGitAccount: (accountId: string) =>
    request<void>(`/git/accounts/${accountId}`, { method: 'DELETE' }),
  refreshGitAccount: (accountId: string) =>
    request<GitAccount>(`/git/accounts/${accountId}/refresh`, { method: 'POST' }),
  listGitRepositories: (accountId: string, params: { page: number, pageSize: number, search?: string, includePublic?: boolean }) => {
    const search = new URLSearchParams({ page: String(params.page), pageSize: String(params.pageSize) })
    if (params.search)
      search.set('search', params.search)
    if (params.includePublic)
      search.set('includePublic', 'true')
    return request<{ items: GitRepository[], page: number, pageSize: number }>(`/git/accounts/${accountId}/repositories?${search.toString()}`)
  },
  listGitBranches: (accountId: string, owner: string, repo: string, params?: { search?: string, limit?: number }) => {
    const search = new URLSearchParams()
    if (params?.search)
      search.set('search', params.search)
    if (params?.limit)
      search.set('limit', String(params.limit))
    const suffix = search.toString() ? `?${search.toString()}` : ''
    return request<{ items: GitBranch[], total: number, matchedTotal: number, limited: boolean }>(`/git/accounts/${accountId}/repositories/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/branches${suffix}`)
  },
  readGitFile: (accountId: string, owner: string, repo: string, path: string, ref?: string) => {
    const search = new URLSearchParams({ path })
    if (ref)
      search.set('ref', ref)
    return request<GitFileContent>(`/git/accounts/${accountId}/repositories/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/file?${search.toString()}`)
  },
  listGitContents: (accountId: string, owner: string, repo: string, path = '', ref?: string) => {
    const search = new URLSearchParams()
    if (path)
      search.set('path', path)
    if (ref)
      search.set('ref', ref)
    return request<GitContentItem[]>(`/git/accounts/${accountId}/repositories/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/contents?${search.toString()}`)
  },
  getGitRepositoryBuildOptions: (accountId: string, owner: string, repo: string, ref?: string) => {
    const search = new URLSearchParams()
    if (ref)
      search.set('ref', ref)
    const suffix = search.toString() ? `?${search.toString()}` : ''
    return request<GitRepositoryBuildOptions>(`/git/accounts/${accountId}/repositories/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/build-options${suffix}`)
  },
}
