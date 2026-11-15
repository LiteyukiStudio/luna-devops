import { fileURLToPath, URL } from 'node:url'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

export default defineConfig(({ mode }) => ({
  define: {
    'process.env.NODE_ENV': JSON.stringify(mode === 'production' ? 'production' : 'development'),
  },
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: './src/test/setup.ts',
  },
  build: {
    rolldownOptions: {
      output: {
        codeSplitting: {
          minSize: 20_000,
          groups: [
            {
              name: 'react-vendor',
              test: /\/node_modules\/(?:\.pnpm\/[^/]+\/node_modules\/)?(?:react|react-dom)\//,
              priority: 30,
            },
            {
              name: 'router-query-vendor',
              test: /\/node_modules\/(?:\.pnpm\/[^/]+\/node_modules\/)?(?:react-router|react-router-dom|@tanstack\/react-query)\//,
              priority: 20,
            },
            {
              name: 'ui-vendor',
              test: /\/node_modules\/(?:\.pnpm\/[^/]+\/node_modules\/)?(?:lucide-react|radix-ui|@radix-ui\/|cmdk|sonner|class-variance-authority|clsx|tailwind-merge)\//,
              priority: 10,
            },
          ],
        },
      },
    },
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        ws: true,
      },
      '/healthz': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
}))
