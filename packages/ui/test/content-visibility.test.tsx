import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { ContentVisibility, useContentVisible } from "../dist/index.js";
function Probe() {
  return <output>{useContentVisible() ? "visible" : "hidden"}</output>;
}

it("is visible by default and hidden when any enclosing host hides it", () => {
  const { rerender } = render(<Probe />);
  expect(screen.getByRole("status").textContent).toBe("visible");

  rerender(
    <ContentVisibility visible={false}>
      <ContentVisibility visible>
        <Probe />
      </ContentVisibility>
    </ContentVisibility>,
  );
  expect(screen.getByRole("status").textContent).toBe("hidden");

  rerender(
    <ContentVisibility visible>
      <ContentVisibility visible>
        <Probe />
      </ContentVisibility>
    </ContentVisibility>,
  );
  expect(screen.getByRole("status").textContent).toBe("visible");
});
