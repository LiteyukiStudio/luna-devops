import antfu from '@antfu/eslint-config'

export default antfu({
  react: true,
  typescript: true,
  ignores: ['dist'],
}, {
  files: ['src/**/*.{ts,tsx}'],
  rules: {
    'no-restricted-imports': ['error', {
      patterns: [{
        regex: '^\\.\\./(?:\\.\\./)*(?:api|app|components|i18n|layouts|lib|pages)(?:/|$)',
        message: 'src shared modules must use @/ root imports. Keep relative imports only for local page/component files.',
      }],
    }],
  },
})
