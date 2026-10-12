import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { runInNewContext } from "node:vm";
import { test } from "node:test";
import { pluginMessageFrames } from "../src/platform/pluginMessageFrames.ts";

const html = await readFile(new URL("../public/plugin-carrier.html", import.meta.url), "utf8");
const script = html.match(/<script>([\s\S]*?)<\/script>/)[1];
function receiver() {
  const messages = [];
  const resources = [];
  const frames = [];
  const window = {
    webkit: {
      messageHandlers: { carrier: { postMessage: (text) => messages.push(JSON.parse(text)) } },
    },
  };
  runInNewContext(script, {
    window,
    addEventListener() {},
    document: {
      createElement: () => ({ sandbox: { add() {} }, setAttribute() {} }),
      body: { append: (frame) => frames.push(frame) },
    },
    URL: {
      createObjectURL: (blob) => {
        resources.push(blob);
        return "blob:fixture";
      },
    },
    Blob,
    console,
  });
  return { messages, frames, resources, receive: window.flameCarrierReceive };
}

test("the shipped receiver reassembles a large boot before allocating its guest", async () => {
  const page = receiver();
  const message = { type: "boot", html: "汉😀\u0000\\".repeat(1 << 20), initial: { data: [] } };
  const frames = [...pluginMessageFrames(message, 4 << 20)];
  assert.ok(frames.length > 1);
  for (const frame of frames.slice(0, -1)) {
    assert.ok(Buffer.byteLength(frame) <= 4 << 20);
    page.receive(JSON.parse(frame));
    assert.equal(page.frames.length, 0);
  }
  page.receive(JSON.parse(frames.at(-1)));
  assert.equal(page.frames.length, 1);
  assert.ok((await page.resources[0].text()).endsWith(message.html));
  assert.deepEqual(page.messages, []);
});

test("malformed or out-of-order fragments permanently refuse that transport", () => {
  for (const fragment of [
    { type: "fragment", index: 1, last: true, data: "{}" },
    { type: "fragment", index: 0, last: false, data: "{}", extra: true },
    { type: "fragment", index: 0, last: true, data: "{" },
  ]) {
    const page = receiver();
    page.receive(fragment);
    page.receive(JSON.parse([...pluginMessageFrames({ type: "boot", html: "valid" })][0]));
    assert.equal(page.frames.length, 0);
    assert.equal(page.messages.length, 1);
    assert.equal(page.messages[0].type, "failure");
  }
});
