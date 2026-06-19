import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  base: './', // PENTING: Mengubah penulisan path aset dari absolut menjadi relatif
  plugins: [vue()],
  server: {
    port: 3000
  }
})
