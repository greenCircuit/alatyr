import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  // GitHub Pages serves the demo from /<repo>/, so asset URLs need that prefix.
  // Unset for the normal build, where the Go binary serves the SPA at root.
  base: process.env.VITE_BASE ?? '/',
  plugins: [react()],
  server: {
    host: '0.0.0.0',
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
  test: {
    environment: 'node',
    coverage: {
      provider: 'v8',
      include: ['src/store/filters.ts'],
      reporter: ['text', 'lcov'],
    },
  },
})
