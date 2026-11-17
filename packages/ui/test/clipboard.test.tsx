import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  ClipboardReceiptProvider,
  useCopy,
  type CopyReceipt,
} from "../dist/index.js";
afterEach(cleanup);
function Copy({
  text,
  write,
}: {
  text: string;
  write(text: string): Promise<boolean>;
}) {
  const copy = useCopy(text, text, write);
  return <button onClick={() => void copy()}>Copy</button>;
}
it("publishes each captured operation after its source unmounts", async () => {
  const first = Promise.withResolvers<boolean>();
  const second = Promise.withResolvers<boolean>();
  const write = vi
    .fn<(text: string) => Promise<boolean>>()
    .mockReturnValueOnce(first.promise)
    .mockReturnValueOnce(second.promise);
  const receipts: CopyReceipt[] = [];
  const onReceipt = (receipt: CopyReceipt) => receipts.push(receipt);
  const view = render(
    <ClipboardReceiptProvider onReceipt={onReceipt}>
      <Copy text="original" write={write} />
    </ClipboardReceiptProvider>,
  );
  fireEvent.click(screen.getByRole("button"));
  view.rerender(
    <ClipboardReceiptProvider onReceipt={onReceipt}>
      <Copy text="replacement" write={write} />
    </ClipboardReceiptProvider>,
  );
  fireEvent.click(screen.getByRole("button"));
  view.unmount();
  await act(async () => {
    second.resolve(false);
    await second.promise;
  });
  await act(async () => {
    first.resolve(true);
    await first.promise;
  });
  expect(write.mock.calls).toEqual([["original"], ["replacement"]]);
  expect(receipts.map(({ label, status }) => ({ label, status }))).toEqual([
    { label: "replacement", status: "failed" },
    { label: "original", status: "copied" },
  ]);
  expect(new Set(receipts.map(({ id }) => id)).size).toBe(2);
  expect(receipts.every(Object.isFrozen)).toBe(true);
});
