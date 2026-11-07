import { afterEach, describe, expect, it, vi } from "vitest";
import { installFixtureClock } from "./fixtureClock";

const NativeDate = globalThis.Date;
const nativeNow = NativeDate.now;
const START = NativeDate.UTC(2001, 1, 3, 9, 30, 15, 250);
const cleanups: Array<() => void> = [];

function install(now: () => number) {
  const restore = installFixtureClock(now);
  cleanups.push(restore);
  return restore;
}

afterEach(() => {
  for (const restore of cleanups.splice(0).reverse()) restore();
});

describe("the Lab fixture clock", () => {
  it("has no global effect on import", async () => {
    vi.resetModules();
    await import("./fixtureClock");
    expect(globalThis.Date).toBe(NativeDate);
    expect(Date.now).toBe(nativeNow);
  });

  it("serves every argument-free wall-clock read from one advancing timestamp", () => {
    let current = START;
    install(() => current);

    expect(Date.now()).toBe(START);
    expect(new Date().getTime()).toBe(START);
    expect(new Date().toISOString()).toBe("2001-02-03T09:30:15.250Z");
    expect(Date()).toBe(new NativeDate(START).toString());

    current = START + 61_000;
    expect(Date.now()).toBe(START + 61_000);
    expect(new Date().getTime()).toBe(START + 61_000);
    expect(Date.parse(Date())).toBe(Math.floor((START + 61_000) / 1000) * 1000);
  });

  it("keeps the native function-call contract of returning a string and ignoring arguments", () => {
    install(() => START);
    const called = (Date as unknown as (...args: unknown[]) => unknown)(0, "2001-01-01");
    expect(called).toBe(new NativeDate(START).toString());
  });

  it("preserves explicit constructor arguments, parsing and UTC", () => {
    install(() => START);
    const copied = new NativeDate(0);

    expect(new Date(undefined as unknown as number).getTime()).toBeNaN();
    expect(new Date(0).getTime()).toBe(0);
    expect(new Date("2020-01-02T03:04:05.000Z").getTime()).toBe(
      NativeDate.UTC(2020, 0, 2, 3, 4, 5),
    );
    expect(new Date(2020, 0, 2, 3, 4, 5).getTime()).toBe(
      new NativeDate(2020, 0, 2, 3, 4, 5).getTime(),
    );
    expect(new Date(copied).getTime()).toBe(0);
    expect(Date.parse("2020-01-02T03:04:05.000Z")).toBe(NativeDate.UTC(2020, 0, 2, 3, 4, 5));
    expect(Date.UTC(2020, 0, 2)).toBe(NativeDate.UTC(2020, 0, 2));
  });

  it("produces native Date instances and formatting", () => {
    install(() => START);
    const date = new Date();
    const native = new NativeDate(START);
    const format = new Intl.DateTimeFormat("en-US", {
      timeZone: "UTC",
      dateStyle: "full",
      timeStyle: "long",
    });

    expect(Object.getPrototypeOf(date)).toBe(NativeDate.prototype);
    expect(Date.prototype).toBe(NativeDate.prototype);
    expect(date).toBeInstanceOf(Date);
    expect(native).toBeInstanceOf(Date);
    expect(Object.prototype.toString.call(date)).toBe("[object Date]");
    expect(format.format(date)).toBe(format.format(native));
    expect(date.toLocaleString("zh-CN", { timeZone: "UTC" })).toBe(
      native.toLocaleString("zh-CN", { timeZone: "UTC" }),
    );
  });

  it("leaves performance, timers and animation frames on their real clocks", async () => {
    const performanceNow = performance.now;
    const before = performance.now();
    const animationFrame = globalThis.requestAnimationFrame;
    install(() => START);

    expect(performance.now).toBe(performanceNow);
    expect(globalThis.requestAnimationFrame).toBe(animationFrame);
    await expect(new Promise((resolve) => setTimeout(() => resolve("fired"), 0))).resolves.toBe(
      "fired",
    );
    expect(performance.now()).toBeGreaterThanOrEqual(before);
  });

  it("restores the captured Date without touching the native clock", () => {
    const restore = install(() => START);
    restore();

    expect(globalThis.Date).toBe(NativeDate);
    expect(Date.now).toBe(nativeNow);
    expect(Date.now()).not.toBe(START);
    expect(new Date().getTime()).not.toBe(START);
  });

  it("does not overwrite a successor installation on teardown", () => {
    const restoreFirst = install(() => START);
    restoreFirst();
    const restoreSecond = install(() => START + 1_000);

    restoreFirst();
    expect(Date.now()).toBe(START + 1_000);
    expect(new Date().getTime()).toBe(START + 1_000);

    restoreSecond();
    expect(globalThis.Date).toBe(NativeDate);
    expect(Date.now).toBe(nativeNow);
  });
});
