import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// Where the Go server is listening in development. Override with POKER_API
// when running it somewhere else: POKER_API=http://127.0.0.1:8080 npm run dev
const API = process.env.POKER_API ?? 'http://127.0.0.1:9011'

// The Go server is a pure JSON + SSE API under /api. Everything else — the app
// itself, the card art in public/cards — is served from this bundle, in dev by
// Vite and in production by the web server. Proxying /api keeps the session
// cookie same-origin and the EventSource stream behaving as it does in prod.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    // Bind every interface, not just loopback: this is a phone-first app and
    // most of the real testing happens on a handset pointed at this machine's
    // LAN address. Vite 8 otherwise listens on [::1] only, which is neither
    // reachable from the network nor from anything that resolves localhost to
    // 127.0.0.1.
    host: true,
    proxy: {
      // /api carries both JSON and the SSE stream, which the proxy passes
      // through unbuffered so live updates behave as they do in production.
      // Use an explicit IP rather than localhost: the Go server listens on
      // IPv4, and resolving localhost to ::1 leaves the proxy talking to
      // nothing.
      '/api': { target: API, changeOrigin: true },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
