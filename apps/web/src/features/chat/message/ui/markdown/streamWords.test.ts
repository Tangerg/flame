import { expect, it, vi } from "vitest";
import * as segmentation from "@/lib/i18n/segmentWords";
import { createStreamWords } from "./streamWords";

it.each([
  "Earlier paragraph.\ncan't stop the final word growing",
  "中文说明。\n下一行，继续追加标点！？最后一个词",
  "ภาษาไทยกำลังอ่าน\nภาษาไทยต่อเนื่องไม่มีช่องว่าง",
  "A Unicode boundary\r\n。👩🏽‍💻 e\u0301 🇨🇳🇯🇵 العربية १२३",
])("retains Intl word semantics while the tail grows: %s", (source) => {
  const stream = createStreamWords();
  for (let end = 1; end <= source.length; end += 1) {
    const prefix = source.slice(0, end);
    stream.update(prefix);
    expect(stream.words).toEqual(segmentation.segmentWords(prefix));
    expect(stream.words.join("")).toBe(prefix);
  }
});

it("segments the mutable line and retires that cache on rollback or replacement", () => {
  const stream = createStreamWords();
  const prefix = "A committed first line.\nA committed second line.\n";
  stream.update(`${prefix}tail`);
  const segment = vi.spyOn(segmentation, "segmentWords");
  try {
    const change = stream.update(`${prefix}tail grows`);
    expect(segment).toHaveBeenCalledExactlyOnceWith("\ntail grows");
    expect(change.changedAt).toBe(prefix.length - 1);
    segment.mockClear();
    stream.update("A committed");
    expect(segment).toHaveBeenCalledExactlyOnceWith("A committed");
    segment.mockClear();
    stream.update("A replacement reply");
    expect(segment).toHaveBeenCalledExactlyOnceWith("A replacement reply");
  } finally {
    segment.mockRestore();
  }
});
