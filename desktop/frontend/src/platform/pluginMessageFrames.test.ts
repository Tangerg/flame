import { expect, it } from "vitest";
import { pluginMessageFrames } from "./pluginMessageFrames";

it.each(["initial", "reply"])("delivers complete %s data across bounded UTF-8 frames", (type) => {
  const data = Array.from({ length: 100 }, (_, index) => ({
    index,
    body: "\u0000\\😀汉字\ud800".repeat(100),
  }));
  const message = { type, data };
  const frames = [...pluginMessageFrames(message, 3072)];
  expect(frames.length).toBeGreaterThan(1);
  let text = "";
  for (const [index, frame] of frames.entries()) {
    expect(new TextEncoder().encode(frame).byteLength).toBeLessThanOrEqual(3072);
    const fragment = JSON.parse(frame) as { index: number; last: boolean; data: string };
    expect(fragment).toMatchObject({ type: "fragment", index, last: index === frames.length - 1 });
    text += fragment.data;
  }
  expect(JSON.parse(text)).toEqual(message);
});

it("captures the logical message before its physical frames are consumed", () => {
  const message = { body: "captured" };
  const frames = pluginMessageFrames(message, 128);
  message.body = "replacement";
  expect(
    JSON.parse([...frames].map((frame) => (JSON.parse(frame) as { data: string }).data).join("")),
  ).toEqual({
    body: "captured",
  });
});

it("refuses capacities that cannot hold a JSON fragment", () => {
  expect(() => pluginMessageFrames({}, 1)).toThrow("insufficient");
  expect(() => pluginMessageFrames(undefined)).toThrow("JSON values");
});
