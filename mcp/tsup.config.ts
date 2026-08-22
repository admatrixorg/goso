import { defineConfig } from "tsup";

export default defineConfig({
  entry: {
    index: "src/index.ts",
    http: "src/http.ts",
  },
  format: ["esm"],
  target: "node20",
  outDir: "dist",
  clean: true,
  sourcemap: true,
  dts: false,
  splitting: true,
  banner: {
    // Shebang for stdio CLI entry point
    js: "#!/usr/bin/env node",
  },
});
