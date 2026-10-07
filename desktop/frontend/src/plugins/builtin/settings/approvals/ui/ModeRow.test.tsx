import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type {
  ApprovalMode,
  ApprovalModeResult,
} from "@/plugins/builtin/agent/public/approvalPolicy";

const model = vi.hoisted(() => ({
  setApprovalMode: vi.fn(),
}));

vi.mock("@/plugins/builtin/agent/public/approvalPolicy", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/plugins/builtin/agent/public/approvalPolicy")>()),
  setApprovalMode: model.setApprovalMode,
}));

import { ModeRow } from "./ModeRow";

beforeEach(() => {
  model.setApprovalMode.mockReset();
});

describe("ModeRow", () => {
  it("owns the visible selection and duplicate admission only while a save is pending", async () => {
    const saving = Promise.withResolvers<ApprovalModeResult>();
    model.setApprovalMode.mockReturnValue(saving.promise);
    const view = render(<ModeRow approval={approval("balanced")} />);

    const safe = screen.getByRole("radio", { name: "Safe" });
    const balanced = screen.getByRole("radio", { name: "Balanced" });
    const auto = screen.getByRole("radio", { name: "Auto" });
    fireEvent.click(auto);

    const pendingSelection = {
      auto: auto.getAttribute("aria-checked"),
      balanced: balanced.getAttribute("aria-checked"),
      busy: auto.getAttribute("aria-busy"),
      disabled: [safe, balanced, auto].every(
        (button) => button.getAttribute("aria-disabled") === "true",
      ),
    };
    fireEvent.click(safe);
    const callsWhileSaving = model.setApprovalMode.mock.calls.length;

    view.rerender(<ModeRow approval={approval("yolo")} />);
    await act(async () => {
      saving.resolve(approval("yolo"));
      await saving.promise;
    });
    await waitFor(() => expect(auto.getAttribute("aria-disabled")).toBeNull());

    expect(pendingSelection).toEqual({
      auto: "true",
      balanced: "false",
      busy: "true",
      disabled: true,
    });
    expect(callsWhileSaving).toBe(1);
    expect(auto.getAttribute("aria-checked")).toBe("true");
  });

  it("retires a rejected intent and admits a corrected choice", async () => {
    const rejected = Promise.withResolvers<ApprovalModeResult>();
    model.setApprovalMode
      .mockReturnValueOnce(rejected.promise)
      .mockResolvedValueOnce(approval("safe"));
    render(<ModeRow approval={approval("balanced")} />);

    const safe = screen.getByRole("radio", { name: "Safe" });
    const balanced = screen.getByRole("radio", { name: "Balanced" });
    const auto = screen.getByRole("radio", { name: "Auto" });
    fireEvent.click(auto);
    await act(async () => {
      rejected.reject(new Error("not saved"));
      await rejected.promise.catch(() => undefined);
    });

    await waitFor(() => expect(auto.getAttribute("aria-disabled")).toBeNull());
    expect(balanced.getAttribute("aria-checked")).toBe("true");
    fireEvent.click(safe);

    expect(model.setApprovalMode.mock.calls.map(([mode]) => mode)).toEqual(["yolo", "safe"]);
  });

  it("describes each mode with the gates the Runtime published", () => {
    render(<ModeRow approval={approval("balanced")} />);

    expect(screen.getByRole("radio", { name: "Balanced" }).textContent).toContain(
      "Edits: allowed · Commands: asks first · Network: asks first",
    );
    expect(screen.getByRole("radio", { name: "Safe" }).textContent).toContain(
      "Edits: asks first · Commands: asks first · Network: refused",
    );
  });
});

function approval(mode: ApprovalMode): ApprovalModeResult {
  return {
    mode,
    modes: [
      { mode: "safe", write: "prompt", exec: "prompt", network: "deny" },
      { mode: "balanced", write: "pass", exec: "prompt", network: "prompt" },
      { mode: "yolo", write: "pass", exec: "pass", network: "pass" },
    ],
  };
}
