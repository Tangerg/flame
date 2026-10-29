import { expect, it, vi } from "vitest";
import { LIMITS } from "../../../../plugins/deliverables/contracts/limits";
import {
  downloadAsset,
  encodeChunk,
  sha256,
} from "../../../../plugins/deliverables/component/transfer";
import type { Asset } from "../../../../plugins/deliverables/component/types";

async function source(size = LIMITS.chunkBytes + 11) {
  const bytes = new Uint8Array(size);
  for (let index = 0; index < size; index++) bytes[index] = index % 251;
  const asset: Asset = {
    id: "a".repeat(64),
    filename: "report.docx",
    mime: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    size,
    sha256: await sha256(bytes),
  };
  const download = vi.fn(async ({ offset, length }: { offset: unknown; length: unknown }) => {
    const start = Number(offset);
    const next = Math.min(size, start + Number(length));
    return {
      asset,
      offset: start,
      next,
      done: next === size,
      base64: encodeChunk(bytes.slice(start, next)),
    };
  });
  return { bytes, asset, download };
}

it("downloads bounded contiguous chunks and verifies the exact source hash", async () => {
  const fixture = await source();
  const progress = vi.fn();
  const result = await downloadAsset(
    fixture.download,
    fixture.asset,
    new AbortController().signal,
    progress,
  );
  expect(result).toEqual(fixture.bytes);
  expect(fixture.download.mock.calls.map(([input]) => input)).toEqual([
    { assetId: fixture.asset.id, offset: 0, length: LIMITS.chunkBytes },
    { assetId: fixture.asset.id, offset: LIMITS.chunkBytes, length: 11 },
  ]);
  expect(progress).toHaveBeenLastCalledWith(fixture.bytes.length, fixture.bytes.length);
});

it.each(["identity", "offset", "length", "hash", "completion"])(
  "refuses a %s mismatch before a save can receive bytes",
  async (fault) => {
    const fixture = await source(8);
    const download = async (input: { offset: unknown; length: unknown }) => {
      const value = await fixture.download(input);
      if (fault === "identity") value.asset = { ...value.asset, filename: "different.docx" };
      if (fault === "offset") value.offset = 1;
      if (fault === "length") value.next = 7;
      if (fault === "hash") value.base64 = encodeChunk(new Uint8Array(8));
      if (fault === "completion") value.done = false;
      return value;
    };
    await expect(
      downloadAsset(download, fixture.asset, new AbortController().signal),
    ).rejects.toThrow("deliverables:");
  },
);

it("discards a late chunk after the captured preview or download is retired", async () => {
  const fixture = await source(8);
  const gate = Promise.withResolvers<void>();
  const arrived = Promise.withResolvers<void>();
  const controller = new AbortController();
  const progress = vi.fn();
  const result = downloadAsset(
    async (input) => {
      arrived.resolve();
      await gate.promise;
      return fixture.download(input);
    },
    fixture.asset,
    controller.signal,
    progress,
  );
  const observation = result.then(
    () => undefined,
    (error: unknown) => error,
  );
  await arrived.promise;
  controller.abort();
  gate.resolve();
  expect(await observation).toMatchObject({ name: "AbortError" });
  expect(progress).not.toHaveBeenCalled();
});
