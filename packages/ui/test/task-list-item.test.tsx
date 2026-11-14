import { cleanup, render, screen } from "@testing-library/react";
import type { ComponentProps, ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { TaskListItem } from "../dist/index.js";
import { taskListContent } from "../src/markdown";

// This projection does not use callout parsing or its compiled visual styles.
vi.mock("../src/callout", () => ({ readCalloutMarker: () => null }));
afterEach(cleanup);

function Paragraph({
  node: _node,
  ...props
}: ComponentProps<"p"> & { node: { tagName: string } }) {
  return <p {...props} />;
}

function Checkbox({
  node: _node,
  ...props
}: ComponentProps<"input"> & { node: { tagName: string } }) {
  return <input {...props} />;
}

function show(children: ReactNode) {
  const task = taskListContent(children);
  expect(task).not.toBeNull();
  return render(
    <ul>
      <TaskListItem checked={task!.checked} label={task!.label}>
        {task!.children}
      </TaskListItem>
    </ul>,
  );
}

it.each([true, false])(
  "names a tight captured task with checked=%s",
  (checked) => {
    const view = show([
      <input key="marker" type="checkbox" checked={checked} disabled />,
      " Accepted ",
      <strong key="emphasis">decision</strong>,
      <ul key="nested">
        <li>Nested context</li>
      </ul>,
    ]);
    const marker = screen.getByRole("checkbox", { name: "Accepted decision" });
    expect(marker.getAttribute("aria-checked")).toBe(String(checked));
    expect(marker.getAttribute("aria-disabled")).toBe("true");
    expect(marker.hasAttribute("tabindex")).toBe(false);
    expect(view.container.querySelector("input")).toBeNull();
    expect(view.container.textContent).toContain("Nested context");
  },
);

it.each(["native", "adapter"] as const)(
  "names a loose task with a %s paragraph without consuming later or nested text",
  (kind) => {
    const children = [
      <input key="checkbox" type="checkbox" checked disabled />,
      " Accepted ",
      <strong key="emphasis">decision</strong>,
    ];
    const lead =
      kind === "native" ? (
        <p key="lead" data-paragraph="primary">
          {children}
        </p>
      ) : (
        <Paragraph key="lead" node={{ tagName: "p" }} data-paragraph="primary">
          {children}
        </Paragraph>
      );
    const view = show([
      "\n",
      lead,
      "\n",
      <p key="detail">Supporting paragraph.</p>,
      "\n",
      <ul key="nested">
        <li>Nested ordinary item</li>
      </ul>,
      "\n",
    ]);
    expect(
      screen
        .getByRole("checkbox", { name: "Accepted decision" })
        .getAttribute("aria-checked"),
    ).toBe("true");
    expect(view.container.querySelector("input")).toBeNull();
    const item = view.container.querySelector('[data-slot="task-list-item"]')!;
    expect([...item.children].map((node) => node.tagName)).toEqual([
      "SPAN",
      "P",
      "P",
      "UL",
    ]);
    expect(item.querySelector('[data-paragraph="primary"]')?.textContent).toBe(
      " Accepted decision",
    );
    expect(item.querySelectorAll("p")).toHaveLength(2);
    expect(item.textContent).toContain("Supporting paragraph.");
    expect(item.textContent).toContain("Nested ordinary item");
  },
);

it("recognizes an adapter checkbox by its Markdown AST tag", () => {
  const view = show([
    <Checkbox
      key="marker"
      node={{ tagName: "input" }}
      type="checkbox"
      checked={false}
      disabled
    />,
    " Pending item",
  ]);
  expect(
    screen
      .getByRole("checkbox", { name: "Pending item" })
      .getAttribute("aria-checked"),
  ).toBe("false");
  expect(view.container.querySelector("input")).toBeNull();
});

it("does not claim a checkbox nested inside an ordinary item's content", () => {
  const children = (
    <>
      <p>Ordinary item</p>
      <ul>
        <li>
          <input type="checkbox" disabled />
          Nested task
        </li>
      </ul>
    </>
  );
  expect(taskListContent(children.props.children)).toBeNull();
});
