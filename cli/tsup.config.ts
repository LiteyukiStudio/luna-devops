import { defineConfig } from 'tsup'
import process from 'node:process'

const version = process.env.LUNA_CLI_VERSION ?? '0.0.0-development'

export default defineConfig({
  entry: ['src/entry.ts'],
  format: ['esm'],
  dts: false,
  clean: true,
  sourcemap: false,
  noExternal: [
    '@luna-devops/api-client',
    '@luna-devops/api-contract',
  ],
  define: {
    __LUNA_CLI_VERSION__: JSON.stringify(version),
  },
})
