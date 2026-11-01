// @vitest-environment happy-dom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { ComponentContext, CapturedMessage } from "@flame/component-sdk";
import { RpcTransportError } from "@flame/runtime-client";
import { RateResponse } from "./component";

afterEach(cleanup);
const Rate = RateResponse.create({
  runtimeIdentity: {},
  component: {
    installationId: "owner",
    componentId: "rate-response",
    digest: "original",
  },
  signal: new AbortController().signal,
});
const message = {
  sessionId: "ses_original",
  runId: "run_original",
  itemId: "item_original",
  role: "assistant",
};
const binding = {
  installationId: "owner",
  digest: "original",
  contribution: "feedback",
  contract: "test",
};
function context(create: ReturnType<typeof vi.fn>, target?: CapturedMessage) {
  return {
    message: target,
    signal: new AbortController().signal,
    locale: "en",
    binding,
    runtime: { feedback: { create } },
    track: (run: () => unknown) => run(),
  } as unknown as ComponentContext;
}
it("submits the displayed item to the captured producer and marks it only after acceptance", async () => {
  let accept = () => {};
  const promise = new Promise<void>((resolve) => {
    accept = resolve;
  });
  const create = vi.fn(() => promise);
  const view = render(<Rate context={context(create, message)} />);
  const good = screen.getByRole("button", { name: "Good response" });
  fireEvent.click(good);
  fireEvent.click(good);
  expect(create).toHaveBeenCalledExactlyOnceWith({
    source:{installationId:binding.installationId,release:binding.digest,contribution:binding.contribution},
    sessionId: message.sessionId,
    runId: message.runId,
    itemId: message.itemId,
    rating: "positive",
  });
  expect(good.getAttribute("aria-pressed")).toBe("false");
  accept();
  await waitFor(() => expect(good.getAttribute("aria-pressed")).toBe("true"));
  view.unmount();
  render(<Rate context={context(create, message)} />);
  expect(
    screen
      .getByRole("button", { name: "Good response" })
      .getAttribute("aria-pressed"),
  ).toBe("true");
});
it("refuses user messages and missing targets", () => {
  const c = context(vi.fn());
  const { rerender } = render(
    <Rate context={{ ...c, message: { ...message, role: "user" } }} />,
  );
  expect(screen.queryByRole("button")).toBeNull();
  rerender(<Rate context={c} />);
  expect(screen.queryByRole("button")).toBeNull();
});

it("recovers an unconfirmed vote with its original command after remount", async () => {
  const retry = vi.fn(() => Promise.resolve());
  const create = vi.fn(() =>
    Object.assign(Promise.reject(new RpcTransportError("reply lost")), {
      retry,
      idempotencyKey: "first",
    }),
  );
  const props = {
    context: context(create, { ...message, itemId: "item_lost" }),
  };
  const view = render(<Rate {...props} />);
  fireEvent.click(screen.getByRole("button", { name: "Poor response" }));
  await screen.findByRole("alert");
  view.unmount();
  render(<Rate {...props} />);
  expect(
    (screen.getByRole("button", { name: "Good response" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "Poor response" }));
  await waitFor(() =>
    expect(
      screen
        .getByRole("button", { name: "Poor response" })
        .getAttribute("aria-pressed"),
    ).toBe("true"),
  );
  expect(retry).toHaveBeenCalledTimes(1);
  expect(create).toHaveBeenCalledTimes(1);
});
