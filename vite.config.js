import { defineConfig } from 'vite';
import { resolve } from 'path';

export default defineConfig({
  build: {
    outDir: 'web/static/dist',
    emptyOutDir: true,
    rollupOptions: {
      input: resolve(__dirname, 'web/static/css/app.css'),
      output: {
        assetFileNames: 'assets/main[extname]',
      },
    },
    cssMinify: true,
  },
});
