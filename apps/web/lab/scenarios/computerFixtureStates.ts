export const COMPUTER_FIXTURE_VARIANTS = [
  { id: "default", title: { en: "Control lease", "zh-CN": "控制授权" } },
  { id: "images", title: { en: "Recorded images", "zh-CN": "记录的图片" } },
  { id: "unreadable", title: { en: "Unreadable image", "zh-CN": "无法读取的图片" } },
  { id: "resources", title: { en: "Captured resources", "zh-CN": "捕获的资源" } },
  { id: "unreadable-audio", title: { en: "Unreadable audio", "zh-CN": "无法读取的音频" } },
  { id: "links", title: { en: "Resource links", "zh-CN": "资源链接" } },
] as const;

export type ComputerFixtureState = (typeof COMPUTER_FIXTURE_VARIANTS)[number]["id"];
