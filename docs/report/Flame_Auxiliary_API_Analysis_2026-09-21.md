# Flame 旁路 API 分析

## 对照 OpenCode v2 与 Codex main：文件、Diff、Skills、配置、MCP 与资源生命周期

**报告日期：2026-09-21**\
**用途：为 Flame 前后端旁路 API 的演进提供取舍依据；本报告不包含仓库代码修改。**\
**配套报告：《Flame 核心 AI 流式协议分析》覆盖 Session / Run / Segment / Item、流、恢复、幂等和人工中断。**

## 1. 核心判断

Flame 旁路 API 最值得改进的部分，是让客户端知道**读到什么、属于哪里、是否完整、写入承诺了什么、什么时候真正生效**。现有接口已经有显式 WorkspaceRef、受工作区约束的文件读、文本与材料预算、目录分页、结构化 diff、技能管理生命周期、MCP 状态与授权尝试，以及 runtime subscription 和重同步。不能把这些已有能力重新列成“向两个参考项目补课”的任务。

本报告主张：

1. **保留现有领域 owner 和统一协议生成链。** 文件、技能、MCP、模型、计划与记忆各有自己的资源语义；前端不应通过任意工具调用或任意 shell 拼接出旁路产品。
2. **Diff 先补语义，再考虑扩功能。** 工作区与 Git 基线的差异、某次 Run 范围内的变化、一个工具动作的 patch，以及将要执行的审批预览，都应能分清。
3. **Skill 先补解释和审阅体验。** 列表继续轻量，由实际 resolver 提供来源、按需正文和诊断；保留 Flame 已有 proposal revision，把冲突从通用参数错误提升为可操作的结果。
4. **配置明确生效边界。** 已保存、已配置、已连接、下一次 Run 使用，与当前执行已经使用，是不同事实。先校准现有 owner 文档和界面，不引入一套无所不包的配置平台。
5. **旁路通知主要用于失效重读。** 保留现有 generation / resync；有测量依据时，再消费 wire 已有的 path、workspace 和 resource IDs，减少重复读取。
6. **FS 写入、二进制资源、终端、远端环境和市场功能都由产品需求触发。** 它们不是“两个项目都有，所以 Flame 必须都有”的清单。

这些判断以 Flame 的 [所有权约束](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/PROJECT_RULES.md)、[文件契约](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/workspace_files.go)、[Diff 契约](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/workspace_diff.go)、[Skill 契约](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/skills.go)、[资源事件](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/runtime_events.go)和 [方法 manifest](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/contract/manifest.json) 为起点。后文明确区分源码事实、静态推断和建议增量。

## 2. 基线、覆盖与证据边界

