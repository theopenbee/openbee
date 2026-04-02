import { defineConfig } from "vite"
import react from "@vitejs/plugin-react"
import path from "path"

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    proxy: {
      "/api": "http://localhost:8080",
      "/mcp": "http://localhost:8080",
      "/internal": "http://localhost:8080",
    },
  },
  build: {
    target: "es2020",
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes("node_modules")) return;
          if (id.includes("/react/") || id.includes("/react-dom/")) return "vendor-react";
          if (id.includes("/react-router-dom/") || id.includes("/react-router/")) return "vendor-router";
          if (id.includes("/@tanstack/react-query/")) return "vendor-query";
          if (id.includes("/i18next/") || id.includes("/react-i18next/")) return "vendor-i18n";
          if (id.includes("/@base-ui/react/")) return "vendor-ui";
          if (id.includes("/lucide-react/")) return "vendor-icons";
        },
      },
    },
  },
})
