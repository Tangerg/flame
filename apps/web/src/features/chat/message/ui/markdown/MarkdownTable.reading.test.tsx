import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { readingFocusScenario } from "@/test/readingFocusScenario";
import { MarkdownTable } from "./MarkdownTable";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("MarkdownTable reading", () => {
  function tableScenario() {
    const s = readingFocusScenario(testClient, {
      children: (
        <MarkdownTable markdownSource="| Owner |\n| --- |\n| Runtime |">
          <thead>
            <tr>
              <th>Owner</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td>Runtime</td>
            </tr>
          </tbody>
        </MarkdownTable>
      ),
      targetLabel: "Expand table",
      draftLabel: "Later draft",
      draftValue: "Keep this draft",
    });
    const frame = s.target.closest<HTMLElement>("[data-markdown-table]")!;
    vi.spyOn(frame, "getBoundingClientRect").mockImplementation(
      () => new DOMRect(0, 170 - s.viewport.scrollTop, 300, 130),
    );
    s.shape.top = 190;
    return { ...s, frame };
  }

  it("keeps the real table's rows visible when its focused action still fits after reflow", () => {
    const s = tableScenario();
    s.shape.height = 300;

    s.resize();

    expect(s.frame.getBoundingClientRect().bottom).toBe(220);
    expect(document.activeElement).toBe(s.target);
    expect(s.target.getBoundingClientRect().top).toBeGreaterThanOrEqual(20);
  });

  it("leaves later draft focus and reading position intact when the real table reflows", () => {
    const s = tableScenario();
    const draft = screen.getByRole("textbox", { name: "Later draft" });
    act(() => draft.focus());
    s.shape.height = 300;

    s.resize();

    expect(s.viewport.scrollTop).toBe(0);
    expect(document.activeElement).toBe(draft);
    expect((draft as HTMLInputElement).value).toBe("Keep this draft");
  });
});
