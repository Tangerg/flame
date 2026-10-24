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
