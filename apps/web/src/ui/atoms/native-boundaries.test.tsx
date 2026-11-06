import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { fireEvent, screen } from "@testing-library/react";
import { createRef } from "react";
import { describe, expect, it, vi } from "vitest";
import { ColorPickerInput } from "@/ui/atoms/color-picker-input";
import { TextLink } from "@/ui/atoms/text-link";
import { HiddenFileInput } from "@/ui/atoms/hidden-file-input";
import { SelectTrigger } from "@/ui/atoms/select-trigger";
describe("native design-system boundaries", () => {
  it("owns browser-native input types inside atoms", () => {
    const { container } = testClient.render(
      <>
        <HiddenFileInput aria-label="Attach" accept="image/*" />
        <ColorPickerInput aria-label="Accent" value="#fb6e30" onValueChange={vi.fn()} />
      </>,
    );

    expect(screen.getByLabelText("Attach").getAttribute("type")).toBe("file");
    expect(container.querySelector('input[type="color"]')?.getAttribute("aria-hidden")).toBe(
      "true",
    );
    expect(screen.getAllByRole("button", { name: "Accent", description: "#FB6E30" })).toHaveLength(
      1,
    );
  });

  it("names a select trigger by its visible current value and purpose", () => {
    const ref = createRef<HTMLButtonElement>();
    const onClick = vi.fn();
    const view = testClient.render(
      <SelectTrigger ref={ref} label="Inter" purpose="Interface font" onClick={onClick} />,
    );

    const trigger = screen.getByRole("button", { name: "Inter, Interface font" });
    expect(trigger.textContent).toContain("Inter");
    expect(ref.current).toBe(trigger);
    fireEvent.click(trigger);
    expect(onClick).toHaveBeenCalledOnce();

    view.rerender(
      <SelectTrigger ref={ref} label="Georgia" purpose="Interface font" onClick={onClick} />,
    );
    expect(screen.getByRole("button", { name: "Georgia, Interface font" })).toBe(trigger);
    expect(trigger.textContent).toContain("Georgia");
    expect(screen.queryByRole("button", { name: "Inter, Interface font" })).toBeNull();
  });

  it("owns safe external-link policy", () => {
    testClient.render(<TextLink href="https://example.com">Source</TextLink>);

    const link = screen.getByRole("link", { name: "Source" });
    expect(link.getAttribute("target")).toBe("_blank");
    expect(link.getAttribute("rel")).toBe("noopener noreferrer");
  });

  it("keeps a document reference and its reading relation in the current document", () => {
    testClient.render(
      <TextLink href="#note" id="reference" aria-describedby="notes">
        1
      </TextLink>,
    );
    const link = screen.getByRole("link", { name: "1" });
    expect(link.getAttribute("href")).toBe("#note");
    expect(link.getAttribute("id")).toBe("reference");
    expect(link.getAttribute("aria-describedby")).toBe("notes");
    expect(link.hasAttribute("target")).toBe(false);
    expect(link.hasAttribute("rel")).toBe(false);
  });
});
