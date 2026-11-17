import antfu from '@antfu/eslint-config'

const staticViewportPattern = '(?:\\b(?:min-h|max-h|h)-screen\\b|\\d+(?:\\.\\d+)?vh\\b)'

export default antfu({
  react: true,
  typescript: true,
  ignores: ['dist', 'src/api/generated/**'],
}, {
  files: ['src/**/*.{ts,tsx}'],
  rules: {
    'no-restricted-imports': ['error', {
      patterns: [{
        regex: '^\\.\\./(?:\\.\\./)*(?:api|app|components|i18n|layouts|lib|pages)(?:/|$)',
        message: 'src shared modules must use @/ root imports. Keep relative imports only for local page/component files.',
      }],
    }],
    'no-restricted-syntax': ['error', {
      selector: ':matches(CallExpression[callee.name=/^(?:useEffect|useLayoutEffect)$/], CallExpression[callee.property.name=/^(?:useEffect|useLayoutEffect)$/]) CallExpression[callee.property.name="scrollIntoView"]',
      message: 'Do not call scrollIntoView() inside useEffect/useLayoutEffect; it scrolls ancestor containers and causes mobile scroll snap-back. Update the local container scrollLeft / scrollTo({ left }) instead.',
    }, {
      selector: 'NewExpression[callee.name="IntersectionObserver"] CallExpression[callee.property.name="scrollIntoView"]',
      message: 'Do not call scrollIntoView() inside IntersectionObserver callbacks; update the local container scrollLeft / scrollTo({ left }) instead.',
    }],
  },
}, {
  files: ['src/**/*.{ts,tsx}'],
  ignores: ['src/**/*.test.{ts,tsx}'],
  rules: {
    'no-restricted-syntax': ['error', {
      selector: ':matches(CallExpression[callee.name=/^(?:useEffect|useLayoutEffect)$/], CallExpression[callee.property.name=/^(?:useEffect|useLayoutEffect)$/]) CallExpression[callee.property.name="scrollIntoView"]',
      message: 'Do not call scrollIntoView() inside useEffect/useLayoutEffect; it scrolls ancestor containers and causes mobile scroll snap-back. Update the local container scrollLeft / scrollTo({ left }) instead.',
    }, {
      selector: 'NewExpression[callee.name="IntersectionObserver"] CallExpression[callee.property.name="scrollIntoView"]',
      message: 'Do not call scrollIntoView() inside IntersectionObserver callbacks; update the local container scrollLeft / scrollTo({ left }) instead.',
    }, {
      selector: `Literal[raw=/^['"]/][value=/${staticViewportPattern}/]`,
      message: 'Use dynamic viewport units (dvh: h-dvh, min-h-dvh, max-h-dvh, *[...dvh]) instead of static vh or *-screen height classes.',
    }, {
      selector: `TemplateElement[value.raw=/${staticViewportPattern}/]`,
      message: 'Use dynamic viewport units (dvh: h-dvh, min-h-dvh, max-h-dvh, *[...dvh]) instead of static vh or *-screen height classes.',
    }],
  },
})
