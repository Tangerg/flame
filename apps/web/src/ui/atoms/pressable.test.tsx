import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Pressable } from "@/ui/atoms/pressable";
describe("Pressable", () => {
  it("provides button semantics without taking ownership of content layout", () => {
    testClient.render(
      <Pressable aria-label="Open row" className="grid grid-cols-2">
        <span>Row content</span>
      </Pressable>,
    );

    const pressable = screen.getByRole("button", { name: "Open row" });
    expect(pressable.getAttribute("type")).toBe("button");
    expect(pressable.className).toContain("grid-cols-2");
    expect(pressable.className).not.toContain("rounded-md");
  });
});
