import { describe, expect, it } from "vitest";
import { CLIENT_CAPABILITIES, runtimeRequestMeta } from "@/app/runtimeProtocol";
describe("first-party runtime protocol profile", () => {
  it("opts every request into durable Run trees", () => {
    expect(CLIENT_CAPABILITIES.features?.childRuns).toEqual({ enabled: true });
    expect(runtimeRequestMeta("web").clientCapabilities?.features?.childRuns).toEqual({
      enabled: true,
    });
  });
});
