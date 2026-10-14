import {defineConfig} from 'vite'
import react from '@vitejs/plugin-react'
import { readFile } from 'node:fs/promises'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react(), {
    name: 'desktop-mascot-bytes',
    enforce: 'pre',
    async load(id) {
      if (!/\/assets\/mascot\/[^/]+\.png$/.test(id)) return null;
      // Decode the original bytes directly, avoiding stale transparent images
      // returned by the native webview's cached wails:// asset URLs.
      const bytes = await readFile(id);
      return `export default ${JSON.stringify(`data:image/png;base64,${bytes.toString('base64')}`)}`;
    },
  }],
  server: {
    fs: {
      allow: ['../..']
    }
  }
})
