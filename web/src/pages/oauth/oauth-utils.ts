import type { TFunction } from 'i18next'

export function splitOAuthScopes(value: string) {
  return value.split(/[\s,]+/).filter(Boolean)
}

export function oauthScopeLabel(t: TFunction, scope: string) {
  const key = `accessTokens.scopeLabels.${scope.replaceAll(':', '.').replaceAll('_', '-')}`
  const translated = t(key)
  return translated === key ? scope : translated
}
