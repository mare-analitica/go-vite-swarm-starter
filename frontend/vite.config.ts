import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// In development the API runs on :8080 and is proxied, so the browser talks
// to a single origin. In production the API has its own domain (VITE_API_URL).
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
