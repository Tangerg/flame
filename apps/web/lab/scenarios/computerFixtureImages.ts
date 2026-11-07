export const COMPUTER_IMAGE_SAMPLES = [
  { id: "recorded-landscape", width: 1440, height: 900 },
  { id: "recorded-portrait", width: 900, height: 1440 },
  { id: "recorded-small", width: 240, height: 160 },
] as const;

// These synthetic PNGs make crop, aspect and source-reading errors visible without capturing a desktop.
export function createComputerFixtureImage({
  width,
  height,
}: (typeof COMPUTER_IMAGE_SAMPLES)[number]): string {
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const context = canvas.getContext("2d");
  if (!context) throw new Error("computer fixture needs a canvas image context");
  const inset = width < 400 ? 16 : 48;
  const fontSize = width < 400 ? 14 : 36;
  context.fillStyle = "#FAF9F6";
  context.fillRect(0, 0, width, height);
  context.fillStyle = "#FB6E30";
  for (const x of [inset, width - inset - 8]) {
    for (const y of [inset, height - inset - 8]) context.fillRect(x, y, 8, 8);
  }
  context.fillStyle = "#222222";
  context.font = `${fontSize}px sans-serif`;
  context.fillText("Recorded workspace", inset, inset + fontSize * 2);
  context.font = `${fontSize / 2}px monospace`;
  context.fillText(`${width} × ${height} · synthetic capture`, inset, inset + fontSize * 3);
  if (width > 400) {
    for (const [index, line] of [
      "Runtime boundary review",
      "Keep the original image and operation identity.",
      "Return to the selected record and unfinished draft.",
    ].entries()) {
      context.fillText(line, inset, inset + fontSize * (5 + index * 1.5));
    }
  }
  context.textAlign = "right";
  context.fillText("End of capture", width - inset, height - inset - fontSize);
  return canvas.toDataURL("image/png").split(",")[1]!;
}
