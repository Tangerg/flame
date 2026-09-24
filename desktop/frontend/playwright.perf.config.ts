import { defineConfig } from "@playwright/test";
import visual from "./playwright.visual.config";

const PERF_PORT = 4175;

export default defineConfig({
  ...visual,
  testMatch: "**/*.perf.spec.ts",
  workers: 1,
  use: {
    ...visual.use,
    baseURL: `http://127.0.0.1:${PERF_PORT}`,
    reducedMotion: "no-preference",
  },
  webServer: {
    command:
      "vite build --config vite.visual.config.ts && vite preview --config vite.visual.config.ts",
    url: `http://127.0.0.1:${PERF_PORT}/visual/`,
    reuseExistingServer: false,
    timeout: 180_000,
  },
});
