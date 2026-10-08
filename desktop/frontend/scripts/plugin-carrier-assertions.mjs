import assert from "node:assert/strict";

export function assertCarrierIsolation(result) {
  assert.equal(result.error, undefined, "carrier probe failed before observing isolation");
  assert.equal(result.frames.length, 1, "only the bound frame may publish its bootstrap");
  const frame = result.frames[0];
  assert.equal(frame.origin, "null", "plugin content must have an opaque origin");
  for (const operation of [
    "parentDOM",
    "parentStorage",
    "storage",
    "cookieCanary",
    "topNavigation",
    "popup",
    "networkRead",
  ]) {
    assert.equal(frame[operation], false, `plugin frame escaped through ${operation}`);
  }
  assert.equal(result.channel, true, "the bound frame must complete its channel exchange");
  assert.ok(result.messagesBeforeRetirement > 0, "publication must work before retirement");
  assert.equal(result.messagesAfterRetirement, 0, "a retired channel must stop publication");
  assert.equal(result.navigationOrigin, "null", "the navigating frame must witness its attempt");
  assert.equal(result.navigationBlocked, "frame-src", "the host CSP must reject frame navigation");
}
