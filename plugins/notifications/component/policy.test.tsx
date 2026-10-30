// @vitest-environment happy-dom
import { cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { announceSettlement } from "./policy";
import { CompletionNotifications } from "./component";
import { writePreference } from "./preferences";
import { playCompletionChime } from "./chime";
import { notificationFixture } from "./testing";

vi.mock("./chime", () => ({ playCompletionChime: vi.fn(async () => {}) }));
beforeEach(() => {
  vi.spyOn(document, "hasFocus").mockReturnValue(false);
});
afterEach(() => {
  cleanup();
  localStorage.removeItem("flame.completion-sound");
  localStorage.removeItem("flame.system-notifications");
  vi.restoreAllMocks();
  vi.mocked(playCompletionChime).mockClear();
});
const finished = {
  sessionId: "captured",
  status: "finished",
  errorMessage: null,
} as const;

it("stays quiet while focused and reads the current preference for each settlement", async () => {
  const { context, channel } = notificationFixture();
  vi.mocked(document.hasFocus).mockReturnValueOnce(true);
  await announceSettlement(context, finished, context.signal);
  expect(channel.requestAuthorization).not.toHaveBeenCalled();
  writePreference("systemNotifications", false);
  await announceSettlement(context, finished, context.signal);
  expect(channel.send).not.toHaveBeenCalled();
  writePreference("systemNotifications", true);
  await announceSettlement(context, finished, context.signal);
  expect(channel.send).toHaveBeenCalledExactlyOnceWith({
    id: "run:captured",
    sessionId: "captured",
    title: "Flame finished",
    body: "The agent finished its turn.",
  });
});

it.each(["denied", "unsupported", "default"] as const)(
  "does not send when the platform reports %s permission",
  async (authorization) => {
    const { context, channel, setAuthorization } = notificationFixture();
    setAuthorization(authorization);
    await announceSettlement(context, finished, context.signal);
    expect(channel.send).not.toHaveBeenCalled();
  },
);

it("keeps sound independent of the system channel preference", async () => {
  const { context, channel } = notificationFixture();
  writePreference("completionSound", true);
  writePreference("systemNotifications", false);
  await announceSettlement(context, finished, context.signal);
  expect(playCompletionChime).toHaveBeenCalledExactlyOnceWith(context.signal);
  expect(channel.requestAuthorization).not.toHaveBeenCalled();
});

it("refuses a late grant after withdrawal and preserves the accepted Session target", async () => {
  const { context, channel, controller } = notificationFixture();
  const permission = Promise.withResolvers<"granted">();
  vi.mocked(channel.requestAuthorization).mockReturnValueOnce(
    permission.promise,
  );
  const announcing = announceSettlement(context, finished, context.signal);
  controller.abort(new Error("source retired"));
  permission.resolve("granted");
  await expect(announcing).rejects.toBe(context.signal.reason);
  expect(channel.send).not.toHaveBeenCalled();
});

it("uses the recorded error and preserves a real channel failure", async () => {
  const { context, channel } = notificationFixture();
  const failure = new Error("platform send failed");
  vi.mocked(channel.send).mockRejectedValueOnce(failure);
  await expect(
    announceSettlement(
      context,
      {
        ...finished,
        status: "error",
        errorMessage: "Provider refused the request",
      },
      context.signal,
    ),
  ).rejects.toBe(failure);
  expect(channel.send).toHaveBeenCalledExactlyOnceWith(
    expect.objectContaining({
      title: "Flame hit an error",
      body: "Provider refused the request",
      sessionId: "captured",
    }),
  );
});

it("keeps one root-fact subscription through locale changes and withdraws it on unmount", async () => {
  const { context, channel, settle, settlements } = notificationFixture();
  const mounted = render(<CompletionNotifications context={context} />);
  expect(settlements.size).toBe(1);
  const subscription = [...settlements][0];
  mounted.rerender(
    <CompletionNotifications context={{ ...context, locale: "zh" }} />,
  );
  expect([...settlements][0]).toBe(subscription);
  await settle({ ...finished, status: "needsInput" });
  expect(channel.send).toHaveBeenCalledWith(
    expect.objectContaining({
      sessionId: "captured",
      title: "Flame 需要你处理",
    }),
  );
  mounted.unmount();
  expect(settlements.size).toBe(0);
});
