import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const dir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(dir, "../..");

const demoMode =
  process.env.VITE_DEMO_MODE === "true" || process.env.VITE_DEMO_MODE === "1"
    ? process.env.VITE_DEMO_MODE
    : "false";

export default defineConfig({
  plugins: [react()],
  clearScreen: false,
  define: {
    "import.meta.env.VITE_DEMO_MODE": JSON.stringify(demoMode),
  },
  resolve: {
    alias: {
      "@control-plane": path.resolve(repoRoot, "control-plane/src"),
      react: path.resolve(dir, "node_modules/react"),
      "react-dom": path.resolve(dir, "node_modules/react-dom"),
    },
    dedupe: ["react", "react-dom"],
  },
  server: {
    fs: { allow: [repoRoot] },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
