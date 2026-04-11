import { defineConfig } from "vitest/config"
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
          // Extract the actual package name from the last node_modules segment in the path.
          // The greedy .* ensures we pick the last occurrence, which is correct for pnpm
          // virtual store paths like node_modules/.pnpm/pkg@ver/node_modules/pkg/...
          const pkgMatch = id.match(/.*\/node_modules\/((?:@[^/]+\/)?[^/]+)/);
          if (!pkgMatch) return;
          const pkg = pkgMatch[1];
          if (pkg === "react" || pkg === "react-dom") return "vendor-react";
          if (pkg === "react-router-dom" || pkg === "react-router") return "vendor-router";
          if (pkg === "@tanstack/react-query") return "vendor-query";
          if (pkg === "i18next" || pkg === "react-i18next") return "vendor-i18n";
          if (pkg === "@base-ui/react") return "vendor-ui";
          if (pkg === "lucide-react") return "vendor-icons";
        },
      },
    },
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"],
  },
})
