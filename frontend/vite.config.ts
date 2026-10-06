import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { createHash } from 'node:crypto'
import { readFile, writeFile } from 'node:fs/promises'
import { gzipSync } from 'node:zlib'

const rootDir = path.dirname(fileURLToPath(import.meta.url))

export default defineConfig({
  plugins: [react(), {
    name: 'compressed-fonts',
    apply: 'build',
    async closeBundle() {
      for (const weight of ['Regular', 'Bold']) {
        const filename = path.resolve(rootDir, `../web/fonts/harmonyos-sans/HarmonyOS_Sans_SC_${weight}.ttf`)
        const original = await readFile(filename)
        const digest = createHash('sha256').update(original).digest('hex')
        await writeFile(`${filename}.${digest}.gz`, gzipSync(original, { level: 9 }))
      }
    },
  }],
  resolve: {
    alias: {
      '@': path.resolve(rootDir, 'src'),
    },
  },
  build: {
    outDir: '../web',
    emptyOutDir: false,
    rollupOptions: {
      output: {
        entryFileNames: 'assets/app.js',
        chunkFileNames: 'assets/[name].js',
        assetFileNames: 'assets/[name].[ext]',
        // 将 React 全家桶与图标库拆为独立 chunk，利用浏览器长缓存
        manualChunks: {
          vendor: ['react', 'react-dom', 'react-router-dom'],
          icons: ['lucide-react'],
        },
      },
    },
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
      '/health': 'http://localhost:8080',
    },
  },
})
