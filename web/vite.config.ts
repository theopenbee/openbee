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
  build: {
    target: "es2020",
    rollupOptions: {
      output: {
        manualChunks: {
          "vendor-react":  ["react", "react-dom"],
          "vendor-router": ["react-router-dom"],
          "vendor-query":  ["@tanstack/react-query"],
          "vendor-i18n":   ["i18next", "react-i18next"],
          "vendor-ui":     ["@base-ui/react"],
          "vendor-icons":  ["lucide-react"],
        },
      },
    },
  },
})
