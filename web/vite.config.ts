import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

declare const process: { env: Record<string, string | undefined> };
type Headers = { setHeader(name: string, value: string): void };
type Incoming = { headers: Record<string, string | undefined> };

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: { outDir: "dist", emptyOutDir: true },
  server: {
    allowedHosts: true,
    proxy: {
      "/api": {
        ws: true,
        target: process.env.DORY_DEV_API ?? "http://localhost:8080",
        // Keep the browser's Host so the backend's same-origin check accepts any dev hostname.
        configure: (proxy) => {
          (proxy as unknown as { on(e: string, f: (req: Headers, inc: Incoming) => void): void })
            .on("proxyReq", (req, inc) => req.setHeader("host", inc.headers.host ?? ""));
        },
      },
    },
  },
});
