import { defineConfig } from "@playwright/test";

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
    baseURL: "http://127.0.0.1:4175",
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
      command: "vite --config vite.lab.config.ts",
      url: "http://127.0.0.1:4174/lab/",
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      command: "npm run lab:build && vite preview --config vite.lab.config.ts",
      url: "http://127.0.0.1:4175/lab/",
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
      use: { baseURL: "http://127.0.0.1:4174" },
    },
    {
      name: "performance",
      testMatch: "**/*.perf.spec.ts",
      use: { reducedMotion: "no-preference" },
    },
  ],
});
