// @vitest-environment happy-dom
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { type ComponentContext } from "@flame/component-sdk";
import { Brand } from "./component";

vi.mock("@flame/brand-assets", () => ({
  mark: "data:image/png;base64,AQID",
  companion: "data:image/png;base64,BAUG",
  plush: "data:image/png;base64,BwgJ",
  FLAME_ORANGE: "#fb6e30",
}));
afterEach(cleanup);

function fixture() {
  const controller = new AbortController();
  const saveFile = vi
    .fn<ComponentContext["saveFile"]>()
    .mockResolvedValue("saved");
  const context = {
    locale: "en",
    signal: controller.signal,
    saveFile,
  } as unknown as ComponentContext;
  return { controller, saveFile, context };
}

it("saves the selected embedded bytes through the captured host and preserves failure", async () => {
  const f = fixture();
  f.saveFile.mockRejectedValueOnce(new Error("destination unavailable"));
  render(<Brand context={f.context} />);
  fireEvent.click(
    screen.getByRole("button", { name: "Download Primary logo" }),
  );
  await screen.findByRole("alert");
  expect(f.saveFile).toHaveBeenCalledExactlyOnceWith({
    filename: "flame-mark.png",
    mime: "image/png",
    bytes: new Uint8Array([1, 2, 3]),
  });
  expect(screen.getByRole("alert").textContent).toContain(
    "destination unavailable",
  );
  fireEvent.click(
    screen.getByRole("button", { name: "Download 3D companion" }),
  );
  await waitFor(() => expect(f.saveFile).toHaveBeenCalledTimes(2));
  expect(f.saveFile.mock.calls[1]![0].bytes).toEqual(new Uint8Array([4, 5, 6]));
});

it("joins an accepted save through locale changes and refuses new work after retirement", async () => {
  const accepted = Promise.withResolvers<"saved">();
  const f = fixture();
  f.saveFile.mockReturnValue(accepted.promise);
  const view = render(<Brand context={f.context} />);
  fireEvent.click(
    screen.getByRole("button", { name: "Download Primary logo" }),
  );
  view.rerender(<Brand context={{ ...f.context, locale: "zh" }} />);
  for (const button of screen.getAllByRole("button"))
    if (button.textContent?.includes("PNG")) fireEvent.click(button);
  expect(f.saveFile).toHaveBeenCalledOnce();
  await act(async () => accepted.resolve("saved"));
  f.controller.abort();
  for (const button of screen.getAllByRole("button")) fireEvent.click(button);
  expect(f.saveFile).toHaveBeenCalledOnce();
});
