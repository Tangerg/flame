export const businessMessages = {
  groups: {
    conversation: { "zh-CN": "会话与输入", en: "Conversation and input" },
    workspace: { "zh-CN": "工作区与数据", en: "Workspace and data" },
    scenarios: { "zh-CN": "完整业务场景", en: "Product scenarios" },
  },
  stories: {
    status: {
      title: { "zh-CN": "运行状态", en: "Run status" },
      summary: {
        "zh-CN": "直接比较 AgentStatusPill 的状态色与长文案。",
        en: "Compare the actual AgentStatusPill tones and longer labels.",
      },
    },
    navigation: {
      title: { "zh-CN": "工作索引行", en: "Work index rows" },
      summary: {
        "zh-CN": "切换选中项，检查详情、禁用与长名称。选择只保留在此示例中。",
        en: "Select local rows and inspect details, disabled controls and long names.",
      },
    },
    activity: {
      title: { "zh-CN": "活动详情", en: "Activity disclosure" },
      summary: {
        "zh-CN": "真实展开组件的行式、卡片、进度和失败展示。",
        en: "Exercise the production line, card, progress and failure presentations.",
      },
    },
    composer: {
      title: { "zh-CN": "输入区选择器", en: "Composer chip" },
      summary: {
        "zh-CN": "用真实弹层切换本地示例模型，检查容器宽度与禁用行为。",
        en: "Choose a local sample model through a real popover and inspect width and disabled behavior.",
      },
    },
    attachments: {
      title: { "zh-CN": "输入区附件", en: "Composer attachments" },
      summary: {
        "zh-CN": "在窄输入区检查读取中、失败的图片与文件引用；移除后回到同一输入框。",
        en: "Inspect reading and failed images and file references in a narrow composer; removal returns to the same input.",
      },
    },
    references: {
      title: { "zh-CN": "引用阅读", en: "Reference reading" },
      summary: {
        "zh-CN": "检查脚注、重复引用和返回原文的路径。",
        en: "Inspect supporting notes, repeated references and return to the answer.",
      },
    },
    reasoning: {
      title: { "zh-CN": "推理阅读", en: "Reasoning reader" },
      summary: {
        "zh-CN": "检查实时预览、正文阅读与收起后保留的阅读位置。",
        en: "Inspect live previews, supporting text and retained reading position.",
      },
    },
    catalog: {
      title: { "zh-CN": "面板目录", en: "Panel catalog" },
      summary: {
        "zh-CN": "搜索与选择生产目录组件中的本地条目。",
        en: "Search and select local entries in the production catalog component.",
      },
    },
    data: {
      title: { "zh-CN": "数据读取状态", en: "Data reading states" },
      summary: {
        "zh-CN": "固定输入驱动加载、空态、失败与成功；重试仅记录回调。",
        en: "Fixed inputs drive loading, empty, failed and loaded states; Retry only records its callback.",
      },
    },
    message: {
      title: { "zh-CN": "业务反馈", en: "Product feedback" },
      summary: {
        "zh-CN": "比较真实 SystemMessage 的语义色、可访问角色与操作。",
        en: "Compare SystemMessage semantics, accessible roles and actions.",
      },
    },
    files: {
      title: { "zh-CN": "文件标识", en: "File identity" },
      summary: {
        "zh-CN": "编辑本地路径，观察 FilePath 的目录、文件名与溢出展示。",
        en: "Edit a local path and inspect FilePath directory, filename and overflow presentation.",
      },
    },
    agent: {
      title: { "zh-CN": "Agent 场景", en: "Agent scenarios" },
      summary: {
        "zh-CN": "复用已有协议快照与生产投影；内存 transport 不连接 Runtime。",
        en: "Existing protocol snapshots and production projections over an in-memory transport; no live Runtime.",
      },
    },
    workspace: {
      title: { "zh-CN": "工作台场景", en: "Workspace scenarios" },
      summary: {
        "zh-CN": "复用真实 Dock 与安装组件，展示 Diff、Skills、Memory 等已有场景。",
        en: "Reuse the real Dock and installed components for existing Diff, Skills, Memory and other scenes.",
      },
    },
    settings: {
      title: { "zh-CN": "设置场景", en: "Settings scenarios" },
      summary: {
        "zh-CN": "真实设置组件使用离线数据；未建模的命令会明确拒绝。",
        en: "Production settings consume offline data; unmodelled commands explicitly refuse execution.",
      },
    },
    shell: {
      title: { "zh-CN": "工作索引场景", en: "Work index scenarios" },
      summary: {
        "zh-CN": "使用现有 Sidebar 投影检查已加载、空态、加载与失败。",
        en: "Inspect populated, empty, loading and failed states through existing Sidebar projections.",
      },
    },
  },
  variants: {
    all: { "zh-CN": "全部状态", en: "All states" },
    long: { "zh-CN": "长内容", en: "Long content" },
    default: { "zh-CN": "默认", en: "Default" },
    details: { "zh-CN": "详情", en: "Details" },
    disabled: { "zh-CN": "禁用", en: "Disabled" },
    line: { "zh-CN": "行式", en: "Line" },
    card: { "zh-CN": "卡片", en: "Card" },
    running: { "zh-CN": "生成中", en: "Streaming" },
    complete: { "zh-CN": "已完成", en: "Complete" },
    incomplete: { "zh-CN": "已中断", en: "Interrupted" },
    short: { "zh-CN": "短内容", en: "Short content" },
    progress: { "zh-CN": "进度", en: "Progress" },
    failure: { "zh-CN": "失败", en: "Failure" },
    compact: { "zh-CN": "窄容器", en: "Narrow container" },
    loading: { "zh-CN": "加载中", en: "Loading" },
    empty: { "zh-CN": "空态", en: "Empty" },
    ready: { "zh-CN": "已加载", en: "Loaded" },
    info: { "zh-CN": "信息", en: "Information" },
    warning: { "zh-CN": "警告", en: "Warning" },
    success: { "zh-CN": "成功", en: "Success" },
    unicode: { "zh-CN": "多语言路径", en: "Unicode path" },
    pending: { "zh-CN": "读取中", en: "Reading" },
    reference: { "zh-CN": "文件引用", en: "File reference" },
  },
  samples: {
    status: {
      idle: { "zh-CN": "等待输入", en: "Ready for input" },
      running: { "zh-CN": "正在运行", en: "Running" },
      waiting: { "zh-CN": "等待审批", en: "Waiting for approval" },
      ok: { "zh-CN": "已完成", en: "Completed" },
      err: { "zh-CN": "执行失败", en: "Failed" },
    },
    longStatus: {
      "zh-CN": "正在等待你确认工作区文件的修改权限",
      en: "Waiting for your approval to change workspace files",
    },
    selected: { "zh-CN": "当前示例选择", en: "Current sample selection" },
    overview: { "zh-CN": "项目概览", en: "Project overview" },
    tasks: { "zh-CN": "当前任务", en: "Current tasks" },
    history: { "zh-CN": "历史会话", en: "Previous sessions" },
    rowDetail: {
      "zh-CN": "保留的会话与工作区材料",
      en: "Retained sessions and workspace material",
    },
    longRow: {
      "zh-CN": "整理跨模块依赖关系并逐项检查任务中产生的文件修改",
      en: "Review cross-module dependencies and every file changed during this task",
    },
    activity: { "zh-CN": "检查工作区", en: "Inspect workspace" },
    activityDetail: { "zh-CN": "读取已记录的结果", en: "Read captured results" },
    activityBody: {
      "zh-CN": "已读取组件接口、主题令牌与交互约束。\n示例内容保留换行，展开后可以选择文字。",
      en: "Component interfaces, theme tokens and interaction contracts were read.\nThis sample preserves line breaks and supports text selection.",
    },
    activityFailure: {
      "zh-CN": "示例读取失败。\n  workspace/example.tsx\n  原始详情仍由生产组件展示。",
      en: "The sample read failed.\n  workspace/example.tsx\n  The production component retains the captured details.",
    },
    referenceAnswer: {
      "zh-CN":
        '# 带来源的结论\n\n已确认的观察支持当前结论。[^来源] 阅读[来源说明](https://example.test/contract "查看来源说明")可以了解上下文。\n\n继续比较材料时，再次引用同一份来源。[^来源] 返回后仍应停留在选中的这处引用。',
      en: '# A supported answer\n\nThe accepted observation supports the current conclusion.[^来源] The [source guide](https://example.test/contract "Read the source guide") supplies its context.\n\nCompare the same supporting material again.[^来源] Returning should keep this chosen reference in view.',
    },
    referenceShort: {
      "zh-CN": "保留原始观察及其对应的上下文。",
      en: "Keep the original observation with the context that supports it.",
    },
    referenceLong: {
      "zh-CN": [
        "来源说明保留已经观察到的材料，并说明它与当前结论之间的关系。读者可以先检查这份说明，再决定是否继续查看原始记录。这里的文字属于支持材料，使用较轻的视觉层级，但仍然需要保持清楚可读。引用的对象不会因为展开来源、调整窗口或改变字号而发生变化。",
        "比较多份材料时，需要区分原始记录和读者对记录的解释。同一段说明可以被正文中的不同位置引用，每个返回入口都应该回到对应的引用位置。来源内容较长时，读者可以在当前文档里继续阅读，不必关闭整个任务或重新寻找刚才的结论。",
        "完成比较后，返回原文并继续当前工作。保留已经确认的观察，明确仍然未知的部分，并让来源与结论之间的联系保持可见。返回入口应当直接可辨识，而不依赖悬停之后才出现的提示。较窄的窗口和较大的字号仍需保留完整的来源内容和各自的返回路径。",
      ].join("\n\n"),
      en: [
        "The source note retains the observed material and explains how it supports the current conclusion. Read this context before deciding whether to inspect the original record. Supporting text keeps a quieter hierarchy while remaining readable. Opening a source, resizing the window or changing the text size must not change the object being referenced. The note belongs to this answer and keeps its original identity throughout the reading task.",
        "When comparing several materials, distinguish the captured record from an interpretation of that record. Different parts of an answer can refer to the same source, and each return link must lead to its corresponding reference. A longer note remains readable within the current document. The reader should not need to close the task or search for the conclusion again merely because the supporting context takes more than one screen to read.",
        "After comparing the material, return to the answer and continue the current work. Retain confirmed observations, identify what remains unknown and keep the relationship between source and conclusion clear. Return links should be recognizable before hovering over them. Narrow windows and larger text must retain the full note and each distinct return path. The reader can then use the same source to check another part of the answer without starting the task over.",
      ].join("\n\n"),
    },
    reasoningShort: {
      "zh-CN": "已核对当前回复与来源材料之间的关系。",
      en: "The current reply and its supporting material have been compared.",
    },
    reasoningBody: {
      "zh-CN": [
        "当前回复需要区分已经观察到的结果和支持这个结果的材料。读者应当先理解结论，再决定是否展开细节，而不必从一长串过程记录里自行拼出答案。来源和结果之间的关系需要保持清楚。",
        "检查材料时，应当保留对象原来的身份和上下文。文件、工具记录和引用可以支持同一个判断，但它们表达的内容各不相同。展开其中一项时，其余材料仍然应该留在原来的阅读顺序中。",
        "未完成的工作需要保留原始输入。已经确认的结果可以继续阅读，还在等待的部分则保留自己的状态。新的内容到达时，不应当覆盖已经接受的记录，也不应当让读者失去刚才正在比较的位置。",
        "展开后的详情应当提供足够的阅读空间。较长的文字可以在自己的范围内滚动，短内容则按自然高度展示。无论内容长短，标题、展开入口和下一项材料之间的对齐关系都应当保持稳定。",
        "当读者停下来检查前文时，后续到达的内容仍然留在下方。关闭详情可以暂时回到结果摘要，再次展开时则继续刚才的阅读位置。这里不需要重新开始整个任务，也不需要重新输入问题。",
        "返回回复之后，原来的草稿仍然可用。读者可以继续补充说明、选择另一份材料，或者把刚才得到的信息用于下一步操作。这些操作应当沿用同一个对象和范围，避免把返回变成新的入口。",
        "文字本身也需要清楚可读。次要内容可以保持较轻的视觉层级，但阅读不应当依赖悬停。不同主题、较大的字号和较窄的窗口都需要保留完整内容及明确的展开和收起方式。",
        "最后，再次核对结论与证据是否一致。保留已经确认的观察，明确仍未解决的部分，并让读者能够回到来源继续检查。此时，当前回复和后续对话应当组成一条连续的阅读路径。",
      ].join("\n\n"),
      en: [
        "The current reply needs a clear distinction between an observed result and the material supporting it. Readers should understand the conclusion before deciding whether to open the details, without having to reconstruct the answer from a long sequence of activity records.",
        "Supporting material keeps its original identity and context. A file, a tool record and a reference may support the same decision while describing different observations. Opening one item should preserve the order and availability of the other material.",
        "Unfinished work retains its original input. Confirmed results remain available while pending observations keep their own state. New material should neither overwrite an accepted record nor displace the passage the reader is comparing.",
        "Expanded details need enough space for reading. Long text scrolls within its own window, while short content uses its natural height. Both preserve the alignment between the heading, the disclosure control and the next supporting item.",
        "When the reader pauses to inspect an earlier passage, new content stays below it. Closing the details returns to the summary. Reopening continues at the retained reading position, without requiring the task or question to be started again.",
        "Returning to the reply leaves the draft available. Readers can add context, choose another source or use the observation in their next action. These steps should retain the same object and scope so the return continues the current task.",
        "Supporting text still needs to be readable. A quieter hierarchy should not depend on hovering to reveal the words. Both themes, larger text and narrower windows retain the content and a clear way to open and close its details.",
        "Finally, compare the conclusion with its evidence once more. Keep confirmed observations, identify unresolved questions and preserve access to the original material. Reading the reply and continuing the conversation then remain one continuous path.",
      ].join("\n\n"),
    },
    progress: { "zh-CN": "示例进度", en: "Sample progress" },
    modelChoice: { "zh-CN": "选择示例模型", en: "Choose a sample model" },
    localModel: { "zh-CN": "本地示例模型", en: "Local sample model" },
    reasoningModel: { "zh-CN": "推理示例模型", en: "Reasoning sample model" },
    longModel: {
      "zh-CN": "用于长名称和窄宽度检查的本地示例模型",
      en: "Local sample model with a long name for narrow-width inspection",
    },
    attachmentDraft: {
      "zh-CN": "检查附件后继续编辑这份草稿。",
      en: "Keep this draft while reviewing attachments.",
    },
    catalog: { "zh-CN": "浏览面板", en: "Browse panels" },
    search: { "zh-CN": "搜索面板…", en: "Search panels…" },
    noMatches: { "zh-CN": "没有匹配的面板", en: "No matching panels" },
    clear: { "zh-CN": "清空搜索", en: "Clear search" },
    files: { "zh-CN": "文件", en: "Files" },
    changes: { "zh-CN": "修改对比", en: "Changes" },
    trajectory: { "zh-CN": "会话轨迹", en: "Session trajectory" },
    diagnostics: { "zh-CN": "诊断", en: "Diagnostics" },
    emptyTitle: { "zh-CN": "没有已记录的文件", en: "No captured files" },
    emptyDescription: {
      "zh-CN": "选择“已加载”变体查看本地示例条目。",
      en: "Select the Loaded variant to inspect local sample entries.",
    },
    errorTitle: { "zh-CN": "无法读取示例文件", en: "Could not read the sample files" },
    errorDescription: {
      "zh-CN": "此场景保留失败状态，重试操作记录在事件面板中。",
      en: "This scene retains its failure; Retry is recorded in the event panel.",
    },
    info: {
      "zh-CN": "此示例使用本地记录，无需连接 Runtime。",
      en: "This sample uses local records and needs no Runtime connection.",
    },
    warning: {
      "zh-CN": "保留的结果可能已过期，请检查源信息。",
      en: "The retained result may be stale; inspect its source.",
    },
    error: {
      "zh-CN": "读取未完成，原始输入仍然保留。",
      en: "The read did not complete; the original input is retained.",
    },
    success: { "zh-CN": "本地示例输入已更新。", en: "The local sample input was updated." },
    inspect: { "zh-CN": "检查详情", en: "Inspect details" },
    path: { "zh-CN": "示例路径", en: "Sample path" },
    pathPreview: { "zh-CN": "路径展示", en: "Path presentation" },
    readOnly: { "zh-CN": "仅预览", en: "Preview only" },
  },
} as const;
