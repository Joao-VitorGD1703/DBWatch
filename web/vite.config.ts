import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': {
        target: 'http://api:8080',
        changeOrigin: true,
      },
      '/ws': {
        target: 'ws://api:8080',
        ws: true,
      }
    }
  }
})
