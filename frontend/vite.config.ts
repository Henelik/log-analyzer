import { writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { defineConfig, type Plugin } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

const backend = 'http://localhost:3000';

// Keeps the embed directory present for go:embed after a build wipes it.
function keepDir(): Plugin {
  return {
    name: 'keep-embed-dir',
    closeBundle() {
      writeFileSync(resolve(import.meta.dirname, '../ui/dist/.gitkeep'), '');
    }
  };
}

export default defineConfig({
  plugins: [svelte(), keepDir()],
  server: {
    proxy: {
      '/api': backend,
      '/healthz': backend,
      '/readyz': backend
    }
  },
  build: {
    outDir: '../ui/dist',
    emptyOutDir: true
  }
});
