export function pluginMessageFrames(message: unknown, maxFrameBytes?: number): Generator<string> {
  const text = JSON.stringify(message);
  if (text === undefined) throw new Error("Plugin messages must be JSON values.");
  const overhead = JSON.stringify({
    type: "fragment",
    index: Number.MAX_SAFE_INTEGER,
    last: false,
    data: "",
  }).length;
  // One UTF-16 code unit can require six bytes when JSON escapes it.
  const capacity =
    maxFrameBytes === undefined ? text.length : Math.floor((maxFrameBytes - overhead) / 6);
  if (capacity < 1) throw new Error("Plugin carrier frame capacity is insufficient.");
  return (function* () {
    for (let offset = 0, index = 0; offset < text.length; offset += capacity, index++)
      yield JSON.stringify({
        type: "fragment",
        index,
        last: offset + capacity >= text.length,
        data: text.slice(offset, offset + capacity),
      });
  })();
}