| 项目 | 指定分支 | 固定提交 |
| --- | --- | --- |
| Flame | main | [5989445dbd787c0914a72013bb511b9a08399138](https://github.com/Tangerg/flame/commit/5989445dbd787c0914a72013bb511b9a08399138) |
| OpenCode | v2 | [2f0c861af098ec914a19751fe62d715448f6bf84](https://github.com/anomalyco/opencode/commit/2f0c861af098ec914a19751fe62d715448f6bf84) |
| Codex | main | [40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f](https://github.com/openai/codex/commit/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f) |

通过 GitHub 插件读取仓库与固定提交文件，三个递归目录树都返回完整结果。对重点协议沿公开注册表 → schema → handler → 所有者实现 → 前端消费者与相关测试追踪。OpenCode 使用本次 v2 的 protocol / server / client，不混用旧版 API；Codex 区分当前方法、实验门控、废弃接口及仅在内部存在的能力。

本次为**静态源码和测试代码审阅**，没有启动应用、执行真实模型、运行上游完整测试或测量性能。本文的测试引用说明已有代码表达了某种预期，不表示本次执行通过；性能收益、极端竞态和新的产品能力均需后续验证。非实验路径也不自动等于永久稳定：OpenCode Api 总说明仍有 experimental / 0.0.1 标记；Codex 部分类型注释与实际方法门控亦有历史差异，实际可用性以注册和处理器为准。[OpenCode API 声明](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/protocol/src/api.ts#L140-L215)、[Codex 方法与门控](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/common.rs)

### 2.1 “旁路”按领域划分，不按是否流式划分

| 领域 | 权威对象 | 观察方式 | 与 AI Run 的关系 |
| --- | --- | --- | --- |
| 文件、目录、Git 状态 | 工作区当前状态和比较结果 | 查询、资源失效、必要时范围读取 | 可独立于 Run 被用户或外部程序修改 |
| Skills、MCP、模型、Hooks | 配置、发现结果、实际运行状态 | 目录、详情、命令、状态变化 | 影响可用能力；不等于已经进入当前模型上下文 |
| 授权尝试、安装或刷新 | 某次操作及其状态 | command + status / notification | 可在没有 AI Run 时发生 |
| 交互终端、搜索 | 连接或独立资源拥有的过程 | 输出流、增量结果、取消 | 可能流式，但不需要成为 AI Run |
| Diff / artifact | 比较物料、结果资源或来源引用 | 查询；核心事件可引用或触发刷新 | 可能源于 Run，但其读取和生命周期需要另行定义 |

旁路不能被视为“若干低重要性的 CRUD”。一项配置成功却未生效、一份 diff 缺少基线、一次搜索失败却返回空结果，都足以让界面产生错误判断。反过来，也无需为每次文件变化保存永久事件日志；通过查询恢复当前事实通常更直接。

## 3. 三家的接口版图与边界

| 领域 | Flame 当前 | OpenCode v2 | Codex main | 对 Flame 的取舍 |
| --- | --- | --- | --- | --- |
| Scope | 显式 WorkspaceRef 与 Runtime owner | 公共 Location 以 directory 定位 | Thread / cwd / connection 等多种 scope；FS 仍取 local environment | 保留显式作用域，按域校验实际执行位置 |
| 文件 | 有界 UTF-8 读、列表分页、metadata/head、内容搜索 | raw bytes 读、薄目录项、文件名 fuzzy find、实验写 | Base64 读写、metadata、目录 CRUD、watch | 保留 Flame 读取边界；新写能力按需求设计 |
| Diff | worktree / base；行结构、rename、binary、截断 | working / branch / committed；session snapshot diff / revert | Turn 聚合 diff、FileChange Item、废弃 Git 查询 | 学比较范围与来源，保留丰富 DTO |
| Skills | discovered / library / proposals / archive / restore | list 含全文；另有实验 session enable | metadata、逐 cwd 错误、配置、plugin skill 详情 | 轻列表 + 同一 resolver 的详情和诊断 |
| MCP | 持久 server 配置、状态、probe、OAuth attempt | 目录与资源 catalog；实验临时覆盖 | 有效配置状态、resource read、受 Thread 约束的 tool call | 保留状态机；补有真实 UI 消费者的资源能力 |
| 配置 | provider / model role / MCP / hook 等 owner 命令 | 来源目录、有限 shell patch、location reload | 层级来源、expectedVersion、override 和 reload | 借生效解释，不迁入整套层级配置框架 |
| 终端 | 无通用公开 PTY / process 域；工具输出另有语义 | Shell、PTY、实验 persistent PTY | command/exec、实验 process/spawn | 有交互终端需求再建独立生命周期 |
| 通知 | runtime subscription，scope / sequence / resync | volatile event + scoped cache refresh | fs/skills/apps 等通知，各有 owner | 保留失效后重读；不泛化为可靠日志 |
| 其他 | 目标、计划、调度、知识、记忆、用量、反馈 | project / worktree / plugin / integration 等 | account / apps / project / environment / attachments 等 | 保留 Flame 产品差异，避免照搬商业服务面 |

表中只表示公开协议面，不把 Agent 内部工具自动计入。具体依据见 Flame [API reference](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/contract/API_REFERENCE.md)、OpenCode [当前 OpenAPI](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/protocol/openapi.json)和 Codex [方法注册](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/common.rs)；重点域在后续逐一给出处理器证据。

### 3.1 作用域必须是协议事实

Flame 的 WorkspaceRef 和工作区解析是应保留的基础。OpenCode LocationQuery 当前公开的是 location.directory，查询参数优先，其次 header，再到进程 cwd；公共类型删除了内部 workspaceID。不能因此声称其 v2 HTTP 已提供通用远端 workspace CRUD。Codex 虽有实验 environment / project API，但已审查的 FS handler 取 local environment，且请求无 workspaceId / environmentId；看到一个环境类型，不代表所有旁路调用都能路由到它。[Flame 工作区身份](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/workspace.go#L7-L45)、[公共 Location](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/schema/src/location.ts#L1-L37)、[Location 解析](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/server/src/location.ts#L14-L69)、[Codex FS 环境](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/request_processors/fs_processor.rs#L53-L190)

客户端缓存至少应绑定现有连接 / Runtime identity 与已解析工作区，再带具体资源参数；不能只用一个相对 path 或显示名称。相同路径在不同 Runtime 上不保证是同一个文件。将来如有远端环境，先定义“由谁解析路径、谁拥有文件、请求实际在哪执行”，再增加必要字段；当前单机模型不必提前长出项目、环境、租户等全部实体。

## 4. 文件系统、搜索与附件

### 4.1 对比真实公共契约

| 问题 | Flame | OpenCode v2 | Codex main |
| --- | --- | --- | --- |
| 文本或字节 | 明确的 UTF-8 文本 DTO、读取窗口与预算 | read 返回原始 bytes + MIME | read / write 使用 Base64 bytes |
| 路径边界 | WorkspaceRef + 词法 / realpath / symlink 等边界处理 | read 约束 Location；list 可浏览父或兄弟；实验 write 允许根外 | AbsolutePathBuf + host local FS，操作传 sandbox=None |
| 目录枚举 | 稳定游标、结构化条目 | 单层薄数组 | 直接子项数组，公开字段无 cursor |
| 搜索 | 正则内容搜索；其他工具通道有 glob / grep | fs.find 是路径 / 文件名 fuzzy 搜索 | 旧 fuzzy 查询 + 实验搜索 session |
| UI 写入 | 当前无通用公开 FS write | 实验 raw body write，可建父目录 | write / remove / copy / createDirectory 等 |
| 内容版本前置条件 | 当前只读，不需要凭空增加写 CAS | 已审查 write 无 expectedVersion | 已审查 FS schema 无 contentRevision / ifMatch |
| 文件监听 | Runtime 资源变更与 workspace watch scope | filesystem.changed 等临时事件，无独立公共通用 watch CRUD | connection + watchId；非递归监听 |

证据：[Flame 文件 DTO](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/workspace_files.go#L17-L116)、[Flame 路径处理](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/adapter/workspace/path_resolver.go#L53-L99)、[OpenCode FS API](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/protocol/src/groups/fs.ts#L1-L97)、[OpenCode 文件操作边界](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/core/src/filesystem.ts#L73-L171)、[Codex FS schema](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/v2/fs.rs#L7-L205)、[Codex host FS 实现](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/request_processors/fs_processor.rs#L53-L190)

这里不能简单排“谁的权限更大谁更完善”。OpenCode 与 Codex 的部分操作面向受信任的本地主机客户端。Flame 已经选择工作区约束，就应由同一个文件 owner 在实际打开文件的路径上维护，不因增加一个新 UI 操作而旁路穿透。

### 4.2 Flame 现有有界文本读取值得保留

Flame 的 grep 不只是对字符串 clean：它使用根目录文件访问、验证 regular file、预算与 UTF-8 corpus，并在读取后核对文件版本。返回 total 的精确性只针对已纳入的文本集合，不能把它描述成所有磁盘字节的完整检索。[搜索入口与预算](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/adapter/workspace/file_search.go#L18-L63)、[读取与版本验证](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/adapter/workspace/file_search.go#L77-L164)

OpenCode raw read 对图片与下载很方便；Codex Base64 对二进制传输语义明确。这些都能启发未来的图片 / PDF / 产物预览，但不应替换 Flame 的文本窗口。可采用**按需的有界二进制资源读取**：来源仍由 Runtime 解析，明确 MIME、大小、可用性和 range / 截断语义。大文件未必需要经 JSON Base64 往返；具体承载在有消费需求时选择。OpenCode 的生成客户端已区分 JSON 与 raw bytes，TUI diff 图片也按需读取。[raw read 客户端](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/client/src/promise/generated/client.ts#L1540-L1600)、[图片按需读取](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/tui/src/feature-plugins/system/diff-viewer.tsx#L98-L180)

若以后增加内嵌编辑，再设计 expectedRevision 或同等前置条件，并明确“文件已被 Agent / 外部编辑器改变”的冲突。**当前缺少文件写 CAS 不是 Flame 的缺陷，因为当前公开产品能力是只读。** 也不要误称 OpenCode 或 Codex 已提供完整的这套保护；其已审查公开 FS schema 并未声明。

### 4.3 两种搜索不能互相替代

“输入 @ 时补全文件名”需要相关性排序、短延迟和旧请求取消；“查某段代码出现在哪里”需要内容匹配、行号、预算和完整性。OpenCode fs.find 的 limit 不提供 continuation 或完整总数，适合补全；Flame 现有正则内容查询服务的是另一件事。[OpenCode 搜索类型](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/schema/src/filesystem.ts#L1-L49)、[索引与刷新](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/core/src/filesystem/search.ts#L26-L203)

Codex 实验搜索 session 复用索引，并检查结果 query 是否仍为 latestQuery，避免用户从 A 打到 AB 后 A 的慢结果覆盖 AB。这一点有实际价值。但其 session key 主要是字符串，相关 reporter 使用全局通知；不应照搬为 Flame 多窗口的连接所有权模型。旧搜索的一些错误会记录后返回空结果，也不能把“检索失败”和“没有命中”混成一个 UI 状态。[增量搜索与最新 query](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/fuzzy_file_search.rs#L18-L227)、[查询错误与取消](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/request_processors/search.rs#L24-L132)、[请求作用域](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/request_serialization.rs#L23-L116)

对 Flame 的改进应由测量触发：只有大仓库搜索确实阻塞交互，才增加 query generation / 取消 / incremental result，并明确 complete、partial、truncated 与 total 的含义。不能把 unary 改成 stream 就宣称检索更快。

### 4.4 附件与生成物不等于主机路径

OpenCode 的文件读取和本地材料准备可以服务其 daemon；Codex 则有独立 thread/attachment 资源，按 Thread + type + identityKey 去重，先提交再响应和通知；它并不是一个通用二进制上传协议。两者提醒我们区分“磁盘文件”“已接纳的输入材料”“属于会话的资源关联”和“模型输出中的资源引用”。[Attachment 契约](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/v2/thread_attachment.rs#L1-L107)、[Attachment 提交与去重](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/request_processors/thread_attachments.rs#L36-L173)

OpenCode 的附件内容准备与宿主 file URI 解析分别可见[材料固化](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/core/src/session/prompt.ts#L93-L132)、[file URI 解析](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/core/src/session/prompt.ts#L151-L200)。

Flame 若需要跨设备附件或可长期下载的生成物，可在既有资源 owner 下引入稳定引用，带所属作用域、内容版本和来源；消息只引用它。上传、下载、清理、删除关联与删除正文分别承诺什么，要先说清。当前不应因为界面里出现了一个 file path，就把它当作永久、授权过、可供浏览器访问的 artifact。

## 5. Diff：比较基线、运行来源与可执行操作

### 5.1 四个问题必须分开

| 用户在问什么 | 应回答的对象 | 不能用来冒充的对象 |
| --- | --- | --- |
| 现在工作区改了什么 | 某个明确 Git 基线到当前工作区的差异 | 某次 Run 的全部因果结果 |
| 这个分支准备提交 / 合并什么 | merge-base 或显式 base 到 HEAD / worktree | 默认分支名的模糊猜测 |
| 这一轮任务改变了什么 | Run / Segment 范围、快照区间或可信操作来源 | 当前 git status 或任意工具输出文字 |
| 批准 / 撤回后会怎样 | 明确 before / after、前置版本、动作和冲突 | 已经执行过的 patch，或纯展示 diff |

这不是要求现在同时创建四套 API。它首先要求现有字段、标题和错误不误导用户；只有确实需要独立读取与操作时，再划出相应资源。

### 5.2 工作区比较：学 OpenCode 的基线解释，保留 Flame 的物料表达

Flame 的 worktree 对比 HEAD 并纳入未跟踪文件；base 对比默认分支的 merge-base。DTO 已有逐行结构、rename 的 previousPath、binary、计数和按文件边界截断。前端有意读取整个 comparison，当前选中文件只是焦点；这适合工作区 review，但没有 runId / sessionId，因此不能命名为“本轮 AI 改动”。[Flame Diff 输入与输出](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/workspace_diff.go#L3-L65)、[前端比较范围](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/desktop/frontend/src/plugins/builtin/workspace/application/diffViewModel.ts#L31-L58)

| 模式 | OpenCode v2 比较 | 对 Flame 的潜在价值 |
| --- | --- | --- |
| working | HEAD → working copy | 对应现有工作区未提交审查 |
| branch | merge-base(base) → working copy | 解释完整分支改动，包含未提交 |
| committed | merge-base(base) → HEAD | 只审查当前分支已提交内容 |
| base 查询 | name / ref / source，或无可用基线 | 让用户知道自动选择依据，必要时显式选择 |

OpenCode 的 base override 只影响该次读取，不写全局配置；其 base 解析还考虑 reflog 来源。值得借鉴的是“基线是可解释、可选择的查询参数”。但它把需要选择基线、基线缺失及部分 provider 错误映为 503，Flame 若实现应让“需用户选择”和“服务暂时不可用”有不同恢复动作。[VCS 比较与基线](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/protocol/src/groups/vcs.ts#L1-L104)、[VCS 错误映射](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/server/src/handlers/vcs.ts#L1-L57)、[基线测试](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/server/test/vcs.test.ts#L11-L78)

OpenCode FileDiff.Info 更薄，主要是 file / patch / additions / deletions / added-deleted-modified；不能为了对齐丢掉 Flame 已有的 rename、binary 和行结构。Flame 最小增量是返回**实际解析到的比较基线**，例如 resolved commit 与基线来源，并让 UI 显示清楚。是否增加 committedOnly、用户选 base 或历史变化集，取决于 review 产品需求。[OpenCode FileDiff](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/schema/src/file-diff.ts#L1-L17)、[Flame 结构化 Diff](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/workspace_diff.go#L42-L65)

### 5.3 Codex 有三个不同的公开 Diff 表面

| 表面 | 当前实际语义 | 对 Flame 的启发 |
| --- | --- | --- |
| turn/diff/updated | Thread / Turn 下最新聚合 unified diff | Run 范围的 review 可以是明确投影，不能与 workspace diff 混同 |
| FileChange Item / item/fileChange/patchUpdated | Item 给 path、kind、diff 与执行状态；patchUpdated 给该 Item 的最新 changes | 动作、审批、执行和结果有各自身份；不是所有 patch 都已成功落盘 |
| gitDiffToRemote | 废弃 v1 旁路查询；返回远端可见基线 SHA 与当前工作区 patch | baseline SHA 有价值；不代表本轮修改或完整 Git 产品 |

证据：[Turn diff 契约](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/v2/turn.rs#L553-L563)、[Turn diff 发出路径](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/bespoke_event_handling.rs#L1252-L1287)、[FileChange 结构](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/v2/item.rs#L1132-L1162)、[patchUpdated 与废弃 outputDelta](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/v2/item.rs#L1507-L1528)、[Git 旁路处理器](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/request_processors/git_processor.rs#L11-L35)、[废弃 v1 注册](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/common.rs#L1442-L1452)

gitDiffToRemote 的基线由仓库远端与分支历史解析，不是固定 origin/main。它返回大 patch 字符串，也没有 Flame 丰富的逐文件材料状态；不值得作为所有 Diff 的统一替代。相反，Codex 同时保留三种表面，正说明同样叫 diff 的物料在产品上承担不同含义。[远端基线入口](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/git-utils/src/info.rs#L352-L365)、[候选基线选择](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/git-utils/src/info.rs#L670-L702)

### 5.4 Session snapshot diff 提供时间范围，不天然提供严格归因

OpenCode session diff 以用户消息范围选择开始与结束快照；busy 时追加的输入可能属于同一段执行。这个模式很适合“查看这一段交互期间文件变成什么样”。但 snapshot 是工作区状态捕获，外部编辑器或其他进程在同一窗口写入的内容也可能进入比较；这是从实现推导的限制，本次未运行并发复现。[Session diff 请求](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/protocol/src/groups/session.ts#L579-L603)、[范围与快照选择](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/core/src/session/diff.ts#L25-L138)

另一个值得避免的歧义：OpenCode Snapshot.capture 是 best effort，缺乏支持、关闭或捕获失败可返回 undefined；范围受 Git / ignore 等策略约束，未跟踪文件还有大小门槛。SessionDiff 缺可用起始快照时返回空数组；只有当前 active Session 最后一步未结束才尝试捕获 working copy，捕获失败还可能退回最后记录的 end snapshot。因此除可用性外，结果的新鲜度也需要解释；跨 Location 的范围被拒绝。若 Flame 以后增加 Run change set，应区分 available、partial、unavailable 及原因；**没有快照不能被界面显示成“这一轮没有改文件”。** [快照覆盖与失败边界](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/core/src/snapshot.ts#L44-L204)、[缺快照分支](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/core/src/session/diff.ts#L106-L137)

精确“由本 Run 导致”的承诺通常需要工具动作来源与快照共同支撑，而且仍要解释外部并发修改。可以先提供诚实的“本轮时间区间差异”，无需为漂亮的归因文案增加不可能证明的因果保证。

### 5.5 撤回和预览需要清楚的副作用边界

OpenCode revert.stage 可立即恢复文件，它不是纯 preview；clear / commit 另有语义。Flame 已有 Session rollback 的 history / files / both 模式及既有 owner，应先审阅和复用这些边界，不为参考项目再建另一个回滚系统。[OpenCode revert 契约](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/protocol/src/groups/session.ts#L530-L566)、[revert 文件恢复](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/server/src/handlers/session.ts#L451-L503)、[Flame Session 与 rollback](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/sessions.go)

如果将来 UI 提供“预览撤回”，读操作应无副作用；真正 apply 时验证当前版本与预览时基线一致，不能盲目 reverse patch。会话历史截断、模型上下文压缩、文件回退不是同一事务。只有实际实现证明了跨文件与数据库的原子性，才可以在契约中承诺。

### 5.6 大 Diff 的最小改进顺序

Flame 已设 5,000 files / 5,000 rows / 64 MiB 物料边界，并在文件边界截断；原始材料超过 64 MiB 会失败，而非默默返回伪完整 patch。当前不是无界实现。[Diff 限额](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/application/workspace/vcs_reads.go#L12-L17)、[有界物料生成](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/application/workspace/vcs_reads.go#L174-L238)

建议先测大仓、超大首文件和频繁外部修改时的 review 体验。若诚实截断仍使用户无法审阅，才分离文件摘要与单文件 / hunk 读取，明确同一比较版本；不要先建持久 snapshot 平台。基线来源和 truncated 文案是低成本清晰度改进，分片物料和精确 Run 归因则是不同成本等级。

## 6. Skills：目录、正文、生命周期与真实可用性

### 6.1 三家的重点不同

| 层次 | Flame | OpenCode v2 | Codex main |
| --- | --- | --- | --- |
| 发现目录 | name / description / scope；project 覆盖 user | id / name / description / autoinvoke / path / content | metadata、path / scope / enabled、plugin、依赖等 |
| 列表正文 | 轻量目录，无通用 discovered 详情读 | list 直接包含全文 | list 不塞全文；远端 marketplace plugin skill 有单独读取 |
| 生命周期 | managed library、active / archived、proposal approve / reject | 非 experimental skill 组只有 list；Session 单独激活 skill 属实验接口 | 配置启停、extra roots、plugin 安装与来源 |
| 审阅一致性 | proposal immutable instructions + revision | 不能从 list 推导审阅事务 | 配置版本和来源是另一层能力 |
| 错误与可用性 | 当前目录解释较少；proposal conflict 归入通用参数错 | HTTP list 不等于经过 session permission 过滤的 runnable set | 每 cwd 返回 skills + errors，局部失败可解释 |
| 更新提示 | runtime events 与 catalog 重读 | event 驱动刷新 | skills/changed 粗失效、forceReload、watcher |

证据：[Flame 目录](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/skills.go#L11-L38)、[Flame proposal](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/skillproposals.go#L11-L33)、[OpenCode Skill.Info](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/schema/src/skill.ts#L27-L47)、[OpenCode list 与 available](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/core/src/skill.ts#L1-L134)、[Codex skill metadata](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/v2/plugin.rs#L453-L554)、[Codex 按 cwd 读取](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/request_processors/catalog_processor.rs#L474-L684)

OpenCode 当前 skill 组与实验激活入口可分别核对[Skill list](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/protocol/src/groups/skill.ts#L1-L28)、[Session skill 激活](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/protocol/src/groups/session.ts#L433-L451)。

Flame 的技能管理比一个“选择提示词”弹窗承担更多产品语义。OpenCode 的全文目录不适合原样搬入；Codex 的 metadata / 详情分离、局部错误和来源解释更值得借鉴；其中 plugin/skill/read 当前限定远端 marketplace plugin skill，并非所有本地 discovered skill 的通用详情 API。[详情实际范围](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/request_processors/plugins.rs#L1204-L1243)两家都不能取代 Flame 已有的提案审阅、归档和恢复。

### 6.2 建议的最小增量：同一个 resolver 提供详情

先让用户能回答四个问题：这个技能从哪里来、为什么这个版本胜出、内容是什么、为什么看得到却不能使用。可以在现有 discovered 资源上增加按当前有效身份读取的详情，返回 bounded instructions、source / scope、内容 revision 与必要诊断。

不需要立即发明全球统一 skill ID 或市场包模型；在 Flame 当前按名称覆盖的规则下，WorkspaceRef + 有效名称可继续定位，响应明确实际来源和版本即可。关键在于由实际 resolver 输出，不能前端自己扫描目录、拼 ~/.flame 路径或重新实现 project / user 优先级。[实际技能读取与解析 owner](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/application/workspace/skills.go#L74-L102)

列表仍然轻量，详情按需读取；无效或缺依赖项可保留必要诊断，但不把所有文件系统错误暴露成未经处理的原始路径与堆栈。这里的详情方法和字段是建议，不是声称 Flame 当前已实现或 Codex 每一个 metadata 都适合照搬。

### 6.3 已有 revision 应产生可分支的冲突

Flame 的 SkillProposalRef 已包含 exact revision，这是“批准自己看到的那份内容”的可靠基础。问题在于 handler 把 ErrConflict、ErrProposalChanged、ErrNotFound 映为 invalid_params，客户端难以区分“请求写错”和“审阅对象改变了”。[Proposal 错误映射](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/delivery/handler_skills.go#L122-L149)、[领域冲突处理](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/delivery/handler_skills.go#L187-L199)

最小改进是按真实恢复分支复用 revision_conflict 或明确的 proposal-state error：告诉 UI 重新读哪个对象、保留审阅位置、提示用户内容已经变更。不要解析错误字符串，也不必给所有 archive / restore 操作强加 CAS。当前前端无论成功失败都会重读相关 catalogs，恢复方向是正确的；typed conflict 可以让解释更准确。[现有审阅刷新](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/desktop/frontend/src/plugins/builtin/workspace/application/skillCuration.ts#L45-L65)、[Codex 版本冲突与有效值处理](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/config_manager_service.rs#L218-L441)

### 6.4 enabled 仍不保证 callable

Codex skills/config/write 返回 effectiveEnabled，但所读实现直接用请求的 enabled 填这个字段，不能据名称就说它已验证所有依赖、策略和运行环境。OpenCode HTTP skill.list 也不等于特定 Session 权限过滤后的可执行集合。[effectiveEnabled 实际构造](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/request_processors/catalog_processor.rs#L648-L684)、[list 与权限过滤](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/core/src/skill.ts#L1-L134)

对 Flame 更有用的规则是：显示状态要对应可证明事实；归档影响后续解析，不能擦掉已经注入当前上下文的 instructions。仅在 UI 真正需要区分依赖、授权和生效时增加字段，避免为了表达一条很长的状态链先建一套复杂工作流。


## 7. 配置、模型与能力：写成功后发生了什么

### 7.1 借有效值解释，不搬入整套层级配置

Codex config/read 返回 effective config、origins 与版本，并可在 includeLayers 请求下附层级信息；写入面向 user layer，支持可选 expectedVersion，响应区分 ok 与 okOverridden，并给出 overridingLayer / effectiveValue。它解决“写了值却被更高层覆盖”的解释问题。expectedVersion 是可选条件，不能称为所有写入强制 CAS；RPC 串行化也不自动保证外部编辑器的并发一致性。[配置读取](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/v2/config.rs#L317-L408)、[写入响应](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/v2/config.rs#L345-L383)、[写入参数](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/v2/config.rs#L1068-L1109)、[版本与覆盖](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/config_manager_service.rs#L218-L441)

OpenCode GET /api/config（config.get）返回文档与目录来源，当前实验 patch 只处理 shell；location.reload 重建 loaded Location，影响 pending permission / form 等，范围较大。不能看到 reload 就认为它是无副作用、所有对象原子切换的热更新。[配置 API](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/protocol/src/groups/config.ts#L1-L47)、[配置来源](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/schema/src/config.ts#L110-L139)、[Location reload](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/protocol/src/groups/location.ts#L7-L64)

Flame 已有 provider、model roles、MCP、hooks、skills 等明确 owner 命令，适合目前产品。值得学习的是**返回值与实际生命周期一致**，而非通用 config blob、第二条整体写入口和跨所有领域的 reloader。

### 7.2 Flame 各配置域的真实生效边界

| 修改 | 当前成功响应主要证明 | 后续生效位置 | 应如何解释 |
| --- | --- | --- | --- |
| providers.update | 持久配置保存，返回脱敏 configured 资源 | provider probe、模型目录与 Run 组装 | configured 是配置满足性，不等于网络健康或当前 Run 换实例 |
| models.setUtilityRole | 持久保存并更新 live role cell | 下个 Run boundary | 新运行使用，不改写旧 Run 实际身份 |
| models.setEmbeddingRole | 持久保存并更新 live cell | 后续 search 读取 | 生效节奏与 UtilityRole 不同 |
| mcp.servers.update | registry 保存与策略协调；启用后进入 connecting | 后台连接及状态通知 | 保存与连接成功已经分离，应保留 |
| hooks.setTrust | 信任设置提交 | 下个 Run 重读 trust | 当前运行中的 hook 不被追溯撤销 |
| skills archive / restore | 管理文件更改提交，后续 catalog 更新 | 后续解析与输入材料构建 | 不抹去已注入模型上下文的正文 |

证据：[Provider configured](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/application/integration/models/providers.go#L76-L110)、[Provider 保存](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/application/integration/models/providers.go#L261-L289)、[Role 边界](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/application/integration/models/roles.go#L11-L80)、[MCP 更新](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/application/integration/mcp/servers.go#L93-L148)、[MCP 后台连接](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/application/integration/mcp/servers.go#L238-L267)、[Hook trust](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/delivery/handler_hooks.go#L81-L85)、[技能管理](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/application/workspace/skills.go#L135-L159)

最小工作通常是校准权威注释、生成参考与面板文案。只有实际有多个可选生效策略，才为对应 owner 添加 tagged result 或状态字段。不要给所有 command 都加 appliedAt、effectiveRevision、restartRequired；如果没有确定推进者，它们只会制造新的模糊状态。

Codex 也未消除这一问题：reloadUserConfig 排除 model / effort 等 session-static 默认值；单个 Thread 刷新失败可 warning 后继续。写成功不等于所有线程都成功应用。[reload 失败边界](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/request_processors/config_processor.rs#L356-L388)、[静态配置例外](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/v2/config.rs#L1068-L1109)

### 7.3 模型能力与实际运行身份

OpenCode Model.Info 表达 tools、输入输出模态、context / input / output limits、variants 与 cost；Codex model/list 和 provider capabilities 表达模态、reasoning effort、service tier、namespace tools 等。共同价值是客户端按能力呈现，服务端再校验，不按名称猜。[OpenCode Model.Info](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/schema/src/model.ts#L85-L162)、[Codex 模型能力](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/v2/model.rs#L38-L183)

Flame 已有 models / providers 与协议 capabilities，应继续按自身 Scope 的真实支持定义；无需把两家的产品枚举都变成 Flame 概念。设置页此刻选中什么，与旧 Run 实际使用什么不同。Session selection、Run admission、waiting continuation 各有冻结和兼容规则，前端不能用最新设置覆盖历史。[Flame 能力](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/capabilities.go)、[Session 模型选择](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/application/agent/runs/opening.go#L262-L294)、[continuation](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/adapter/agentexec/interaction_executor.go#L482-L623)

### 7.4 探测错误要可行动，且保持脱敏

Flame providers.test 的公开失败主要是 notConfigured / testFailed，MCP probe 对运行失败也用稳定通用失败。这保护凭证与内部细节，却限制界面区分认证失败、地址错误、超时和暂不可用。[Provider 探测映射](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/delivery/handler_providers.go#L81-L102)、[MCP 探测映射](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/delivery/handler_mcp.go#L81-L95)

若设置面板确有针对性引导需求，可由 integration 边界提供有限、脱敏的 reason enum 与诊断关联。不要直接透传 raw provider response，不让 UI 解析字符串猜状态。只抽取对应不同处理动作的公共分支，不为每个底层 error 建公共类型。

## 8. MCP、授权尝试与工具资源

### 8.1 配置、连接、认证与可用性分别表达

| 维度 | Flame | OpenCode v2 | Codex main |
| --- | --- | --- | --- |
| Server 配置 | 持久 registry，update 与状态 | list；实验 add / remove / connect / disconnect 为临时覆盖 | 配置来源、Thread 有效 context、reload |
| 运行状态 | disabled / disconnected / connecting / connected / failed / needsAuth 等 | pending / connected / disabled / failed / needs_auth | runtimeStatus、server info、toolsError、authStatus |
| 认证 | 已有 MCP authorization attempt 与查询 | Integration OAuth / command attempts，Credential 管理 | MCP OAuth；账号登录有 loginId / generation |
| 资源目录与读取 | 以公开方法为准；内部类型不自动成为 API | resources / templates catalog，无通用 resource-read route | 状态目录 + resource/read |
| 直接工具 | tools.invoke 只开放受控诊断集合 | MCP 控制与公共 RPC 不等于任意 Agent tool | mcpServer/tool/call 需 Thread 和策略校验 |
| 特定事件流 | Runtime 资源状态通知 | volatile event | 实验 hosted-app stream，独立 owner / 限额 |

Flame 状态机已经存在，不能再建议“加一个 connected boolean”。OpenCode 实验 MCP 覆盖可能只持续到重启，204 不代表永久配置保存；Flame 的持久 owner 更符合现有产品。Codex 状态查询可带 threadId，提醒我们全局目录与特定执行上下文的有效能力未必一致。[Flame MCP 契约](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/mcp.go)、[OpenCode MCP](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/protocol/src/groups/mcp.ts#L1-L104)、[Codex MCP 状态](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/v2/mcp.rs#L46-L205)、[有效配置与状态](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/request_processors/mcp_processor.rs#L268-L426)

### 8.2 OAuth attempt 是独立过程，Flame 已有这个方向

OpenCode Integration 与 Credential 分开，OAuth start 返回 attemptID、URL、instructions、mode 和时间，status / complete / cancel 独立；公开目录只投影 credential ID / label 或 env 名，不返回 access / refresh / key。存储 schema 有 secret 不代表公开目录泄露秘密。[授权尝试](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/protocol/src/groups/integration.ts#L1-L209)、[Attempt 状态](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/schema/src/integration.ts#L19-L118)、[公开 credential 摘要](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/schema/src/connection.ts#L1-L23)

Flame 已有授权尝试与 retention 能力，值得核对的是关闭 modal、重连、取消和过期后的 UI。Codex 用 loginId 与身份 generation 防止旧登录结果污染新流程，可作为将来多账号 OAuth 的原则；无需复制账号套餐等商业服务。[Flame 保留能力](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/capabilities.go)、[Codex 登录身份](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/v2/account.rs#L132-L189)、[身份代际保护](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/request_processors/account_processor.rs#L883-L948)

Run waiting 中断与 MCP 登录尝试是不同资源。不能因为都需要用户点击，就塞进无所不包的通用任务表。

### 8.3 Resource read 的价值在来源与权限

Codex resource/read 可关联 threadId、originCallId 和 connectorId；特定 hosted-app 资源需要从来源调用恢复 app scope。测试覆盖历史模式、压缩与重启后的来源读取，说明映射不应只依赖当前 UI 文字；这不等于所有 MCP 资源被永久保存。[来源校验](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/request_processors/mcp_processor.rs#L429-L570)、[来源恢复测试](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/tests/suite/v2/mcp_resource_origin.rs#L29-L278)

OpenCode 有 resource catalog，但无对应通用公共 read route；schema 出现 ResourceContent 不算已经实现前端读取。Flame 只有在工具结果需要图片、文档、widget 或 artifact 独立展示时，才补资源读取：稳定引用、所属作用域、call 来源和读取时授权。不要任意取 URL / 主机路径，也不必为读已存在结果再启动 AI Run。[资源 API 范围](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/protocol/src/groups/mcp.ts#L1-L104)

### 8.4 tools.invoke 不应长成万能出口

Flame directTools 明确只有 read / glob / grep，属于受控诊断。Agent 内部 shell、写文件或 LSP 不代表前端可从此公开调用。[直接调用集合](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/adapter/toolset/direct_diagnostics.go#L20-L34)、[诊断边界](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/application/workspace/diagnostic_tools.go#L13-L26)、[公开 handler](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/delivery/handler_tools.go#L31-L45)

Codex 直接 MCP tool call 要求 threadId，加载 Thread 并检查 direct input 策略，再进入该 runtime。可借鉴的是调用者和有效上下文明确；不能学成任意 server / tool / arguments 的无约束执行。传输错误、MCP isError 和工具业务负面结果也应分层。[Direct MCP 调用](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/request_processors/mcp_processor.rs#L429-L570)

### 8.5 目录刷新应保留最后已提交的事实

Codex app/installed 返回 committed runtime snapshot 的 enabled / callable，refresh 失败保留旧 snapshot 并报错；plugin/reconcile.changedPlugins 只表示本次包或安装 / 启用状态变化，不是 runtime readiness。[Committed snapshot](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/request_processors/apps_processor/installed.rs#L35-L201)、[刷新失败测试](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/tests/suite/v2/app_installed.rs#L244-L403)、[reconcile 边界](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/v2/plugin.rs#L195-L239)

Flame 无需增加 Apps / marketplace；这个原则可用于现有 MCP 或技能目录：保留最后有效结果并明确刷新失败，避免“刷新中”被理解为“所有能力消失”。是否仍可调用由 Runtime 当前策略决定，缓存不能授权执行。

Codex 实验 MCP stream 只面向 hosted apps，要求连接订阅对应 Thread，等 active 才 ready，有连接限额和有限重试。它不是任意 MCP 的通用可靠事件流。Flame 可借 owner、ready 屏障和停止清理，暂无消费者则不引入。[MCP stream 生命周期](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/request_processors/mcp_event_stream.rs#L28-L284)

## 9. 旁路一致性：读模型与失效提示

### 9.1 三家共同支持“当前查询 + 变更提示”

| 方面 | Flame | OpenCode v2 | Codex main |
| --- | --- | --- | --- |
| 默认恢复方式 | Runtime subscription / sequence / resync，查询读模型 | volatile event，断连后重新同步目录 | 各资源独立通知与重新读取 |
| 作用域保护 | generation / retarget，workspace watch 和资源 IDs | effective Location、canonical cache key | watch / process 连接所有；其他目录 scope 各异 |
| 刷新合并 | 当前按资源类别较粗地失效 | 同 key 串行合并，MCP burst 合并，部分按消费者需求 | 目录 watcher / changed 多为粗失效 |
| 是否所有变更可重放 | 旁路 sequence 不是无限日志 | 主 event 无历史重放承诺 | 一般旁路通知无 durable cursor |
| 值得保留或借鉴 | resync 与旧 generation 拒绝 | 刷新成本控制 | 显式 owner、停止屏障 |

OpenCode createSync 吸收尚未开始的同类刷新，MCP 状态风暴会合并；断连时失效缓存，重连先恢复必要资源，其他目录按消费者继续同步。它并不是每次重连立刻读取全部 catalog。[Scope cache](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/client/src/solid/data.ts#L145-L201)、[刷新合并](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/client/src/solid/data.ts#L1175-L1296)、[事件与重连](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/client/src/solid/data.ts#L1840-L1937)、[刷新测试](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/client/test/solid-refresh.test.ts#L1-L251)

server.connected 后的具体重读范围可另见[重连 hydration](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/client/src/solid/data.ts#L593-L626)。

### 9.2 Flame 可以更充分使用现有 scope 信息

RuntimeEvent 已有 paths / workspace / watchId、names、serverIds、scheduleIds、sessionIds / runIds 和 resync topics。WorkspaceEventLike 当前主要保留 type / sequence / sessionIds / topics，query invalidation 多按整类 key 执行，因此一处文件变化可能使文件读取、目录、head、搜索、diff 和相关 catalog 一起重读。[Wire 资源提示](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/runtime_events.go#L131-L162)、[客户端简化](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/desktop/frontend/src/plugins/builtin/workspace/domain/eventInvalidation.ts#L55-L84)、[整类 query 失效](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/desktop/frontend/src/plugins/builtin/workspace/adapters/queryInvalidation.ts#L79-L103)

这首先是保守可靠的实现，不是已经证实的性能缺陷。应先测量事件 burst 下的请求数、字节数、延迟和无消费者查询；若重复 IO 显著，再传递已有 scope / IDs，按资源缩小范围。遇到 ignore / config 文件改变发现规则、scope 不完整或 resync，仍需整类重读；不能为了省请求漏掉依赖变化。

Flame 现有 loop 在打开 tail 后再 invalidateAll，sequence 只在订阅 generation 内有意义，旧 retarget 事件不会写到新 workspace。这一顺序与代际保护应保留。旁路 seq 连续规则也不能直接套到核心 Run eventId 或 OpenCode aggregate seq。[订阅与重读顺序](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/desktop/frontend/src/plugins/builtin/workspace/application/workspaceEventLoop.ts#L94-L166)

### 9.3 失效提示不应变成第二个业务写主

收到 files.changed 后让文件 query 重读，与用事件里的一段数据直接修改所有文件状态，是不同一致性模型。Flame 的 Runtime 是业务 owner，前端缓存和 invalidation 只服务可重建投影。即使以后缩小刷新范围，也应保留这一边界；不必让所有旁路通知都携带完整业务值，或为其建立永久事件日志。

Codex skills/changed 是粗失效，watcher 对远端环境还有范围限制；它不能被理解为包含 skill revision 的可靠变更流。OpenCode experimental Session log 也不是旁路 cache 的通用恢复通道。[技能失效与环境边界](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/skills_watcher.rs#L25-L168)、[目录恢复方式](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/client/src/solid/data.ts#L1840-L1937)

### 9.4 Watch 停止 ACK 可以成为明确屏障

Codex fs/watch 是 connection + watchId 的非递归 watcher，200 ms debounce；unwatch 等任务停止再响应，之后不应有该 watcher 通知，连接关闭清理。changedPaths 无完整 journal 字段，适合重读。[Watch owner 与停止](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/fs_watch.rs#L27-L170)

Flame 已有自己的 watch / topic 体系，无需复制第二组 API。可借鉴的是：任何新订阅都说明创建成功时是否已 active、停止后是否还会有数据、断线是否销毁。如果做不到某个屏障，应如实声明并保留前端 generation 保护，不能用“最终一致”省略生命周期。

## 10. 终端与进程：流式旁路有独立语义

### 10.1 先确认交互终端需求

Flame manifest 没有通用 terminal / PTY / process API，direct tools 只开放 read / glob / grep。内部 LSP integration 与模型工具不等于 Desktop 通用 LSP 管理端点；Agent 工具输出也不等于可写 stdin、resize、跨窗口接管的终端。[当前公开方法](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/contract/API_REFERENCE.md#L36-L100)、[内部 LSP 工具](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/adapter/toolset/builtin/lsp.go#L1-L30)

若产品只展示 Agent 命令输出，继续用 Item 和工具结果。若用户要主动操作终端，需要独立 process / terminal owner，不能把每次键盘输入塞进 runs.steer。

### 10.2 多种生命周期不能混成一个 create

| 对象 | ACK 与输出 | 结束 / 所有权 |
| --- | --- | --- |
| OpenCode Shell | 创建非交互命令，按 byte cursor / limit 分页，默认 64 KiB | Location 资源；list 只列 running，结束后可按保留规则 get |
| OpenCode PTY | HTTP 控制，WS IO；attach 有 replay → live 交接 | Location 与 attachment |
| OpenCode persistent PTY | 实验 Session / daemon 资源，replay 范围、controller / observer / takeover | 跨连接能力明确，维护成本高 |
| Codex command/exec | 可先推输出；最终响应等退出与 output drain；不重复流正文 | connection + processId；断线终止 |
| Codex process/spawn | spawn 并注册后立即 ACK；outputDelta / exited 后续到达 | connection + handle；断线终止；实验、无 Codex sandbox |

证据：[Shell API](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/protocol/src/groups/shell.ts#L1-L89)、[输出游标](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/schema/src/shell.ts#L23-L82)、[PTY API](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/protocol/src/groups/pty.ts#L1-L145)、[Persistent PTY](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/schema/src/persistent-pty.ts#L1-L71)、[command/exec](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/v2/command_exec.rs#L21-L213)、[process/spawn](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/v2/process.rs#L19-L203)、[输出 drain 与 final](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/command_exec.rs#L481-L626)

值得借鉴的细节是 argv 与 shell 意图区分、stdin bytes 与 close 分开、resize 合法性、stdout / stderr 合并规则、handle 冲突、默认限额和超时、结束后是否可重读。Codex 两族过程的 nullable / cap 编码与 ACK 存在差异；Flame 只需要一种过程时就选一种清楚契约，避免复制历史双轨。

### 10.3 输出位置与恢复不能只用字符串长度

OpenCode Shell 的 output / cursor / size / truncated 以 byte 为游标单位，默认每次 64 KiB。其 byte slice 转 UTF-8 的实现提示一个边界：切在多字节字符中间可能产生替换字符，这是静态推断，本次未做回归复现。Flame 可传 bytes 或保证切片合法；JSON string length 不能冒充 byte offset。[输出切片](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/core/src/shell.ts#L195-L225)、[输出单位与默认限制](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/schema/src/shell.ts#L23-L82)

Persistent PTY 的 attached 帧包含 role、generation、replay requested / available / end offsets 和 truncated，再发送 replay_complete 后转 live。这些是有用的恢复契约，也需要 screen snapshot、输入控制权、daemon 清理和慢消费者策略。已审查的 OpenCode 服务器 outbox 使用无界 queue，不能据此放弃 Flame 的有限缓冲原则。[replay 交接](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/server/src/handlers/persistent-pty.ts#L109-L229)、[PTY outbox 与连接](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/server/src/handlers/pty.ts#L135-L226)

如果产品希望刷新窗口后终端继续，需要独立可重连资源与保留策略。把连接 handle 改名为 terminalSessionId 不会自动赋予这些能力；普通一次性命令也没有必要一律升级成持久 daemon。

### 10.4 浏览器 WS 短票据按需采用

OpenCode 通过常规 HTTP 授权链领取 60 秒、单次、scope 绑定票据，再升级 WebSocket，并做 Origin 校验和原子消费；启用 server auth 时先校验凭证。这适用于浏览器终端；Flame 若采用，应绑定实际 Runtime / workspace / terminal，而不仅是路径，避免长期凭证进入 URL。[票据保留与消费](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/core/src/pty/ticket.ts#L1-L57)、[签票与升级](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/server/src/handlers/pty.ts#L112-L159)、[可配置 HTTP 授权](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/server/src/middleware/authorization.ts#L47-L64)、[单次与作用域测试](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/core/test/pty/ticket.test.ts#L1-L65)

这个建议不要求核心 AI 流改承载。Flame 现有 HTTP / SSE 控制与观察已服务核心任务；只有终端对双向频率、bytes 和延迟提出真实需求时，再局部选通道。

## 11. 其他 API 领域：保留 Flame 的产品差异

### 11.1 Hooks 与长期授权规则

Flame hooks.list / setTrust 与 approval 模式、规则查询 / forget 已是公开能力。ApprovalRule 有 session / project / global scope、tool 与 subject，subject 可约束命令或路径；长期规则与一次 pending interrupt 的答复是不同类型。这个划分值得保留，不应将“批准一次”误做永久工具放行。[授权模式与规则](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/approval.go)、[Hook 目录](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/hooks.go)、[Trust 生效边界](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/delivery/handler_hooks.go#L81-L85)

OpenCode Permission / Form 面向执行中交互；Codex 除审批请求还提供 hooks/list 等来源与状态目录。可以吸收的经验是来源、适用范围和失效状态可见，不能因为参考项目类型多就扩大 Flame 审批 union。一次交互、持久策略与下一 Run 的配置各由现有 owner 推进。[OpenCode Permission](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/core/src/permission.ts#L121-L146)、[Codex Hook 与审批方法](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/common.rs)

### 11.2 目标、计划与调度

Flame 已有 goals start / update / clear / get / stop / resume，schedules list / create / update / delete / runNow，以及 plan.get；这些资源是 Flame 自身的产品能力，并非参考两家名字后补出来的模块。Schedule 带 revision，update 要 expectedRevision；runNow 返回 SessionID / RunID，将调度配置与实际执行身份连接。[公开方法集合](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/contract/manifest.json)、[Schedule 契约](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/schedules.go)

这里的启发来自两家共同的“控制对象与实际执行分开”原则：编辑计划不等于任务已经运行，runNow 成功后应使用返回的权威运行身份观察。没有必要把 scheduler、自主 Goal、人工输入队列与终端都装进通用 Job。OpenCode queue / Codex Turn 是核心交互机制，不是与 Flame Schedule 一对一的替代。

### 11.3 知识、记忆、Recipe 与 Agent 文档

Flame knowledge 以 cwd / projectRoot / home 定位，更新有 expectedRevision；AgentMemory 有 project / user scope、auto / user 来源、pending / active、pin 和 review，已经能表达用户审阅与机器提取的区别。recipes.list、agentDocs.list 又是不同材料目录。[知识版本](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/knowledge.go)、[记忆来源与审阅](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/agentmemory.go)、[材料目录](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/contract/manifest.json)

OpenCode command / agent / reference / skill 目录和 Codex skills / memory / plugin 提供相邻经验，但不能把它们全部压成“提示词文件”。共同可借鉴的约束是：来源明确、覆盖规则由同一 resolver 解释、读取与注入分开、用户修改与实际上下文的生效边界可说明。当前 AgentMemory 公共更新没有 knowledge 那样的 revision 条件；仅在出现多人 / 多窗口同时编辑或精确审阅要求时，再评估相应保护，不能据此断言已经发生覆盖错误。[OpenCode Reference](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/protocol/src/groups/reference.ts#L1-L43)、[Codex 相邻方法域](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/common.rs)

### 11.4 用量、模型调用记录与反馈

Flame usage.session / summary、modelInvocations.list 和 feedback.create 已在 manifest。这里应沿用可查询事实，不让前端按接收 token delta 数推算最终用量。反馈作为独立提交与核心 Run 状态分开，既不应阻塞正常历史读取，也不能因反馈上传失败将 Run 标成失败。[Flame 观测与反馈方法](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/contract/manifest.json)

Codex 的 account、rate limits、usage、反馈和诊断服务有自身账号与商业产品背景；OpenCode 的 provider / integration / model 成本信息也不是最终账单。Flame 可以学习来源和 snapshot / delta 的明确区分，不需要复制套餐、额度和商业账号枚举。对于没有深入追踪的计费算法，本文不作准确性或经济性比较。[Rate-limit 更新字段](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/v2/account.rs#L593-L605)、[Model cost 元信息](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/schema/src/model.ts#L85-L162)

### 11.5 Project、Worktree、Environment 与插件市场

OpenCode project / worktree 区分已知项目、工作树 inventory 和实际创建 / 删除；list 不应因查看目录就启动配置发现，create 可含 setup script，remove 有 forceRequired。Codex project / environment 是实验族，environment/status 观察状态而不启动或恢复环境。它们共同提示**读状态与触发副作用分开**。[Worktree 契约](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/protocol/src/groups/worktree.ts#L1-L78)、[Inventory 与创建](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/server/src/handlers/worktree.ts#L1-L64)、[Environment 状态](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/v2/environment.rs#L9-L128)、[状态读取测试](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/tests/suite/v2/environment_status.rs#L28-L157)

Flame 若未来支持隔离工作树或远端执行，才划清 project=组织、workspace=文件与权限范围、environment=执行机器、cwd=目录。单机一项目一目录并不因此落后；必要的是每个公开请求不依赖模糊的 process-global mutable cwd。

插件安装、更新、账户切换也有不同阶段。OpenCode plugin.update 可能部分成功后整体返回错误；Codex installed-state / runtime snapshot 明确区分。若 Flame 出现动态安装需求，应按项说明结果与实际生效，不能以一个 batch 失败误导用户“什么都没变”。当前无需为对标建设 marketplace / sharing / enterprise policy。[批量更新结果](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/server/src/handlers/plugin.ts#L62-L86)、[安装与 runtime readiness](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/v2/plugin.rs#L195-L239)

## 12. 横向协议规则：在已有注册体系内演进

### 12.1 不新增第二个协议源

Flame registry 已记录 query / command / subscription、unary / stream、错误、能力、分页、幂等与回放，生成前端类型和文档。OpenCode 分离 Schema / Protocol / Server / Client；Codex 从 Rust 类型生成 TS / JSON Schema。三家的共同经验支持 Flame 现有方向，而非再加手写 REST facade、手写 DTO 或新事件总线。[Registry 验证](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/delivery/registry.go#L101-L123)、[注册合同](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/delivery/registry.go#L175-L215)、[客户端依赖边界](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/client/README.md#L1-L23)、[协议生成规则](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/AGENTS.md)

新增 API 的最低说明集合应服务真实消费者：

| 问题 | 必须明确的语义 | 复用方式 |
| --- | --- | --- |
| 对哪个对象操作 | Workspace / Session / resource / connection / attempt 等身份 | 沿用现有 ref 与 owner |
| 成功是什么意思 | 已提交、已接纳、已启动、已应用或已结束 | 在具体响应与文档表达，不强制统一 ACK |
| 读取是否完整 | 完整集合、分页、摘要、截断、局部失败 | 沿用现有 DTO；只有新差异才加字段 |
| 命令如何重试 | idempotency、revision、未知结果 | 现有 endpoint / command journal |
| 通知丢失怎么办 | 重读、resync 或专门恢复 | 当前 runtime subscription / query layer |
| 是否改变核心执行 | 下个 Run、当前 owner live cell、持久中断答复等 | 禁止 UI 自行改历史或运行结论 |

### 12.2 分页不是随意加 cursor

Flame SDK 只对生成 manifest 标记为分页的方法 autopage；部分 UI 用 autoPagingToArray。这个机制已经存在，后续需要的是按屏幕需求决定是否读全，而非再造分页框架。大目录或长会话实测有瓶颈时，可渐进读取；cursor 是否跨查询参数、workspace、版本有效，需由具体 owner 定义。[生成分页消费](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/desktop/frontend/src/rpc/wireCallPath.ts#L207-L225)、[Session 读取](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/desktop/frontend/src/plugins/builtin/defaults/adapters/runtimeDataProviders.ts#L121-L126)、[文件列表读取](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/desktop/frontend/src/plugins/builtin/defaults/adapters/runtimeDataProviders.ts#L344-L356)

OpenCode 很多目录用数组或 limit；Codex MCP status 的 opaque cursor 当前为 offset。数组不等于坏设计，opaque 也不等于一致快照。只有数据量和变动性要求时才扩大承诺；不要把普通目录 continuation 与核心事件 replay cursor 混用。[Limit 型目录](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/protocol/src/groups/vcs.ts#L1-L104)、[MCP 分页实现](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/request_processors/mcp_processor.rs#L268-L426)

### 12.3 错误按用户能采取的动作分类

| 情况 | 应提供的区别 | 不应混为 |
| --- | --- | --- |
| Skill proposal 版本变化 | 重新审阅明确资源 | invalid_params |
| Diff 基线不可判定 | 选基线 / 换范围 | 一概服务不可用并自动重试 |
| 搜索执行失败 | 失败或部分结果 | 无命中 |
| Provider 探测失败 | 有限脱敏 reason | 原始异常全文或单一无法行动的字符串 |
| 配置被覆盖 | 保存成功但有效值不同 | 保存失败 |
| 连接成功而目录未就绪 | 状态 / discovery 问题 | 所有工具为空且永久不可用 |
| 命令 ACK 丢失 | 原身份继续核对 | 立即换 key 重做 |
| 批量操作部分提交 | 每项结果或可查询事实 | 整体原子回滚的错觉 |

这些是分支原则，不是要求所有 API 立即增加统一 errors 枚举。Flame 已有 ProblemData 与 RecoveryAction；只有新增分支改变真实消费者行为，才值得成为公开类型。有关未知结果和崩溃 reservation 的分析见配套核心报告，本报告不重复创建旁路幂等体系。[既有错误机制](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/errors.go#L5-L110)

## 13. 建议顺序与最小落地范围

以下优先级是设计判断，成本为相对范围，不是未经执行的工期估算。核心报告的 C0 权威 fold 异常恢复优先于性能和新增旁路能力。

| 编号 | 建议 | 价值与当前证据 | 最小落点 | 相对成本 / 触发 |
| --- | --- | --- | --- | --- |
| S1 | Skill proposal typed conflict | exact revision 已有，错误不能机器分支 | handler 映射、生成合同、审阅错误 UI | 低；可直接排入 |
| S2 | Diff 基线可见 | 现有两种模式明确，响应未给实际解析基线 | VCS owner 返回基线事实，UI 标注比较对象 | 低至中；先不扩 changeset |
| S3 | 配置生效语义校准 | 不同 owner 的 ACK / 生效节奏已经不同 | 权威注释、生成参考、设置文案 | 低；不引入统一 reloader |
| S4 | Skill 按需详情与来源 | discovered 目录轻但解释不足 | 同一 resolver 的有界 detail、必要诊断、详情面板 | 中；有明确用户查看需求 |
| S5 | 脱敏可行动 probe 原因 | 当前通用 testFailed 限制恢复指导 | Integration 错误分类与设置 UI | 低至中；按实际分支逐步做 |
| S6 | 细化旁路失效 | wire 已有 scope / IDs，UI 当前较粗 | 透传既有字段，query key 过滤，保留广域 fallback | 中；测得重复 IO 后做 |
| S7 | 运行范围 review / 分片 Diff | workspace diff 不能代替 Run 变化集，大材料有截断 | 复用 checkpoint / rollback owner，明确来源与可用性 | 中至高；真实 review 需求 |
| S8 | 二进制、编辑、artifact、终端 | 当前未公开不等于产品缺陷 | 每次选一个真实场景，明确边界 / 限额 / 冲突 | 条件项；不一次上齐 |
| S9 | 远端、多工作树、marketplace | 需要独立环境与身份生命周期 | 先定义产品用例和 owner | 后置；无需为参考项目预建 |

### 13.1 可以一起完成的第一批工作

S1、S2、S3 都是在现有 owner 上使事实更清楚，可以分别评审，不要求互相依赖。Skill proposal 的 revision 不应重新设计；Diff 先返回已解析基线，不先引入历史快照服务；设置先纠正文案，不先增加有效配置状态机。它们都应沿既有 schema / registry / generated types / consumer 的单一路径落地。

S4 的详情可以复用 discovered 解析，与 library proposal 审阅保持各自语义；S5 只收录确实对应不同修复方式的原因。这一批工作的验收是用户能够理解与核对，而不是 API 数量增加。

### 13.2 需要测量后决定的第二批工作

S6 的评价指标是相同资源变化下减少多少无效读取，同时不漏失效；不以“缓存代码更精巧”作为成功。S7 的评价指标是用户能否正确辨认范围、看到完整性、审阅被截断的大文件，以及并发编辑后能否明确拒绝过期操作。没有性能或使用数据时，保留当前可靠降级更合适。

### 13.3 明确不照搬的部分

- 不用 Codex host FS / 无沙箱 process 替代 Flame 工作区边界。
- 不将 OpenCode 的全文 skill.list 当作所有目录的默认加载方式。
- 不用 OpenCode 临时 MCP override 替代 Flame 持久配置 owner。
- 不用 Codex deprecated gitDiffToRemote 取代 Flame rich Diff。
- 不把粗 changed 通知、offset cursor 或有一个 log API 宣称为完整可靠回放。
- 不把所有旁路过程包装成 AI Run，也不开放任意工具调用作为新 UI 默认入口。
- 不照搬 persistent PTY daemon、marketplace、账号商业协议与多层配置体系，除非出现明确需求。

这些选择保留两家的有效原则，也保留 Flame 当前更合适的能力和复杂度边界。

## 14. 验收矩阵

| 场景 | 需要验证的可观察行为 | 对应范围 |
| --- | --- | --- |
| 同一路径在两个 Runtime / Workspace | 缓存、文件读、订阅互不串用 | Scope / 现有 query layer |
| symlink 指向根外 | 按 Flame 既定读取边界处理 | 文件 owner |
| UTF-8、binary、超大文本 | 内容类型、窗口和预算明确，不伪装完整 | FS / 新二进制能力 |
| 搜索 A 慢于 AB | 仅当前 generation 更新界面，失败与零命中不同 | 若新增增量搜索 |
| Diff 基线消失或变更 | 明确错误或实际 baseline，不能显示零改动 | S2 |
| rename / binary / untracked / 超大首文件 | 结构与完整度保留，截断有解释 | S2 / S7 |
| Run 期间有外部编辑 | 不宣称所有差异都由 Agent 造成 | S7 |
| 预览后文件被改 | apply 明确冲突，不盲目逆 patch | 仅新增写 / 撤回预览时 |
| Skill 同名覆盖 | 详情与执行 resolver 来源一致 | S4 |
| Skill 解析局部失败 | 可用项仍可见，诊断与空目录不同 | S4 |
| 批准后端已变化 proposal | typed conflict、保留审阅位置、重新读准确对象 | S1 |
| 更新 utility / embedding / hook / MCP | 各自真实生效边界与文案一致 | S3 |
| Provider / MCP 探测失败 | 有限脱敏原因可行动，不暴露 secret | S5 |
| OAuth 旧尝试迟到 / 过期 / 取消 | 不污染新尝试，状态可重新读取 | 既有授权流程 |
| 资源刷新失败 | 不把旧有效目录误清空为权威空集合 | MCP / Skill consumer |
| 事件 burst / 断线 / retarget | 合并正确，漏通知后 resync，旧范围不回写 | S6 |
| command ACK 丢失 | 保留同一身份，未知结果不自动重做 | 既有 idempotency，详核心报告 |
| watch 停止 | 若承诺屏障，ACK 后没有旧通知 | 新订阅生命周期 |
| 终端最后输出与 exit 竞争 | 输出 drain 后 final，不重复正文，bytes 位置一致 | 仅新增终端时 |
| 终端断连、慢读者、控制权交接 | 所有权、保留与上限按契约执行 | 仅新增终端时 |

已审阅的代表测试：[Flame 有界文件读](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/application/workspace/file_reads_test.go#L71-L210)、[OpenCode Location 浏览](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/server/test/fs.test.ts#L9-L79)、[OpenCode 路径边界](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/core/test/location-filesystem.test.ts#L34-L167)、[VCS 基线](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/server/test/vcs.test.ts#L11-L78)、[刷新合并](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/client/test/solid-refresh.test.ts#L1-L251)、[Codex 二进制与 FS](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/tests/suite/v2/fs.rs#L330-L550)、[配置版本](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/tests/suite/v2/config_rpc.rs#L2013-L2158)、[逐 cwd Skill](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/tests/suite/v2/skills_list.rs#L672-L1102)、[进程隔离与输出](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/tests/suite/v2/command_exec.rs#L899-L1280)。

本次未执行这些测试，也未实施上表新增验证。后续改进只对改变的可观察行为和实际缺口补断言，复用既有设施；不应为只改文案或低风险可逆调整创建镜像实现的测试。

## 15. 可核对的 API 索引与覆盖边界

### 15.1 Flame 当前方法索引

以下按生成 manifest 分组，包含核心交界资源，合计 87 个方法、4 个流式方法、2 个通知名称。列出交界资源是为了核对覆盖，不代表它们全部属于旁路。具体类型、幂等、分页和错误以[固定 manifest](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/contract/manifest.json)及[API reference](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/contract/API_REFERENCE.md)为准。

| 方法族 | 方法 | 主要关系 |
| --- | --- | --- |
| runtime | runtime.discover、runtime.subscribe | 共享协议 / 资源订阅 |
| sessions | sessions.list、sessions.get、sessions.snapshot、sessions.create、sessions.update、sessions.delete、sessions.fork、sessions.rollback、sessions.export、sessions.import | 核心 / 历史交界 |
| modelInvocations | modelInvocations.list | 核心 / 历史交界 |
| runs | runs.start、runs.resume、runs.subscribe、runs.cancel、runs.steer、runs.get、runs.list | 核心 / 历史交界 |
| interrupts | interrupts.list | 核心 / 历史交界 |
| plan | plan.get | 核心 / 历史交界 |
| items | items.list | 核心 / 历史交界 |
| workspaces | workspaces.resolve、workspaces.list | 旁路或跨域控制 |
| workspace | workspace.changes.list、workspace.diff.get、workspace.files.head、workspace.files.search、workspace.files.list、workspace.files.read | 旁路或跨域控制 |
| skills | skills.discovered.list、skills.library.list、skills.library.archive、skills.library.restore、skills.proposals.list、skills.proposals.approve、skills.proposals.reject | 旁路或跨域控制 |
| recipes | recipes.list | 旁路或跨域控制 |
| agentDocs | agentDocs.list | 旁路或跨域控制 |
| mcp | mcp.servers.list、mcp.servers.create、mcp.servers.update、mcp.servers.delete、mcp.servers.test、mcp.tools.list、mcp.servers.reconnect、mcp.authorizationAttempts.create、mcp.authorizationAttempts.get | 旁路或跨域控制 |
| hooks | hooks.list、hooks.setTrust | 旁路或跨域控制 |
| approval | approval.getMode、approval.setMode、approval.listRules、approval.forgetRule | 旁路或跨域控制 |
| schedules | schedules.list、schedules.create、schedules.update、schedules.delete、schedules.runNow | 旁路或跨域控制 |
| goals | goals.start、goals.update、goals.clear、goals.get、goals.stop、goals.resume | 旁路或跨域控制 |
| providers | providers.list、providers.update、providers.test | 旁路或跨域控制 |
| models | models.list、models.getUtilityRole、models.setUtilityRole、models.getEmbeddingRole、models.setEmbeddingRole | 旁路或跨域控制 |
| tools | tools.list、tools.invoke | 旁路或跨域控制 |
| usage | usage.session、usage.summary | 旁路或跨域控制 |
| knowledge | knowledge.list、knowledge.get、knowledge.update | 旁路或跨域控制 |
| agentMemory | agentMemory.list、agentMemory.review、agentMemory.update、agentMemory.delete、agentMemory.add | 旁路或跨域控制 |
| feedback | feedback.create | 旁路或跨域控制 |

### 15.2 OpenCode v2 旁路组索引

本次固定 OpenAPI 实际有 113 个路径、136 个 HTTP 操作；其中 29 个操作路径包含 experimental。下表按报告组织口径列出 24 个旁路组、82 个操作，排除 Session / Permission / Form / Event / Generate 的核心组；Session diff / revert 等交界能力已在正文分析。计数用于确认范围，不是成熟度评分；非 experimental 不等于 GA 承诺。[当前 OpenAPI](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/protocol/openapi.json)、[API 声明](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/protocol/src/api.ts#L140-L215)

| 组 | 操作数 | experimental 操作数 | Operation ID |
| --- | --- | --- | --- |
| server | 1 | 0 | server.info |
| location | 2 | 0 | location.get、location.reload |
| agent | 2 | 0 | agent.list、agent.get |
| plugin | 3 | 0 | plugin.list、plugin.check、plugin.update |
| model | 2 | 0 | model.list、model.default |
| provider | 2 | 0 | provider.list、provider.get |
| integration | 11 | 1 | integration.list、integration.get、experimental.integration.wellknown.add、integration.connect.key、integration.oauth.connect、integration.oauth.status、integration.oauth.cancel、integration.oauth.complete、integration.command.connect、integration.command.status、integration.command.cancel |
| mcp | 6 | 4 | mcp.list、experimental.mcp.add、experimental.mcp.remove、experimental.mcp.connect、experimental.mcp.disconnect、mcp.resource.catalog |
| credential | 3 | 0 | credential.update、credential.remove、credential.activate |
| project | 2 | 0 | project.list、project.update |
| filesystem | 4 | 1 | fs.read、fs.list、fs.find、experimental.fs.write |
| command | 1 | 0 | command.list |
| skill | 1 | 0 | skill.list |
| rpc | 1 | 0 | rpc.call |
| pty | 7 | 0 | pty.list、pty.create、pty.get、pty.update、pty.remove、pty.connect.token、pty.connect |
| persistentPty | 11 | 11 | server.experimental.persistentPty.read、server.experimental.persistentPty.list、server.experimental.persistentPty.create、server.experimental.persistentPty.shutdown、server.experimental.persistentPty.handoff、server.experimental.persistentPty.get、server.experimental.persistentPty.update、server.experimental.persistentPty.remove、server.experimental.persistentPty.snapshot、server.experimental.persistentPty.connectToken、persistentPty.connect |
| shell | 5 | 0 | shell.list、shell.create、shell.get、shell.remove、shell.output |
| reference | 1 | 0 | reference.list |
| worktree | 4 | 0 | worktree.list、worktree.create、worktree.remove、worktree.refresh |
| vcs | 5 | 0 | vcs.get、vcs.base、vcs.status、vcs.branch.list、vcs.diff |
| debug | 2 | 0 | debug.location.list、debug.location.evict |
| migration | 1 | 1 | experimental.migration.v1.status |
| websearch | 2 | 0 | websearch.providers、websearch.query |
| config | 3 | 1 | config.get、config.shells、experimental.config.update |

### 15.3 Codex 公开能力覆盖索引

| 域 | 代表方法 / 资源 | 本次证据深度与范围 |
| --- | --- | --- |
| FS / watch | fs/readFile、writeFile、readDirectory、getMetadata、copy、remove、createDirectory、watch / unwatch | 类型、handler、owner、公共测试；host local |
| 搜索 | fuzzyFileSearch 与实验 sessionStart / Update / Stop | latest query、取消和通知 scope |
| Diff | turn/diff/updated、FileChange、gitDiffToRemote | 三种表面分别追踪；最后一种为废弃 v1 |
| 过程 | command/exec 与相关输入 / resize / terminate；实验 process/spawn 族 | ACK、输出 drain、连接 owner、公共测试 |
| Skills / Hooks | skills/list、config/write、extraRoots/set、changed；hooks/list | 技能深入到消费者契约与 watcher；Hook 做协议范围审阅 |
| Plugin | list、installed、read、skill/read、install / uninstall / reconcile、marketplace 与 share | 重点核对目录、来源、安装与 readiness；不评估完整市场产品 |
| MCP | status、resource/read、tool/call、OAuth、reload；实验 event stream | 有效 Thread、来源、policy、owner 与测试 |
| Apps | app/list / read / installed、更新通知 | 重点追踪 committed snapshot 与刷新失败 |
| 配置 / 模型 | config/read、value/write、batchWrite、requirements；model/list 与 provider capabilities | 版本、来源、有效值、reload 与类型能力 |
| Auth / account | login / cancel / logout / read、usage / rate limits 等 | 登录代际深入追踪；商业计费仅接口范围 |
| Project / environment | 项目 CRUD / import、环境 add / info / status | 实验族；状态读取与副作用边界 |
| Attachment | thread/attachment/add / list / remove、updated | 持久身份、去重、分页与提交顺序 |
| 其他 | feedback、external config、remote control、Windows sandbox、memory、rollout、diagnostics | 注册 / 类型目录核对；未声称完整实现或业务质量审计 |

注册入口：[common.rs](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/common.rs)。本表将相邻领域合并展示；正文对 Flame 有关的文件、Diff、Skills、配置、MCP、终端和一致性做深入分析，其他功能给出边界与后置理由。

## 16. 与核心报告一起使用

核心报告用于确认“输入与命令如何进入 Runtime、事实怎样被流式观察、断线与异常怎样恢复”；本报告用于确认“读什么材料、以谁为准、范围和新鲜度是什么、资源命令何时生效”。二者共用同一个 Runtime 所有权模型。

建议实施时先处理核心权威投影的异常恢复与输入精确关联，再并行推进旁路低成本契约改进。每一项都要求有真实消费者、明确 owner 和可验证行为；参考项目提供设计证据，最终协议仍应服务 Flame 自己的产品。
