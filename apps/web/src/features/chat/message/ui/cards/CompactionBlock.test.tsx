import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { fireEvent, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { CompactionBlock } from "@/features/chat/message/ui/cards/CompactionBlock";
describe("CompactionBlock", () => {
  it("presents the automatic context boundary without implementation counts", () => {
    testClient.render(<CompactionBlock summary="Retained architecture decisions" />);

    const trigger = screen.getByRole("button", { name: "Context automatically compacted" });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByText(/8/)).toBeNull();

    fireEvent.click(trigger);
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByText("Retained architecture decisions")).toBeTruthy();
  });
});
