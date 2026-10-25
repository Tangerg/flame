import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MESSAGE_CONTENT_CLASS } from "@/features/chat/message/ui/messageContent";
import { MarkdownRenderer } from "@/features/chat/message/ui/markdown/MarkdownRenderer";
import { ImageBlock } from "@/features/chat/message/ui/cards/ImageBlock";
const mocks = vi.hoisted(() => ({
  saveImage: vi.fn<(source: string) => Promise<boolean>>(),
}));

vi.mock("@/features/chat/message/application/imageSave", () => ({
  saveInlineImage: mocks.saveImage,
}));

const FIRST_PNG =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=";

const SECOND_GIF = "R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==";

async function openPreview(button: HTMLElement): Promise<void> {
  fireEvent.click(button);
  await act(async () => Promise.resolve());
}

async function settle(action: () => void): Promise<void> {
  await act(async () => {
    action();
    await Promise.resolve();
  });
}

async function failLater(save: PromiseWithResolvers<boolean>): Promise<void> {
  await act(async () => {
    save.reject(new Error("disk full"));
    await save.promise.catch(() => {});
  });
}

describe("ImageBlock", () => {
  beforeEach(() => {
    mocks.saveImage.mockReset();
    mocks.saveImage.mockResolvedValue(true);
  });

  it("saves the active gallery image through the packaged desktop owner", async () => {
    render(
      <div className={MESSAGE_CONTENT_CLASS}>
        <ImageBlock mime="image/png" data={FIRST_PNG} />
        <ImageBlock mime="image/gif" data={SECOND_GIF} />
      </div>,
    );

    await openPreview(screen.getAllByRole("button", { name: "View attached image" })[0]!);
    fireEvent.click(screen.getByRole("button", { name: "Next image" }));
    fireEvent.click(screen.getByRole("button", { name: "Download image" }));

    await waitFor(() => {
      expect(mocks.saveImage).toHaveBeenCalledWith(`data:image/gif;base64,${SECOND_GIF}`);
    });
  });

  it("keeps a failed save in the preview and clears it when the retry saves", async () => {
    mocks.saveImage.mockRejectedValueOnce(new Error("disk full"));
    render(<ImageBlock mime="image/png" data={FIRST_PNG} />);

    await openPreview(screen.getByRole("button", { name: "View attached image" }));
    const dialog = screen.getByRole("dialog", { name: "Image preview" });
    const download = within(dialog).getByRole("button", { name: "Download image" });
    await settle(() => fireEvent.click(download));

    const alert = await within(dialog).findByRole("alert");
    expect(alert.textContent).toContain("Image preview");
    expect(alert.textContent).not.toContain("disk full");
    expect(download.getAttribute("aria-describedby")).toBe(alert.id);
    expect(dialog.querySelector("img")?.getAttribute("src")).toBe(
      `data:image/png;base64,${FIRST_PNG}`,
    );

    await settle(() => fireEvent.click(download));
    expect(within(dialog).queryByRole("alert")).toBeNull();
    expect(download.hasAttribute("aria-describedby")).toBe(false);
    expect(mocks.saveImage).toHaveBeenCalledTimes(2);
  });

  it("stays quiet when the save chooser is cancelled", async () => {
    mocks.saveImage.mockResolvedValueOnce(false);
    render(<ImageBlock mime="image/png" data={FIRST_PNG} />);

    await openPreview(screen.getByRole("button", { name: "View attached image" }));
    const dialog = screen.getByRole("dialog", { name: "Image preview" });
    const download = within(dialog).getByRole("button", { name: "Download image" });
    await settle(() => fireEvent.click(download));

    expect(mocks.saveImage).toHaveBeenCalledOnce();
    expect(within(dialog).queryByRole("alert")).toBeNull();
    expect(download.hasAttribute("aria-describedby")).toBe(false);
  });

  it("keeps a failure from a closed preview out of the reopened one", async () => {
    const save = Promise.withResolvers<boolean>();
    mocks.saveImage.mockReturnValueOnce(save.promise);
    render(<ImageBlock mime="image/png" data={FIRST_PNG} />);

    await openPreview(screen.getByRole("button", { name: "View attached image" }));
    fireEvent.click(screen.getByRole("button", { name: "Download image" }));
    fireEvent.keyDown(screen.getByRole("dialog", { name: "Image preview" }), { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await openPreview(screen.getByRole("button", { name: "View attached image" }));
    await failLater(save);

    const dialog = screen.getByRole("dialog", { name: "Image preview" });
    expect(within(dialog).queryByRole("alert")).toBeNull();
    expect(
      within(dialog)
        .getByRole("button", { name: "Download image" })
        .hasAttribute("aria-describedby"),
    ).toBe(false);
  });

  it("names the image whose save failed after moving to another image", async () => {
    const save = Promise.withResolvers<boolean>();
    mocks.saveImage.mockReturnValueOnce(save.promise);
    render(
      <div className={MESSAGE_CONTENT_CLASS}>
        <MarkdownRenderer
          text={`![Architecture](data:image/png;base64,${FIRST_PNG})`}
          reveal="instant"
        />
        <ImageBlock mime="image/gif" data={SECOND_GIF} />
      </div>,
    );

    await openPreview(screen.getByRole("button", { name: "Architecture" }));
    fireEvent.click(screen.getByRole("button", { name: "Download image" }));
    fireEvent.click(screen.getByRole("button", { name: "Next image" }));
    await failLater(save);

    expect(mocks.saveImage).toHaveBeenCalledWith(`data:image/png;base64,${FIRST_PNG}`);
    const dialog = screen.getByRole("dialog", { name: "Image preview" });
    const alert = within(dialog).getByRole("alert");
    expect(alert.textContent).toContain("Architecture");
    expect(alert.textContent).not.toContain("Image preview");
    expect(dialog.querySelector("img")?.getAttribute("src")).toBe(
      `data:image/gif;base64,${SECOND_GIF}`,
    );
  });

  it("navigates protocol images inside one message preview gallery", async () => {
    render(
      <div className={MESSAGE_CONTENT_CLASS}>
        <ImageBlock mime="image/png" data={FIRST_PNG} />
        <ImageBlock mime="image/gif" data={SECOND_GIF} />
      </div>,
    );

    await openPreview(screen.getAllByRole("button", { name: "View attached image" })[0]!);
    expect(screen.getByRole("dialog", { name: "Image preview" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Next image" }));

    expect(
      screen
        .getByRole("dialog", { name: "Image preview" })
        .querySelector("img")
        ?.getAttribute("src"),
    ).toBe(`data:image/gif;base64,${SECOND_GIF}`);
  });

  it("keeps protocol and Markdown images in the same message gallery", async () => {
    render(
      <div className={MESSAGE_CONTENT_CLASS}>
        <ImageBlock mime="image/png" data={FIRST_PNG} />
        <MarkdownRenderer
          text={`![Inline](data:image/gif;base64,${SECOND_GIF})`}
          reveal="instant"
        />
      </div>,
    );

    await openPreview(screen.getByRole("button", { name: "View attached image" }));
    fireEvent.click(screen.getByRole("button", { name: "Next image" }));

    expect(screen.getByRole("dialog", { name: "Inline" })).toBeTruthy();
  });

  it("uses the current image name when moving from named Markdown to an unnamed attachment", async () => {
    render(
      <div className={MESSAGE_CONTENT_CLASS}>
        <MarkdownRenderer
          text={`![Architecture](data:image/png;base64,${FIRST_PNG})`}
          reveal="instant"
        />
        <ImageBlock mime="image/gif" data={SECOND_GIF} />
      </div>,
    );

    await openPreview(screen.getByRole("button", { name: "Architecture" }));
    expect(screen.getByRole("dialog", { name: "Architecture" })).toBeTruthy();
    expect(screen.getByRole("region", { name: "Architecture" })).toBeTruthy();
    const name = screen.getByText("Architecture", { selector: "span" });
    expect(name.getAttribute("aria-hidden")).toBe("true");
    const position = screen.getByRole("status");
    expect(position.textContent).toBe("1 / 2");
    fireEvent.click(screen.getByRole("button", { name: "Next image" }));

    const dialog = screen.getByRole("dialog", { name: "Image preview" });
    expect(screen.getByRole("region", { name: "Image preview" })).toBeTruthy();
    expect(screen.getByText("Image preview", { selector: "span" })).toBeTruthy();
    expect(screen.queryByText("Architecture", { selector: "span" })).toBeNull();
    expect(position.textContent).toBe("2 / 2");
    expect(dialog.querySelector("img")?.getAttribute("src")).toBe(
      `data:image/gif;base64,${SECOND_GIF}`,
    );
    expect(screen.queryByRole("dialog", { name: "Architecture" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Previous image" }));
    expect(screen.getByRole("dialog", { name: "Architecture" })).toBeTruthy();
    expect(screen.getByRole("region", { name: "Architecture" })).toBeTruthy();
    expect(screen.getByText("Architecture", { selector: "span" })).toBeTruthy();
    expect(position.textContent).toBe("1 / 2");
  });

  it("shows a non-interactive decode failure and restores preview when the source changes", async () => {
    const { container, rerender } = render(<ImageBlock mime="image/png" data={FIRST_PNG} />);
    fireEvent.error(container.querySelector("img")!);

    const unavailable = screen.getByRole("img", { name: "Image unavailable" });
    expect(unavailable.textContent).toContain("Image unavailable");
    expect(screen.queryByRole("button", { name: "View attached image" })).toBeNull();
    expect(container.querySelector("img")).toBeNull();

    rerender(<ImageBlock mime="image/gif" data={SECOND_GIF} />);
    const preview = screen.getByRole("button", { name: "View attached image" });
    expect(preview.querySelector("img")?.getAttribute("src")).toBe(
      `data:image/gif;base64,${SECOND_GIF}`,
    );
    expect(screen.queryByRole("img", { name: "Image unavailable" })).toBeNull();
    await openPreview(preview);
    expect(screen.getByRole("dialog", { name: "Image preview" })).toBeTruthy();
  });

  it("keeps the focused zoom control while arrow keys move through the gallery", async () => {
    render(
      <div className={MESSAGE_CONTENT_CLASS}>
        <ImageBlock mime="image/png" data={FIRST_PNG} />
        <ImageBlock mime="image/gif" data={SECOND_GIF} />
      </div>,
    );
    const source = screen.getAllByRole("button", { name: "View attached image" })[0]!;
    source.focus();
    await openPreview(source);

    const dialog = screen.getByRole("dialog", { name: "Image preview" });
    const pan = dialog.querySelector<HTMLElement>("[data-image-zoom]")!;
    const position = within(dialog).getByRole("status");
    const zoomIn = within(dialog).getByRole("button", { name: /zoom in/i });
    const image = () => dialog.querySelector("img")!;
    let scrollLeft = 0;
    let scrollTop = 0;
    Object.defineProperty(pan, "scrollLeft", {
      configurable: true,
      get: () => scrollLeft,
      set: (value: number) => {
        scrollLeft = value;
      },
    });
    Object.defineProperty(pan, "scrollTop", {
      configurable: true,
      get: () => scrollTop,
      set: (value: number) => {
        scrollTop = value;
      },
    });

    zoomIn.focus();
    fireEvent.click(zoomIn);
    expect(pan.getAttribute("data-image-zoom")).toBe("125");
    scrollLeft = 40;
    scrollTop = 30;

    fireEvent.keyDown(zoomIn, { key: "ArrowRight" });
    expect(position.textContent).toBe("2 / 2");
    expect(image().getAttribute("src")).toBe(`data:image/gif;base64,${SECOND_GIF}`);
    expect(dialog.querySelector("[data-image-zoom]")).toBe(pan);
    expect(within(dialog).getByRole("button", { name: /zoom in/i })).toBe(zoomIn);
    expect(document.activeElement).toBe(zoomIn);
    expect(pan.getAttribute("data-image-zoom")).toBe("100");
    expect(within(dialog).getByText("100%")).toBeTruthy();
    expect([scrollLeft, scrollTop]).toEqual([0, 0]);

    // The last image is a boundary: the key does nothing, so its zoom is kept.
    fireEvent.click(zoomIn);
    fireEvent.keyDown(zoomIn, { key: "ArrowRight" });
    expect(position.textContent).toBe("2 / 2");
    expect(pan.getAttribute("data-image-zoom")).toBe("125");
    expect(document.activeElement).toBe(zoomIn);

    fireEvent.keyDown(zoomIn, { key: "ArrowLeft" });
    expect(position.textContent).toBe("1 / 2");
    expect(image().getAttribute("src")).toBe(`data:image/png;base64,${FIRST_PNG}`);
    expect(within(dialog).getByRole("button", { name: /zoom in/i })).toBe(zoomIn);
    expect(document.activeElement).toBe(zoomIn);
    expect(pan.getAttribute("data-image-zoom")).toBe("100");

    fireEvent.keyDown(zoomIn, { key: "ArrowLeft" });
    expect(position.textContent).toBe("1 / 2");
    expect(document.activeElement).toBe(zoomIn);

    fireEvent.click(within(dialog).getByRole("button", { name: /close/i }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(source));
  });

  describe("image source geometry", () => {
    const wideSource = `data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="240" height="96" viewBox="0 0 240 96"/>')}`;
    const tallSource = `data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="300" height="600" viewBox="0 0 300 600"/>')}`;
    const natural: Record<string, [number, number]> = {
      [wideSource]: [240, 96],
      [tallSource]: [300, 600],
    };
    // Models native WKWebView at page zoom 200%: a displayed image's natural getters follow its
    // rendered CSS size and page zoom, while a detached decoder reports the source once loaded.
    const pageZoom = 2;
    let decoders: HTMLImageElement[] = [];
    let loaded = new Set<HTMLImageElement>();

    const size = (image: HTMLImageElement) => natural[image.getAttribute("src") ?? ""] ?? [0, 0];
    const naturalGetter = (axis: 0 | 1, css: "width" | "height") =>
      function (this: HTMLImageElement) {
        if (decoders.includes(this)) return loaded.has(this) ? size(this)[axis] : 0;
        return (Number.parseFloat(this.style[css]) || size(this)[axis]) * pageZoom;
      };
    const decoderFor = (src: string) =>
      decoders.filter((decoder) => decoder.getAttribute("src") === src).at(-1)!;
    const load = (decoder: HTMLImageElement | undefined) => {
      if (!decoder) return;
      loaded.add(decoder);
      act(() => {
        decoder.dispatchEvent(new Event("load"));
      });
    };
    const renderGallery = () =>
      render(
        <div className={MESSAGE_CONTENT_CLASS}>
          <MarkdownRenderer
            text={`![Wide](${wideSource}) ![Tall](${tallSource})`}
            reveal="instant"
          />
        </div>,
      );

    beforeEach(() => {
      decoders = [];
      loaded = new Set();
      vi.stubGlobal("Image", function Image() {
        const decoder = document.createElement("img");
        decoders.push(decoder);
        return decoder;
      });
      const spies = [
        vi
          .spyOn(HTMLImageElement.prototype, "naturalWidth", "get")
          .mockImplementation(naturalGetter(0, "width")),
        vi
          .spyOn(HTMLImageElement.prototype, "naturalHeight", "get")
          .mockImplementation(naturalGetter(1, "height")),
        vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(400),
        vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockReturnValue(400),
      ];
      return () => {
        for (const spy of spies) spy.mockRestore();
        vi.unstubAllGlobals();
      };
    });

    it("refits each selected image from its own geometry", async () => {
      renderGallery();
      await openPreview(screen.getByRole("button", { name: "Wide" }));
      const dialog = screen.getByRole("dialog", { name: "Wide" });
      const pan = dialog.querySelector<HTMLElement>("[data-image-zoom]")!;
      pan.style.padding = "0px";
      const first = dialog.querySelector("img")!;
      const wideDecoder = decoderFor(wideSource);
      fireEvent.load(first);
      load(wideDecoder);
      expect([first.style.width, first.style.height]).toEqual(["240px", "96px"]);

      const zoomIn = within(dialog).getByRole("button", { name: /zoom in/i });
      fireEvent.click(zoomIn);
      expect([first.style.width, first.style.height]).toEqual(["300px", "120px"]);
      fireEvent.load(first);
      load(wideDecoder);
      expect([first.style.width, first.style.height]).toEqual(["300px", "120px"]);

      const next = within(dialog).getByRole("button", { name: "Next image" });
      next.focus();
      fireEvent.click(next);
      const second = dialog.querySelector("img")!;
      dialog.querySelector<HTMLElement>("[data-image-zoom]")!.style.padding = "0px";
      load(decoderFor(tallSource));
      expect(second).not.toBe(first);
      expect([second.style.width, second.style.height]).toEqual(["200px", "400px"]);
      expect(dialog.querySelector("[data-image-zoom]")?.getAttribute("data-image-zoom")).toBe(
        "100",
      );
      expect(document.activeElement).toBe(next);
      expect(dialog.querySelectorAll("img")).toHaveLength(1);

      fireEvent.click(within(dialog).getByRole("button", { name: "Previous image" }));
      const again = dialog.querySelector("img")!;
      dialog.querySelector<HTMLElement>("[data-image-zoom]")!.style.padding = "0px";
      const returning = decoderFor(wideSource);
      fireEvent.load(again);
      load(returning);
      expect([again.style.width, again.style.height]).toEqual(["240px", "96px"]);
    });

    it("drops source loads from items that are no longer selected or open", async () => {
      renderGallery();
      await openPreview(screen.getByRole("button", { name: "Wide" }));
      const dialog = screen.getByRole("dialog", { name: "Wide" });
      const pan = dialog.querySelector<HTMLElement>("[data-image-zoom]")!;
      pan.style.padding = "0px";
      const zoomIn = within(dialog).getByRole("button", { name: /zoom in/i });
      const wideDecoder = decoderFor(wideSource);

      fireEvent.click(within(dialog).getByRole("button", { name: "Next image" }));
      const tall = dialog.querySelector("img")!;
      const tallDecoder = decoderFor(tallSource);
      load(wideDecoder);
      expect(tall.style.width).toBe("");

      load(tallDecoder);
      expect([tall.style.width, tall.style.height]).toEqual(["200px", "400px"]);
      load(wideDecoder);
      expect([tall.style.width, tall.style.height]).toEqual(["200px", "400px"]);

      fireEvent.click(zoomIn);
      expect(dialog.querySelector("[data-image-zoom]")).toBe(pan);
      expect(within(dialog).getByRole("button", { name: /zoom in/i })).toBe(zoomIn);
      expect(pan.getAttribute("data-image-zoom")).toBe("125");
      expect(pan.hasAttribute("data-pannable")).toBe(true);
      expect([tall.style.width, tall.style.height]).toEqual(["250px", "500px"]);
      load(wideDecoder);
      expect([tall.style.width, tall.style.height]).toEqual(["250px", "500px"]);

      fireEvent.click(within(dialog).getByRole("button", { name: "Previous image" }));
      const closedDecoder = decoderFor(wideSource);
      fireEvent.click(within(dialog).getByRole("button", { name: /close/i }));
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

      await openPreview(screen.getByRole("button", { name: "Wide" }));
      const reopened = screen.getByRole("dialog", { name: "Wide" });
      reopened.querySelector<HTMLElement>("[data-image-zoom]")!.style.padding = "0px";
      const image = reopened.querySelector("img")!;
      const openDecoder = decoderFor(wideSource);
      load(closedDecoder);
      expect(image.style.width).toBe("");
      load(openDecoder);
      expect([image.style.width, image.style.height]).toEqual(["240px", "96px"]);
      load(closedDecoder);
      expect([image.style.width, image.style.height]).toEqual(["240px", "96px"]);
    });
  });

  it("resets viewing state for distinct gallery entries that share a source", async () => {
    render(
      <div className={MESSAGE_CONTENT_CLASS}>
        <MarkdownRenderer
          text={`![First](data:image/png;base64,${FIRST_PNG}) ![Second](data:image/png;base64,${FIRST_PNG})`}
          reveal="instant"
        />
      </div>,
    );
    await openPreview(screen.getByRole("button", { name: "First" }));
    const zoomIn = screen.getByRole("button", { name: "Zoom in image" });
    zoomIn.focus();
    fireEvent.click(zoomIn);
    expect(screen.getByText("125%")).toBeTruthy();

    fireEvent.keyDown(zoomIn, { key: "ArrowRight" });
    const second = screen.getByRole("dialog", { name: "Second" });
    expect(within(second).getByText("100%")).toBeTruthy();
    expect(within(second).getByRole("button", { name: "Zoom in image" })).toBe(zoomIn);
    expect(document.activeElement).toBe(zoomIn);

    fireEvent.click(zoomIn);
    fireEvent.keyDown(zoomIn, { key: "ArrowLeft" });
    const first = screen.getByRole("dialog", { name: "First" });
    expect(within(first).getByText("100%")).toBeTruthy();
    expect(document.activeElement).toBe(zoomIn);
  });

  it("does not include images from a nested message body", async () => {
    render(
      <div className={MESSAGE_CONTENT_CLASS}>
        <ImageBlock mime="image/png" data={FIRST_PNG} />
        <div className={MESSAGE_CONTENT_CLASS}>
          <ImageBlock mime="image/gif" data={SECOND_GIF} />
        </div>
      </div>,
    );

    await openPreview(screen.getAllByRole("button", { name: "View attached image" })[0]!);

    expect(screen.queryByRole("button", { name: "Next image" })).toBeNull();
    expect(screen.queryByRole("status")).toBeNull();
  });
});
