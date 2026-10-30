import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ContentVisibility, EvidenceImage } from "../dist/index.js";

const source =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=";

beforeEach(() => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

async function openImage() {
  const trigger = screen.getByRole("button", {
    name: "View image: Boundary map",
  });
  act(() => trigger.focus());
  fireEvent.click(trigger);
  const dialog = await screen.findByRole("dialog", { name: "Boundary map" });
  await waitFor(() =>
    expect(dialog.contains(document.activeElement)).toBe(true),
  );
  return { trigger, dialog };
}

it("opens the original source, zooms and returns to its thumbnail without changing a draft", async () => {
  render(
    <>
      <input aria-label="Draft" defaultValue="Continue from this evidence" />
      <EvidenceImage src={source} alt="Boundary map" />
    </>,
  );
  const thumbnail = screen.getByRole("img", { name: "Boundary map" });
  const { trigger, dialog } = await openImage();
  expect(within(dialog).getByRole("img").getAttribute("src")).toBe(source);
  const viewport = within(dialog).getByRole("region", { name: "Boundary map" });
  expect(
    within(dialog).getByRole("region", { name: "Image description" })
      .textContent,
  ).toBe("Boundary map");
  expect(viewport.getAttribute("data-image-zoom")).toBe("100");
  fireEvent.click(
    within(dialog).getByRole("button", { name: "Zoom in image" }),
  );
  expect(viewport.getAttribute("data-image-zoom")).toBe("125");
  expect(viewport.tabIndex).toBe(0);
  fireEvent.keyDown(dialog, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(trigger));
  expect(screen.getByRole("img", { name: "Boundary map" })).toBe(thumbnail);
  expect(screen.getByRole("textbox", { name: "Draft" })).toHaveProperty(
    "value",
    "Continue from this evidence",
  );
});

it("retires a hidden reading view and keeps it closed when its owner returns", async () => {
  const scene = (visible: boolean) => (
    <>
      <ContentVisibility visible={visible}>
        <EvidenceImage src={source} alt="Boundary map" />
      </ContentVisibility>
      <button>Later action</button>
    </>
  );
  const view = render(scene(true));
  const { trigger } = await openImage();
  view.rerender(scene(false));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(document.activeElement).not.toBe(trigger);
  view.rerender(scene(true));
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(
    screen.getByRole("img", { name: "Boundary map" }).getAttribute("src"),
  ).toBe(source);
});

it("ends an open capture when its source is replaced instead of retargeting the viewer", async () => {
  const view = render(<EvidenceImage src={source} alt="Boundary map" />);
  await openImage();
  const replacement =
    "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==";
  view.rerender(<EvidenceImage src={replacement} alt="Next page" />);
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(
    screen.getByRole("img", { name: "Next page" }).getAttribute("src"),
  ).toBe(replacement);
});

it("uses supplied action labels while retaining the captured image identity", async () => {
  render(
    <EvidenceImage
      src={source}
      alt="原始证据图"
      labels={{
        view: "查看图片",
        close: "关闭图片",
        zoomIn: "放大图片",
        zoomOut: "缩小图片",
        description: "图片说明",
      }}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "查看图片: 原始证据图" }));
  const dialog = await screen.findByRole("dialog", { name: "原始证据图" });
  expect(within(dialog).getByRole("button", { name: "放大图片" })).toBeTruthy();
  expect(within(dialog).getByRole("button", { name: "缩小图片" })).toBeTruthy();
  fireEvent.click(within(dialog).getByRole("button", { name: "关闭图片" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});
