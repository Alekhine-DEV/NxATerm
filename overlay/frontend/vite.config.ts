import { defineConfig } from "vite";
import wails from "@wailsio/runtime/plugins/vite";

// https://vitejs.dev/config/
export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  // o noVNC usa "top-level await": precisa de um alvo moderno (WebView2 e WebKitGTK atuais aceitam)
  build: {
    target: "esnext",
    chunkSizeWarningLimit: 2000,
  },
  plugins: [wails("./bindings")],
});
