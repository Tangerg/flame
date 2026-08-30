import { defineConfig } from "@playwright/test";

const VISUAL_PORT = 4174;

export default defineConfig({
  testDir: "./visual",
  testMatch: "**/*.visual.spec.ts",
  fullyParallel: true,
  forbidOnly: true,
  retries: 0,
  workers: 2,
  reporter: "line",
  outputDir: "../.cache/playwright-results",
  use: {
    baseURL: `http://127.0.0.1:${VISUAL_PORT}`,
    browserName: "chromium",
    colorScheme: "light",
    deviceScaleFactor: 1,
    locale: "en-US",
    reducedMotion: "reduce",
    timezoneId: "UTC",
    viewport: { width: 1120, height: 720 },
  },
  expect: {
    toHaveScreenshot: {
      animations: "disabled",
      caret: "hide",
      scale: "css",
      // Playwright's 0.2 default can treat a whole semantic ink-rung change as
      // "the same" image. Geometry has explicit DOM/CSS assertions and contrast
      // has Axe; the raster layer still needs to catch subtle colour drift.
      threshold: 0.05,
      // …but a per-pixel threshold with no pixel budget fails on text
      // antialiasing, which the renderer does not reproduce bit-for-bit between
      // runs. The composer's pills drifted ~515 px this way while their
      // bounding boxes stayed identical across loads, and a suite that reports
      // that as a regression is one everybody learns to ignore. Two thousandths
      // of the frame absorbs the noise and is still an order of magnitude below
      // a moved element or a changed ink rung.
      maxDiffPixelRatio: 0.002,
    },
  },
  webServer: {
    command: "npm run visual:dev",
    url: `http://127.0.0.1:${VISUAL_PORT}/visual/`,
    reuseExistingServer: false,
    timeout: 120_000,
  },
});
