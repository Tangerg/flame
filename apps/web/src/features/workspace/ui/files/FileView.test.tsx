import { createRef } from "react";
import { render, cleanup } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { FileView } from "./FileView";
const renderCode = vi.hoisted(() => vi.fn());
vi.mock("@/lib/highlight/PierreFile", () => ({
  PierreFile: (props: unknown) => {
    renderCode(props);
    return null;
  },
}));
afterEach(cleanup);
it("passes the captured source and line intent to Pierre while retaining quote identity", () => {
  const ref = createRef<HTMLDivElement>();
  const intent = {};
  render(
    <FileView
      ref={ref}
      path="src/main.go"
      content="package main"
      startLine={20}
      targetLine={24}
      intent={intent}
    />,
  );
  expect(renderCode).toHaveBeenLastCalledWith({
    path: "src/main.go",
    source: "package main",
    startLine: 20,
    targetLine: 24,
    intent,
  });
  expect(ref.current?.dataset.quotePath).toBe("src/main.go");
});
