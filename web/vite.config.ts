import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Build straight into the Go package that embeds it, so `go build` ships the
// dashboard with no extra copy step.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: "../internal/server/dashboard/dist",
    emptyOutDir: true,
    assetsDir: "assets",
  },
  server: {
    proxy: {
      "/ws": { target: "ws://localhost:8080", ws: true },
      "/health": "http://localhost:8080",
    },
  },
});
