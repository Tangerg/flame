import { cleanup, fireEvent, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { HoverTrack } from "@/ui/atoms/hover-track";
import { AgentRow } from "./navigation-row";

afterEach(cleanup);

function renderTrack() {
  const { container } = render(
    <HoverTrack>
      <AgentRow>First</AgentRow>
      <AgentRow>Second</AgentRow>
      <AgentRow look="search">Search</AgentRow>
    </HoverTrack>,
  );
  const found = [...container.querySelectorAll<HTMLElement>(".agent-row")];
  const at = (index: number): HTMLElement => {
    const row = found[index];
    if (!row) throw new Error(`no row ${index}`);
    return row;
  };
  const lit = () =>
    found.flatMap((row, index) => (row.querySelector("span[aria-hidden]") ? [index] : []));
  const track = container.firstElementChild;
  if (!track) throw new Error("the track renders nothing");
  return { track, at, lit };
}

describe("a navigation row inside a hover track", () => {
  it("carries one highlight, and moves it rather than painting a second", () => {
    const { at, lit } = renderTrack();
    expect(lit()).toEqual([]);

    fireEvent.pointerEnter(at(0));
    expect(lit()).toEqual([0]);

    fireEvent.pointerEnter(at(1));
    expect(lit()).toEqual([1]);
  });

  it("holds the last row the pointer was on until it leaves the list", () => {
    const { track, at, lit } = renderTrack();
    fireEvent.pointerEnter(at(0));
    expect(lit()).toEqual([0]);

    fireEvent.pointerOut(at(0), { relatedTarget: track });
    expect(lit(), "the gutter between two rows is not a state").toEqual([0]);

    fireEvent.pointerOut(track, { relatedTarget: document.body });
    expect(lit()).toEqual([]);
  });

  it("leaves the search field its own fill", () => {
    const { at, lit } = renderTrack();
    fireEvent.pointerEnter(at(2));
    expect(lit()).toEqual([]);
  });

  it("paints nothing extra when no track owns the row", () => {
    const { container } = render(<AgentRow>Alone</AgentRow>);
    const row = container.querySelector<HTMLElement>(".agent-row");
    if (!row) throw new Error("the row renders nothing");
    fireEvent.pointerEnter(row);
    expect(row.querySelector("span[aria-hidden]")).toBeNull();
  });
});
