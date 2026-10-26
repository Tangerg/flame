import { beforeEach, describe, expect, it, onTestFinished } from "vitest";
import { fireEvent, screen } from "@testing-library/react";
import { useState } from "react";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { ToolCard } from "@/features/chat/tools/ui/ToolCard";
import { ToolGroupMember } from "@/features/chat/tools/ui/ToolGroupMember";

let client: TestClient;
beforeEach(() => {
  client = createTestClient();
  onTestFinished(() => client.dispose());
});

const rows = [
  ["standalone", ToolCard],
  ["grouped", ToolGroupMember],
] as const;

function denied(args: string): ToolCall {
  return {
    id: "declined-read",
    runId: "run",
    name: "read",
    category: "read",
    fn: "runtime/session/store.go",
    fnKind: "path",
    status: "denied",
    args,
  };
}

function Record({ Row, tool }: { Row: typeof ToolCard; tool: ToolCall }) {
  const [expanded, setExpanded] = useState(false);
  return <Row tool={tool} expanded={expanded} onToggleExpand={() => setExpanded(!expanded)} />;
}

describe("recorded denied Tool arguments", () => {
  it.each(rows)(
    "keeps %s declined parameters readable without claiming a result",
    async (_, Row) => {
      const args = '{"path":"runtime/session/store.go","start_line":1,"end_line":80}';
      client.render(<Record Row={Row} tool={denied(args)} />);
      const summary = screen.getByRole("button", { name: /Read.*denied/i });
      expect(summary.getAttribute("aria-expanded")).toBe("false");
      fireEvent.click(summary);

      const reader = await screen.findByRole("region", { name: "Arguments" });
      expect(reader.textContent).toBe(JSON.stringify(JSON.parse(args), null, 2));
      expect(screen.getByText("denied")).toBeTruthy();
      expect(screen.queryByRole("region", { name: "Result" })).toBeNull();
      expect(screen.queryByText("(no result body — tool returned empty)")).toBeNull();
      expect(summary.getAttribute("aria-expanded")).toBe("true");

      fireEvent.click(summary);
      expect(summary.getAttribute("aria-expanded")).toBe("false");
      expect(reader.isConnected && !reader.closest("[inert]")).toBe(false);
    },
  );

  it.each(rows)("preserves captured whitespace in %s declined parameters", async (_, Row) => {
    const args = "\t \n";
    client.render(<Record Row={Row} tool={denied(args)} />);
    fireEvent.click(screen.getByRole("button", { name: /Read.*denied/i }));
    expect((await screen.findByRole("region", { name: "Arguments" })).textContent).toBe(args);
  });

  it.each(rows)("keeps a %s denied record without parameters static", (_, Row) => {
    const { container } = client.render(
      <Row tool={denied("")} expanded onToggleExpand={() => {}} />,
    );
    expect(screen.getByText("denied")).toBeTruthy();
    expect(container.querySelector("[aria-expanded]")).toBeNull();
    expect(screen.queryByRole("region")).toBeNull();
  });
});
