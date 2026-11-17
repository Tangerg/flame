import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { Panel } from "../dist/index.js";

afterEach(cleanup);

it("keeps its DOM ref attached across ordinary content updates", () => {
  const projected: (HTMLElement | null)[] = [];
  const reference = (node: HTMLElement | null) => {
    projected.push(node);
  };
  const view = render(
    <Panel ref={reference} label="Draft">
      Before editing
    </Panel>,
  );
  const panel = screen.getByRole("region", { name: "Draft" });
  view.rerender(
    <Panel ref={reference} label="Draft">
      During editing
    </Panel>,
  );
  expect(projected).toEqual([panel]);
  view.unmount();
  expect(projected).toEqual([panel, null]);
});
