# Flame Desktop UI 深度对照审查与改动清单

审查日期：2026-09-23。对象：Flame desktop；参考：ZCode v3.14.3。

**共 49 项改动：2 项 P0、31 项 P1、16 项 P2。** 包含行为缺陷、交互契约修正、设计改进和待测优化；并非 49 个已运行复现的 bug。

## 1. 结论与边界

Flame 现在已经具备明确的 Agent 桌面产品结构和相当完整的 UI 基础设施。当前粗糙感主要来自三个方面：**信息展示的真实性与操作反馈仍有漏洞，日常工作链路存在断点，视觉规范与实际交付缺乏同一基线**。继续增加主题、面板或装饰，收益小于把“输入 → 执行 → 检查结果 → 继续工作”这条链路打磨完整。

ZCode 值得借鉴的是成熟交互的组织方法：输入候选如何接管键盘、附件如何审阅、长日志如何限制高度、文件和 diff 如何定位、异步保存如何保护编辑中的内容。实现应继续采用 Flame 已有的 React、StyleX、Base UI、插件扩展点和 Runtime 事实投影，不需要为这些改进迁移技术栈或复制 ZCode 的目录与协议。

### 1.1 固定版本

| 仓库 | 分支与提交 | 说明 |
| --- | --- | --- |
| Flame | main / `a7b29b7e43625692a5c61254b375756a2268aefe` | 已通过 GitHub 连接器核验，并完整克隆；最新提交将 reasoning effort 合并到模型选择器。 |
| ZCode | main / `328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f` | 已通过 GitHub 连接器核验，并完整克隆；版本 v3.14.3。 |

