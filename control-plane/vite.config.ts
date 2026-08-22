import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/healthz": { target: "http://127.0.0.1:8080", changeOrigin: true },
      "/api": { target: "http://127.0.0.1:8080", changeOrigin: true },
      "/ws": { target: "ws://127.0.0.1:8080", ws: true, changeOrigin: true },
      // Optional CORS-free path to goso-crm (default http://127.0.0.1:8089).
      // Set VITE_GOSOCRM_API_URL=/crm-api or leave unset to use this prefix.
      "/crm-api": {
        target: "http://127.0.0.1:8089",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/crm-api/, ""),
      },
    },
  },
});
