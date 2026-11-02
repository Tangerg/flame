import { expect, it, vi } from "vitest";
import { createNotificationCentre } from "./systemNotifier";
import { createBrowserHost } from "@/platform/browserHost";

it("routes a notification only to the Runtime that originally produced it", async () => {
  let endpoint = "https://one.example";
  let target: string | undefined;
  let clicked: ((value: string) => void) | undefined;
  const centre = createNotificationCentre(
    {
      ...createBrowserHost(),
      async sendNotification(value) {
        target = value.target;
        return true;
      },
      async onNotificationOpened(fn) {
        clicked = fn;
        return () => {};
      },
    },
    () => endpoint,
    () => {},
  );
  const open = vi.fn();
  centre.onOpened(open);
  await centre.send({ id: "id", title: "title", body: "body", target: "session" });
  clicked!(target!);
  endpoint = "https://two.example";
  clicked!(target!);
  expect(open.mock.calls).toEqual([["session"]]);
});
it("reports native listener registration failure to its lifecycle owner", async () => {
  const failure = new Error("binding unavailable");
  const report = vi.fn();
  const centre = createNotificationCentre(
    {
      ...createBrowserHost(),
      async onNotificationOpened() {
        throw failure;
      },
    },
    () => "https://example.test",
    report,
  );
  const stop = centre.onOpened(() => {});
  await Promise.resolve();
  expect(report).toHaveBeenCalledWith(failure);
  stop();
});
