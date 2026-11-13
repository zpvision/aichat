import { defineConfig } from "vite";
export default defineConfig({
  publicDir: false,
  build: {
    emptyOutDir: false,
    lib: {
      entry: "src/loader.ts",
      name: "SiteAssistantLoader",
      formats: ["iife"],
      fileName: () => "loader.js",
    },
  },
});
