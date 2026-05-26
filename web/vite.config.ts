import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The build writes straight into the Go server's embed target. emptyOutDir is
// required because that dir lives outside the Vite project root and holds the
// committed index.html placeholder, which this build replaces.
//
// The /api proxy supports the "open Vite on :5173 directly" dev workflow; the
// alternative is running the Go binary with `-server -dev`, which reverse-
// proxies the UI to this dev server instead.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: '../server/webdist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
