import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, ".", "");
  const apiProxyTarget = env.VITE_API_PROXY_TARGET || "http://localhost:9000";

  return {
    plugins: [react()],
    server: {
      proxy: {
        "/api": {
          target: apiProxyTarget,
          // Preserve the browser-facing Host for Gateway's strict WebSocket
          // origin check. Rewriting Host to the backend target makes the
          // terminal Origin (the Vite address) look cross-origin and the
          // upgrade is rejected with HTTP 403.
          changeOrigin: false,
          secure: false,
          ws: true,
        },
      },
    },
    esbuild: {
      minifyIdentifiers: false, // 防止跟 xterm 压缩冲突
      keepNames: true,
    },
    build: {
      minify: false,
      rollupOptions: {
        output: {
          manualChunks: {
            xterm: [
              "@xterm/xterm",
              "@xterm/addon-fit",
              "@xterm/addon-web-links",
            ],
          },
        },
      },
    },
  };
});
