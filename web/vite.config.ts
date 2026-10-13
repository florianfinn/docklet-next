import { fileURLToPath, URL } from "node:url";

import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

import { productionBoundary } from "./build/production-boundary.ts";

const port = Number(process.env.PORT ?? 5180);

export default defineConfig(({ command }) => ({
  plugins: [react(), tailwindcss(), ...(command === "build" ? [productionBoundary()] : [])],
  resolve: {
    alias: [
      { find: "@/runtime", replacement: fileURLToPath(new URL(command === "serve" ? "./src/runtime.dev.ts" : "./src/runtime.ts", import.meta.url)) },
      { find: "@", replacement: fileURLToPath(new URL("./src", import.meta.url)) },
    ],
  },
  server: { host: "0.0.0.0", port, strictPort: true },
  preview: { host: "0.0.0.0", port, strictPort: true },
}));
