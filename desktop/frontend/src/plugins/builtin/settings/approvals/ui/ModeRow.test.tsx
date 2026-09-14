import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ApprovalMode } from "../application/approvalConfig";

const model = vi.hoisted(() => ({
  setApprovalMode: vi.fn(),
}));

vi.mock("@/plugins/builtin/agent/public/approvalPolicy", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/plugins/builtin/agent/public/approvalPolicy")>()),
  setApprovalMode: model.setApprovalMode,
}));

vi.mock("../application/approvalConfig", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../application/approvalConfig")>()),
}));

import { ModeRow } from "./ModeRow";

beforeEach(() => {
  model.setApprovalMode.mockReset();
});

describe("ModeRow", () => {
  it("owns the visible selection and duplicate admission while a save is pending", async () => {
    const saving = Promise.withResolvers<ApprovalMode>();
    model.setApprovalMode.mockReturnValue(saving.promise);
    const view = render(<ModeRow mode="balanced" />);

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

    await act(async () => {
      saving.resolve("yolo");
      await saving.promise;
    });
    const acceptedBeforeProjection = {
      auto: auto.getAttribute("aria-checked"),
      balanced: balanced.getAttribute("aria-checked"),
      disabled: [safe, balanced, auto].every(
        (button) => button.getAttribute("aria-disabled") === "true",
      ),
    };

    view.rerender(<ModeRow mode="yolo" />);
    await waitFor(() => expect(auto.getAttribute("aria-disabled")).toBeNull());

    expect(pendingSelection).toEqual({
      auto: "true",
      balanced: "false",
      busy: "true",
      disabled: true,
    });
    expect(callsWhileSaving).toBe(1);
    expect(acceptedBeforeProjection).toEqual({
      auto: "true",
      balanced: "false",
      disabled: true,
    });
    expect(auto.getAttribute("aria-checked")).toBe("true");
  });

  it("retires a rejected intent and admits a corrected choice", async () => {
    const rejected = Promise.withResolvers<ApprovalMode>();
    model.setApprovalMode.mockReturnValueOnce(rejected.promise).mockResolvedValueOnce("safe");
    render(<ModeRow mode="balanced" />);

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
});
