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
