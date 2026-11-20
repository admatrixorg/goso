import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const backendPort = env.VITE_BACKEND_PORT || "8080";
  const backendHost = env.VITE_BACKEND_HOST || "127.0.0.1";
  const target = "http://" + backendHost + ":" + backendPort;

  return {
    plugins: [react(), tailwindcss()],
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
      },
    },
    server: {
      port: 3000,
      proxy: {
        "/ws": { target, ws: true, changeOrigin: true },
        "/v1": { target, changeOrigin: true, timeout: 30000 },
        "/api": { target, changeOrigin: true, timeout: 30000 },
        "/health": { target, changeOrigin: true },
        "/healthz": { target, changeOrigin: true },
      },
    },
    build: {
      outDir: "dist",
      emptyOutDir: true,
    },
  };
});
