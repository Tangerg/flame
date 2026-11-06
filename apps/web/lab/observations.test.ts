import { afterEach, describe, expect, it, vi } from "vitest";
import {
  installPreviewObservations,
  OBSERVATION_CHANNEL,
  OBSERVATION_LIMITS,
  OBSERVATION_NAMES,
  OBSERVATION_VERSION,
  ObservationLog,
  parseObservationMessage,
  type ObservationMessage,
} from "./observations";

function message(overrides: Partial<ObservationMessage> = {}): ObservationMessage {
  return {
    channel: OBSERVATION_CHANNEL,
    version: OBSERVATION_VERSION,
    kind: "event",
    instanceId: "preview-1",
    name: "selection.changed",
    detail: "agent",
    ...overrides,
  };
}

const cleanups: Array<() => void> = [];

afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup();
  document.body.replaceChildren();
  vi.restoreAllMocks();
});

function install() {
  const send = vi.fn();
  vi.spyOn(window, "parent", "get").mockReturnValue({ postMessage: send } as unknown as Window);
  const observations = installPreviewObservations(window, document, { instanceId: "preview-1" });
  cleanups.push(observations.dispose);
  return { observations, send };
}

describe("preview observation boundary", () => {
  it("accepts a complete message and returns an immutable projection", () => {
    const input = message();
    const result = parseObservationMessage(input);
    expect(result).toEqual(input);
    expect(result).not.toBe(input);
    expect(Object.isFrozen(result)).toBe(true);
  });

  it.each([
    undefined,
    null,
    [],
    "observation",
    { ...message(), command: "write" },
    { ...message(), channel: "another-channel" },
    { ...message(), version: OBSERVATION_VERSION + 1 },
    { ...message(), kind: "command" },
    { ...message(), instanceId: "../preview" },
    { ...message(), instanceId: "x".repeat(OBSERVATION_LIMITS.instanceIdLength + 1) },
    { ...message(), name: "" },
    { ...message(), name: " untrimmed " },
    { ...message(), name: "first\nsecond" },
    { ...message(), name: "x".repeat(OBSERVATION_LIMITS.nameLength + 1) },
    { ...message(), detail: "x".repeat(OBSERVATION_LIMITS.detailLength + 1) },
    { ...message(), detail: { secret: "must not be serialized" } },
  ])("rejects malformed, oversized or non-observation data %#", (input) => {
    expect(parseObservationMessage(input)).toBeNull();
  });

  it("does not install side effects for an invalid instance", () => {
    const listen = vi.spyOn(document, "addEventListener");
    expect(() => installPreviewObservations(window, document, { instanceId: "<secret>" })).toThrow(
      "invalid preview observation instance",
    );
    expect(listen).not.toHaveBeenCalled();
  });
});

describe("observation log", () => {
  it("retains a stable snapshot, sequences accepted entries and bounds retained history", () => {
    const log = new ObservationLog();
    const initial = log.snapshot();
    expect(log.snapshot()).toBe(initial);
    const listener = vi.fn();
    const unsubscribe = log.subscribe(listener);
    for (let index = 0; index < OBSERVATION_LIMITS.entries + 3; index++) {
      expect(log.append(message(), index)).toBe(true);
    }
    const entries = log.snapshot();
    expect(entries).toHaveLength(OBSERVATION_LIMITS.entries);
    expect(entries[0]).toMatchObject({ sequence: 4, receivedAt: 3 });
    expect(entries.at(-1)).toMatchObject({
      sequence: OBSERVATION_LIMITS.entries + 3,
      receivedAt: OBSERVATION_LIMITS.entries + 2,
    });
    expect(initial).toEqual([]);
    expect(log.snapshot()).toBe(entries);
    expect(Object.isFrozen(entries)).toBe(true);
    expect(Object.isFrozen(entries[0])).toBe(true);
    expect(listener).toHaveBeenCalledTimes(OBSERVATION_LIMITS.entries + 3);
    unsubscribe();
    log.clear();
    expect(listener).toHaveBeenCalledTimes(OBSERVATION_LIMITS.entries + 3);
    expect(log.snapshot()).toEqual([]);
  });

  it("rejects invalid data and receive times without replacing or notifying a snapshot", () => {
    const log = new ObservationLog();
    const listener = vi.fn();
    log.subscribe(listener);
    const before = log.snapshot();
    for (const time of [NaN, Infinity, -1]) expect(log.append(message(), time)).toBe(false);
    expect(log.append({ ...message(), kind: "execute" }, 1)).toBe(false);
    log.clear();
    expect(log.snapshot()).toBe(before);
    expect(listener).not.toHaveBeenCalled();
    log.append(message(), 123);
    log.clear();
    log.append(message(), 456);
    expect(log.snapshot()[0]).toMatchObject({ sequence: 2, receivedAt: 456 });
  });
});

