import assert from "node:assert/strict";

export function assertCarrierIsolation(result) {
  assert.equal(result.error, undefined, "carrier probe failed before observing isolation");
  assert.equal(result.frame.origin, "null", "only the bound opaque-origin frame may bootstrap");
  const observations = result.frame.observations;
  assert.equal(
    observations.origin,
    "forged-origin",
    "the frame must attempt to forge host metadata",
  );
  for (const operation of [
    "parentDOM",
    "parentStorage",
    "storage",
    "cookieCanary",
    "topNavigation",
    "popup",
    "networkRead",
  ]) {
    assert.equal(observations[operation], false, `plugin frame escaped through ${operation}`);
  }
  assert.ok(result.channel.receivedBeforeClose > 0, "publication must work before retirement");
  assert.equal(
    result.channel.retirementOrigin,
    "null",
    "the live frame must attempt a retired send",
  );
  assert.equal(
    result.channel.receivedAfterClose,
    result.channel.receivedBeforeClose,
    "a retired channel must stop publication while its frame remains alive",
  );
  assert.equal(result.navigation.origin, "null", "the navigating frame must witness its attempt");
  assert.equal(
    result.navigation.blockedDirective,
    "frame-src",
    "the host CSP must reject frame navigation",
  );
}
