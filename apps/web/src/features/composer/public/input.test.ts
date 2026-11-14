import { afterEach, expect, it, vi } from "vitest";
import { fileToInputImage } from "./input";

afterEach(() => vi.restoreAllMocks());

it("settles an aborted native image read as a failure", async () => {
  vi.spyOn(FileReader.prototype, "readAsDataURL").mockImplementation(function (this: FileReader) {
    this.dispatchEvent(new ProgressEvent("abort"));
  });
  await expect(
    fileToInputImage(new File(["image"], "cancelled.png", { type: "image/png" })),
  ).rejects.toMatchObject({ name: "AbortError" });
});

it("aborts the actual FileReader when its owner cancels and removes the cancellation listener", async () => {
  const controller = new AbortController();
  const abort = vi.spyOn(FileReader.prototype, "abort");
  const remove = vi.spyOn(controller.signal, "removeEventListener");
  const reading = fileToInputImage(
    new File(["image"], "cancelled.png", { type: "image/png" }),
    controller.signal,
  );
  controller.abort();
  await expect(reading).rejects.toMatchObject({ name: "AbortError" });
  expect(abort).toHaveBeenCalledOnce();
  expect(remove).toHaveBeenCalledWith("abort", expect.any(Function));
});

it("does not allocate a reader for an already cancelled selection", async () => {
  const controller = new AbortController();
  controller.abort();
  const read = vi.spyOn(FileReader.prototype, "readAsDataURL");
  await expect(
    fileToInputImage(new File([""], "cancelled.png"), controller.signal),
  ).rejects.toMatchObject({ name: "AbortError" });
  expect(read).not.toHaveBeenCalled();
});
