import path from "path"
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    proxy: {
      // The backend port is configurable (SQLFLOW_SERVER_PORT), so hardcoding
      // 8080 here breaks any setup that moved it — including running a second
      // instance alongside one that already holds the default port.
      '/api': process.env.SQLFLOW_API_TARGET ?? 'http://localhost:8080',
    },
  },
})