版本来源：[Flame 提交](https://github.com/Tangerg/flame/commit/a7b29b7e43625692a5c61254b375756a2268aefe)、[ZCode 提交](https://github.com/zai-org/ZCode/commit/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f)。以下源码链接固定到上述提交，避免后续 main 更新使结论失去对应关系。

### 1.2 适用约束

已读取 Flame 的 `AGENTS.md`、`PROJECT_RULES.md`、`DEVELOPMENT.md`、`DESIGN_PHILOSOPHY.md`、`desktop/README.md`、相关前端设计及工作区文档，并读取 ZCode 的适用约束与设计规范。

本报告沿用这些边界：Runtime 拥有 Session、Run、Goal、Plan、Interrupt、执行与持久化事实；Desktop 拥有选择、草稿、导航、展开状态和呈现。左侧仍为工作与会话索引，中间为 Agent 叙事，右侧为当前上下文材料。建议基于可确认的用户需求，不建立没有当前用途的通用框架。

依据：[Flame 项目规则](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/PROJECT_RULES.md)、[桌面约束](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/README.md)、[工作区模型](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/docs/FRONTEND_AGENT_WORKSPACE_MODEL.md)。文档中与当前实现不一致的部分，在清单中单独指出，未作为现有能力的证明。

### 1.3 证据等级与优先级

- **代码确认**：已追踪实现、调用方及相关现有测试；可从逻辑确定行为。它不等于已经在原生桌面中现场复现。
- **设计改进**：实现存在，但交互契约、可发现性或视觉层次值得调整；不把审美选择包装成 bug。
- **待实测**：真实像素效果、性能、闪烁时长、原生窗口行为等，需要浏览器或真机测量。

| 优先级 | 使用标准 |
| --- | --- |
| P0 | 用户输入可能丢失，或一个常规编辑操作可能触发非预期执行控制；先于外观调整处理。 |
| P1 | 高频路径存在误导、断点、状态丢失或明显可读性问题；第一轮正式打磨处理。 |
| P2 | 提高效率、发现性或专业完成度；在核心闭环稳定后处理。 |
| 后置 | 需要新增产品能力或 Runtime 契约；不能仅为接近参考产品而开工。 |

### 1.4 本次验证实际完成到哪里

Flame 按 lockfile 安装依赖成功，`npm run typecheck` 通过；模型选择、Composer 输入、Provider 配置、Diff 焦点等 **6 个现有测试文件、43 项测试通过**。ZCode 仓库 freshness 检查通过。两仓源码工作树均保持干净。

Flame 视觉开发入口输出了 ready 日志，但本环境缺少可用浏览器，锁定 Chromium 下载得到无效 ZIP；因此本次**没有取得当前 HEAD 的现场 UI 截图，也没有执行浏览器交互、WCAG、IME 或 WebKit 套件**。ZCode 未启动桌面或 Agent。视觉判断来自现有仓库快照与源码，后续验收明确列出了必须真机复测的项目。43 项已有测试通过，仅说明已有断言通过，不能替代下列缺口的回归覆盖。

特别注意：现有 `agent-light-narrative-darwin.png` 仍显示独立 `Medium` chip，而当前 `ModelPicker` 已合并 effort；Agent fixture 左侧的 `Agent states` 则是测试状态目录。因此未把旧版 Composer 或测试专用侧栏当成当前生产 UI 的缺陷。依据：[当前 ModelPicker](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/composer/ui/ModelPicker.tsx#L251-L258)、[视觉 fixture 说明](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/visual/README.md)、[测试专用状态栏](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/visual/VisualAgentStateFixture.tsx#L40-L108)。

## 2. 应保留的基础与对照原则

Flame 已有紧凑的生产侧栏、可折叠 Context Dock、上箭头发送、模型和推理强度选择、运行中 steer/stop、过程与最终回答分层、Markdown 表格与 Mermaid 放大、工具输出复制与长输出虚拟化，以及多主题、键盘、IME、视觉回归入口。改动应完善现有能力的边界，避免重复建设。

| 领域 | 从 ZCode 借鉴的具体方法 | Flame 的适配方向 |
| --- | --- | --- |
| 输入 | 候选菜单、附件预览、模式和控件宽度协调 | 在已有 Composer 与贡献点上补齐交互状态，先解决输入内键盘路径。 |
| 执行过程 | 日志摘要、展开体高度、失败与等待反馈 | 保留线性 Agent 叙事；分组只压缩密度，不丢状态和操作。 |
| 结果检查 | 文件、patch、每轮变更的明确归属 | 清楚区分执行当时结果与当前工作区；右侧承接完整检查。 |
| 布局 | 以真正可用阅读宽度决定收起和切换 | 保持紧凑左侧、留白中间、按需右侧，不增加永久功能列。 |
| 设置 | 草稿、保存中、测试中、成功与失败明确对应 | 操作结果必须关联用户实际编辑的版本，异常要能恢复。 |
| 复杂功能 | workflow、远程连接、完整编辑器等有专门产品路径 | 只有 Flame 的任务需要时才引入；不把参考产品的能力数量当 UI 质量。 |

下文每一项包含实现定位、适配建议与验收条件；建议部分表达本次审查判断，源码部分表达实际观察。

## 3. 输入区、会话叙事与工具展示（C01–C14）

### C01 · P1 · 分清历史工具结果与当前工作区内容

**代码确认／呈现契约修正。** Read 展开不消费 `tool.result` 和 range，而是重读当前文件前 40 行；卡头可能写读取 200–230 行，正文却是现在的 1–40 行。Grep 优先使用历史 hits，只有不能解析该结构时才重查。文档确实允许“文件头／重查”，所以这是需要修正的产品语义，不是没有任何设计依据的实现回归。[Flame · toolPreviewQueries.ts:6–49](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/tools/application/toolPreviewQueries.ts#L6-L49) [Flame · file.tsx:12–25](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/tools/previews/file.tsx#L12-L25)

**改动：** 展开历史记录时使用已经投影的 result 和真实范围；未保存完整结果就明说。单独的“查看当前文件”进入 Dock。ZCode 可借鉴的是将打开文件作为独立 action，以及区分预览和完整材料；它本身也不能被描述为所有文件打开都拥有历史快照。[Flame · projections.ts:244–257](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/agent/application/fold/projections.ts#L244-L257) [ZCode · read.tsx:254–309](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/ToolCallBlocks/renderers/read.tsx#L254-L309) [ZCode · ToolSnapshotFieldNotice.tsx:21–66](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/ToolCallBlocks/ToolSnapshotFieldNotice.tsx#L21-L66)

**验收：** 读 v1 的特定区间、随后改为 v2 或删除文件，历史展开仍是当时结果或明确缺席；“查看当前文件”才显示 v2；重启、切会话不改写过去的内容。

### C02 · P1 · 工具分组保留失败状态与操作能力

**代码确认。** 相邻 safe 工具会分组，但 `ToolGroupMember` 没有消费 error/denied/running 投影，也没有独立 ToolCard 的注册动作与 Dock opener。组会因 error 展开，但 read 失败可能没有明确“失败”与原因。不能说所有错误都消失：非零 exit code 的负面元信息仍可能显示。[Flame · messageRenderUnits.ts:87–137](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/agent/presentation/messageRenderUnits.ts#L87-L137) [Flame · ToolGroupMember.tsx:35–74](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/tools/ui/ToolGroupMember.tsx#L35-L74) [Flame · ToolCard.tsx:74–143](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/tools/ui/ToolCard.tsx#L74-L143)

**改动：** 独立行和分组行共用最小状态与动作投影；密度可不同，事实和能力一致。失败标记与一句原因可见，更多动作在 hover/focus 出现。ZCode 的紧凑摘要仍保留 status node 和文件操作可作参考。[ZCode · ToolSummaryRow.tsx:130–159](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/ToolCallBlocks/ToolSummaryRow.tsx#L130-L159)

**验收：** 同一 read 单独呈现与分组呈现时，失败、拒绝、运行、复制参数、打开目标均一致；分组不重新变成多层大卡片。

### C03 · P0 · Slash 候选先接管输入按键，避免误发与误停

**代码确认的误触路径。** `/` 候选只有前五项点击，没有输入框内 Up/Down、Enter/Tab 接受、Escape 关闭的完整模型。候选按钮可以通过 Tab 聚焦后激活，因此不能说“完全不能键盘操作”。问题在焦点仍留在输入框时：controller 只让 `@` 先处理，之后 Enter 提交、Escape stop。普通聊天中 `/g` 可被作为文本发出；运行中输入 `/g`，候选打开与 steer 同时可达，Escape 会走当前 session 的 stop 路径。[Flame · SlashSuggestions.tsx:20–44](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/composer/ui/SlashSuggestions.tsx#L20-L44) [Flame · useComposerInputController.ts:175–190](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/composer/ui/useComposerInputController.ts#L175-L190) [Flame · composerKeyBindings.ts:14–37](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/composer/application/composerKeyBindings.ts#L14-L37) [Flame · composerActionLayout.ts:14–17](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/composer/application/composerActionLayout.ts#L14-L17)

**改动：** 候选层拥有选中、接受和关闭语义，优先消费按键，保持编辑焦点；`/` 和 `@` 复用一致行为。Escape 首先关闭候选，不能顺带停止执行。匹配多于五项时可滚动查看，明确补全与真正执行命令的区别。ZCode 的候选键盘优先级可参考，具体行为仍通过 Flame 的原语实现。[ZCode · MentionPlugin.tsx:597–666](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/mentions/MentionPlugin.tsx#L597-L666)

**验收：** `/`、未完成前缀、精确命令、参数、无匹配、中文 composition、运行中 Escape 均覆盖；不会误发前缀或误停 Run。这里的 P0 表示交互改动前必须排除非预期执行控制，并非已在真实 Runtime 现场误停。

### C04 · P1 · 文件候选明确显示加载、失败与搜索范围

**代码确认。** `@` 只读取 data；items 为空就不打开面板，使加载中、失败、工作区未就绪和无结果看起来相同。候选从递归列表前 2,000 项中本地筛选，且只显示八项，范围限制也不可见。[Flame · fileMentions.ts:11–12](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/composer/application/fileMentions.ts#L11-L12) [Flame · fileMentions.ts:57–86](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/composer/application/fileMentions.ts#L57-L86) [Flame · FileMentionPopup.tsx:32–76](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/composer/ui/FileMentionPopup.tsx#L32-L76)

**改动：** 有有效 `@` token 就能打开候选壳，内部根据查询状态显示等待、结果、无结果、失败重试、工作区原因。真实大目录检索复用 Runtime 已有搜索或分页能力；不能取得完整范围时给出限制说明。ZCode 的 query/loading/error 分离有直接参考价值。[ZCode · fileMentionProvider.ts:53–105](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/mentions/providers/fileMentionProvider.ts#L53-L105) [ZCode · ChatPromptActionMenu.tsx:182–192](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/prompt-editor/ChatPromptActionMenu.tsx#L182-L192)

**验收：** 冷加载、无命中、目录失败、大于 2,000 文件、切换工作区后的旧请求返回均可解释；没有结果不再等于没有响应。

### C05 · P1 · 文件引用保留可靠身份，正确处理空格路径

**代码确认／输入表示改进。** 选中后插入原始 `@path`，DraftContext 又把任何 `@非空白` 解析成文件 chip。普通 `@abc` 可被装扮成文件；包含空格的路径只被识别一部分；同一个引用在 textarea 和上方托盘重复出现。[Flame · fileMentions.ts:88–95](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/composer/application/fileMentions.ts#L88-L95) [Flame · draftContext.ts:1–18](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/composer/application/draftContext.ts#L1-L18) [Flame · ComposerAttachments.tsx:58–75](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/composer/ui/ComposerAttachments.tsx#L58-L75)

**改动：** 区分“用户输入的普通文本”与“已选择文件引用”，定义能往返保存空格、中文和特殊字符的表示。chip 展示短名称、完整路径与可移除动作，投影从同一份草稿事实产生。ZCode 的 label/value/path 分离值得借鉴，但这不构成迁移 Lexical 的理由。[ZCode · fileMentionProvider.ts:9–25](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/mentions/providers/fileMentionProvider.ts#L9-L25)

**验收：** 空格路径、括号、中文、同名文件、重复引用、@姓名、撤销重做、粘贴及草稿恢复均不混淆文件身份。

### C06 · P2 · “+”菜单暴露已有的上下文能力

**设计改进。** 当前“+”直接选择图片，并在不支持图片的模型下禁用；用户无法从这里发现已有的文件引用和 Slash/recipe。[Flame · toolbar.tsx:25–50](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/composer/toolbar.tsx#L25-L50)

**改动：** 变成紧凑菜单，包含图片、引用项目文件、可用命令/recipe。模型能力只影响图片项，整个上下文入口保持可用。各项调用现有插件贡献和原有路径，不额外复制一套目录。参考 ZCode 的输入 action menu。[ZCode · ChatPromptActionMenu.tsx:126–194](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/prompt-editor/ChatPromptActionMenu.tsx#L126-L194)

**验收：** 不知道 `@`、`/` 的用户仅用鼠标也能引用文件；文本模型仍可用非图像上下文；插入位置和关闭后焦点正确。

### C07 · P1 · 附件可在发送前完整审阅，托盘高度受控

**代码确认／设计改进。** 图片只有 56px cover 缩略图与删除；大粘贴转 chip 后仅有前 160 字 tooltip，无全文预览或编辑。引用、图片、粘贴分别换行，正文虽有限高，附件合计没有高度预算。[Flame · ComposerAttachments.tsx:31–129](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/composer/ui/ComposerAttachments.tsx#L31-L129) [Flame · largePaste.ts:1–9](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/composer/domain/largePaste.ts#L1-L9) [Flame · composerStyles.ts:11–26](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/composer/ui/composerStyles.ts#L11-L26)

**改动：** 统一附件托盘；图片点击按原比例查看，文本可阅读全文、编辑或还原到输入。多材料使用有限高度或“+N 更多”，模型、权限、输入和发送保持可见。复用 Flame 现有图片预览能力。ZCode 的发送前媒体 gallery 可参考，但其 wrap 布局也不是容量问题的完整答案。[ZCode · ConversationComposer.tsx:1699–1809](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/v4/ConversationComposer.tsx#L1699-L1809)

**验收：** 3/10/30 个材料、多行正文、窄列组合下控件可达；长代码的开头和结尾都能检查，切会话不附错材料。

### C08 · P2 · 拖入反馈锚定 Composer，保留当前工作内容可见

**设计改进。** 当前图片拖入会出现整窗 scrim 与大型虚线区，遮挡正在参照的对话和代码。[Flame · ComposerImageDrop.tsx:25–85](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/composer/ui/ComposerImageDrop.tsx#L25-L85) [Flame · composerStyles.ts:30–53](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/composer/ui/composerStyles.ts#L30-L53)

**改动：** 可以全窗识别，但视觉反馈主要高亮真实输入落点，说明添加到哪个会话；项目文件引用与外部图片附件区分。已有对话框或明确 drop target 时尊重其所有权。ZCode 的局部输入壳反馈提供参考。[ZCode · ChatPromptEditor.tsx:324–360](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/prompt-editor/ChatPromptEditor.tsx#L324-L360)

**验收：** 拖入时仍看得见任务；进入子元素不闪烁，取消/离开提示消失；不支持图片时原因清楚。

### C09 · P1 · 长用户消息可折叠，保留完整输入操作

**设计改进。** 用户气泡最大 70% 宽并完整渲染 Markdown；大粘贴发送前是 chip，发送时又拼回完整正文，容易占多屏。[Flame · MessageBlock.tsx:92–116](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/message/ui/MessageBlock.tsx#L92-L116) [Flame · messageStyles.ts:75–89](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/message/ui/messageStyles.ts#L75-L89) [Flame · sendIntent.ts:23–25](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/composer/domain/sendIntent.ts#L23-L25)

**改动：** 基于实际溢出，对长用户输入提供约 5–7 行等效高度与“展开完整输入”，短消息保持原样；窄列适当放宽用户消息比例。复制、编辑、重跑始终取完整模型数据。ZCode 的用户输入溢出检测可参考。[ZCode · ConversationUserInputBody.tsx:7–21](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/v4/ConversationUserInputBody.tsx#L7-L21) [ZCode · ConversationUserInputBody.tsx:101–140](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/v4/ConversationUserInputBody.tsx#L101-L140)

**验收：** 长需求、代码和日志不淹没后续回答；展开、复制、编辑不丢尾部；若以后增加内容搜索，命中隐藏部分须能揭示。

### C10 · P1 · 推理默认轻量，主动展开由用户控制

**设计调整。** 目前 running reasoning 自动展开，并有约 240px 上限和完成后折叠。已有手动展开记忆与单行 glimpse，不能当成“缺少折叠”。[Flame · ReasoningBlock.tsx:51–128](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/message/ui/cards/ReasoningBlock.tsx#L51-L128)

**改动：** 默认呈现“思考中 + 一行片段”，用户需要时展开；保留用户决定，不因新 token 或终态强制关闭正在阅读的内容。ZCode 默认收起流式推理的做法有利于把空间留给正文与实际工具工作，可借鉴这一优先级。[ZCode · ConversationRowView.tsx:1592–1622](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/v4/ConversationRowView.tsx#L1592-L1622)

**验收：** 长推理期间主要阅读高度稳定；展开后能键盘滚动，手动选择不会被状态更新覆盖；更改现有自动展开测试与文档契约。

### C11 · P1 · 日志摘要显示新进展，展开高度保持一致

**代码确认。** 收起预览永远是前九行；展开 10–1,000 行全部进入 transcript，超过 1,000 行才变成限高虚拟窗口。于是中等日志最占空间，长日志反而突然变短，运行中的最新输出也不在摘要可见。[Flame · ToolOutputPanel.tsx:37–43](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/tools/public/previews/ToolOutputPanel.tsx#L37-L43) [Flame · ToolOutputPanel.tsx:102–159](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/tools/public/previews/ToolOutputPanel.tsx#L102-L159)

**改动：** running 摘要显示尾部和总量；展开无论多少行都遵守同一高度预算，虚拟化只是内部实现。用户上滚暂停跟随，并提供“回到最新”和局部查找；保留已有 ANSI、路径链接、完整复制。ZCode 的 ExecuteOutput 与后台输出面板可分别参考尾随和完整检查路径。[ZCode · ExecuteOutput.tsx:13–39](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/ToolCallBlocks/renderers/ExecuteOutput.tsx#L13-L39) [ZCode · BackgroundBashOutputSidePane.tsx:39–128](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/app-shell/BackgroundBashOutputSidePane.tsx#L39-L128)

**验收：** 9/10/100/999/1,000/1,001/10,000 行布局连续；最新进展可见，手动选中旧日志不被拖走，复制仍完整。工具输出保持只读记录，交互 PTY 是另一项产品能力。

### C12 · P2 · 输入过程中持续说明 steer 的作用

**设计改进。** running 且有文字时，主操作为 steer，但图标仍是上箭头，区别主要在 title；原先解释“引导当前回合”的 placeholder 在用户打字后消失。[Flame · composerActionLayout.ts:14–17](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/composer/application/composerActionLayout.ts#L14-L17) [Flame · send.tsx:55–75](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/composer/send.tsx#L55-L75) [Flame · useComposerInputController.ts:83–84](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/composer/ui/useComposerInputController.ts#L83-L84)

**改动：** 在 Composer 放一条轻量、非 hover 才可见的“补充到当前任务”提示，停止按钮位置稳定。提交后依据 Runtime 确认展示状态，不自行承诺立即生效。ZCode 的提交语义反馈可以借鉴，queue/guide/startNow 不作为 Flame 必须新增的模式。[ZCode · ConversationComposer.tsx:1598–1620](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/v4/ConversationComposer.tsx#L1598-L1620)

**验收：** 有输入时仍能知道作用对象；任务结束时提示与按钮同步变化；停止不清除草稿，不给已结束 Run 发送旧意图。

### C13 · P2 · 选区可以直接引用到输入，贯通答案、代码与右侧文件

**产品增强。** 现有消息菜单主要操作整条消息；文件和 diff 选区也没有加入 Composer 的路径。[Flame · MessageContextMenu.tsx:28–137](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/message/ui/MessageContextMenu.tsx#L28-L137) [Flame · FileView.tsx:39–59](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/workspace/workspace-views/views/FileView.tsx#L39-L59)

**改动：** 先实现一个“引用到输入”动作，引用正文/工具输出时记录来源消息，引用文件时记录路径和行范围，引用 diff 时明确左/右侧。草稿 chip 可检查全文、移除和回到来源；普通选择不自动发送。ZCode 有 Markdown 选区提示与文件行引用两条真实路径可参考。[ZCode · MarkdownSelectionTooltip.tsx:35–106](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/v4/MarkdownSelectionTooltip.tsx#L35-L106) [ZCode · PreviewPane.tsx:708–755](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/PreviewPane.tsx#L708-L755)

**验收：** 选择 10–20 行后可带范围追问；不会混入工具栏按钮文案；A 的引用不进入 B；复制快捷键不受影响。复用一个草稿引用模型，不额外建设持久化 review comment 系统。

### C14 · P2 · Mermaid 大图阅读与流式生成保持稳定

**代码确认／视觉效果待实测。** code 变化触发 300ms 等待，settling 时已有图也可回到 loading；是否频繁发生取决于流式围栏物化。已有放大是 Lightbox 展示 SVG，没有百分比、fit、平移等大图阅读控制。[Flame · MermaidBlock.tsx:36–50](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/message/ui/markdown/MermaidBlock.tsx#L36-L50) [Flame · MermaidBlock.tsx:146–217](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/message/ui/markdown/MermaidBlock.tsx#L146-L217)

**改动：** 生成阶段保持稳定源码/生成提示，完整后渲染，或明确保留上一次成功图；放大器加适应窗口、100%、缩放与平移，复用现有弹层。ZCode 的流式处理和图表弹窗可作参考。[ZCode · message.tsx:1562–1573](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/components/ai-elements/message.tsx#L1562-L1573) [ZCode · diagram-preview-dialog.tsx:603–648](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/components/ai-elements/diagram-preview-dialog.tsx#L603-L648)

**验收：** 流式长图无反复高度跳变；错误可看源码；复杂图能读清文字并恢复适应窗口；关闭后焦点正确返回。

## 4. 壳布局、导航与视觉层次（S01–S08）

### S01 · P1 · 统一首次启动的主题解析

**代码确认／可感知闪帧需真机验证。** 无持久化偏好时 HTML 与 Go 窗口跟随系统，appearance store 却默认 light，随后立即应用；深色系统存在 dark 初始化到 light 的状态切换。[Flame · index.html:18–41](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/index.html#L18-L41) [Flame · main.go:44–59](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/main.go#L44-L59) [Flame · appearanceStore.ts:36–50](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/theme/adapters/appearanceStore.ts#L36-L50) [Flame · documentAppearance.ts:157–165](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/theme/adapters/documentAppearance.ts#L157-L165)

**改动：** 统一产品默认与首帧规则。建议按 Flame 既有承诺采用 system；若最终选择默认 light，三处也必须一致。ZCode 值得借鉴的是入口默认一致，不是必须采用它的默认深色。[ZCode · index.ts:255–257](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/store/index.ts#L255-L257) [ZCode · useTheme.ts:86](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/useTheme.ts#L86)

**验收：** 干净偏好下浅/深 OS、已选 light/dark/system、损坏存储、重启都解析一致。显式主题与 OS 相反时的原生首帧另做录像评估，避免为消除一帧建立双重主题所有者。

### S02 · P1 · 主阅读区优先，窄窗仍能打开上下文材料

**代码推导／像素需实测。** 默认左侧 275px、Dock 480px，在 1120px 窗口中间只剩约 365px；左侧继续拉宽后，剩余行宽低于 672px 会隐藏 Dock 并禁用打开。已有响应逻辑，但左右两侧没有围绕主阅读统一协调。[Flame · shellGeometry.ts:1–11](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/lib/shellGeometry.ts#L1-L11) [Flame · ChatPanel.tsx:234–287](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/shell/kernel/panel/ChatPanel.tsx#L234-L287)

**改动：** 根据真正可用正文宽度协调左右面板。可从 420–480px 有效读区做样张评估；无法并排时，用全幅材料视图或覆盖式 Dock，并有明确返回对话入口。ZCode 会依据会话宽度渐进收起，并区分缩窗与用户主动打开的意图。[ZCode · WorkspaceShellLayout.tsx:459–524](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/app-shell/WorkspaceShellLayout.tsx#L459-L524)

**验收：** 1120/1280/1440px，左右栏最小/默认/最大组合下，发送、标题、Files/Diff 始终可达；自动收起不丢草稿、所选材料和阅读位置。若要支持屏幕分半，先完成单列/覆盖模式，再降低当前 1120×720 最小尺寸。

### S03 · P1 · 显式选择会话后，在左侧揭示当前项

**代码确认。** 列表只取前五项，active 只作用于已经可见的行；favorite 优先排序可把新打开的普通会话挤到后面，折叠项目也不会自动揭示它。[Flame · SessionList.tsx:14–40](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/sidebar/ui/SessionList.tsx#L14-L40) [Flame · buildWorkIndex.ts:15–17](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/navigation/application/buildWorkIndex.ts#L15-L17) [Flame · projects.tsx:38–58](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/sidebar/projects.tsx#L38-L58)

**改动：** 搜索、新建、通知等显式导航后，展开所属组并确保目标在可见范围；用户主动折叠时不要被每次渲染强行展开。ZCode 的受控展开和分页状态可以参考，但本次没有把它声称为完整 active reveal 的现成证明。[ZCode · WorkspaceSidebar.tsx:366–367](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/WorkspaceSidebar.tsx#L366-L367) [ZCode · workspaceTaskPagination.ts:24–43](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/lib/workspaceTaskPagination.ts#L24-L43)

**验收：** 五个以上置顶、50 个会话、折叠组场景下，都能找到刚打开的当前会话；普通态仍紧凑。

### S04 · P2 · “置顶”的文案和排序行为一致

**代码确认／产品规则修正。** Projects 按 favorite 置前，Recent 只按时间；同一菜单却承诺“置顶／Pin to top”，测试还固定了 Recent 不置前的行为。[Flame · buildWorkIndex.ts:15–17](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/navigation/application/buildWorkIndex.ts#L15-L17) [Flame · buildWorkIndex.ts:64–68](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/navigation/application/buildWorkIndex.ts#L64-L68) [Flame · buildWorkIndex.test.ts:96–106](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/navigation/application/buildWorkIndex.test.ts#L96-L106) [Flame · SessionRow.tsx:161–166](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/sidebar/ui/SessionRow.tsx#L161-L166)

**改动：** 统一置顶排序，或提供一致的 Pinned 分区；若只作收藏，就改准确文案并给明确查找入口。继续使用 Runtime 的 favorite，不新建 pin 事实。ZCode 的独立 Pinned 区是一个可选参考。[ZCode · WorkspaceSidebar.tsx:117](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/WorkspaceSidebar.tsx#L117)

**验收：** Projects/Recent 对同一动作结果一致、重启一致；同步改排序、文案与现有测试。

### S05 · P2 · 会话行提供克制的“更多”按钮

**可发现性改进。** 重命名、分叉、置顶、删除都已经存在，但集中在右键菜单；行内没有可见 action。现有 AgentRow 已能在 hover/focus 揭示操作。[Flame · SessionRow.tsx:119–189](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/sidebar/ui/SessionRow.tsx#L119-L189) [Flame · navigation-row.tsx:180–190](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/ui/agent/navigation-row.tsx#L180-L190)

**改动：** 只增加一个 `…`，与右键共用同一动作定义；鼠标经过、键盘聚焦或无 hover 设备时可达。ZCode 的侧栏更多入口可参考，不照搬全部动作。[ZCode · WorkspaceSidebarItem.tsx:904–923](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/WorkspaceSidebarItem.tsx#L904-L923)

**验收：** 左键用户可找到会话管理；菜单内操作不误切会话；Esc 关闭回到原行；默认不铺满四个图标。

### S06 · P1 · 项目折叠后仍显示“需要你处理”的信息

**设计改进。** 会话有 running/waiting 点和可访问名称，但项目收起后只有总会话数，没有等待聚合。[Flame · SessionRow.tsx:95–102](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/sidebar/ui/SessionRow.tsx#L95-L102) [Flame · SessionRow.tsx:134–145](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/sidebar/ui/SessionRow.tsx#L134-L145) [Flame · ProjectRow.tsx:33–39](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/sidebar/ui/ProjectRow.tsx#L33-L39)

**改动：** 给 waiting 短文案或明确形状，折叠项目展示“待处理 1”；普通运行可继续小点，idle 保持安静。只聚合现有 Runtime summary，不能凭 UI 自造未读或故障状态。ZCode 保留折叠组注意信息的做法有参考价值。[ZCode · WorkspaceSidebarItem.tsx:749–769](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/WorkspaceSidebarItem.tsx#L749-L769)

**验收：** 多项目下不逐个展开即可找到等待用户的任务；解除后消失，浅深色及不依赖颜色时都可辨。

### S07 · P2 · 代码字号独立，正文与界面密度分别评审

**设计改进。** 当前 prose/code 都跟 UI base 联动，默认正文 14px、代码 12px；设置有代码字体但没有独立代码字号，改变 UI 字号还带动图标。[Flame · typeLadder.ts:16–23](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/theme/kit/typeLadder.ts#L16-L23) [Flame · FontSection.tsx:97–121](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/settings/appearance/ui/FontSection.tsx#L97-L121) [Flame · documentAppearance.ts:137–141](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/theme/adapters/documentAppearance.ts#L137-L141)

**改动：** 优先让 Markdown code、Diff、日志等技术正文拥有独立代码字号；导航和控件继续紧凑。内容阅读字号 15–16px 可以作评审样张，14px 本身不是 bug，也不应为阅读整体放大所有间距。ZCode 已区分 code preview font size 和 UI font size。[ZCode · index.ts:273–293](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/store/index.ts#L273-L293) [ZCode · DESIGN.md:188–214](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/DESIGN.md#L188-L214)

**验收：** 只调代码字号不改变导航行高/图标，UI 字号不覆盖代码偏好；中英长文、长 diff 在窄列仍可读。

### S08 · P2 · 标题确实溢出才渐隐，统一完整标题揭示节奏

**代码确认／视觉优化。** OverflowLabel 已测量溢出，却始终添加末尾 16px mask；恰好装得下的标题也可能末尾变淡。hover mask、延迟滚动、tooltip 又分别使用不同时间。[Flame · overflow-label.tsx:30–65](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/ui/agent/overflow-label.tsx#L30-L65) [Flame · globals.css:1112–1143](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/styles/globals.css#L1112-L1143)

**改动：** 仅真实溢出时启用渐隐；tooltip 和滚动标题确定一个主要揭示方式，避免光标经过即遮首字。ZCode 对 mask 的条件处理可直接借鉴，不必复制其循环跑马灯。[ZCode · TaskTitleOverflowText.tsx:56–63](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/components/TaskTitleOverflowText.tsx#L56-L63) [ZCode · TaskTitleOverflowText.tsx:194–201](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/components/TaskTitleOverflowText.tsx#L194-L201)

**验收：** 比容器短 1–15px 的标题完整可见；中英长名、键盘 focus、减少动画、实时 resize 都能看全且无互相覆盖的提示。

## 5. 文件、Diff、搜索和成果检查（W01–W12）

### W01 · P1 · 文件截断之后可以继续读

**代码确认／闭环缺口。** 从指定行打开文件时只读前后各 200 行，截断后只有提示，工具栏主要动作是回到 Files。没有前后续读、任意跳行或外部打开出口；Runtime 已有行范围与字节上限能力。[Flame · file.tsx:59–114](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/workspace/workspace-views/file.tsx#L59-L114) [Flame · workspace_fs.go:82–108](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/runtime/internal/delivery/workspace_fs.go#L82-L108)

**改动：** 明确当前显示行范围，提供前后加载、转到行、复制路径、外部打开；保留真实上限。总行数只有已知才展示。ZCode 的大文件降级和编辑器出口可参考，不能误解为无限下载全文。[ZCode · PreviewPane.tsx:209–229](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/PreviewPane.tsx#L209-L229) [ZCode · PreviewPane.tsx:1699–1725](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/PreviewPane.tsx#L1699-L1725)

**验收：** 从搜索打开第 450 行后，可继续看到 700 行；超限、二进制、读失败都有明确可操作出口，重试不会退回目录。

### W02 · P1 · 文件自动刷新保持用户阅读位置

**代码确认。** FileView 的定位 effect 依赖 content；文件变更事件刷新读取内容后，会再次滚回旧 targetLine。用户从搜索跳行后手动读别处，Agent 再改文件就可能打断阅读。[Flame · FileView.tsx:34–37](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/workspace/workspace-views/views/FileView.tsx#L34-L37) [Flame · eventInvalidation.ts:59–74](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/workspace/domain/eventInvalidation.ts#L59-L74)

**改动：** 只有新的显式导航意图执行定位；数据更新保持阅读锚点，可提示“文件已更新”。同路径同一行的再次点击仍应生效，不能仅根据 path/line 值判断。ZCode 对首次定位与后续滚动的区分可参考。[ZCode · PreviewPane.tsx:780–804](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/PreviewPane.tsx#L780-L804)

**验收：** 手动滚动后 files.changed 不拉回；重复点击同一搜索结果可再次定位；行被删除时状态可解释。

### W03 · P1 · 二进制与零文本增删也显示 Review 入口

**代码确认。** HeaderDiffStat 用 added/removed 都为零决定隐藏，因此仅 binary、纯 rename、空文件变更可能没有快捷入口；已有 diff tab badge 则按文件数判断，两处规则不一致。[Flame · HeaderDiffStat.tsx:19–26](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/shell/kernel/panel/HeaderDiffStat.tsx#L19-L26) [Flame · tabBadges.tsx:22–29](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/workspace/workspace-views/tabBadges.tsx#L22-L29)

**改动：** 入口以真实 changed files 是否非空为准，`+/-` 是补充；非文本变更显示文件数量。ZCode 的变更导航同样以文件来源集合为基础。[ZCode · useGitRepository.ts:164–192](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/hooks/useGitRepository.ts#L164-L192)

**验收：** 仅 PNG、仅重命名、仅新增空文件都能从 header 进入 Review；工作树干净时不显示假变更。

### W04 · P1 · Patch 每个文件可检查，“还有 N 个”可展开

**代码确认／闭环缺口。** Patch 行是静态路径，最多显示九条，“还有 N 个”也是纯文本；单个文件无法直接点开检查。[Flame · patch.tsx:51–104](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/tools/previews/patch.tsx#L51-L104) [Flame · previewChrome.tsx:5–10](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/tools/previews/previewChrome.tsx#L5-L10)

**改动：** 每条成为可达动作，超出部分可展开；重命名显示旧→新，删除进入对应 diff。若现在只能打开当前工作树，就在动作上明说，不能暗示那次 patch 的历史内容。ZCode 按具体 FileChangeItem 打开 patch 的做法可参考。[ZCode · ConversationFileSummaryPanel.tsx:48–67](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/v4/ConversationFileSummaryPanel.tsx#L48-L67)

**验收：** 一次修改 15 个文件，第 15 个可找到、可打开；重命名和删除目标正确，操作保持中间会话。

### W05 · P1 · 显式打开 Diff 文件时展开目标

**代码确认。** 当前 fileFocus 只滚动并消费 revision，没有移除 collapsedFiles 中的目标；正文仍折叠。现有定位不是完全失效，而是打开动作没有完成最后一步。[Flame · diff.tsx:85–127](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/workspace/workspace-views/diff.tsx#L85-L127)

**改动：** 新的“打开此文件”意图负责展开、滚动、短暂定位反馈；普通数据刷新继续尊重用户折叠。ZCode 在命中折叠文件时先展开再定位可参考。[ZCode · GitPane.tsx:337–376](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/GitPane.tsx#L337-L376)

**验收：** 折叠 a.ts 后从工具再次打开能直接看内容；同路径重复意图有效；无关刷新不展开其它文件。

### W06 · P1 · Review 增加轻量文件导航、改动类型与定位

**产品增强。** 当前 diff 纵向渲染所有文件，没有文档描述的可筛选变更文件导航；文件标题不 sticky，header 投影还没有使用已存在的 status。已有 unified/split、baseline、rename/binary 处理应保留。[Flame · diff.tsx:176–215](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/workspace/workspace-views/diff.tsx#L176-L215) [Flame · viewStyles.ts:40–55](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/workspace/workspace-views/views/viewStyles.ts#L40-L55) [Flame · diffViewModel.ts:24–29](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/workspace/application/diffViewModel.ts#L24-L29)

**改动：** 先加文件 picker/筛选、上下一文件或变更区、全部折叠/展开、sticky 文件身份和新增/修改/删除/重命名类型。宽布局可有窄目录，窄 Dock 用 picker；不要永久再挤一棵树。ZCode 的查找定位与 sticky change card 值得参考。[ZCode · GitPane.tsx:282–376](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/GitPane.tsx#L282-L376) [ZCode · GitPaneChangeCard.tsx:88–124](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/GitPaneChangeCard.tsx#L88-L124)

**验收：** 50 个文件可以按名字快速定位，滚动长 diff 不丢文件身份；binary、空文件类型明确；窄布局操作栏不挤成多排。

### W07 · P1 · 明确当前工作树与本次执行变更的范围

**代码确认／来源语义。** 历史编辑工具进入的是 cwd/mode 对应的当前 workspace diff，查询没有 source run/item。Header 的工作树增删也可能包含用户原有修改。[Flame · toolRouting.ts:16–23](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/workspace/application/toolRouting.ts#L16-L23) [Flame · diffViewModel.ts:31–37](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/workspace/application/diffViewModel.ts#L31-L37)

**改动：** 立即准确标记“当前工作树”“相对某基线”，不要把它包装成 Agent 此次成果。真正“本次执行的变更”仅在 Runtime 能给出来源确定的材料时添加，不能 UI 按时间猜。ZCode 的按 row/entity 请求每轮 file changes 提供参考。[ZCode · ConversationFileSummaryPanel.tsx:100–140](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/v4/ConversationFileSummaryPanel.tsx#L100-L140)

**验收：** Agent 执行前已有人工修改时，工作树范围明确包含它；历史回顾不被当前工作区悄悄改写。此项先做来源标注，完整 per-Run diff 为后续 Runtime 契约工作。

### W08 · P1 · Files 提供选中、定位和完整路径反馈

**产品增强。** 文件树已有递归按钮和展开，但缺 selectedPath、从外部链接 reveal 祖先、路径复制、长名全文提示、文件类型/Git 小标。按钮可键盘激活，不等于已经具备完整 tree 键盘模型。[Flame · FileTree.tsx:24–105](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/workspace/workspace-views/views/FileTree.tsx#L24-L105)

**改动：** 优先选中与“在目录中定位”、复制相对路径、完整名称、类型图标；有真实 Git facts 再显示状态。文件名过滤/Changed only 可后续补。ZCode 的 reveal/树行反馈可参考，键盘实现遵守 Flame 原语约束。[ZCode · WorkspaceFileTree.tsx:285–342](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/workspace-file-tree/WorkspaceFileTree.tsx#L285-L342) [ZCode · WorkspaceFileTreeRowView.tsx:104–158](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/workspace-file-tree/WorkspaceFileTreeRowView.tsx#L104-L158)

**验收：** 从深层文件打开后可回树找到它，同名不同目录可区分，左右/Enter 操作清晰，窄栏能获取完整路径。

### W09 · P1 · 成果预览支持 Markdown、图片和常用路径动作

**产品增强。** Files 统一走文本读取和代码行，Markdown 显示源码，图片因文本读取不支持而进入错误；toolbar 出口有限。聊天本身已经有 Markdown 和图片能力，应复用。[Flame · file.tsx:55–116](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/workspace/workspace-views/file.tsx#L55-L116) [Flame · file_browser.go:89–105](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/runtime/internal/adapter/workspace/file_browser.go#L89-L105)

**改动：** 优先 Markdown 预览/源码、图片预览或明确支持状态、代码折行、复制路径、系统显示/外部打开；读取仍走 Runtime 或平台公开边界。PDF/Office/音视频按实际任务再增加。ZCode 的格式分派与预览工具栏可参考，不需要搬完整编辑器。[ZCode · previewPaneContent.tsx:195–321](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/previewPaneContent.tsx#L195-L321) [ZCode · PreviewPane.tsx:1608–1725](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/PreviewPane.tsx#L1608-L1725)

**验收：** README 的表格/链接可直观看，图片可检查或得到明确出口，不能处理的格式不会只剩模糊通用报错。

### W10 · P2 · 切会话后恢复材料阅读状态

**产品增强。** Dock tabs 和当前 file path/line 已按 session 保存；但 Search query、树展开、Diff mode/layout/collapsed 是局部状态，session key 重建后丢失。不能误说同一会话内切 Dock tab 也都会丢，它们已有保活。[Flame · contextDockStore.ts:10–59](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/workspace/adapters/contextDockStore.ts#L10-L59) [Flame · ChatPanel.tsx:60–66](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/shell/kernel/panel/ChatPanel.tsx#L60-L66) [Flame · ChatPanel.tsx:256–278](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/shell/kernel/panel/ChatPanel.tsx#L256-L278)

**改动：** 只保存用户可感知的阅读与查询状态，增加轻量文件历史/前进后退；真实多文件对比需求出现后再加固定文件 tabs。URL 所有的当前选中标量仍归 URL，store 不建第二份。ZCode 的材料滚动恢复与 tab overview 可参考。[ZCode · PreviewPane.tsx:1433–1473](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/PreviewPane.tsx#L1433-L1473) [ZCode · SidePaneTabOverview.tsx:29–120](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/app-shell/SidePaneTabOverview.tsx#L29-L120)

**验收：** A 看文件 300 行、查询 X，去 B 再回 A 能继续；相同相对路径不串 session；关闭会话清理记忆，路径失效有说明。

### W11 · P1 · 搜索可以缩小范围，并突出实际命中

**产品增强。** 当前只有输入框、300ms debounce、200 条上限、分文件结果与跳行。snippet 整行截断，无命中高亮；超限要求缩小范围但没有目录输入。query 类型已有 path 能力。[Flame · search.tsx:21–90](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/workspace/workspace-views/search.tsx#L21-L90) [Flame · workspaceQueries.ts:123–128](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/workspace/application/workspaceQueries.ts#L123-L128)

**改动：** 提供目录/文件范围、工作区路径、命中周围文字与高亮、上下命中、文件组折叠；超限提供可执行的缩小范围动作。regex/case/whole-word 只有 Runtime 提供明确语义才加。ZCode 的文件过滤与 Git 内容命中交互可参考，但不能把它的文件名搜索等同 Flame grep。[ZCode · WorkspaceFileTree.tsx:148–189](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/workspace-file-tree/WorkspaceFileTree.tsx#L148-L189) [ZCode · GitPane.tsx:282–376](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/GitPane.tsx#L282-L376)

**验收：** 500 命中能在界面收窄到 src/，长行看得见命中位置；键盘可定位，无结果/失败/查询中区分，已有跳行链路保留。

### W12 · P2 · 计划与成果提供来源，避免额外待办体系

**产品增强／部分依赖 Runtime。** Plan 已正确展示步骤和进度，但 read model 只有 steps/done/total，PlanStep 也没有工具/文件依据；不能前端猜步骤与成果的关联。[Flame · plan.tsx:9–27](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/workspace/workspace-views/plan.tsx#L9-L27) [Flame · planViewModel.ts:18–30](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/workspace/application/planViewModel.ts#L18-L30) [Flame · agentSessionView.ts:6–15](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/sdk/types/agentSessionView.ts#L6-L15)

**改动：** 先在事实可用时提供所属执行、查看对话来源、当前步骤定位；成果由实际文件引用和 change receipt 进入 Files/Review。未来 workflow 真的产生 artifact 后再展示其版本、类型和读取状态。ZCode 的 PlanDetailSidePane 和 Runtime artifact 面板展示了有明确生产者的做法。[ZCode · PlanDetailSidePane.tsx:35–74](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/app-shell/PlanDetailSidePane.tsx#L35-L74) [ZCode · WorkflowArtifactSidePane.tsx:51–103](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/app-shell/WorkflowArtifactSidePane.tsx#L51-L103)

**验收：** 能辨认计划属于哪个执行，当前项可找到；完成/版本/来源只根据 Runtime 事实；文件未创建或删除时有准确反馈，不让用户手动勾选伪装成 Agent 已完成。

## 6. 设置、异常恢复和桌面完成度（R01–R12）

### R01 · P1 · 测试结果必须对应眼前的 Provider 配置

**代码确认／交互语义修正。** Test 只传已保存 provider id，dirty 时仍可点；草稿字段变化也不清旧成功反馈。现有测试明确在 URL 草稿改动后得到 Connection OK，因此需要更改现有产品契约，而非声称 Runtime 测错 provider。[Flame · ProviderRow.tsx:78–168](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/settings/providers/ui/ProviderRow.tsx#L78-L168) [Flame · ProviderRow.test.tsx:78–95](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/settings/providers/ui/ProviderRow.test.tsx#L78-L95)

**改动：** 首选“保存并测试”，保存成功才测试该权威版本；也可 dirty 时禁用并明确“测试已保存配置”。测试期间再编辑，旧结果失效或明确对应旧版本。ZCode 会先提交 pending draft 再测试，可借鉴该顺序。[ZCode · InlineEditableProviderCard.tsx:614–629](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/settings/model-provider-section/InlineEditableProviderCard.tsx#L614-L629)

**验收：** A 可连、B 不可连，编辑 B 不能得到仿佛针对 B 的 A 成功；保存失败不继续 test，后续 C 草稿不继承 B 的结果。

### R02 · P0 · 保存响应不得覆盖保存期间继续输入的草稿

**代码确认的输入丢失路径。** 保存 await 期间两个字段仍可编辑，返回后无条件重建整个 draft；提交 A、继续输入 B、A 返回会覆盖 B。清 key 返回也存在同类 reset。[Flame · ProviderRow.tsx:48–72](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/settings/providers/ui/ProviderRow.tsx#L48-L72) [Flame · ProviderRow.tsx:108–128](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/settings/providers/ui/ProviderRow.tsx#L108-L128)

**改动：** 在草稿边界记录本次提交版本，仅清理提交成功且没有再变的字段；新输入继续 dirty。也可短暂锁定相关输入作为更简单的明确设计。切 pane 的草稿保留/提示策略统一；API key 不因便利存入普通持久存储。ZCode 的 dirty 字段和 revision 保护可参考。[ZCode · InlineEditableProviderCard.tsx:251–330](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/settings/model-provider-section/InlineEditableProviderCard.tsx#L251-L330)

**验收：** deferred save 中输入 B 后返回 A，B 仍在；清 key 不擦掉新 URL；切页/搜索过滤不会无声吞输入。无需为此给 Runtime 新建第二套配置状态机。

### R03 · P1 · 挂载前和局部插件失败都有恢复出口

**代码确认。** host 初始化异常被捕获后会继续挂载；普通 Runtime 离线已有界面。真正需补的是 mount 前 window chrome 等步骤失败后，最终只有 console.error，以及 PluginBoundary 只有原始错误缺少恢复动作。[Flame · renderer.ts:40–58](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/main/renderer.ts#L40-L58) [Flame · main.tsx:36–38](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/main.tsx#L36-L38) [Flame · PluginBoundary.tsx:28–39](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/host/PluginBoundary.tsx#L28-L39)

**改动：** bootstrap 即使主 UI 未挂载也能展示失败与重试/退出/复制诊断；root 和局部插件按各自范围恢复。详细错误折叠，首屏说清哪个功能不可用。ZCode 有 renderer 启动恢复与局部 ErrorBoundary 可参考。[ZCode · main.tsx:165–186](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/desktop/src/renderer/src/main.tsx#L165-L186) [ZCode · ErrorBoundary.tsx:152–175](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/ErrorBoundary.tsx#L152-L175)

**验收：** 注入 windowChrome reject、root render throw、单 pane throw，均有可见且作用范围明确的出口；已正常区域不被局部故障全部重启。

### R04 · P1 · 点击通知回到正确会话与待处理项

**代码确认／原生行为需真机验证。** 通知已拿到 sessionId，但只放 tag；点击仅 window.focus，没有导航。[Flame · completionNotify.ts:12–33](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/shell/status/completionNotify.ts#L12-L33) [Flame · osNotify.ts:15–18](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/shell/status/osNotify.ts#L15-L18)

**改动：** payload 保存明确 session/run 目标，点击通过公开 navigation 定位；宿主恢复最小化和前置窗口。ZCode 原生通知会恢复窗口并传回 taskId，可借鉴职责划分。[ZCode · desktopNotifications.ts:63–80](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/desktop/src/main/desktopNotifications.ts#L63-L80) [ZCode · desktopNotifications.ts:134–145](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/desktop/src/main/desktopNotifications.ts#L134-L145)

**验收：** A 当前、B 等待审批时点击 B 通知必须进入 B；关闭/最小化、权限拒绝、重复点击均有确定行为，不重复发消息或审批。

### R05 · P1 · 角色配置未知时不要显示“使用主模型／已关闭”

**代码确认。** utility/embedding role 的 loading/error 没有完整投影到 UI，空值分别被呈现成主模型、off 或没有可用 provider；外围 DataView 只包 provider 列表。[Flame · providerConfig.ts:31–72](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/settings/providers/application/providerConfig.ts#L31-L72) [Flame · RoleSections.tsx:104–112](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/settings/providers/ui/RoleSections.tsx#L104-L112) [Flame · RoleSections.tsx:169–205](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/settings/providers/ui/RoleSections.tsx#L169-L205)

**改动：** read model 暴露 loading/error/ready；未知时显示等待或错误重试，取得事实后才显示确定值。保留旧快照时注明更新失败，禁止把空默认当用户关闭。ZCode 分组加载和失败重试可作参考。[ZCode · ModelProviderSection.tsx:1036–1059](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/settings/ModelProviderSection.tsx#L1036-L1059)

**验收：** 分别延迟或失败 utility、embedding、provider 请求，不出现假 off/main，也不在状态未知时误写默认。

### R06 · P1 · 设置搜索无结果时仍有内容与恢复路径

**代码确认／检索增强。** 只按 pane 标题匹配，无结果时 grouped 为空，VerticalTabs 没有 empty fallback，导航和正文同时空白。[Flame · SettingsPage.tsx:68–74](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/shell/kernel/SettingsPage.tsx#L68-L74) [Flame · vertical-tabs.tsx:124–155](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/ui/atoms/vertical-tabs.tsx#L124-L155)

**改动：** 先补无结果说明、清除搜索、恢复原 pane；随后纳入真实字段与别名，如字体、API Key、动画，结果显示字段和分区路径并直达。不得因过滤卸载而静默丢草稿。本次未确认 ZCode 有字段级全局设置搜索，故不将其列为参考产品现成优势。

**验收：** 乱字符串有出口，真实字段可找到；Esc/清除恢复合理位置；键盘能选结果，未保存草稿按 R02 策略保留。

### R07 · P2 · Provider 表单有持续标签、校验说明和完整反馈

**设计改进及确定反馈缺口。** 字段主要依赖 placeholder；required URL 的 valid 仅检查非空，禁用原因不可见；保存没有明确成功提示，错误被截断且依赖 title 阅读。[Flame · ProviderRow.tsx:108–168](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/settings/providers/ui/ProviderRow.tsx#L108-L168) [Flame · providerDraft.ts:25–26](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/settings/providers/application/providerDraft.ts#L25-L26)

**改动：** 常驻字段标签、默认 endpoint/必填提示、按需显示 key、已保存提示；校验错误就地解释，长错误可展开复制。环境凭据与存储 key 的实际优先规则要准确呈现。ZCode 的 ProviderDetailFeedback 和表单可参考。[ZCode · ProviderDetailFeedback.tsx:97–149](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/settings/model-provider-section/ProviderDetailFeedback.tsx#L97-L149)

**验收：** 填值后仍知道字段名，每次 save 成败明确，错误不靠 hover 才能读全；反馈和日志不暴露 key。

### R08 · P2 · 动态错误可被读屏感知，并关联字段

**具体无障碍缺口。** Provider 成功/失败为普通 span，角色错误为普通 p；Connection 字段标 invalid，但错误没有关联到字段。连接总体状态已有 live 区，不能泛称全部没有播报。[Flame · ProviderRow.tsx:157–168](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/settings/providers/ui/ProviderRow.tsx#L157-L168) [Flame · RoleSections.tsx:56](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/settings/providers/ui/RoleSections.tsx#L56) [Flame · ConnectionPane.tsx:135–178](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/settings/connection-settings/ui/ConnectionPane.tsx#L135-L178)

**改动：** 成功用 status/polite，失败用适当 alert 与 aria-describedby；错误出现时保持可修正焦点，避免 toast 与 live 重复朗读。ZCode 的 feedback roles 可参考。[ZCode · ProviderDetailFeedback.tsx:105–110](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/settings/model-provider-section/ProviderDetailFeedback.tsx#L105-L110)

**验收：** 键盘提交非法 URL、测试失败/成功、角色保存失败，读屏能立即感知并定位；现有 route-level axe 通过不能替代此场景。

### R09 · P1 · Connection Enter 尊重 IME，失败不强制失焦

**代码确认。** URL 输入的 Enter 直接 apply 并 blur，没有 composition guard；校验失败也会继续 blur。Composer 的 IME 防护不覆盖这条局部 handler。[Flame · ConnectionPane.tsx:86–94](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/settings/connection-settings/ui/ConnectionPane.tsx#L86-L94) [Flame · ConnectionPane.tsx:139–144](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/settings/connection-settings/ui/ConnectionPane.tsx#L139-L144)

**改动：** 复用输入法确认判断，仅真正提交时 apply；错误保留焦点与可修正选择。ZCode 的字段 Enter 会检查 composition，可参考。[ZCode · InlineEditableProviderCard.tsx:591–611](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/settings/model-provider-section/InlineEditableProviderCard.tsx#L591-L611)

**验收：** 中文候选确认不修改 endpoint、不失焦；非法 URL 可直接继续改，正常提交恰好一次；最终在 WebKit 验证真实时序。

### R10 · P2 · 通知语言、权限与声音各自清楚

**代码确认／桌面体验增强。** 完成通知硬编码英文，权限在 setup 请求，拒绝或构造失败静默；设置主要有完成音开关，没有完整的系统通知状态。[Flame · completionNotify.ts:15–28](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/shell/status/completionNotify.ts#L15-L28) [Flame · osNotify.ts:1–19](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/shell/status/osNotify.ts#L1-L19) [Flame · PrefSections.tsx:9–20](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/settings/personalization/ui/PrefSections.tsx#L9-L20)

**改动：** 使用当前 locale，通知启用/权限状态与声音独立；拒绝后给系统设置说明，真实平台支持由宿主投影。ZCode 要求本地化 payload，并明确不支持时的结果。[ZCode · desktopNotifications.ts:94–115](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/desktop/src/main/desktopNotifications.ts#L94-L115)

**验收：** 中文界面下一条通知为中文，拒绝授权不假装开启；声音和通知互不混淆，失败可诊断而不重复打扰。

### R11 · P2 · 常用快捷键可配置且能解释冲突

**产品增强。** 当前已有快捷键检索表和输入过滤，缺的是用户修改/恢复默认及冲突反馈，不是缺键盘基础支持。[Flame · ShortcutsPane.tsx:29–83](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/command/shortcuts/ShortcutsPane.tsx#L29-L83) [Flame · keymap.ts:25–36](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/host/keymap.ts#L25-L36)

**改动：** 先覆盖常用命令的录制、修改、冲突与重置；录制时暂停原命令执行。命令菜单与界面使用同一命令来源。ZCode 的 ShortcutSettingsSection 与原生命令菜单绑定提供参考。[ZCode · ShortcutSettingsSection.tsx:136–198](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/settings/ShortcutSettingsSection.tsx#L136-L198) [ZCode · desktopApplicationMenu.ts:62–80](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/desktop/src/main/desktopApplicationMenu.ts#L62-L80)

**验收：** 列表、命令面板、实际执行一致；录制不误触，冲突不会一键多动作；macOS/Windows 修饰键显示正确。

### R12 · P1 · 首次配置直接通向第一个成功任务

**产品体验改进。** 已有 ProviderSetupPrompt，会引导去 providers；到达页面却先遇到 utility/embedding 高级角色。不能写成完全没有 onboarding。[Flame · ProviderSetupPrompt.tsx:22–41](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/shell/provider-setup/ui/ProviderSetupPrompt.tsx#L22-L41) [Flame · ProvidersPane.tsx:15–23](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/settings/providers/ui/ProvidersPane.tsx#L15-L23)

**改动：** 最短路径为选 provider → key/endpoint → 保存测试 → 返回原任务；高级模型角色收在后面，保留原任务草稿。ZCode 的分步所有权可参考，职业选择、账号与大规模迁移不是 Flame 必要步骤。[ZCode · OnboardingDialog.tsx:38–48](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/onboarding/OnboardingDialog.tsx#L38-L48) [ZCode · OnboardingDialog.tsx:157–179](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/onboarding/OnboardingDialog.tsx#L157-L179)

**验收：** 新用户清楚完成一次可用配置并回到任务，取消不丢输入；已配置用户不被反复强制引导。

## 7. 规范、验证与性能基线（G01–G03）

### G01 · P1 · 清理设计规范中的互相矛盾与过期完成记录

**代码与文档交叉确认。** 当前问题不是缺规范，而是规范曾多轮修改后失去一致性。

| 主题 | 文档中的冲突或过期陈述 | 当前源码事实 |
| --- | --- | --- |
| 区域边缘 | DESIGN 前段禁止 hairline，后段又要求 0.5px；POLISH 仍坚持无边界线 | visual style 与 globals 已实际消费 inset 边界。 |
| 字体与会话行 | 仍出现 bundled Geist、两行会话描述 | native sans；生产 SessionRow 为单行。 |
| 主题默认 | 文档称 system | store 为 light，HTML/native 首帧依 OS。 |
| 阅读宽度 | 文档声称 640px floor | 当前 shell geometry 的 safe area 为 352px。 |
| Review 与密度 | 工作区文档称可筛选文件导航、按 light/review 保存宽度已完成 | 当前 diff 仅纵向 FileCard，dockWidth 为统一比例；旧测试 mock 不是实现。 |

依据：[Flame · DESIGN.md:23–37](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/DESIGN.md#L23-L37) [Flame · DESIGN.md:121–134](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/DESIGN.md#L121-L134) [Flame · tokens.ts:120–125](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/theme/visualStyles/tokens.ts#L120-L125) [Flame · globals.css:168–173](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/styles/globals.css#L168-L173) [Flame · shellGeometry.ts:6–11](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/lib/shellGeometry.ts#L6-L11) [Flame · diff.tsx:176–215](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/workspace/workspace-views/diff.tsx#L176-L215) [Flame · dockWidth.ts:7–14](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/shell/kernel/panel/dockWidth.ts#L7-L14)

**改动：** 主规范只保留现在决定采用的规则；值由 token 拥有，旧方案论证和过期 DONE 不继续指挥修改。不要为迎合旧文档把当前实现改回去。ZCode 的职责明确的字号与缩放约束可作写法参考，不需要复制 Tailwind 技术选择。[ZCode · DESIGN.md:188–214](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/DESIGN.md#L188-L214)

**验收：** 边界、默认、字体、状态、布局每项只有一个现行答案；后续 AI 根据文档不会在相反方向反复修改。

### G02 · P1 · 当前视觉基线跟随真实实现，按变化选择场景

**验证流程改进。** 已有大量视觉/axe/IME/WebKit 基础设施，但总 `check` 不包含独立的 visual:test；当前部分 golden 还保留旧 effort chip。不能仅以文档、测试数量或接受了 snapshot 证明视觉完成。[Flame · package.json](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/package.json) [Flame · README.md](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/visual/README.md) [Flame · ModelPicker.tsx:251–258](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/chat/composer/ui/ModelPicker.tsx#L251-L258)

**改动：** UI 变更必须记录提交、真实运行过的代表场景、平台和人工审阅结论；使用既有 fixture，不增加平行展示体系。优先补本清单的输入候选、异步草稿、分组错误、限高与刷新定位场景。只更新确认正确的 golden，不盲目接受所有变化。

**验收：** 主会话、运行、等待、失败、长内容、窄列、Diff、设置在浅深主题有当前基线；未跑与环境失败明确记录，不能写全绿。

### G03 · P2 · 先建立真实交互时延基线，再优化大材料

**待测风险，不是卡顿结论。** 现有 reducer/Shiki/Mermaid 指标值得保留；Diff 按全部行同步高亮、默认展开文件等路径有可测成本，但源码行数不足以证明用户卡顿。[Flame · metrics.ts:13–51](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/lib/metrics.ts#L13-L51) [Flame · DiffView.tsx:93–107](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/workspace/workspace-views/views/DiffView.tsx#L93-L107) [Flame · diff.tsx:199–211](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/plugins/builtin/workspace/workspace-views/diff.tsx#L199-L211)

**改动：** 同设备测大历史切换、流式时打字滚动、200 文件 Review、长行、复杂 Mermaid、连续 resize 的输入到绘制、长任务和内存。明确瓶颈后再做折叠延迟高亮、可见段渲染或其它局部优化。ZCode 的虚拟文件行与大 diff 降级可作候选方案，不先全局重写 store/memo。[ZCode · GitPane.tsx:302–311](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/GitPane.tsx#L302-L311) [ZCode · GitPaneChangeCard.tsx:159–179](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/ui/src/GitPaneChangeCard.tsx#L159-L179)

**验收：** 改前后负载、设备、样本可复跑，给分布而非单个漂亮数字；普通材料不退化，完整复制不把显示截断当完整数据。

## 8. 一轮可以明显改善观感的具体设计目标

前面的缺陷修复保证界面可信；这一组目标用来形成一致的观看和操作体验。它们是本次设计建议，不是对 ZCode 像素参数的复制，也不是声称每个现有值都错误。

| 区域 | 建议保留与收敛的目标 | 对应清单 |
| --- | --- | --- |
| 左侧 | 保留现有约 30px 单行与项目分组；普通态只显示名称和必要状态，hover/focus 只有一个更多；等待信息在折叠后仍可见。 | S03–S06 |
| 主阅读面 | 保留正常宽度下的居中阅读列和留白；重点保证左右展开后不成为窄条。长用户输入折叠，最终回答持续占据视觉主体。 | S02、C09 |
| 文字 | 系统 UI 字体和技术 mono 分工继续保留；代码字号可以单独调大；用正文 14/15/16px 的同内容样张决定阅读档位，导航不跟着被迫放大。 | S07 |
| Composer | 输入、附件、模式提示和底部动作有固定关系；附件可看全却不能无界增长；发送/停止的位置稳定，running 的提交含义始终清楚。 | C05–C08、C12 |
| Agent 过程 | 默认是一条可扫读的进展信息，展开才是细节；失败和等待始终清晰。推理、工具组、日志各有空间预算，不能轮流把阅读面顶走。 | C02、C10、C11 |
| 右侧 | 每份材料说明当前路径、范围与来源；能继续读取、定位、查找、回到对话，必要时全幅展开；不把只读材料区做成新的管理后台。 | W01–W12 |
| 设置 | 基础任务先出现，高级模型角色渐进展开；字段永久可辨，保存/测试/错误各自有明确反馈；新用户完成配置后回到原任务。 | R01–R03、R05–R09、R12 |
| 视觉语法 | 选定一套当前边缘、焦点、圆角与材料规则，删除文档中的相反答案；不新增渐变、大范围玻璃、每行彩色徽章来补偿层次问题。 | G01、G02 |

建议优先用同一内容制作六张评审状态：正常会话、运行中、等待用户、失败可恢复、长内容、Review 展开。六张同时成立，才代表设计语言收敛；不能只把空白首页做得好看。

## 9. 推荐实施顺序

### 第一批：输入和展示先可信

优先完成 **C03、R02、R01、C01、C02、R05、W02、W03、W05、S01**。这组主要解决误触执行、草稿丢失、错误配置反馈、历史与当前混淆、分组状态丢失和阅读跳转问题。

做法以现有 owner 为中心：候选行为归 Composer，异步输入归 Provider draft，工具历史归持久结果投影，文件定位归导航意图，主题默认归唯一偏好解析。不要在每个症状点分别打补丁。

### 第二批：让主会话第一眼和持续使用都更精致

完成 **S02、S03、S05、S06、C07、C09、C10、C11、R12**，同步推进 **G01、G02**。它们分别处理主阅读宽度、当前项可见、会话更多、等待聚合、附件审阅、长输入折叠、轻量推理、日志限高和首次配置。

这一批最直接回应“当前 UI 粗糙”的感受：输入和阅读稳定，信息有主次，用户知道能点哪里，不用靠隐藏手势发现功能。

### 第三批：贯通输入与成果检查

完成 **C04、C05、C06、C13、W01、W04、W06、W07、W08、W09、W11**。先准确显示当前工作树范围，再讨论需要 Runtime 支持的历史变更；先支持 Markdown/图片和常用代码，再考虑其它成果格式。

### 第四批：高级效率与持续打磨

完成剩余 **C08、C12、C14、S04、S07、S08、W10、W12、R03、R04、R06、R07、R08、R09、R10、R11、G03**。这里的批次表示可组合的实施次序，不降低 P1 的重要性：若首次启动、中文输入或通知是近期发布门槛，R03/R04/R06/R09 应提前。

不以“每批所有项目一起重构”为要求。可按独立 owner 切成可验证的提交，完整修复一条行为及其调用者、测试与文档，再进入下一条。

### 建议的验证矩阵

| 维度 | 至少验证的组合 | 重点 |
| --- | --- | --- |
| 布局 | 1120×720、1280×800、1440×900；侧栏/Dock 开关及边界宽度 | 主阅读宽度、动作可达、附件限高、长路径。 |
| 主题与语言 | light/dark/system；首次启动；中英；长名称 | 首帧一致、等待/失败可辨、输入法、文案溢出。 |
| 输入 | Slash/@ 开启；多附件；长粘贴；running steer；保存时继续编辑 | 不误发、不误停、不覆盖新输入、来源身份正确。 |
| 叙事 | 短/长用户消息、长推理、单工具/分组失败、持续日志、终态 | 正文优先，状态/动作不因折叠分组丢失，手动阅读不被拉走。 |
| 材料 | 文件范围边界、刷新、binary/rename/empty diff、50–200 文件、多 session | 来源明确、截断有出口、导航保持、定位只响应新意图。 |
| 异步状态 | loading/empty/error/ready；迟到响应；断线恢复 | 未取得事实不伪装默认，错误可修，结果对应正确版本。 |
| 可访问性 | 纯键盘、读屏动态错误、无 hover、减少动画 | 同一行为可达，焦点可恢复，通知/错误不过度重复。 |
| 原生平台 | Wails/macOS 冷启动、最小化、通知点击、关窗/退出 | 浏览器 fixture 不能替代真实宿主行为。 |

## 10. 本轮应后置的产品能力

- **完整 IDE、Git commit/push/branch graph、交互 PTY**：当前问题优先通过可检查的材料和输出解决；若将来真的加入，需真实的平台与 Runtime 生命周期，不给只读日志加一个输入框冒充终端。
- **完整 workflow/多 Agent 看板、artifact 版本体系**：先有真实生产者与来源事实，再设计承载 UI；不要因 ZCode 有这些能力就新增空的面板。
- **账号、订阅、SSH/WSL、远程控制、迁移向导与 Office 全格式**：有各自产品目标，不是当前 desktop 粗糙的共同解法。
- **关闭窗口继续后台驻留**：Flame 当前明确选择关闭最后窗口即退出；这不是忘记一个布尔值。要改变，应先决定运行任务、前端退出、外部 Runtime 与重新唤回的契约。[Flame · main.go:36–39](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/main.go#L36-L39) ZCode 的 hide 策略只是另一种选择。[ZCode · desktopDarwinCloseBehavior.ts:13–30](https://github.com/zai-org/ZCode/blob/328c1a0c0ffaa5a4f65e8fa199af5e4c20706e5f/packages/desktop/src/main/desktopDarwinCloseBehavior.ts#L13-L30)
- **主题/动效参数继续扩张**：现有 500ms 面板过渡可与 200–300ms 样张比较，但感受应实际测；优先统一现有规则、减少跳变，而非提供更多旋钮。[Flame · appearance.ts](https://github.com/Tangerg/flame/blob/a7b29b7e43625692a5c61254b375756a2268aefe/desktop/frontend/src/lib/appearance.ts#L16-L27)

## 11. 交付摘要与使用方法

本报告包含 **49 项**：输入和叙事 14 项、壳与视觉 8 项、文件/结果 12 项、设置和桌面 12 项、规范/验证 3 项。条目 ID 可直接用于后续任务拆分。

每次实现一项，先读链接对应的当前代码并确认此后是否已变化；以本报告的触发场景和验收条件讨论行为，不能把旧提交上的结论永久当作新版本问题。已确认的问题与设计建议应分别评审；后者允许依据实际使用结果选择更适合 Flame 的方案。

这轮最有价值的改进，是让 Flame 已有的 Agent 能力稳定、清楚、可检查地呈现在桌面上：用户输入不会被意外处理，执行过程容易扫读，结果能顺着检查下去，下一步操作就在眼前。

## 附录：可勾选执行索引

- [ ] **C01 · P1** 分清历史工具结果与当前工作区内容
- [ ] **C02 · P1** 工具分组保留失败状态与操作能力
- [ ] **C03 · P0** Slash 候选先接管输入按键，避免误发与误停
- [ ] **C04 · P1** 文件候选明确显示加载、失败与搜索范围
- [ ] **C05 · P1** 文件引用保留可靠身份，正确处理空格路径
- [ ] **C06 · P2** “+”菜单暴露已有的上下文能力
- [ ] **C07 · P1** 附件可在发送前完整审阅，托盘高度受控
- [ ] **C08 · P2** 拖入反馈锚定 Composer，保留当前工作内容可见
- [ ] **C09 · P1** 长用户消息可折叠，保留完整输入操作
- [ ] **C10 · P1** 推理默认轻量，主动展开由用户控制
- [ ] **C11 · P1** 日志摘要显示新进展，展开高度保持一致
- [ ] **C12 · P2** 输入过程中持续说明 steer 的作用
- [ ] **C13 · P2** 选区可以直接引用到输入，贯通答案、代码与右侧文件
- [ ] **C14 · P2** Mermaid 大图阅读与流式生成保持稳定
- [ ] **S01 · P1** 统一首次启动的主题解析
- [ ] **S02 · P1** 主阅读区优先，窄窗仍能打开上下文材料
- [ ] **S03 · P1** 显式选择会话后，在左侧揭示当前项
- [ ] **S04 · P2** “置顶”的文案和排序行为一致
- [ ] **S05 · P2** 会话行提供克制的“更多”按钮
- [ ] **S06 · P1** 项目折叠后仍显示“需要你处理”的信息
- [ ] **S07 · P2** 代码字号独立，正文与界面密度分别评审
- [ ] **S08 · P2** 标题确实溢出才渐隐，统一完整标题揭示节奏
- [ ] **W01 · P1** 文件截断之后可以继续读
- [ ] **W02 · P1** 文件自动刷新保持用户阅读位置
- [ ] **W03 · P1** 二进制与零文本增删也显示 Review 入口
- [ ] **W04 · P1** Patch 每个文件可检查，“还有 N 个”可展开
- [ ] **W05 · P1** 显式打开 Diff 文件时展开目标
- [ ] **W06 · P1** Review 增加轻量文件导航、改动类型与定位
- [ ] **W07 · P1** 明确当前工作树与本次执行变更的范围
- [ ] **W08 · P1** Files 提供选中、定位和完整路径反馈
- [ ] **W09 · P1** 成果预览支持 Markdown、图片和常用路径动作
- [ ] **W10 · P2** 切会话后恢复材料阅读状态
- [ ] **W11 · P1** 搜索可以缩小范围，并突出实际命中
- [ ] **W12 · P2** 计划与成果提供来源，避免额外待办体系
- [ ] **R01 · P1** 测试结果必须对应眼前的 Provider 配置
- [ ] **R02 · P0** 保存响应不得覆盖保存期间继续输入的草稿
- [ ] **R03 · P1** 挂载前和局部插件失败都有恢复出口
- [ ] **R04 · P1** 点击通知回到正确会话与待处理项
- [ ] **R05 · P1** 角色配置未知时不要显示“使用主模型／已关闭”
- [ ] **R06 · P1** 设置搜索无结果时仍有内容与恢复路径
- [ ] **R07 · P2** Provider 表单有持续标签、校验说明和完整反馈
- [ ] **R08 · P2** 动态错误可被读屏感知，并关联字段
- [ ] **R09 · P1** Connection Enter 尊重 IME，失败不强制失焦
- [ ] **R10 · P2** 通知语言、权限与声音各自清楚
- [ ] **R11 · P2** 常用快捷键可配置且能解释冲突
- [ ] **R12 · P1** 首次配置直接通向第一个成功任务
- [ ] **G01 · P1** 清理设计规范中的互相矛盾与过期完成记录
- [ ] **G02 · P1** 当前视觉基线跟随真实实现，按变化选择场景
- [ ] **G03 · P2** 先建立真实交互时延基线，再优化大材料
