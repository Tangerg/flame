import { defineConfig } from "@playwright/test";

const BROWSER_PORT = 4174;

export default defineConfig({
  testDir: "./visual",
  testMatch: "**/*.browser.spec.ts",
  fullyParallel: false,
  forbidOnly: true,
  retries: 0,
  workers: 2,
  reporter: "line",
  outputDir: "../.cache/playwright-results",
  use: {
    baseURL: `http://127.0.0.1:${BROWSER_PORT}`,
    browserName: "chromium",
    colorScheme: "light",
    deviceScaleFactor: 1,
    locale: "en-US",
    reducedMotion: "reduce",
    timezoneId: "UTC",
    viewport: { width: 1120, height: 720 },
  },
  webServer: {
    command: "npm run browser:dev",
    url: `http://127.0.0.1:${BROWSER_PORT}/visual/`,
    reuseExistingServer: false,
    timeout: 120_000,
  },
});
