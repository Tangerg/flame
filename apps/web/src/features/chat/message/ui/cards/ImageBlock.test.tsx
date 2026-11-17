import { type FileSaveResult } from "@flame/component-sdk";
import { ImageSaveProvider } from "@/features/chat/public/imageSave";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient({ saveImage: mocks.saveImage });
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MESSAGE_CONTENT_CLASS } from "@/features/chat/message/ui/messageContent";
import { MarkdownRenderer } from "@/features/chat/message/ui/markdown/MarkdownRenderer";
import { ImageBlock } from "@/features/chat/message/ui/cards/ImageBlock";
const mocks = vi.hoisted(() => ({
  saveImage: vi.fn<(source: string) => Promise<FileSaveResult>>(),
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

async function failLater(save: PromiseWithResolvers<FileSaveResult>): Promise<void> {
  await act(async () => {
    save.reject(new Error("disk full"));
    await save.promise.catch(() => {});
  });
}

describe("ImageBlock", () => {
  it("delivers each mounted gallery's save to its own injected callback", async () => {
    const first = vi.fn(async () => "saved" as const);
    const second = vi.fn(async () => "saved" as const);
    testClient.render(
      <>
        <ImageSaveProvider saveImage={first}>
          <div className={MESSAGE_CONTENT_CLASS}>
            <ImageBlock mime="image/png" data={FIRST_PNG} />
          </div>
        </ImageSaveProvider>
        <ImageSaveProvider saveImage={second}>
          <div className={MESSAGE_CONTENT_CLASS}>
            <ImageBlock mime="image/gif" data={SECOND_GIF} />
          </div>
        </ImageSaveProvider>
      </>,
    );
    const previews = screen.getAllByRole("button", { name: "View attached image" });
    await openPreview(previews[0]!);
    let dialog = screen.getByRole("dialog", { name: "Image preview" });
    await settle(() =>
      fireEvent.click(within(dialog).getByRole("button", { name: "Download PNG" })),
    );
    expect(first).toHaveBeenCalledExactlyOnceWith(`data:image/png;base64,${FIRST_PNG}`);
    expect(second).not.toHaveBeenCalled();
    await settle(() => fireEvent.click(within(dialog).getByRole("button", { name: /close/i })));
    await openPreview(previews[1]!);
    dialog = screen.getByRole("dialog", { name: "Image preview" });
    await settle(() =>
      fireEvent.click(within(dialog).getByRole("button", { name: "Download PNG" })),
    );
    expect(second).toHaveBeenCalledExactlyOnceWith(`data:image/gif;base64,${SECOND_GIF}`);
    expect(first).toHaveBeenCalledOnce();
  });

  beforeEach(() => {
    mocks.saveImage.mockReset();
    mocks.saveImage.mockResolvedValue("saved");
  });

  it("saves the active gallery image through the packaged desktop owner", async () => {
    testClient.render(
      <div className={MESSAGE_CONTENT_CLASS}>
        <ImageBlock mime="image/png" data={FIRST_PNG} />
        <ImageBlock mime="image/gif" data={SECOND_GIF} />
      </div>,
    );

    await openPreview(screen.getAllByRole("button", { name: "View attached image" })[0]!);
    fireEvent.click(screen.getByRole("button", { name: "Next image" }));
    fireEvent.click(screen.getByRole("button", { name: "Download PNG" }));

    await waitFor(() => {
      expect(mocks.saveImage).toHaveBeenCalledWith(`data:image/gif;base64,${SECOND_GIF}`);
    });
  });

  it("keeps a failed save in the preview and clears it when the retry saves", async () => {
    mocks.saveImage.mockRejectedValueOnce(new Error("disk full"));
    testClient.render(<ImageBlock mime="image/png" data={FIRST_PNG} />);

    await openPreview(screen.getByRole("button", { name: "View attached image" }));
    const dialog = screen.getByRole("dialog", { name: "Image preview" });
    const download = within(dialog).getByRole("button", { name: "Download PNG" });
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
    mocks.saveImage.mockResolvedValueOnce("cancelled");
    testClient.render(<ImageBlock mime="image/png" data={FIRST_PNG} />);

    await openPreview(screen.getByRole("button", { name: "View attached image" }));
    const dialog = screen.getByRole("dialog", { name: "Image preview" });
    const download = within(dialog).getByRole("button", { name: "Download PNG" });
    await settle(() => fireEvent.click(download));

    expect(mocks.saveImage).toHaveBeenCalledOnce();
    expect(within(dialog).queryByRole("alert")).toBeNull();
    expect(download.hasAttribute("aria-describedby")).toBe(false);
  });

  it("keeps a failure from a closed preview out of the reopened one", async () => {
    const save = Promise.withResolvers<FileSaveResult>();
    mocks.saveImage.mockReturnValueOnce(save.promise);
    testClient.render(<ImageBlock mime="image/png" data={FIRST_PNG} />);

    await openPreview(screen.getByRole("button", { name: "View attached image" }));
    fireEvent.click(screen.getByRole("button", { name: "Download PNG" }));
    fireEvent.keyDown(screen.getByRole("dialog", { name: "Image preview" }), { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await openPreview(screen.getByRole("button", { name: "View attached image" }));
    await failLater(save);

    const dialog = screen.getByRole("dialog", { name: "Image preview" });
    expect(within(dialog).queryByRole("alert")).toBeNull();
    expect(
      within(dialog).getByRole("button", { name: "Download PNG" }).hasAttribute("aria-describedby"),
    ).toBe(false);
  });

  it("names the image whose save failed after moving to another image", async () => {
    const save = Promise.withResolvers<FileSaveResult>();
    mocks.saveImage.mockReturnValueOnce(save.promise);
    testClient.render(
      <div className={MESSAGE_CONTENT_CLASS}>
        <MarkdownRenderer text={`![Architecture](data:image/png;base64,${FIRST_PNG})`} />
        <ImageBlock mime="image/gif" data={SECOND_GIF} />
      </div>,
    );

    await openPreview(screen.getByRole("button", { name: "Architecture" }));
    fireEvent.click(screen.getByRole("button", { name: "Download PNG" }));
    fireEvent.click(screen.getByRole("button", { name: "Next image" }));
    await failLater(save);

    expect(mocks.saveImage).toHaveBeenCalledWith(`data:image/png;base64,${FIRST_PNG}`);
    const dialog = screen.getByRole("dialog", { name: "Image preview" });
    const alert = within(dialog).getByRole("alert");
    expect(alert.textContent?.startsWith("Could not save image")).toBe(true);
    expect(alert.textContent).toContain("Architecture");
    expect(alert.textContent).not.toContain("Image preview");
    expect(dialog.querySelector("img")?.getAttribute("src")).toBe(
      `data:image/gif;base64,${SECOND_GIF}`,
    );
  });

  it("navigates protocol images inside one message preview gallery", async () => {
    testClient.render(
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
    testClient.render(
      <div className={MESSAGE_CONTENT_CLASS}>
        <ImageBlock mime="image/png" data={FIRST_PNG} />
        <MarkdownRenderer text={`![Inline](data:image/gif;base64,${SECOND_GIF})`} />
      </div>,
    );

    await openPreview(screen.getByRole("button", { name: "View attached image" }));
    fireEvent.click(screen.getByRole("button", { name: "Next image" }));

    expect(screen.getByRole("dialog", { name: "Inline" })).toBeTruthy();
  });

  it("keeps the description reader and its focus while changing gallery images", async () => {
    const description =
      "Architecture review — Runtime transaction ownership, provider execution boundary, recovery checkpoints, and recorded evidence from the scheduling workspace";
    testClient.render(
      <div className={MESSAGE_CONTENT_CLASS}>
        <MarkdownRenderer text={`![${description}](data:image/png;base64,${FIRST_PNG})`} />
        <ImageBlock mime="image/gif" data={SECOND_GIF} />
      </div>,
    );
    await openPreview(screen.getByRole("button", { name: description }));
    const reader = screen.getByRole("region", { name: "Image description" });
    expect(reader.textContent).toBe(description);
    expect(reader.tabIndex).toBe(0);
    expect(reader.hasAttribute("aria-hidden")).toBe(false);
    reader.focus();
    fireEvent.keyDown(reader, { key: "ArrowRight" });
    expect(screen.getByRole("region", { name: "Image description" })).toBe(reader);
    expect(reader.textContent).toBe("Image preview");
    expect(document.activeElement).toBe(reader);
    expect(screen.getByRole("dialog", { name: "Image preview" })).toBeTruthy();
    fireEvent.keyDown(reader, { key: "ArrowLeft" });
    expect(reader.textContent).toBe(description);
    expect(document.activeElement).toBe(reader);
    expect(screen.getByRole("dialog", { name: description })).toBeTruthy();
  });

  it("uses the current image name when moving from named Markdown to an unnamed attachment", async () => {
    testClient.render(
      <div className={MESSAGE_CONTENT_CLASS}>
        <MarkdownRenderer text={`![Architecture](data:image/png;base64,${FIRST_PNG})`} />
        <ImageBlock mime="image/gif" data={SECOND_GIF} />
      </div>,
    );

    await openPreview(screen.getByRole("button", { name: "Architecture" }));
    expect(screen.getByRole("dialog", { name: "Architecture" })).toBeTruthy();
    expect(screen.getByRole("region", { name: "Architecture" })).toBeTruthy();
    const name = screen.getByText("Architecture", { selector: "span" });
    expect(name.getAttribute("role")).toBe("region");
    expect(name.getAttribute("aria-label")).toBe("Image description");
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
    const { container, rerender } = testClient.render(
      <ImageBlock mime="image/png" data={FIRST_PNG} />,
    );
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
    testClient.render(
      <div className={MESSAGE_CONTENT_CLASS}>
        <ImageBlock mime="image/png" data={FIRST_PNG} />
        <ImageBlock mime="image/gif" data={SECOND_GIF} />
      </div>,
    );
    const source = screen.getAllByRole("button", { name: "View attached image" })[0]!;
    source.focus();
    await openPreview(source);

    const dialog = screen.getByRole("dialog", { name: "Image preview" });
    const position = within(dialog).getByRole("status");
    const zoomIn = within(dialog).getByRole("button", { name: /zoom in/i });
    const image = () => dialog.querySelector("img")!;
    zoomIn.focus();
    fireEvent.click(zoomIn);

    fireEvent.keyDown(zoomIn, { key: "ArrowRight" });
    expect(position.textContent).toBe("2 / 2");
    expect(image().getAttribute("src")).toBe(`data:image/gif;base64,${SECOND_GIF}`);
    expect(within(dialog).getByRole("button", { name: /zoom in/i })).toBe(zoomIn);
    expect(document.activeElement).toBe(zoomIn);
    expect(within(dialog).getByText("100%")).toBeTruthy();

    // The last image is a boundary: the key does nothing, so its zoom is kept.
    fireEvent.click(zoomIn);
    fireEvent.keyDown(zoomIn, { key: "ArrowRight" });
    expect(position.textContent).toBe("2 / 2");
    expect(document.activeElement).toBe(zoomIn);

    fireEvent.keyDown(zoomIn, { key: "ArrowLeft" });
    expect(position.textContent).toBe("1 / 2");
    expect(image().getAttribute("src")).toBe(`data:image/png;base64,${FIRST_PNG}`);
    expect(within(dialog).getByRole("button", { name: /zoom in/i })).toBe(zoomIn);
    expect(document.activeElement).toBe(zoomIn);

    fireEvent.keyDown(zoomIn, { key: "ArrowLeft" });
    expect(position.textContent).toBe("1 / 2");
    expect(document.activeElement).toBe(zoomIn);

    fireEvent.click(within(dialog).getByRole("button", { name: /close/i }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(source));
  });

  it("preserves gallery controls across distinct entries that share a source", async () => {
    testClient.render(
      <div className={MESSAGE_CONTENT_CLASS}>
        <MarkdownRenderer
          text={`![First](data:image/png;base64,${FIRST_PNG}) ![Second](data:image/png;base64,${FIRST_PNG})`}
        />
      </div>,
    );
    await openPreview(screen.getByRole("button", { name: "First" }));
    const zoomIn = screen.getByRole("button", { name: "Zoom in image" });
    zoomIn.focus();
    fireEvent.click(zoomIn);

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
    testClient.render(
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
