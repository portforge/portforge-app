import {defineConfig} from 'vite'
import vue from '@vitejs/plugin-vue'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
  css: {
    preprocessorOptions: {
      scss: {
        // Auto-import include-media into every <style lang="scss"> block so
        // breakpoint mixins are available without a manual @use in each file.
        additionalData: `@use 'include-media' as *;\n`,
      },
    },
  },
})
