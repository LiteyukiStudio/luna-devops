const DEFAULT_DOCUMENTATION_BASE_URL = 'https://luna-devops.liteyuki.org'

function documentationBaseUrl() {
  const configured = String(import.meta.env.VITE_DOCS_BASE_URL || DEFAULT_DOCUMENTATION_BASE_URL).trim()
  try {
    const url = new URL(configured)
    if (url.protocol === 'http:' || url.protocol === 'https:')
      return url.toString().replace(/\/+$/, '')
  }
  catch {
    // Invalid public build configuration falls back to the official documentation site.
  }
  return DEFAULT_DOCUMENTATION_BASE_URL
}

export function cliDocumentationUrl(language: string) {
  const localizedPath = language.toLowerCase().startsWith('zh') ? '/use/cli' : '/en/use/cli'
  return `${documentationBaseUrl()}${localizedPath}`
}
