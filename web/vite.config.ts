import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Build straight into the Go package that embeds it, so `go build` ships the
// dashboard with no extra copy step.
const backend = "http://localhost:8080";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: "../internal/server/dashboard/dist",
    emptyOutDir: true,
    assetsDir: "assets",
  },
  server: {
    proxy: {
      "/ws": { target: backend.replace("http", "ws"), ws: true },
      "/health": backend,
    },
  },
});
