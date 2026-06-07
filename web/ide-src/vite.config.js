import { defineConfig } from 'vite'

export default defineConfig({
  base: '/ide-assets/',
  build: {
    outDir: '../static/ide-assets',
    emptyOutDir: true,
    rollupOptions: {
      input: { main: './src/main.js' },
      output: {
        entryFileNames: '[name].js',
        chunkFileNames: '[name]-[hash].js',
        assetFileNames: (info) => info.name === 'main.css' ? 'main.css' : '[name]-[hash][extname]',
      },
    },
  },
})
