import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
export default defineConfig({
  base: "/widget/",
  plugins: [react()],
  build: { sourcemap: false },
  server: { port: 4173 },
});
