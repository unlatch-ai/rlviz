import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  // Keep committed embedded assets byte-identical across macOS and Linux.
  // Esbuild's native minifier can vary by platform; Terser is pure JavaScript.
  build: { outDir: "dist", emptyOutDir: true, minify: "terser" },
  test: {
    environment: "jsdom",
    setupFiles: "./src/test/setup.ts",
    css: true,
  },
});
