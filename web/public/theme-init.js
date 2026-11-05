(() => {
  const root = document.documentElement
  const validBrandPresets = new Set(['blue', 'violet', 'teal', 'green', 'orange', 'pink'])
  const readStorage = (key) => {
    try {
      return localStorage.getItem(key)
    }
    catch {
      return null
    }
  }

  const mode = readStorage('luna-devops-theme')
  const resolvedMode = mode === 'light' || mode === 'dark'
    ? mode
    : window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'

  root.dataset.theme = resolvedMode
  root.classList.remove('light', 'dark')
  root.classList.add(resolvedMode)
  root.style.colorScheme = resolvedMode

  const injectedPreset = validBrandPresets.has(root.dataset.brandTheme) ? root.dataset.brandTheme : null
  const cachedSitePreset = readStorage('luna-devops-site-brand-color-preset')
  const sitePreset = injectedPreset || (validBrandPresets.has(cachedSitePreset) ? cachedSitePreset : 'blue')
  try {
    localStorage.setItem('luna-devops-site-brand-color-preset', sitePreset)
  }
  catch {
    // The injected site preset remains available when browser storage is unavailable.
  }

  const activeUserId = readStorage('luna-devops-theme-active-user')
  const userPreset = activeUserId
    ? readStorage(`luna-devops-user-brand-color-preset:${activeUserId}`)
    : null
  root.dataset.brandTheme = validBrandPresets.has(userPreset) ? userPreset : sitePreset
})()
