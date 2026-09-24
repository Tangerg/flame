import { describe, expect, it } from "vitest";
import { composerActionLayout } from "./composerActionLayout";

describe("composerActionLayout", () => {
  it("offers send when nothing is running", () => {
    expect(composerActionLayout({ running: false, hasInput: true })).toEqual({
      submit: "send",
      stop: null,
    });
  });

  it("offers steer during a run while stop stays in its own place", () => {
    expect(composerActionLayout({ running: true, hasInput: true })).toEqual({
      submit: "steer",
      stop: "quiet",
    });
  });

  it("emphasizes stop when there is nothing to steer with", () => {
    expect(composerActionLayout({ running: true, hasInput: false })).toEqual({
      submit: null,
      stop: "emphasized",
    });
  });

  it("always offers stop while a run is in flight", () => {
    for (const hasInput of [true, false]) {
      expect(composerActionLayout({ running: true, hasInput }).stop).not.toBeNull();
    }
  });
});
