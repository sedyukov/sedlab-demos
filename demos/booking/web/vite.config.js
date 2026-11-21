import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { resolve } from 'node:path'
import { writeFileSync } from 'node:fs'

// keep the placeholder that lets `go:embed all:static` compile without a web build (CI tests)
const keepGitkeep = { name: 'keep-gitkeep', closeBundle() { writeFileSync(resolve(__dirname, '../server/static/.gitkeep'), '') } }

// Multi-page build → ../server/static (embedded into the Go binary)
export default defineConfig({
  plugins: [svelte(), keepGitkeep],
  build: {
    outDir: '../server/static',
    emptyOutDir: true,
    rollupOptions: {
      input: {
        emulator: resolve(__dirname, 'index.html'),
        app: resolve(__dirname, 'app/index.html'),
        admin: resolve(__dirname, 'admin/index.html'),
        pay: resolve(__dirname, 'pay/index.html'),
      },
    },
  },
  server: { proxy: { '/api': 'http://localhost:8080', '/demo': 'http://localhost:8080', '/tg': 'http://localhost:8080', '/payments': 'http://localhost:8080' } },
})
