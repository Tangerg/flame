import assert from "node:assert/strict";

export function assertCarrierIsolation(result, peerPackets, enforcement) {
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
  ]) {
    assert.equal(observations[operation], false, `plugin frame escaped through ${operation}`);
  }
  assert.equal(observations.network.read, false, "plugin frame escaped through network");
  assert.equal(
    observations.network.blockedDirective,
    "connect-src",
    "the child CSP must reject its network request",
  );
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
  assert.equal(peerPackets, 0, "a plugin WebRTC packet reached the trusted UDP witness");
  switch (enforcement.kind) {
    case "connection-allowlist":
      assert.equal(observations.peer.available, true, "the WebRTC positive control must exist");
      assert.equal(observations.peer.attempted, true, "the frame must negotiate a data channel");
      assert.equal(
        observations.peer.policyEnforced,
        true,
        "the executing carrier must enforce its network allowlist",
      );
      break;
    case "webkit-lockdown":
      assert.equal(
        enforcement.lockdownEnabled,
        true,
        "native WebKit must own the security configuration",
      );
      assert.equal(
        observations.peer.available,
        false,
        "native WebKit must withdraw the WebRTC API",
      );
      break;
    default:
      assert.fail("unknown executing carrier enforcement");
  }
}
