const documentationBaseUrl = String(import.meta.env.VITE_DOCS_BASE_URL || 'https://luna-devops.liteyuki.org').replace(/\/+$/, '')

export function getDocumentationUrl(path = '', language?: string) {
  const languagePrefix = language?.toLowerCase().startsWith('en') ? 'en' : ''
  const normalizedPath = path.replace(/^\/+|\/+$/g, '')
  return [documentationBaseUrl, languagePrefix, normalizedPath].filter(Boolean).join('/')
}
