import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { Tabs, useContentVisible } from "../dist/index.js";

afterEach(cleanup);

function Draft() {
  const [value, setValue] = useState("");
  const visible = useContentVisible();
  return (
    <input
      aria-label="Review draft"
      data-visible={visible}
      value={value}
      onChange={(event) => setValue(event.currentTarget.value)}
    />
  );
}

it("keeps retained drafts and visibility under the controlled tab value", async () => {
  function Scene() {
    const [value, setValue] = useState("draft");
    return (
      <Tabs
        value={value}
        onValueChange={setValue}
        ariaLabel="Report views"
        items={[
          { value: "draft", label: "Draft", content: <Draft /> },
          { value: "source", label: "Source", content: "Accepted source" },
        ]}
      />
    );
  }
  render(<Scene />);
  const draft = screen.getByRole("textbox", { name: "Review draft" });
  fireEvent.change(draft, { target: { value: "Keep this note" } });
  fireEvent.click(screen.getByRole("tab", { name: "Source" }));
  expect(draft.isConnected).toBe(true);
  expect(draft.getAttribute("data-visible")).toBe("false");
  await waitFor(() => expect(screen.queryByRole("textbox")).toBeNull());
  const panel = screen.getByRole("tabpanel", { name: "Source" });
  const tab = screen.getByRole("tab", { name: "Source" });
  expect(tab.getAttribute("aria-controls")).toBe(panel.id);
  fireEvent.click(screen.getByRole("tab", { name: "Draft" }));
  expect(screen.getByRole("textbox")).toBe(draft);
  expect(draft).toHaveProperty("value", "Keep this note");
  expect(draft.getAttribute("data-visible")).toBe("true");
});

it("delegates keyboard navigation without advancing an unconfirmed value", async () => {
  const change = vi.fn();
  render(
    <Tabs
      value="first"
      onValueChange={change}
      ariaLabel="Report views"
      items={[
        { value: "first", label: "First", content: "First panel" },
        {
          value: "disabled",
          label: "Unavailable",
          content: "Unavailable panel",
          disabled: true,
        },
        { value: "last", label: "Last", content: "Last panel" },
      ]}
    />,
  );
  await act(async () => {});
  const first = screen.getByRole("tab", { name: "First" });
  const unavailable = screen.getByRole("tab", { name: "Unavailable" });
  const last = screen.getByRole("tab", { name: "Last" });
  await waitFor(() => expect(first.tabIndex).toBe(0));
  act(() => first.focus());
  fireEvent.keyDown(first, { key: "ArrowRight" });
  await waitFor(() => expect(document.activeElement).toBe(unavailable));
  fireEvent.click(unavailable);
  expect(change).not.toHaveBeenCalled();
  fireEvent.keyDown(unavailable, { key: "ArrowRight" });
  await waitFor(() => expect(document.activeElement).toBe(last));
  expect(change).toHaveBeenCalledWith("last");
  expect(first.getAttribute("aria-selected")).toBe("true");
  expect(last.getAttribute("aria-selected")).toBe("false");
  expect(screen.getByRole("tabpanel", { name: "First" })).toBeTruthy();
  expect(fireEvent.keyDown(last, { key: "ArrowDown" })).toBe(true);
  fireEvent.click(unavailable);
  expect(change).toHaveBeenCalledTimes(1);
});
