import { afterEach, expect, it } from "vitest";
import { createTheme, type Theme } from "@/features/theme/adapters/theme";
import { createBrowserFontAvailability } from "@/features/theme/adapters/browserFontAvailability";

const owners: Theme[] = [];
function construct() {
  const owner = createTheme(document, window);
  owners.push(owner);
  return owner;
}
afterEach(() => {
  for (const owner of owners.splice(0)) owner.dispose();
  document.documentElement.removeAttribute("style");
});

it("retires the previous document painter before its successor can publish", () => {
  const previous = construct();
  const next = construct();
  const releasePrevious = previous.start();
  previous.store.getState().setRadiusScale(1.25);
  expect(document.documentElement.style.getPropertyValue("--radius-scale")).toBe("1.25");

  next.start();
  previous.store.getState().setRadiusScale(2);
  releasePrevious();
  previous.dispose();
  expect(next.store.getState().radiusScale).toBe(1);
  expect(document.documentElement.style.getPropertyValue("--radius-scale")).toBe("1");

  next.store.getState().setRadiusScale(1.5);
  expect(document.documentElement.style.getPropertyValue("--radius-scale")).toBe("1.5");
  next.dispose();
  next.store.getState().setRadiusScale(3);
  expect(document.documentElement.style.getPropertyValue("--radius-scale")).toBe("1.5");
  expect(() => next.start()).toThrow("Theme is disposed");
});

it("does not let a prior plugin installation release its restarted painter", () => {
  const owner = construct();
  const releasePrevious = owner.start();
  const releaseCurrent = owner.start();
  releasePrevious();
  owner.store.getState().setFontSize(16);
  expect(owner.access.appearance.read().uiFontSize).toBe(16);
  expect(document.documentElement.style.getPropertyValue("--fs-ui-md")).toBe("16px");

  releaseCurrent();
  owner.store.getState().setFontSize(18);
  expect(owner.access.appearance.read().uiFontSize).toBe(16);
});

it("keeps font measurements in the detector created for that renderer", () => {
  const original = document.body.append.bind(document.body);
  let measurements = 0;
  document.body.append = (...nodes) => {
    measurements += 1;
    original(...nodes);
  };
  try {
    const previous = createBrowserFontAvailability(document);
    previous.isAvailable("Inter");
    const firstRead = measurements;
    expect(firstRead).toBeGreaterThan(0);
    previous.isAvailable("Inter");
    expect(measurements).toBe(firstRead);
    createBrowserFontAvailability(document).isAvailable("Inter");
    expect(measurements).toBe(firstRead * 2);
  } finally {
    document.body.append = original;
  }
});
