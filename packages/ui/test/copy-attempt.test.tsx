import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ContentVisibility, useCopyAttempt } from "../dist/index.js";

afterEach(cleanup);

function Copy({
  source,
  capture,
}: {
  source: object;
  capture: (current: () => boolean) => void;
}) {
  const begin = useCopyAttempt(source);
  return (
    <button
      onClick={() => {
        const current = begin();
        if (current) capture(current);
      }}
    >
      Copy
    </button>
  );
}

it("gives only the latest attempt permission to publish and never revives a retired source", () => {
  const first = {};
  const second = {};
  const capture = vi.fn<(current: () => boolean) => void>();
  const view = render(<Copy source={first} capture={capture} />);
  fireEvent.click(screen.getByRole("button"));
  const older = capture.mock.calls[0]![0];
  expect(older()).toBe(true);
  fireEvent.click(screen.getByRole("button"));
  const latest = capture.mock.calls[1]![0];
  expect(older()).toBe(false);
  expect(latest()).toBe(true);
  view.rerender(<Copy source={second} capture={capture} />);
  expect(latest()).toBe(false);
  view.rerender(<Copy source={first} capture={capture} />);
  expect(latest()).toBe(false);
  fireEvent.click(screen.getByRole("button"));
  const returned = capture.mock.calls[2]![0];
  expect(returned()).toBe(true);
  view.unmount();
  expect(returned()).toBe(false);
});

it("retires hidden scope attempts and admits a fresh attempt only after reveal", () => {
  const source = {};
  const capture = vi.fn<(current: () => boolean) => void>();
  const scene = (visible: boolean) => (
    <ContentVisibility visible={visible}>
      <Copy source={source} capture={capture} />
    </ContentVisibility>
  );
  const view = render(scene(true));
  fireEvent.click(screen.getByRole("button"));
  const current = capture.mock.calls[0]![0];
  act(() => view.rerender(scene(false)));
  expect(current()).toBe(false);
  fireEvent.click(screen.getByRole("button"));
  expect(capture).toHaveBeenCalledTimes(1);
  view.rerender(scene(true));
  fireEvent.click(screen.getByRole("button"));
  expect(current()).toBe(false);
  expect(capture.mock.calls[1]![0]()).toBe(true);
});
