import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ColorPickerInput } from "./color-picker-input";
import { TextLink } from "./text-link";
import { HiddenFileInput } from "./hidden-file-input";

describe("native design-system boundaries", () => {
  it("owns browser-native input types inside atoms", () => {
    render(
      <>
        <HiddenFileInput aria-label="Attach" accept="image/*" />
        <ColorPickerInput aria-label="Accent" />
      </>,
    );

    expect(screen.getByLabelText("Attach").getAttribute("type")).toBe("file");
    expect(screen.getByLabelText("Accent").getAttribute("type")).toBe("color");
  });

  it("owns safe external-link policy", () => {
    render(<TextLink href="https://example.com">Source</TextLink>);

    const link = screen.getByRole("link", { name: "Source" });
    expect(link.getAttribute("target")).toBe("_blank");
    expect(link.getAttribute("rel")).toBe("noopener noreferrer");
  });

  it("keeps a document reference and its reading relation in the current document", () => {
    render(
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