describe("read-only preview observation bridge", () => {
  it("announces readiness once to its own origin without altering clocks or networking", () => {
    const original = { fetch: window.fetch, timeout: window.setTimeout, now: Date.now };
    const { observations, send } = install();
    observations.ready();
    observations.ready();
    expect(document.documentElement.dataset.labReady).toBe("true");
    expect(send).toHaveBeenCalledExactlyOnceWith(
      message({ kind: "ready", name: OBSERVATION_NAMES.ready, detail: "" }),
      window.location.origin,
    );
    expect(window.fetch).toBe(original.fetch);
    expect(window.setTimeout).toBe(original.timeout);
    expect(Date.now).toBe(original.now);
  });

  it("preserves explicit fixture values while bounding text and redacting sensitive names", () => {
    const { observations, send } = install();
    observations.observe("selection.changed", "agent");
    observations.observe("checked.changed", true);
    observations.observe("amount.changed", 42);
    observations.observe("selection.cleared", null);
    observations.observe("text.example", "x".repeat(OBSERVATION_LIMITS.detailLength + 1));
    observations.observe("secret.changed", "must-not-leak");
    observations.observe("invalid.number", Infinity);
    expect(send.mock.calls.map(([entry]) => entry.detail)).toEqual([
      "agent",
      "true",
      "42",
      "null",
      `${"x".repeat(OBSERVATION_LIMITS.detailLength - 1)}…`,
      "[redacted]",
    ]);
    for (const [entry] of send.mock.calls) expect(parseObservationMessage(entry)).not.toBeNull();
  });

  it("observes a nested button click without cancelling native interaction", () => {
    const { send } = install();
    const button = document.createElement("button");
    button.setAttribute("aria-label", "Run scenario");
    const icon = document.createElement("span");
    button.append(icon);
    document.body.append(button);
    const action = vi.fn();
    button.addEventListener("click", action);
    const event = new MouseEvent("click", { bubbles: true, cancelable: true });
    expect(icon.dispatchEvent(event)).toBe(true);
    expect(action).toHaveBeenCalledOnce();
    expect(event.defaultPrevented).toBe(false);
    expect(send).toHaveBeenCalledExactlyOnceWith(
      message({ name: "click: Run scenario", detail: "button" }),
      window.location.origin,
    );
  });

  it("observes input lengths and checked state without exposing text drafts or passwords", () => {
    const { send } = install();
    const text = document.createElement("input");
    text.setAttribute("aria-label", "Title");
    text.value = "private draft";
    const password = document.createElement("input");
    password.type = "password";
    password.value = "a-secret-password";
    const token = document.createElement("input");
    token.name = "api_token";
    token.value = "another-secret";
    const textarea = document.createElement("textarea");
    textarea.name = "description";
    textarea.value = "another private draft";
    const checkbox = document.createElement("input");
    checkbox.type = "checkbox";
    checkbox.checked = true;
    document.body.append(text, password, token, textarea, checkbox);
    for (const control of [text, password, token, textarea, checkbox]) {
      control.dispatchEvent(new Event("change", { bubbles: true }));
    }
    expect(send.mock.calls.map(([entry]) => ({ name: entry.name, detail: entry.detail }))).toEqual([
      { name: "change: Title", detail: `text length: ${text.value.length}` },
      { name: "change: input [sensitive]", detail: "[redacted]" },
      { name: "change: input [sensitive]", detail: "[redacted]" },
      { name: "change: description", detail: `text length: ${textarea.value.length}` },
      { name: "change: input", detail: "checked: true" },
    ]);
    const serialized = JSON.stringify(send.mock.calls);
    for (const value of [text.value, password.value, token.value, textarea.value]) {
      expect(serialized).not.toContain(value);
    }
  });

  it("reports render errors without exporting a stack trace", () => {
    const { observations, send } = install();
    observations.observe(OBSERVATION_NAMES.reactError, "fixture failed to render");
    window.dispatchEvent(new ErrorEvent("error", { message: "component error" }));
    const rejection = new Event("unhandledrejection");
    Object.defineProperty(rejection, "reason", { value: new Error("fixture request rejected") });
    window.dispatchEvent(rejection);
    expect(
      send.mock.calls.map(([entry]) => ({
        kind: entry.kind,
        name: entry.name,
        detail: entry.detail,
      })),
    ).toEqual([
      { kind: "error", name: OBSERVATION_NAMES.reactError, detail: "fixture failed to render" },
      { kind: "error", name: OBSERVATION_NAMES.windowError, detail: "component error" },
      {
        kind: "error",
        name: OBSERVATION_NAMES.unhandledRejection,
        detail: "fixture request rejected",
      },
    ]);
    expect(send.mock.calls.every(([entry]) => !Object.hasOwn(entry, "stack"))).toBe(true);
  });

  it("does not receive commands and retires all observation paths on cleanup", () => {
    const listen = vi.spyOn(window, "addEventListener");
    const { observations, send } = install();
    expect(listen.mock.calls.some(([type]) => type === "message")).toBe(false);
    const button = document.createElement("button");
    button.textContent = "Action";
    document.body.append(button);
    observations.dispose();
    observations.dispose();
    button.click();
    button.dispatchEvent(new Event("change", { bubbles: true }));
    window.dispatchEvent(new ErrorEvent("error", { message: "retired" }));
    observations.observe("retired", true);
    observations.ready();
    expect(send).not.toHaveBeenCalled();
  });

  it("allows replacement after page retirement without duplicate listeners", () => {
    const first = install();
    window.dispatchEvent(new Event("pagehide"));
    const second = install();
    const button = document.createElement("button");
    button.textContent = "Action";
    document.body.append(button);
    button.click();
    expect(first.send).not.toHaveBeenCalled();
    expect(second.send).toHaveBeenCalledOnce();
  });

  it("remains silent outside a preview iframe", () => {
    const send = vi.spyOn(window, "postMessage");
    const observations = installPreviewObservations(window, document, { instanceId: "standalone" });
    cleanups.push(observations.dispose);
    observations.ready();
    observations.observe("selection.changed", "agent");
    expect(send).not.toHaveBeenCalled();
  });

  it("never qualifies an errored or retired standalone document as ready", () => {
    const { observations } = install();
    observations.observe(OBSERVATION_NAMES.reactError, "failed before commit");
    observations.ready();
    expect(document.documentElement.hasAttribute("data-lab-ready")).toBe(false);
    observations.dispose();

    const replacement = install().observations;
    replacement.ready();
    expect(document.documentElement.dataset.labReady).toBe("true");
    window.dispatchEvent(new ErrorEvent("error", { message: "failed after commit" }));
    expect(document.documentElement.hasAttribute("data-lab-ready")).toBe(false);
    replacement.dispose();
    replacement.ready();
    expect(document.documentElement.hasAttribute("data-lab-ready")).toBe(false);
  });
});
