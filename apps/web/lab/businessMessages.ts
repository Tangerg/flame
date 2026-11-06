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
    progress: { "zh-CN": "示例进度", en: "Sample progress" },
    modelChoice: { "zh-CN": "选择示例模型", en: "Choose a sample model" },
    localModel: { "zh-CN": "本地示例模型", en: "Local sample model" },
    reasoningModel: { "zh-CN": "推理示例模型", en: "Reasoning sample model" },
    longModel: {
      "zh-CN": "用于长名称和窄宽度检查的本地示例模型",
      en: "Local sample model with a long name for narrow-width inspection",
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
