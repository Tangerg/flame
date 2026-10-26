import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { type AgentUnresolvedEffect } from "@/features/agent/public/sessionView";
import { UnresolvedRunEffects } from "@/features/chat/message/ui/UnresolvedRunEffects";

const OUTPUT_LABEL = "Observed output (outcome unconfirmed)";

function effects(): AgentUnresolvedEffect[] {
  return [
    {
      processId: "process-ledger",
      effectId: "write-ledger",
      cause: "unknown",
      reason: "response_lost",
      detail: "The ledger connection closed before a receipt.",
      output: Array.from({ length: 400 }, (_, line) => `ledger line ${line}`).join("\n"),
    },
    {
      processId: "process-billing",
      effectId: "send-invoice",
      cause: "unknown",
      detail: "The invoice provider did not confirm delivery.",
      output: '{"invoice":"INV-7"}',
    },
    { processId: "process-billing", effectId: "notify-webhook", cause: "unknown" },
  ];
}

function operation(index: number, effectId: string) {
  return screen.getByRole("button", {
    name: new RegExp(`^Unconfirmed operation ${index}\\s*${effectId}$`),
  });
}

function evidence(trigger: HTMLElement) {
  return within(document.getElementById(trigger.getAttribute("aria-controls")!)!);
}

describe("UnresolvedRunEffects", () => {
  afterEach(cleanup);

  it.each([{ value: undefined }, { value: [] }])(
    "renders nothing without unresolved effects ($value)",
    ({ value }) => {
      const { container } = testClient.render(<UnresolvedRunEffects effects={value} />);
      expect(container.firstChild).toBeNull();
    },
  );

  it("names every operation in the overview without mounting any evidence", () => {
    testClient.render(<UnresolvedRunEffects effects={effects()} />);
    expect(
      screen.getByText(
        "This run has ended. The outcomes of these external operations are still unconfirmed.",
      ),
    ).toBeTruthy();
    const triggers = [
      operation(1, "write-ledger"),
      operation(2, "send-invoice"),
      operation(3, "notify-webhook"),
    ];
    for (const trigger of triggers) expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(screen.getAllByRole("button")).toHaveLength(3);
    expect(screen.queryByText("Process ID")).toBeNull();
    expect(screen.queryByText(/ledger line/)).toBeNull();
    expect(screen.queryByRole("region", { name: OUTPUT_LABEL })).toBeNull();
  });

  it("opens only the chosen operation's original evidence", () => {
    testClient.render(<UnresolvedRunEffects effects={effects()} />);
    const second = operation(2, "send-invoice");
    fireEvent.click(second);

    expect(second.getAttribute("aria-expanded")).toBe("true");
    const panel = evidence(second);
    expect(panel.getByText("process-billing")).toBeTruthy();
    expect(panel.getByText("send-invoice")).toBeTruthy();
    expect(panel.getByText("The invoice provider did not confirm delivery.")).toBeTruthy();
    expect(panel.queryByText("Reason code")).toBeNull();
    expect(panel.getByRole("region", { name: OUTPUT_LABEL }).textContent).toBe(
      '{"invoice":"INV-7"}',
    );
    expect(screen.getAllByText("Process ID")).toHaveLength(1);
    expect(screen.getAllByRole("region", { name: OUTPUT_LABEL })).toHaveLength(1);
    expect(screen.queryByText(/ledger line/)).toBeNull();
    expect(screen.queryByText("The ledger connection closed before a receipt.")).toBeNull();
    expect(operation(1, "write-ledger").getAttribute("aria-expanded")).toBe("false");
    expect(operation(3, "notify-webhook").getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByRole("button", { name: /retry|resume/i })).toBeNull();
  });

  it("opens and closes operations independently while focus stays on the used trigger", () => {
    testClient.render(<UnresolvedRunEffects effects={effects()} />);
    const first = operation(1, "write-ledger");
    const third = operation(3, "notify-webhook");

    act(() => first.focus());
    fireEvent.click(first);
    act(() => third.focus());
    fireEvent.click(third);
    expect(first.getAttribute("aria-expanded")).toBe("true");
    expect(third.getAttribute("aria-expanded")).toBe("true");
    expect(evidence(first).getByText("process-ledger")).toBeTruthy();
    expect(evidence(third).getByText("notify-webhook")).toBeTruthy();
    expect(screen.getAllByText("Process ID")).toHaveLength(2);

    act(() => first.focus());
    fireEvent.click(first);
    expect(first.getAttribute("aria-expanded")).toBe("false");
    expect(third.getAttribute("aria-expanded")).toBe("true");
    expect(screen.queryByText(/ledger line/)).toBeNull();
    expect(screen.getAllByText("Process ID")).toHaveLength(1);
    expect(document.activeElement).toBe(first);
    expect(operation(1, "write-ledger")).toBe(first);
  });

  it("keeps each operation's identity and reading state when the effects reorder", () => {
    const original = effects();
    const { rerender } = testClient.render(<UnresolvedRunEffects effects={original} />);
    const ledger = operation(1, "write-ledger");
    act(() => ledger.focus());
    fireEvent.click(ledger);

    rerender(<UnresolvedRunEffects effects={[...original].reverse()} />);
    expect(operation(3, "write-ledger")).toBe(ledger);
    expect(ledger.getAttribute("aria-expanded")).toBe("true");
    expect(document.activeElement).toBe(ledger);
    expect(evidence(ledger).getByText("process-ledger")).toBeTruthy();
    expect(operation(1, "notify-webhook").getAttribute("aria-expanded")).toBe("false");
    expect(operation(2, "send-invoice").getAttribute("aria-expanded")).toBe("false");
    expect(screen.getAllByText("Process ID")).toHaveLength(1);
  });

  it("renders captured literals byte-for-byte without changing the source", () => {
    const output =
      '{"receipt":9007199254740993,"amount":1.10,"id":"00042","text":"<script>unexpected()</script>"}';
    const source: AgentUnresolvedEffect[] = [
      {
        processId: "00042",
        effectId: "effect-9007199254740993",
        cause: "unknown",
        reason: "response_lost",
        detail: "Receipt 9007199254740993 was never acknowledged.",
        output,
      },
    ];
    const snapshot = structuredClone(source);
    const { container } = testClient.render(<UnresolvedRunEffects effects={source} />);
    const trigger = operation(1, "effect-9007199254740993");
    fireEvent.click(trigger);

    const panel = evidence(trigger);
    expect(panel.getByText("00042")).toBeTruthy();
    expect(panel.getByText("effect-9007199254740993")).toBeTruthy();
    expect(panel.getByText("Receipt 9007199254740993 was never acknowledged.")).toBeTruthy();
    expect(panel.getByRole("region", { name: OUTPUT_LABEL }).textContent).toBe(output);
    expect(container.querySelector("script")).toBeNull();
    expect(source).toEqual(snapshot);
  });
});
