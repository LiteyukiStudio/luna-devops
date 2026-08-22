import i18next, { i18nextReady } from '@/i18n'
import { lazyTranslationResourcePath, lazyTranslationResources } from './deployment-bundle-resources'

function registerTranslations() {
  for (const [language, resources] of Object.entries(lazyTranslationResources))
    i18next.addResourceBundle(language, 'translation', nestResource(lazyTranslationResourcePath, resources), true, true)
}

function nestResource(resourcePath: string, resources: object) {
  return resourcePath.split('.').reverse().reduce<object>((nested, segment) => ({ [segment]: nested }), resources)
}

if (i18next.isInitialized)
  registerTranslations()
else
  void i18nextReady.then(registerTranslations)
