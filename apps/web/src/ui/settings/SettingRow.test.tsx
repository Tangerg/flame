import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Panel, Segmented, Slider } from "@flame/ui";
import { SettingRow } from "@/ui/settings/SettingRow";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

it.each([
  ["slider", "focus"],
  ["segmented", "focus"],
  ["slider", "pointer"],
  ["segmented", "pointer"],
] as const)("%s preserves reading context on %s activation", (kind, cause) => {
  const viewport = new DOMRect(0, 238, 561, 287);
  const view = testClient.render(
    <div data-testid="viewport" style={{ height: 287, overflowY: "auto" }}>
      <Panel label="Appearance">
        <div data-testid="row-host">
          <SettingRow label="Paper texture">
            {kind === "slider" ? (
              <Slider
                value={3}
                min={1}
                max={5}
                ariaLabel="Texture strength"
                onValueChange={() => {}}
              />
            ) : (
              <Segmented
                value="cotton"
                ariaLabel="Paper choices"
                options={[{ value: "cotton", label: "Cotton paper" }]}
                onChange={() => {}}
              />
            )}
          </SettingRow>
        </div>
      </Panel>
    </div>,
  );
  const row = screen.getByTestId("row-host").firstElementChild!;
  const field =
    kind === "slider"
      ? screen.getByRole("slider", { name: "Texture strength" })
      : screen.getByRole("tab", { name: "Cotton paper" });
  vi.spyOn(document.documentElement, "clientHeight", "get").mockReturnValue(525);
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (
    this: HTMLElement,
  ) {
    if (this === screen.getByTestId("viewport")) return viewport;
    if (this === row) return new DOMRect(0, 190, 471, 96);
    if (this === field) return new DOMRect(45, 250, 180, 32);
    return new DOMRect(0, 0, 561, 525);
  });
  const revealed: Element[] = [];
  vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(function (
    this: HTMLElement,
  ) {
    revealed.push(this);
  });
  if (cause === "pointer") fireEvent.pointerDown(field);
  act(() => field.focus());
  expect(revealed).toEqual(cause === "pointer" ? [] : [row]);
  expect(document.activeElement).toBe(field);
  expect(view.getByText("Paper texture")).toBeTruthy();
});
