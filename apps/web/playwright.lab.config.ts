import { defineConfig } from "@playwright/test";
import { LAB_DEVELOPMENT_SERVER, LAB_PREVIEW_SERVER } from "./labServers.ts";

export default defineConfig({
  testDir: "./lab",
  fullyParallel: false,
  forbidOnly: true,
  retries: 0,
  workers: 1,
  timeout: 600_000,
  outputDir: "../../out/web/lab-results",
  reporter: [["line"], ["html", { outputFolder: "../../out/web/lab-report", open: "never" }]],
  use: {
    baseURL: LAB_PREVIEW_SERVER.origin,
    browserName: "chromium",
    colorScheme: "light",
    deviceScaleFactor: 1,
    locale: "en-US",
    reducedMotion: "reduce",
    timezoneId: "UTC",
    viewport: { width: 1120, height: 720 },
  },
  webServer: [
    {
      name: LAB_DEVELOPMENT_SERVER.name,
      command: "vite --config vite.lab.config.ts",
      url: `${LAB_DEVELOPMENT_SERVER.origin}/lab/`,
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      name: LAB_PREVIEW_SERVER.name,
      command: "npm run lab:build && vite preview --config vite.lab.config.ts",
      url: `${LAB_PREVIEW_SERVER.origin}/lab/`,
      reuseExistingServer: false,
      timeout: 180_000,
    },
  ],
  projects: [
    { name: "light-en", testMatch: "**/*.lab.spec.ts" },
    { name: "dark-zh", testMatch: "**/*.lab.spec.ts", use: { colorScheme: "dark" } },
    {
      name: "interactions",
      testMatch: "**/*.browser.spec.ts",
      timeout: 30_000,
      use: { baseURL: LAB_DEVELOPMENT_SERVER.origin },
    },
    {
      name: "security",
      testMatch: "**/*.security.spec.ts",
      timeout: 30_000,
    },
    {
      name: "performance",
      testMatch: "**/*.perf.spec.ts",
      use: { reducedMotion: "no-preference" },
    },
  ],
});
