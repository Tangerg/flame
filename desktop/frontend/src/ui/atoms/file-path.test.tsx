import { describe, expect, it } from "vitest";
import { render } from "@testing-library/react";
import { FilePath } from "./file-path";

describe("FilePath", () => {
  it("pins the filename and lets the directory take the squeeze", () => {
    const { container } = render(<FilePath path="src/plugins/builtin/chat/Composer.tsx" />);
    const parts = [...(container.firstElementChild?.children ?? [])];

    expect(parts.map((p) => p.textContent)).toEqual([
      "src/plugins/builtin/chat",
      "/",
      "Composer.tsx",
    ]);
    expect(parts[0]?.getAttribute("dir")).toBe("rtl");
    expect(parts).toHaveLength(3);
    expect(parts[0]?.tagName).toBe("SPAN");
    expect(parts[2]?.tagName).toBe("SPAN");
    expect(parts[1]?.textContent).toBe("/");
  });

  it("isolates the directory's own text direction inside the rtl clip", () => {
    const { container } = render(<FilePath path="/Users/me/app/tools/preview.ts" />);
    const clip = container.querySelector("[dir=rtl]");
    expect(clip?.children).toHaveLength(1);
    expect(clip?.firstElementChild?.getAttribute("dir")).toBe("ltr");
    expect(clip?.firstElementChild?.textContent).toBe("/Users/me/app/tools");
  });

  it("renders a bare filename as just the filename", () => {
    const { container } = render(<FilePath path="go.mod" />);
    expect(container.textContent).toBe("go.mod");
    expect(container.querySelector("[dir=rtl]")).toBeNull();
  });

  it("keeps an absolute path's leading slash out of the filename", () => {
    const { container } = render(<FilePath path="/etc/hosts" />);
    expect(container.textContent).toBe("/etc/hosts");
  });

  it("keeps the slash of a root-level path, which has no directory to hold it", () => {
    const { container } = render(<FilePath path="/LICENSE" />);
    expect(container.textContent).toBe("/LICENSE");
  });

  it("carries the whole path as the title, since part of it may be clipped", () => {
    const { container } = render(<FilePath path="a/b/c.ts" />);
    expect(container.firstElementChild?.getAttribute("title")).toBe("a/b/c.ts");
  });
});
