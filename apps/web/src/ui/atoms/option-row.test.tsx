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
import { OptionRow } from "@/ui/atoms/option-row";
describe("OptionRow", () => {
  it("becomes an option, with its state, when a listbox drives selection by hand", () => {
    testClient.render(
      <>
        <OptionRow selected>picked</OptionRow>
        <OptionRow selected={false}>other</OptionRow>
      </>,
    );

    const picked = screen.getByText("picked");
    expect(picked.getAttribute("role")).toBe("option");
    expect(picked.getAttribute("aria-selected")).toBe("true");

    const other = screen.getByText("other");
    expect(other.getAttribute("aria-selected")).toBe("false");
  });

  it("claims neither role nor selection state when it is not told", () => {
    testClient.render(<OptionRow>plain</OptionRow>);

    const row = screen.getByText("plain");
    expect(row.hasAttribute("role")).toBe(false);
    expect(row.hasAttribute("aria-selected")).toBe(false);
    expect(row.tagName).toBe("BUTTON");
  });
});
