import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useState } from "react";
import { browserPluginCarrier } from "@/platform/browserPluginCarrier";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import { useTelemetryStore } from "@/lib/observability/stores";
import { ClientDiagnosticsPackageView } from "./ClientDiagnosticsPackageView";

vi.mock("./PackageView", () => ({
  PackageView: () => {
    const [count] = useState(() => useTelemetryStore.getState().logs.length);
    return <p>{count} local observations</p>;
  },
}));
afterEach(() => {
  cleanup();
  useTelemetryStore.getState().clear();
});

it("clears through the telemetry owner and replaces the displayed observations", () => {
  useTelemetryStore
    .getState()
    .ingestLogs([{ id: "one", timeMs: 0, severity: "INFO", body: "one", attrs: {} }]);
  const reads = vi.fn();
  render(
    <ClientDiagnosticsPackageView
      reads={reads}
      title="Diagnostics"
      lifetime={{} as ContributionLifetime}
      carrier={browserPluginCarrier}
    />,
  );
  expect(useTelemetryStore.getState().logs).toHaveLength(1);
  expect(screen.getByText("1 local observations")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Clear" }));
  expect(useTelemetryStore.getState().logs).toEqual([]);
  expect(screen.getByText("0 local observations")).toBeTruthy();
  expect(reads).toHaveBeenCalledOnce();
});
