import type { TFunction } from 'i18next'

export function splitAccessTokenScopes(scopeText: string) {
  return scopeText.split(/[\s,]+/).map(scope => scope.trim()).filter(Boolean)
}

export function accessTokenScopeLabel(t: TFunction, scope: string) {
  const key = `accessTokens.scopeLabels.${accessTokenScopeKey(scope)}`
  const label = t(key)
  return label === key ? scope : label
}

export function accessTokenScopeKey(scope: string) {
  return scope.replaceAll(':', '.').replaceAll('_', '-')
}
