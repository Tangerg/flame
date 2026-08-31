# Flame Runtime 合同基线

> 状态：Runtime Protocol Baseline 2
>
> 基线日期：2026-08-30
>
> 适用范围：Runtime Protocol 制品、持久化 shape、Agent Framework 消费边界和重构期间的内部防腐合同

本文只记录可被机器比较的边界事实和版本。它不是向旧消费者承诺兼容；仓库允许 breaking change，但任何变化必须显式、一次性、可验收。

## 1. 基线含义

Runtime 是应用后端，同时从模块根提供公共 Go binding。只有模块根、`protocol` 与窄部署交接包 `localruntime` 的 exported surface 构成 Go API；`internal` exported identifiers 仍只服务模块内组合。因此本基线不冻结全部内部 `go doc`，而冻结四类真实合同：

1. 外部 Runtime Protocol、公共 Go root/protocol/localruntime surface 和生成制品；
2. SQLite/artifact/checkpoint 等持久 shape；
3. Application 与 Agent adapter 的防腐边界；
4. Clean Architecture import DAG 和外部 SDK isolation。

任何基线变化必须：

- 有对应 ADR 或已授权阶段；
- 同批更新 owner codec/schema/GoDoc/tests；
- 直接替换旧 shape，不保留 alias、双读或兼容 shim；
- 更新本文件和自动守卫；
- 运行该 owner 的 strict round-trip、malformed input、integration 和 consumer tests。

Digest 只用于发现未审计漂移，不能替代语义测试。

当前 Runtime 与 Desktop module 统一使用 Go `1.27.0`。重构代码使用该版本已经提供的标准库和测试能力，不引入为旧 Go 版本服务的兼容写法；两个 app module 的 `go` directive 与 Desktop 隔离 workspace 必须保持一致。

P186 为仓库内 85 个非 `app/**` Go module 建立首个 canonical `v0.0.1`，标签名称精确为 `<module-dir>/v0.0.1`；上层 module 只引用已发布的下层正式版本。4 个 app module 不属于该发布集合，`runtime` 只把非 app Scope 依赖切换为 `v0.0.1`，内部 `localruntime` 部署交接仍由 app 自身版本拥有。该发布事实不改变 Runtime Protocol、公共 Go surface、Artifact、SQLite、Agent Framework contract 或 Desktop generated binding。

P187 将产品身份一次性切换为 `flame` / `Flame`：协议元数据、OpenRPC 扩展键、生成 TypeScript package 与公共客户端名称不再发布旧品牌。Runtime 只接受 `FLAME_*` 环境变量，默认 durability 只位于 `~/.flame`，知识文件只名为 `FLAME.md`；旧环境变量、目录、文件名、别名与兼容 reader 均不存在。共享 `localruntime.DataDirectory` 是 Runtime/Desktop 对 database 与 local-token 布局的唯一部署值对象。该 breaking cutover 将 Protocol 精确前移到 `2026-08-28`；Artifact v23 与 SQLite epoch 83 的数据 shape 不变。

P188 不改变 Runtime Protocol Baseline 2、Artifact v23、SQLite epoch 83 或 public Go/Desktop binding shape。它把内部模型上下文的预算检查从 terminal Run maintenance 扩展到每次主模型调用前，但只有 message/token footprint达到阈值才执行压缩与 durable rewrite；阈值以下不运行 hook、摘要或写事务，压缩后也不立即重复。durable root rewrite继续使用同一 SQLite transaction/CAS owner，transient Delegate只更新 Agent recovery state；compaction Item仍使用既有 Protocol shape。Runtime 独立 module消费 canonical `agent/v0.2.0`、Agent Baseline 33 与 Interaction state/protocol v8/v8，不双读 v7 settlement。Agent 的 `ToolInvocation.ModelResult` 是普通 Tool model-visible result 的唯一映射 owner；Runtime 不从 client presentation 反推 provider Conversation。

P189 不改变任何 public contract或持久化 shape。Runtime internal context provenance新增 isolated `session_goal` data source，并把 `session_plan`从 instruction重新归类为 data；两者只在每次主模型调用的 fixed Session-state context中出现，不进入 Deployment identity、Conversation store、summary或 public Protocol。configured Goal/Plan read failure与非法/foreign state现在 fail closed。Agent release、Interaction protocol/state version、Artifact v23、SQLite epoch 83、Desktop generated binding与 CLI均保持不变。

P190 不改变任何生产合同、wire或storage shape。真实retry-exhaustion与SIGKILL回归确认SQLite conversation+Run watermark事务是compaction唯一durable commit；运行中的未结算Strategy generation不构成可恢复状态，失败后按既有failed/lost语义退出。没有新增journal、checkpoint字段、两阶段提交、SQLite epoch、Agent protocol、Runtime operation/event或Desktop binding；HTTP E2E测试数增至45。

P191 不改变 Runtime Protocol、Artifact v23、SQLite epoch 83、Desktop binding、Agent Framework release或CLI。主模型上下文预算改为完整provider-neutral request的单一估算：全部Message Part、metadata、Tool manifest与Options同属一个owner，media transport payload不冒充文本token；provider成功响应的input usage校准同一Process下一次调用的阈值判断。低于message/token阈值仍不运行hook、summary或rewrite。executor-owned opaque Interaction checkpoint envelope因新增per-Process calibration从schema v3一次性升至v4；这是Runtime internal recovery shape，旧schema确定拒绝且不双读。

P192 不改变 Runtime Protocol、Artifact v23、SQLite epoch 83、Desktop binding、Agent Framework release或CLI。Runtime internal `CompactionConfig` 与调用签名一次性增加provider catalog已有的`MaxInputTokens`事实；有效token阈值不超过selected model硬输入上限，且不从default model向部分已知的selected model借用硬限。低于阈值的路径语义、provider usage校准、checkpoint schema v4与所有公开合同保持不变。

P193 不改变 Runtime Protocol、Artifact v23、SQLite epoch 83、Desktop binding、Agent Framework release、checkpoint schema v4或CLI。Runtime internal model-limit边界一次性收敛为immutable `modelref.TokenLimits`，把provider独立发布的context window、max input与max output事实映射到同一值对象；显式`runs.start.params.maxTokens`必须在durable Run admission前满足selected catalog model的output上限，并从总context中保留对应generation空间。catalog未知的私有兼容model仍允许通过，未显式设置output ceiling时不猜provider默认值。provider usage为零表示缺失，不产生虚构校准；负值、溢出与其他非法usage继续由既有`chat.Response`校验拒绝，正值仍是同一Process下一次调用的唯一校准事实。

P196 将模型选择从 exact provider/model pair 一次性扩展为 exact provider/model + 可选、model-owned `reasoningEffort`。Session 是可编辑默认的唯一 durable owner；Run 在 opening 时冻结自己的选择，Goal 与 Schedule 同样冻结完整选择，恢复、fork、occurrence、interrupt、checkpoint 与 Artifact 不得丢失该事实。仅修改 effort 保留 identity；显式切换 provider/model 而未同时给 effort 时清空旧 effort，禁止把上一模型的等级泄漏给新模型。已知 catalog model 在任何 durable write/staging 前校验 effort 是否属于其精确等级集合；catalog miss 的私有模型继续允许。`runs.resume` 的可选 `input` 与已接受 HITL responses 在同一 continuation opening 中提交：answer claim、用户 Item、checkpoint removal 与 next Segment 只有一个 durable winner；Agent 在同一 safe boundary 先结算 answer Tool result、再应用 input steer，Conversation 按同一合法 `tool → user` 顺序各追加一次，已由 opening 创建的 exact Item 不得重复投影。Artifact 前移到 v24、SQLite 前移到 epoch 84、executor-owned Interaction checkpoint envelope 前移到 private schema v5；旧 shape 全部确定性拒绝，不双读、不迁移。Desktop 直接消费 catalog 的 context/input/output limits、reasoning/default/levels、modalities、tool use 与 structured-output 事实；Composer 控件写回同一选择，active Run 的 Context gauge 使用该 Run 冻结的选择，不被 Session 后续默认修改污染。发布图最终落为 `models/catalog/v0.0.2`、`runtime/localruntime/v0.0.1`、`runtime/v0.0.1` 与 Wails beta.15 `desktop/v0.0.2`；`desktop/v0.0.1` 保留为不可变初始发布。CLI/TUI 不变。

P197 不改变 Runtime Protocol Baseline 2、Artifact v24、SQLite epoch 84、Desktop binding、Agent Framework 或 CLI/TUI。它 breaking 删除通用 tools module 的六方法 `fs.Executor`、公开 mutable `LocalExecutor.Root` 以及 Glob/Grep backend input 的 authority-replacing `Root`；新的六个单操作端口与 `Path` 子树语义由所有仓库消费者同批迁移。Local backend 的构造根经 `os.Root` 成为不可扩张 capability，默认空参数冻结构造时 CWD，不再授予无限 host filesystem；Runtime model mutations 不再因 non-isolated mode 扩张到 workspace 外。共享 scanner 从 Runtime internal 移到 `tools/textread`，不产生 wire/storage shape，也不为旧 Go API 保留 alias/shim；本批不创建 release tag。

P198 将 compaction summary 从“仅写回模型上下文、用户 transcript 为空”的重复表示收敛为一个 canonical 领域值。Run-boundary 与每次主模型调用前的长上下文压缩都发布同一原始摘要；模型专用 `[Earlier conversation summary]` 前缀只停留在 adapter 写回消息中。`CompactionResult` 不再暴露可矛盾的布尔/计数字段，语义压缩由非空摘要派生；Application/Transcript、SQLite、Artifact、Protocol、Desktop 与 CLI 投影共同拒绝空白或非 canonical 摘要，CLI 不再编造 fallback 文案。Item union 同批把领域已证明必有的载荷与状态收回各变体：UserMessage/Question/Compaction 只有 completed，AgentMessage/Reasoning 只有 provisional running 与 durable completed，ToolCall 独占 running/completed/incomplete；started/completed StreamEvent 进一步约束其嵌套 Item lifecycle。UserMessage/Question/ToolCall 始终携带 content/question/tool，terminal AgentMessage/Reasoning 携带完整 phase+content/text，durable Artifact 六个变体不再发布可缺省主体且完全禁止 running；Desktop 只为真正的 AgentMessage/Reasoning 流式 anchor 保留 provisional optionality。Application 的 Item delta 同批由可矛盾的扁平字段袋收敛为四个封闭值对象，空 chunk 不再制造 anchor 或无效帧；上游不提供 content block position，因此 Runtime、Protocol、Desktop 与 CLI 一次性删除固定写成 `0` 的伪 index 及其稀疏排序分支，并删除不存在的 plan-delta 叙述。Application 的 `RunProgress` 同批删除从未投影的 `ToolName` 假事实，拒绝空进度、负数、非法 usage 与非 canonical activity；`PlanSnapshot` 在发布前拒绝 revision 0、缺失/非 UTC 时间、非法步骤与 session identity，并在终态 fence 复制步骤所有权。wire `Plan` 同批把 revision/steps/updatedAt 收进可选的 committed `PlanState`：缺席表示从未写入，存在的空 steps 表示显式清空，`plan.updated` 强制携带 state，彻底删除 revision 0 的双重语义；Runtime Domain 同步拆为表示 unwritten/committed 的 `Current`、只表示已提交值的 `State` 与 typed `Version` CAS，SQLite 不再以 expected revision `0` 选择 insert；CLI 用同一不可变富模型持有版本与内容，mock 脚本只声明 `PlanContent`，由 Runtime 提交单调版本并拒绝溢出，Desktop presentation identity 也显式区分 unwritten/committed。契约注册表新增 typed allowed-value set 与 `requiredAny` 交叉字段约束，从同一声明生成 Go validator、JSON Schema、manifest 与 TypeScript validator；wire `RunProgress` 至少携带 step/usage/contextTokens/activity 之一，计量、Plan identity/step text 同时获得生成的值约束，不由消费者复制状态矩阵或接受无意义 `{}`。该 breaking shape 将 Protocol 精确前移到 `2026-08-29`、Artifact 前移到 v25、SQLite 前移到 epoch 85；旧版本确定性拒绝，不迁移、不双读、不保留 optional payload/status 兼容路径。

P198 后续把 Goal budget 从三个“`0` 等于无限”的可矛盾数字收敛为显式领域值。Domain 零值非法，只允许明确的 unlimited 或至少一个严格正、有限上限；Protocol/模型 Tool 以整个 `budget` 缺席表达 unlimited，present object 由 `requiredAny` 与 positive constraints 共同拒绝空对象、零和负数；Desktop 与 CLI 不再补零。SQLite epoch 86 只接受 `unlimited | limited` 判别 record，旧全零 JSON 不迁移、不双读。该变更不改 Protocol 日期、Artifact v25、method/topic 集合或 Agent Framework shape。

P198 Run-limits 子批继续删除另一套跨层数字哨兵。Domain `run.Limits` 与 CLI `agent.RunLimits` 只通过私有值/存在性对和行为访问器表达策略；有限值必须至少含一个严格正、有限轴，typed unlimited 不再等于三组公开数字 `0`。`runs.start` 删除三个扁平限额字段并改为可缺席的 `limits` 对象；RunRef 与 Artifact 同样以整个字段缺席表示 unlimited，present object 由 `requiredAny` 和 positive constraints 拒绝空对象、零、负数与非有限数。SQLite epoch 87 以 NULL 保存缺席轴并加正数约束，Pending continuation 与 executor application-policy schema v4 使用严格判别 record；旧 shape 不迁移、不双读。Artifact 前移到 v26；Protocol 日期、method/topic 集合与 Agent Framework shape 不变。

P199 将 provider model 的三项 token-limit fact 从扁平数字哨兵收敛为显式存在性模型。Runtime Domain `modelref.TokenLimits` 与 CLI `ModelTokenLimits` 都以私有 value/presence 对持有事实；Application 只传递富值，Protocol `Model` 删除 flat `contextWindow` / `maxInputTokens` / `maxOutputTokens`，改为可缺席的 `tokenLimits`。present `ModelTokenLimits` 必须至少含一项严格正整数，unknown 只由整个对象缺席表达。Desktop 同步使用不可变 nested value，不再依赖 truthiness 或补零。最新 catalog 的 streaming/multimodal 反例允许 `maxOutputTokens > contextWindow`，Domain 只在该事实未证明 output 独立时才从 shared window 扣显式 reservation，始终以已知 max input 封顶。该 breaking cutover 不改变 Protocol 日期、Artifact v26、SQLite epoch 87、checkpoint 或 method/topic 集合。

P203 将 MCP server handshake deadline 从 `timeoutSeconds == 0` 的双义字段切换为必有的 `handshakeTimeout` 闭合联合：`unbounded` 不携带 seconds，`bounded` 必须携带可表示为 Go duration 的正整数秒。Runtime Domain、Application、adapter、SQLite、CLI 和 Desktop 一次性迁移，不保留旧字段、补零、双读或 compatibility projection。SQLite 前移到 epoch 88，以 NULL 表示 unbounded、positive nanoseconds 表示 bounded；Protocol 日期与 Artifact v26 保持不变。P204 只把 Bootstrap Host/Instance 的 internal shutdown caller wait 收进 concrete policy，不改变任何公共或持久合同。

P205 将所有 cursor-paginated read 共享的 `PageQuery.limit` 从“可省略 scalar、数字 0 代表默认”的双义字段切换为 pointer-backed optional positive integer。wire 中缺席明确表示采用各 read 自己的命名 ceiling，present 必须严格大于 0；Delivery 只负责把存在性投影为 Application `pagination.RequestedLimit`，sessions/items/runs/interrupts/schedules/workspace files 的 Application 入口不再接收裸 `int`。该 breaking cutover 不改变 JSON 字段名、Protocol 日期、Artifact v26、SQLite epoch 88 或 method/topic 集合。

P206 将 structured workspace diff 的 row budget 从 generic `Limit int` 收敛为 unit-bearing `DiffRowLimit`。Protocol `GetDiffRequest.limit` 只允许缺席或 strictly positive integer；Application rich value 拥有 default、clamp 与非法状态拒绝，raw diff 携带 row limit 会作为无意义组合被拒绝。CLI/TUI 同样以私有存在性值表达 default/explicit rows，不再把数字 0 跨过 adapter。该 breaking cutover 不改变 JSON 字段名、Protocol 日期、Artifact v26、SQLite epoch 88 或 method/topic 集合。

P207 将 workspace file head/search/read 的四种请求单位从 scalar zero sentinel 拆为各自富值：preview line count、retained grep match count、UTF-8 byte budget 与 closed whole/tail/bounded line range。Protocol 五个字段都改为 pointer-backed optional positive integer；`endLine` present 时 schema/validator 同源要求 `startLine`，present 0/negative 在 Delivery 之前拒绝。Application filesystem port 只接收已归一化 `FileReadPlan`，不再接收 caller DTO 或选择默认；CLI/TUI 使用同构但独立的 unit-bearing values，并显式发布自身 80 lines / 200 matches / 2 MiB 产品默认。该 breaking cutover 不改变 JSON 字段名、Protocol 日期、Artifact v26、SQLite epoch 88 或 method/topic 集合。

P208 将 usage summary 的时间范围从 `sinceDays == 0` 双义数字切换为 pointer-backed optional positive integer：wire 字段缺席唯一表达 all-time，present 值唯一表达正数 recent calendar-day window。Runtime Application、CLI 与 Desktop 分别以自己的 rich period model 持有该区别；Delivery 只做 presence projection，Desktop gateway 不再用 truthiness 重建业务含义。该 breaking cutover 不改变 JSON 字段名、Protocol 日期、Artifact v26、SQLite epoch 88 或 method/topic 集合。

P209 只改变 CLI 内部 consumer model：`SessionQuery` / `RunQuery` 以 `PageSize` 表达命名的 20-row 默认或显式 1–100 rows，Runtime adapter 仍向既有 pointer-backed `PageQuery.limit` 发布正整数。Runtime Protocol、生成制品及其 digest、Protocol 日期、Artifact v26、SQLite epoch 88 和 method/topic 集合均不变。

P210 只改变 CLI internal retry/reconnect construction：bounded schedule、immediate test policy、disabled reconnect 与 configured reconnect 以私有不可变字段表达，invalid zero policy 在 mutation/Run I/O 前失败。Runtime Protocol、生成制品及其 digest、Protocol 日期、Artifact v26、SQLite epoch 88 和 method/topic 集合均不变。

P211 只删除 CLI attachment completion 从未生效的 caller limit 参数；Resolver 自己拥有固定 50-result budget。P212 只改变 CLI sideload private manifest decoder：`timeoutSeconds` 缺席采用 10-second default，present 必须为 1–60，`null` 非法。两批都不改变 Runtime Protocol、生成制品、持久化 shape 或 Desktop binding。

P213 将 `RuntimeLimits.maxConcurrentRuns` 的 Go 表示从 omitempty scalar 改为 pointer-backed optional positive integer：JSON 字段缺席精确表示 Runtime 没有实施 process-wide Run cap，present 只允许 admission owner 实际强制的正整数。字段名与 JSON optional shape 不变，但 schema/validator 现在同源发布 `minimum: 1`，公共 Go API 不再暴露 zero sentinel。CLI 立即投影为自己的 `unbounded | bounded(maximum)` immutable value，`runtime info --json` 使用显式 `runConcurrency` union；不保留 `runtime default` 文案或旧 CLI JSON shape。Protocol 日期、Artifact v26、SQLite epoch 88 和 method/topic 集合不变。

P214 只删除 CLI TUI prompt history 的 unused caller limit 与 zero-value fallback，retention capacity 仍为 1000 semantic messages 并由 aggregate 自己命名和执行。Runtime Protocol、生成制品及其 digest、持久化 shape 与 Desktop binding 均不变。

P215 只收敛 CLI 自己的 Runtime discovery projection、durable outbox journal 与 mutation admission：公开 Flame Runtime Protocol、生成制品及其 digest、Artifact、SQLite 与 Desktop binding 均不变。CLI-local workbench persistence intentional breaking：旧的 primitive replay namespace/deadline/retention shape 不再读取，guard 只接受严格 `unprotected | protected(namespace, until)` 联合；仓库不保留兼容双读或 migration shim。

P216 只收紧 CLI 对 `unprotected` guard 的执行语义，不修改 Runtime Protocol、生成制品/digest、Artifact、SQLite、Desktop binding 或 CLI 持久化 JSON shape。AgentOnly 的新 mutation 仍可首发一次，但 Runtime 未发布 replay capability 时不再进行 uncertain retry 或 cold replay；这是 fail-closed 行为修正，不引入 legacy fallback。

P229 将 `sessions.list` 从通用 `PageQuery` 前移为 Flame-owned `ListSessionsRequest`，新增可组合的 optional literal `search` 与 exact `workspace` filter。Application 的 `CatalogFilter` 统一做 Unicode lowercase、UTF-8/NUL/1024-character admission 并把 normalized predicate 固定进 cursor identity；`CatalogAnchor` 与 `CatalogRead` 替代 Store 的 favorite/timestamp/id/limit primitive 参数列。SQLite epoch 89 增加由同一 canonicalizer 写入并严格复验的 `title_search/workspace_search`，因此不依赖 SQLite ASCII-only `lower()`；LIKE wildcard 全部转义，filter 与 keyset 在一个有界查询内组合。CLI 删除逐 Session 一行一 RPC 的本地扫描，Desktop 与生成 TypeScript surface 消费同一请求类型；旧 Go signature 和 epoch 88 不迁移、不双读。

P217 只删除 CLI consumer-local optional profile→policy projection；Runtime Protocol、生成制品/digest、Artifact、SQLite、Desktop binding、CLI command output 与 workbench persistence 均不变。

P218 只删除 CLI Runtime adapter 的 profile presence 推断；生产 in-process Runtime 本就只在 successful validated discovery 后构造，因此 Runtime Protocol、生成制品/digest、Artifact、SQLite、Desktop binding、CLI output 与 persistence 均不变。invalid partial Runtime 现在在 Services validation 显式失败。

P219 只改变 CLI 内部 Workbench construction/config API：generic `Open(directory, Config)` 与 primitive capacity fields 被显式 memory/directory constructors 和 `*Capacity` 取代。公开 Runtime Protocol、生成制品/digest、Artifact、SQLite、Desktop binding、CLI command output 及已有 Workbench JSON envelope/records 均不变；空白或相对 durable root 现在 fail closed，不提供兼容 constructor、silent fallback 或 migration shim。

P220 只收紧 CLI 内部 request value construction：catalog/workspace read policy 的 zero value 从 named default/whole 改为 invalid，所有真实 caller 使用既有命名 constructor。Runtime Protocol、生成制品/digest、Artifact、SQLite、Desktop binding、CLI flags/output 与 wire optional-positive shape 均不变；这是 intentional internal Go API breaking，不保留 zero-value fallback。

P221 不改变 Runtime Protocol、生成制品/digest、Artifact、SQLite、Desktop binding、CLI flags/output 或 Runtime wire `limits` shape；它收紧 CLI internal `RunLimits` construction，并 intentional breaking CLI-local Workbench JSON：旧 `{}` 不再读取，新记录必须是 strict `{"type":"unlimited"}` 或 `{"type":"limited", ...positive caps...}`。不提供旧记录迁移、兼容双读或 silent unlimited fallback。

P222 只收紧 CLI internal `SummaryPeriod` construction：zero value 从 all-time 改为 invalid，所有真实 caller 使用 `AllTime()` 或 `RecentDays(positive)`。Runtime Protocol、生成制品/digest、Artifact、SQLite、Desktop binding、CLI `/usage` 语法/output、wire `sinceDays` optional-positive shape 与 Workbench persistence 均不变；不保留 zero-value fallback。

P223 只 breaking 收紧 CLI internal `RunLineage` Go API：公开 child identity fields 被私有 closed value 与 root/child constructors 取代，zero value 不再表示 root。Runtime Protocol、生成制品/digest、Artifact、SQLite、Desktop binding、CLI flags/text/JSON/NDJSON output 与 Workbench persistence 均不变；Runtime adapter 继续把 wire 的全缺席 tuple 精确投影为 root、完整 tuple 投影为 child，partial tuple fail closed，不提供旧 struct literal 兼容面。

P224 只 breaking 收紧 CLI internal `modelconfig.Role` Go API 与 TUI auxiliary-role command grammar：公开 kind/provider/model bag 被 inherited-utility、disabled-embedding、configured-role constructors 取代，zero value 非法；`/utility off` 与 `/embedding inherit` 这两个跨语义别名删除，分别只保留 `inherit` 与 `off`。Runtime Protocol、生成制品/digest、Artifact、SQLite、Desktop binding、wire role shape、CLI 持久化与既有 role output 文案均不变。

P225 只 breaking 收紧 CLI internal prompt-queue identity API：Entry/mutation/view action 的裸 `uint64` 改为 private-value `EntryID`，transaction State 的 scalar `DispatchingID` 改为 optional `Dispatching *EntryID`。Runtime Protocol、生成制品/digest、Artifact、SQLite、Desktop binding、CLI flags/output、Workbench durable command records与用户交互均不变；本地 queue identity 不持久化，overflow 现在 fail closed 且不回绕。

P226 breaking 删除 CLI internal `promptqueue.Snapshot.Revision`；该字段从未进入 CLI output、Workbench persistence、Runtime wire、UI ownership 或任何生产判断。Runtime Protocol、生成制品/digest、Artifact、SQLite、Desktop binding、用户交互与 queue transactional State 均不变，不提供 replacement counter。

P227 只改变 CLI internal changefeed consumer watermark；Runtime Protocol/wire `sequence`、生成制品/digest、Artifact、SQLite、Desktop binding、CLI output 与 persistence 均不变。stale/duplicate frame 现在被确定丢弃，gap 仍触发同一 authoritative resync；同一 subscription watermark 不倒退、不回绕，新 subscription 从空 tracker 重新建立序列。

P231 breaking 收紧公共 pagination cursor 资源合同：`protocol.MaximumPaginationCursorCharacters = 65536` 是 wire 公开上限，`PageQuery.cursor` 及所有 flattened embedding request 的 Go validator、JSON Schema、OpenRPC 与 TypeScript validator 从同一 registry rule 生成。Application cursor authority 在 Base64 decode 前做大小准入，生成端以显式 error 拒绝过大 continuation；CLI 直接消费公共常量，不保留独立数值。公共 method/result shape、Protocol version、Artifact v24 与 SQLite epoch 89 不变；公共 Go API 只 additive 增加该常量，internal `pagination.Encode/PageOf` 允许 breaking change且无兼容 wrapper。

P232 breaking 收紧 Run event replay cursor 的完整资源合同：Application opaque token 最大 65536 characters，公开 `evt_` event identity 最大 65540 characters；`RunEvent.eventId` 与可选 `SubscribeRunResponse.headEventId` 的 Go/Schema/TypeScript validator 同源发布限制。`AfterEventID` 继续是 binding metadata而非 params 字段，operation 在 handler 前校验资源与 framing；generated TypeScript 发布 prefix/limit，Desktop 与 CLI 不另写数值。Replay token format、公共 method/result shape、Protocol version、Artifact v26 与 SQLite epoch 89 不变；公共 Go API additive 增加 event-id 上限，internal replay encoder/journal constructors 允许 breaking error API且无兼容 wrapper。Contract generator 写 validator 后再扫描 public Go surface，因此单次 generation 即稳定，不再出现 exported `ValidateWire` method 的一轮滞后。

P233 breaking 调整公共 Go `Page[T]` construction，但不改变其 JSON shape：新增 `PageContinuation` 值对象并由 generic Page 嵌入，使 `nextCursor` 的 65536-character maxLength 只在 Registry 声明一次，同时经 method promotion、Schema flattening 与 TypeScript schema compiler覆盖当前及未来全部 Page instantiation。该 owner只作为 Page 的组成值存在，不额外发布无消费者的独立 JSON frame/Schema definition。Runtime method/result、Protocol date、Artifact v26、SQLite epoch 89与 pagination token format不变；旧复合字面量不保留 compatibility shim。

P234 breaking 把 Run event identity 的 `evt_` framing 收进正式 value-constraint vocabulary：Registry 的 `prefix("evt_")` 同源生成 Go validator、JSON Schema anchored pattern、manifest 与 TypeScript validator，覆盖 `RunEvent.eventId` 和 `SubscribeRunResponse.headEventId`；长度上限及 transport metadata preflight保持 P232 语义。公共 Go `SubscribeRunResponse.HeadEventID` 改为 `*string`，nil 唯一表示订阅建立时尚无 head，present empty/错误 prefix/oversized全部非法；JSON/TypeScript optional shape、Protocol date、Artifact v26、SQLite epoch 89与 replay token format不变，不保留 string-zero compatibility projection。

P235 收紧 `RuntimeEvent.sequence` 为 JSON/JavaScript exact-integer envelope 1–9007199254740991；公开最大值及 generated Go/Schema/TypeScript validator同源，wire字段类型与 JSON shape不变。每个 connection-local subscription 在最终 exact sequence 后停止接收新 signal，排空已入队 frame再释放 watcher/hub/channel owner；不回绕为 0、不 panic、不影响其他 subscription。Protocol date、Artifact v26、SQLite epoch 89、method/topic集合与 replay/pagination token均不变。

P236 breaking 将所有公开数字 revision/CAS identity 收紧到同一个 exact JSON integer envelope 1–9007199254740991。`MaximumExactJSONInteger` 与 cross-ring pure `exactint.Counter`分别是wire projection和transport-neutral advancement owner；Session、Schedule、Plan output及UpdateSession/UpdateSchedule expectedRevision的generated Go/Schema/OpenRPC/TypeScript validator同源发布上下界。Session/Plan Domain reconstruction与replacement、Schedule Edit/Claim/Accept/RecordRun、Application/SQLite successor proof均在持久化前拒绝耗尽或不精确值；当时的SQLite epoch 90为sessions/session_plans/schedules revision添加正数+exact maximum CHECK，旧epoch不迁移、不双读。字段名、JSON number shape、Protocol date、Artifact v26、method/topic集合与token格式不变。

P237–P239 breaking 只收紧internal Schedule构造与firing生命周期，不改变公共或持久化shape。Schedule、Execution、Occurrence、Claim与RunRequest均以private state + named behavior持有；Draft/Snapshot是明确的边界值，manual run与durable occurrence不再共享残缺对象，pre-claim CAS只存在于Claim而不以Occurrence revision-zero哨兵表示。Application identity端口由adapter/composition root实现，SQLite继续写读同一`schedules/schedule_firings`列与状态值。Protocol date、Artifact v26、SQLite epoch 90、method/topic/JSON/schema/token格式及四份contract digest均不变。

P240–P241 breaking 继续只收紧internal端口与模型Tool shape。Schedule firing stored state由单一private SQLite codec拥有，typed Acceptance替换occurrence/run primitive pair，run fact统一单调推进；表、列、CHECK取值与事务语义保持。无界internal Schedule List删除，模型`list_schedules`增加opaque cursor输入/continuation输出并以100 rows/page有界；公共Runtime `schedules.list`早已分页，其Protocol/Schema/SDK完全不变。Protocol date、Artifact v26、SQLite epoch 90与四份contract digest不变。

P242 breaking 把Schedule所有durable timestamp统一为Domain-owned UTC millisecond precision，并以private RunRecord替换manual completion的schedule-id/time primitive pair；SQLite本就以Unix milliseconds存储，因此表shape、值域与epoch不变。公共Schedule time字段仍是相同RFC3339 JSON类型，只有此前不可恢复的亚毫秒尾数不再短暂出现在Create响应。Protocol date、Artifact v26、SQLite epoch 90与四份contract digest不变。

P243–P244 breaking 只改变internal Schedule construction/identity端口：显式Disabled与error-producing constructors替换partial nil wiring，private Worker不再形成公共半构造面；ManagementIdentities/OccurrenceIdentities由同一production adapter实现并在composition root注入。公共Runtime、模型Tool JSON、SQLite、Protocol/Artifact版本与四份contract digest均不变。

P245–P246 breaking 只改变CLI internal Goal projection construction：公开Goal/Reason/Usage字段袋改为private immutable values，Runtime adapter经technical Snapshot与Restore形成合法值，TUI改读accessor；zero/regressing durable time、status/reason错配与active exhausted budget在进入CLI状态前拒绝。公共Flame Runtime Protocol JSON、生成合同、Desktop、SQLite epoch 90与Artifact v26均不变。

P247 breaking 只改变CLI internal Steer outbox construction：`PendingSteer`公开持久字段袋改为private immutable aggregate，Workbench以private record编码同一`pendingSteer` JSON字段和值，format version 1不变；canonical instruction不再通过trimmed equality冒充exact source ownership。Flame Runtime Protocol、Runtime/SQLite/Artifact与Desktop合同不变。

P248 breaking 只改变CLI internal TUI operation lease identity；unchecked uint64 wrap改为allocation-before-cancel的checked private successor，穷尽后拒绝新operation且保留当前owner。无wire、Workbench JSON、Runtime、Desktop、SQLite或Artifact合同变化。

P249 breaking 只改变CLI internal TUI draft writer identity与错误表达：unchecked uint64 revision wrap改为checked private successor，`Schedule`从混装closed/exhausted/no-op的bool改为显式error；穷尽后保留最后pending snapshot并拒绝autosave/Flush。Workbench draft文件、Flame Runtime Protocol、Desktop、SQLite与Artifact合同均不变。

P250 breaking 只改变CLI internal TUI stream callback ownership：删除与operation lease重复且可回绕的`streamSeq`，所有stream follower提交统一由exact operation lease裁决，owner allocation失败显式终止本地projection。Run event wire、Workbench、Flame Runtime Protocol、Desktop、SQLite与Artifact合同均不变。

P251 breaking 只改变CLI internal plugin command operation tracking：primitive `commandSeq`与`map[uint64]`收敛为checked rich registry，identity穷尽显式拒绝新command并保留既有cancellation owners；dynamic slot仍是TUI内部operation namespace。Plugin API、Workbench、Flame Runtime Protocol、Desktop、SQLite与Artifact合同均不变。

P252 breaking 只改变CLI internal Session presentation ownership：可回绕`sessionContextEpoch`替换为可退休private lease，所有跨Session异步UI commit按exact active lease裁决。无CLI命令、Plugin API、Workbench、Flame Runtime Protocol、Desktop、SQLite或Artifact合同变化。

P253 breaking 只改变CLI internal Transcript frame/content ownership：可回绕`contentEpoch`替换为可退休private lease，Reset前frame不能解释Reset后复用的BlockID。无渲染文本、快捷键、CLI命令、Workbench、Flame Runtime Protocol、Desktop、SQLite或Artifact合同变化。

P254 breaking 只改变CLI internal Tool/ToolGroup reader observer ownership：两套numeric observer maps收敛为non-reusable token registry，exact unsubscribe不再受identity wrap影响。Reader文档shape、Plugin presenter API、CLI命令、Workbench、Flame Runtime Protocol、Desktop、SQLite与Artifact合同均不变。

P255 breaking 只改变CLI internal reusable presentation frame identity：numeric generation与exhaustion panic替换为retirable private identity，dialog/queue输入合同保持“active且current frame已展示”。无CLI命令、Plugin API、Workbench、Flame Runtime Protocol、Desktop、SQLite或Artifact合同变化。

P256 breaking 只改变CLI internal Extension Registry registration identity与失败事务：unchecked shared sequence替换为checked private value，耗尽和duplicate均不推进sequence。Plugin manifest/contribution API、加载顺序语义、CLI命令、Workbench、Flame Runtime Protocol、Desktop、SQLite与Artifact合同均不变。

P257 breaking 只改变CLI internal Conversation presentation bookkeeping：删除无消费者revision，local failure block ID改由现有block index安全派生并避让冲突。Runtime event/Session/Plan wire、Workbench、Desktop、SQLite与Artifact合同均不变。

P258 breaking 只改变CLI production mock backend内部opaque identity representation：固定前缀numeric uint64改为typed namespace + arbitrary-precision sequence，既有低位ID shape保持，越过uint64后继续十进制增长。真实Flame Runtime Protocol、Runtime adapter、Workbench、Desktop、SQLite与Artifact合同均不变。

P259 breaking 只改变CLI production mock backend的Session Update/Rollback失败原子性：revision exhaustion现在显式拒绝且candidate transaction零写入，成功返回shape与revision step保持。真实Flame Runtime Protocol、Runtime adapter、Workbench、Desktop、SQLite与Artifact合同均不变。

P260 breaking 只改变CLI production mock backend的Run lifecycle失败事务与Segment错误投影：Start/park/Resume/Steer/Finish/Cancel在写入前预留完整revision容量，后台失败由同一Segment向当前及重连subscriber确定发布；失败finish不再被`sync.Once`永久吞掉。成功event顺序、Session/Run/Transcript projection、CLI命令与JSON shape保持；真实Flame Runtime Protocol、Runtime adapter、Workbench、Desktop、SQLite与Artifact合同均不变。

P261 breaking 收紧CLI consumer-owned revision domain：Session、Plan、Schedule与production mock统一接受1–`2^53-1`，new/fork/create acknowledgement必须为首revision，CAS acknowledgement必须是expected的exact successor；越界值不再仅因Go `uint64`可表示而被接受。字段Go/JSON shape不变并与既有Flame Runtime Protocol exact-integer合同对齐；Runtime、Desktop、SQLite与Artifact不变。

P262 breaking 只改变Desktop Agent Session的process-local投影身份表示：view epoch/revision、authoritative revision、refresh sequence与projection generation由JavaScript `number`改为`bigint`，Run pump换为对象lease；只在React presentation key边界显式编码，不进入RPC、持久化或导出制品。Flame Runtime Protocol、生成SDK、Wails binding、SQLite、Artifact、CLI与可见交互均不变。

P263 breaking 只改变Desktop进程内生命周期身份：bootstrap、Workspace event loop与cwd retarget由number generation改为对象lease；Runtime stream port的connection generation由递增字符串改为持有process membership行为的名义对象，并删除connection projection中重复的process generation字段。该对象不跨JSON-RPC、Wails、storage或Artifact边界；Flame Runtime Protocol、operation/event shape、Runtime/CLI行为与可见交互均不变。

P264 breaking 只改变Desktop process-local identity与invalidation representation：JSON-RPC request id仍是相同十进制字符串shape，但由任意精度sequence生成；anonymous contribution/log/optimistic identity保持既有前缀；UI async提交权由number revision改为对象lease，material snapshot改为对象或`bigint`。新增foundation层是纯内部依赖方向，不改变Flame Runtime Protocol字段、Wails binding、storage、Artifact、Runtime、CLI或可见交互。

P265 breaking 只改变Desktop全局Task tracker内部生命周期：同id restart由exact `TaskLifecycle`对象身份而非毫秒时间戳区分，旧handle/timer永久失权；Task readout字段、状态、时间戳、linger时长与插件Task API不变。Flame Runtime Protocol、storage、Wails、Runtime与CLI不变。

P266 breaking 只改变Desktop Context Dock本地偏好shape：文件focus revision由number改为`bigint`，localStorage版本前移到v2并以canonical non-negative decimal字符串保存；v1整体丢弃，不迁移、不双读。该revision不进入Flame Runtime Protocol、Wails、Artifact、SQLite或CLI，文件定位可见行为保持且在safe-integer边界后继续正确。

P267 不改变Flame Runtime Protocol wire、Protocol version、Artifact、SQLite、Wails或CLI。生成TypeScript binding additive公开由Runtime `StreamEvent.Replayable()`同源投影的event-type replayability；Desktop SDK内部Run response ownership breaking删除历史事件推导的subagent membership，并以response request identity作为唯一stream scope。Replay ID记忆只保留可重放类型且服从已协商`runReplay.maxEvents/maxBytes`；公开method/result/event shape与可见交互不变。

P268 只改变Desktop内部Run pump/batcher提交合同：projection adapter由void改为返回整批fold acceptance，replay cursor只在acceptance后推进；每个pump拥有独立process-local batcher。Flame Runtime Protocol、生成合同、Wails、storage、Artifact、SQLite、Runtime、CLI与可见交互均不变。

P269 只改变Desktop内部Run batcher容量与pump I/O lease：后台frame queue采用命名256-event上限，到界同步fold；retired pump禁止reattach，迟到foreign stream显式关闭。Flame Runtime Protocol、生成合同、Wails、storage、Artifact、SQLite、Runtime、CLI与可见交互均不变。

P270 breaking只改变Desktop SDK内部transport/channel资源与失败语义：HTTP recv改为rendezvous背压，SSE逐frame解析并拒绝超过命名decoded-character包络的frame；Run/runtime-event inbox有限，ephemeral overflow可丢，authoritative overflow显式结束当前response generation供既有replay/cold-read或resync恢复。`HttpTransportConfig.maximumEventStreamFrameCharacters`是进程内构造选项，不进入Flame Runtime Protocol；wire method/result/event、Protocol version、生成合同、Wails、storage、Artifact、SQLite、Runtime与CLI均不变。

P271 breaking收紧多provider配置纵切的状态归属：Provider Domain以私有identity、opaque API key、validated Base URL、credential provenance和preserve/set/clear change富模型取代公开primitive字段、空字符串和`*string`更新哨兵；SQLite `providers.api_key/base_url`以NULL表达缺省并拒绝空文本，当时前移至epoch 91，不迁移、不双读旧shape。环境credential是启动时验证的immutable overlay，stored优先级由Domain保证且永不落库。Protocol精确前移到`2026-08-30`，Provider只发布可缺席`baseUrl`与可缺席嵌套redacted `credential:{masked,source}`；旧`apiKeyMasked/keySource`直接删除。Desktop与CLI各自以immutable rich model消费新shape，raw secret只在Runtime model-client adapter边界揭示，cache identity只携带digest。

P272 breaking只收紧Runtime internal provider catalog/Application metadata/client construction，不改变Protocol `2026-08-30`、SQLite epoch 91、Artifact v26、generated binding、Desktop或CLI shape。Provider integration由validated catalog aggregate唯一拥有closed endpoint/model-source/embedding policy与builder；Application不再保存公开primitive capability bag。Client input以构造后的credential/endpoint presence表达，空字符串只允许在调用第三方SDK的最内层projection中表示该SDK自身的缺席约定。

P273 breaking把provider readiness与credential presence拆成两个正交事实。Protocol `Provider`新增必有`configured`和闭合`credentialRequirement: apiKeyRequired | apiKeyOptional`；`credential`继续只投影真实stored/environment secret material，不能兼任可用状态。Ollama由catalog声明optional authentication与default local endpoint，因此即使registry无row且无key也可完成chat、embedding、model probe与`providers.test`；若用户显式配置key仍会正常发送。第三方OpenAI-compatible SDK构造器要求非空key时，Runtime最内层防腐适配器只用命名占位满足其本地validation，并在HTTP RoundTripper边界确定删除`Authorization`，占位值绝不离开进程。Desktop与CLI各自恢复rich authentication/readiness model并拒绝矛盾wire state；不保留credential-derived enabled、dummy provider key或provider-name UI分支。Protocol日期保持`2026-08-30`，SQLite epoch 91、Artifact v26及method/topic集合不变。

P274 breaking只改变Runtime internal model-client composition与lifetime。Chat/Embedding resolver删除按credential digest永久累加的process cache；Bootstrap删除静态default client，fresh Run、utility fallback与embedding search均从当前ProviderRegistry解析，active Run仍保有自己staging时的client generation。它不改变Protocol `2026-08-30`、生成制品/digest、SQLite epoch 91、Artifact v26、Desktop或CLI shape。

P275 breaking把provider/model/reasoning effort从跨层裸字符串收敛为Runtime Domain immutable identity value。provider最多64个Unicode code point、model最多256、reasoning effort最多32；configured identity必须是有效UTF-8、非空且不含Unicode whitespace、separator/control/format/private/unassigned等不可打印字符，不trim、不截断、不normalize。Remote model probe整批fail closed，Application catalog与Provider aggregate只能由rich constructor形成；Protocol generator把同一命名上限与identity pattern投影到字段、reasoning-level items及`Usage.byModel`/`ArtifactUsage.byModel` property names，CLI与Desktop在恢复和command admission时执行相同边界。Protocol保持本次尚未发布的`2026-08-30`，Artifact前移到v27，SQLite前移到epoch 92；旧归档和数据库不迁移、不双读。

P276只收紧Runtime internal remote model probe的资源和文档合同。Endpoint response必须是最多1 MiB的完整有效UTF-8单一JSON文档，`data`在去重前最多4096项；overflow、第二JSON值、invalid UTF-8或任一非法model identity使整批probe失败，由既有Application policy回退bundled catalog。Protocol `2026-08-30`、generated digest、Artifact v27、SQLite epoch 92、Desktop与CLI shape不变。

P277只收紧model identity的内部admission和ownership。CLI command/schedule/role/provider路径、Runtime catalog/client/checkpoint/usage路径不再trim或大小写折叠identity；Interaction root与accounting只持有完整`modelref.Selection`，删除provider-only和`"unknown"` fallback。Protocol `2026-08-30`、generated digest、Artifact v27、SQLite epoch 92及消费者shape不变。

P278只收紧Runtime internal model discovery/provider test的取消与错误归属。Caller cancellation和Provider Registry读取故障保持为error；仅endpoint probe自身的不可达、内部timeout、空或非法目录可按既有策略fallback，provider test的真实远端失败仍投影稳定outcome。Protocol `2026-08-30`、generated digest、Artifact v27、SQLite epoch 92及Desktop/CLI shape不变。

P279只收紧CLI internal foreign Session identity admission与durable Workbench ownership。新增`sessionidentity.ID`，删除Session map key、journal恢复、状态文件hash和mutation path的`TrimSpace`修复；invalid UTF-8、NUL及non-exact值在Runtime I/O或durable publish前失败。Runtime Protocol `2026-08-30`、generated digest、Artifact v27、SQLite epoch 92、Runtime与Desktop shape不变。

P280只收紧CLI internal Run/Segment identity admission。Distinct `runidentity.RunID`/`SegmentID`覆盖Run projection/tree、subscription/stream/event、rollback journal、schedule/failure projection、Runtime adapter及TUI异常receipt；不trim、不repair。Runtime Protocol `2026-08-30`、generated digest、Artifact v27、SQLite epoch 92、Runtime与Desktop shape不变。

P281只收紧CLI internal Item/Event identity admission。`runidentity.ItemID`/`EventID`覆盖HITL answer、Interaction、Block/delta、child lineage、RunEvent、subscribe after/head checkpoint与可选feedback target；invalid UTF-8、NUL及non-exact值在Conversation恢复、Runtime command或durable publish前失败。Runtime Protocol `2026-08-30`、generated digest、Artifact v27、SQLite epoch 92、Runtime与Desktop shape不变。

P282 breaking收紧Runtime和CLI对Session/Run/Segment/Item/Event foreign identity的准入：普通资源最多256个Unicode code point，Event使用65540-character replay envelope；所有值必须是valid UTF-8、printable且不含Unicode whitespace，不trim、不repair。Runtime Domain富值贯通Application、SQLite、HTTP replay metadata和Protocol generated validation；Event wire同时要求`evt_`prefix，generator以`allOf`表达prefix与identity alphabet的合取，不再覆盖pattern。CLI rich identity持有同值Domain上限，并在Runtime adapter测试中与Protocol锁定。Protocol日期保持`2026-08-30`，Artifact v27、SQLite epoch 92不变；生成制品digest按本批约束刷新。

P287 breaking 将公共同进程 Go binding 从 `runtime/embedded` 移到模块根 `runtime`，并迁移 CLI 与 external-module compile fixture；旧包物理删除且无兼容层。`contract/go-api.json` 的 binding package path 与错误文本前缀随之更新；public type/method shape、Runtime Protocol、Artifact、SQLite 与 HTTP wire 不变。

P288 只改变 Runtime internal package graph：9 个 cross-ring identity 微包收敛为受限 `internal/identity` shared kernel，distinct value types、grammar、resource envelope 与 construction invariants 保持同值。领域 identity 仍由各自 Domain owner拥有，不新增 generic ID 或可解释 wire；公共 root/protocol/localruntime surface、contract digest、Protocol、Artifact、SQLite 与 HTTP wire 均不变。

P289 只改变 Runtime internal Adapter package graph：唯一服务 `runsegment.Finalizer` 的 model-backed Session title generator 并入 `adapter/runsegment`，`utilitymodel` 外部 SDK 防腐边界保持独立。标题预算、fallback、first-writer 与 maintenance lifecycle 行为不变；公共 Go surface、contract digest、Protocol、Artifact、SQLite、Desktop 与 CLI 均不变。

P290 只改变 CLI internal package graph：三个 identity admission 微包收敛为一个受限 `internal/identity`，并删除无生产类型消费者的 CLI 影子身份表示。CLI 仍对每种外来 Runtime identity 执行 exact、非修复式准入；Runtime root/protocol/localruntime surface、contract digest、Protocol、Artifact、SQLite、HTTP wire 与 Desktop 均不变。

P307 只改变 Runtime/CLI internal package 的物理层级与 import path：CLI 建立 Domain/Application/Adapter/Delivery 四环，Runtime 在既有依赖环内按已证明 context 增加一层无 Go 文件 namespace，并将叶 package 名与目录统一。没有新增 facade 或生产 package；公共 Runtime root/protocol/localruntime surface、contract digest、Protocol、Artifact、SQLite、HTTP wire 与 Desktop 均不变。

P283继续breaking收紧协调身份：Goal incarnation成为独立128-code-point exact富值并贯穿Goal/Run/HITL/checkpoint/SQLite；Schedule补入`sch_`framed 256-code-point公开资源合同，occurrence由Schedule identity与canonical due-millisecond cursor共同拥有，scheduled opening也解析同一occurrence而非只TrimSpace。executor instance/member/wait-request/effect严格区分并共享Agent Framework的256-byte URI-safe port合同；Executable build只接受canonical `sha256:<64 lowercase hex>`；deployment implementation/configuration分别持有最多256个exact printable non-whitespace code point，Framework digest不再读取未验证string；provider ToolCall correlation归Conversation Domain所有并在usage、working context、commit/recovery、普通/delegate Tool与Pending边界接受最多512个exact printable non-whitespace code point；top-level write-set commit统一为`run_commit_`framed、最多256-byte URI-safe技术身份，且富值从Application write-set贯穿adapter port和SQLite marker，不再在每层退化成裸string或保留双轨optional parser。Run replay cursor的process epoch、RunID和SegmentID也已作为独立富值保留在内存 authority 中，版本化JSON是唯一字符串编解码边界；分页的timestamp+Run anchor同样拒绝伪造Run identity。每个Bootstrap进程只发布canonical `runtime_<lowercase UUID>` incarnation；所有产品发射面消费唯一`identity.ProductName=flame`，不再把通用`runtime`当品牌。durable idempotency namespace由独立`identity.IdempotencyNamespace`从SQLite贯穿Bootstrap，operation在业务准入前精确解析，HTTP不再trim身份；公开Go/Schema/TypeScript合同共同声明`^idp_[0-9a-f]{32}$`。process-local MCP OAuth attempt也由Application富值拥有并在lookup前解析，公开request/response共同发布`^mcpauth_[A-Z2-7]{26,64}$`。Agent Memory item同样由独立`ItemID`拥有`^mem_[0-9a-f]{32}$`，Domain、Application、SQLite strict codec与生成Go/Schema/TypeScript校验器共享精确语法；显式用户激活已有proposal时保留原身份且不再预先生成并丢弃随机ID。Protocol日期仍为`2026-08-30`，Artifact v27不变；持久准入CHECK使SQLite前移epoch 93，Go API surface未变，Manifest/OpenRPC/Schema digest按新增约束刷新。

P283内部恢复合同继续收口：SQLite child-start、model/tool invocation和coarse Run lifecycle各自拥有私有持久状态类型，Run扫描统一拒绝未知状态，SQL边界显式投影durable spelling。该子批不改变公开协议、Artifact版本或SQLite schema epoch。

P283后续把MCP server registry identity收敛为独立`ServerName`：唯一canonical spelling为1–32位lowercase ASCII `[a-z0-9._-]`且首位必须是字母或数字。同一富值贯穿durable registry、live connection supersession、OAuth owner、tool policy和tool namespace；Delivery、Scope SDK和SQL绑定才投影string。公开Go/Schema/TypeScript request与read model、Domain codec和fresh SQLite CHECK共同拒绝大小写、空白、路径字符和超长值。Protocol日期与Artifact v27不变，SQLite因持久准入变化前移epoch 94。

P283再把MCP远端工具身份收敛为独立`RemoteToolName`：仅接受协议规定的1–128位ASCII `[A-Za-z0-9_.-]`，并与有损、64-byte的模型可见名严格分离。每台server的disabled/auto-approved平行数组在Domain入口合成为canonical `ServerToolPolicy`，重复、交叉矛盾和合计超过2048条均失败；SQLite删除两列opaque JSON，改由带server外键、tool-name CHECK、decision CHECK、唯一键和cardinality trigger的`mcp_server_tool_policies`关系表持有。公开Go/Schema/TypeScript对`MCPTool.name`和两类策略数组发布同一name grammar、item上限、unique与maxItems规则；Protocol日期与Artifact v27不变，SQLite前移epoch 95。

Interaction组装现保留解析后的build与deployment富值并清除重复raw config；checkpoint写出才投影build文本，dispatch cancellation registry也直接使用Framework `ProcessID`/`EffectID`键。该内部收敛同样不改变公开wire合同。

模型/工具invocation correlation现在由有界摘要富值生成。尤其模型路径不再把最大256-byte Framework EffectID继续拼接成超限CallID，从而避免“外部调用已发生、Application投影才拒绝”的unknown-effect故障。生成规则是内部实现合同，不改变公开wire schema。

后台shell identity现在由process-local owner epoch与canonical nonzero uint64 sequence共同构成，内部ledger持有私有富值，Tool JSON边界保留字符串投影。Runtime重启后旧`shell_id`不会命中新进程中的另一条命令；sequence exhaustion显式失败，不回绕。该内部生命周期收紧不改变公开Tool字段shape、Protocol、Artifact或SQLite版本。

纯进程registry现在直接以task、invocation、admission claim和stream subscriber registration对象为键，不再分配无协议含义且会回绕的整数身份。Segment生成的Item identity改为Segment摘要加canonical uint64 sequence，user opening占用独立discriminator；因此最大合法Segment不能通过复合字符串制造超限Item，sequence exhaustion也不会回绕或发布空身份。Item仍是既有opaque string字段，公开shape与版本不变。

Run reducer的open Tool顺序现在由`openTools` aggregate直接拥有：direct Tool保持registration对象序列，provider Tool按已有model-call position排序，lookup/delete/drain/speculative clone不再依赖外置`int` arrival counter。该内部所有权收敛保持既有Tool事件、持久化和wire顺序。

MCP status callback顺序由queue-owned linked registration表达，不再把`uint64`当作process identity或用pending map等待可能回绕的下一值；out-of-order ready与同步重入仍保持mutation registration顺序。HTTP `Request-Id`保持既有`req_` opaque header shape，但随机材料统一来自`crypto/rand.Text()`，删除时钟与全局原子计数fallback。

MCP live connection的latest-operation-wins不再依赖每服务器`uint64 generation`。服务器直接持有当前`connectionAttempt` registration，supersession、detach与shutdown通过对象同一性判定并取消exact attempt；长时间运行不存在回绕后旧dial/OAuth completion重新取得提交权的路径。该process-local收敛不改变MCP wire、持久化或工具目录语义。

Application `opaquetoken`机制不再暴露无界Encode/Decode入口：每个token authority必须传入自己的positive encoded-character envelope。Encode在分配Base64输出前比较精确长度，Decode在分配decoded payload前拒绝超限；pagination与Run replay继续分别拥有公开错误语义和同一64 KiB合同，不引入通用cursor领域类型。

Agent Effect dispatch的外部副作用边界现在只保存`externalBoundaryCrossed`事实，不再用`uint32 externalCalls`表达不消费基数的布尔问题。任意次数调用都单调保持true，projection失败后的indeterminate恢复裁决不存在计数回绕为零的路径。

Run session-change notifier以实际`sessionRunObserver` registration集合拥有等待者生命周期；stop closure删除exact registration，最后一个观察者退出才回收该Session generation。旧`int observers`派生计数已删除，notify仍一次关闭共享generation并允许后续观察创建全新generation。

## 2. Runtime Protocol Baseline 2

机器真相源位于 [`../contract`](../contract)：

| 制品 | SHA-256 |
|---|---|
| `contract/manifest.json` | `10dd28bfc19312bfed0dc39a58bb25ae6fc4b42b8cf82143ffd744982e22b06f` |
| `contract/openrpc.json` | `f73da812bbe749158344e838692bc1ae2fc2bf108423fdfef16eed5b53b35401` |
| `contract/schema.json` | `02cb972c0d9db91a523f9acd7fa71e55f8decf8488b3ecf837e25d57c99706f8` |
| `contract/go-api.json` | `af15710cf99c5eaf17720c55d931200ec0a1049a48dd97a994588b777c86588f` |

TypeScript generated files 是派生制品，不单独定义语义。它们必须由同一个 contract generator 产生且 diff-free；当前前端/TUI/CLI 是否已经消费最新 shape，由 P10/P12 的 consumer handoff 记录，不通过兼容字段掩盖。

P287 只把同进程 binding 的 canonical package path 从子包提升为模块根 `github.com/Tangerg/flame/runtime`，因此 `go-api.json` digest 显式重新冻结；声明集合、签名、Protocol、Artifact、SQLite 与运行时语义均未变化，也不保留旧 package path。

人读语义 owner：

- [`API.md`](API.md)：业务方法、Run/Item/Event 语义与跨方法不变量；
- [`TRANSPORT.md`](TRANSPORT.md)：HTTP/SSE binding、流、重放和安全；
- [`AUX_API.md`](AUX_API.md)：VCS、MCP、审批等旁路能力。

本文件不复制 method、field、error 或 example catalog。

当前协议版本为精确值 `2026-08-30`，Artifact 为 v26，不存在兼容范围或旧归档 reader。Provider配置只发布可缺席`baseUrl`与可缺席嵌套redacted `credential`，不发布raw secret、空字符串presence或并列credential来源字段。Session 在 Domain、SQLite、Protocol 与生成消费者上只发布 exact provider/model/reasoning selection；省略 Run selection 时读取该 durable selection，不按 model id 推断 provider，也不从上一模型继承 reasoning effort。每个 compaction Item 必须携带 Runtime 实际写回的 canonical 人读摘要，消费者不得从 dropped count 或本地文案重建。Session workspace 在 Domain 中是 exact value，SQLite 只保存 `workspace_path`；Protocol/Artifact 既有 `WorkspaceRef` shape 不因内部 owner 收敛虚增版本。RunEvent 只有七个 Runtime 实际生产的一等变体，Interrupt/response 只有 approval 与 question；没有 custom 旁路、clientTools feature 或 toolResult interrupt。Plan 是一等 `plan.updated` / `plan.changed` / `plan.get` / `SessionSnapshot.plan` / `SessionArtifact.plan` 合同，不再经过通用 state registry、key、scope/writer metadata 或 `states[]` union。Feature 与 Method 合同只发布能改变协商或消费决策的事实，不携带恒为 `stable` 的 stability 标签；method policy 同时发布 idempotency 与 run replay cursor applicability，只有 `runs.start`、`runs.resume`、`runs.subscribe` 接受 run cursor。唯一 replay scope 是 `runtimeInstanceRootSegment`：它准确表达一个 Runtime instance 内的一条 root Segment replay buffer；旧 `processRootSegment` 已直接删除。消费者 breaking surface 与未接线事实由 [`CONSUMER_HANDOFF.md`](CONSUMER_HANDOFF.md) 唯一记录。

P153 直接删除 Codebase semantic-index contract：公共 Go surface、三项 operation、feature、runtime topic、DTO/enum/sample 及 Desktop/CLI direct consumer 同批消失；不存在旧同日 Protocol shape reader、disabled capability 或 compatibility binding。当前 manifest 精确发布 86 个 methods、17 个 features 与 15 个 runtime topics。Embedding role 仍是 Agent Memory 的可选配置，不是被删除能力的残留别名。

P154 不改变 Protocol、公共 Go API 或 Artifact shape。Agent Memory embedding role 仍是同一可选 provider/model pair；服务端内部 search cache 现在把 vector 与 exact embedding space、content digest 一起绑定，role/cache 变化不新增 operation、event、feature 或 consumer handoff。

P155 不改变 Protocol、公共 Go API、Artifact 或 SQLite shape。内部 Agent Memory recall 不再接受任意单 scope，而是对当前项目的 active project items 与全局 active user items 做一次联合 ranking/top-k；`agentMemory.list/add/update/review/delete` 的显式 scope 合同与 Desktop 管理面不变。

P156 将 Agent Memory 的 `add.content`、可选 `update.content` 与 `AgentMemoryItem.content` 精确约束为最多 4096 个 Unicode code point；Go validator、JSON Schema/OpenRPC、TypeScript validator 与 Desktop request/result boundary 均由同一 Contract Registry 生成。Protocol 精确值仍为当前开发日期 `2026-08-24`，不接受同日旧 shape；Artifact v23 不变。

P157 只收紧 Runtime 内部 auxiliary model request envelope：title、compaction、Agent Memory consolidation 与 Skill mining 必须显式携带 aggregate input-byte/output-token 上限，maintenance transcript 受 384KiB total / 24KiB per-message 公平预算约束。该批不改变 Protocol、manifest/OpenRPC/schema、公共 Go API、Artifact v23、SQLite epoch 82、Desktop binding、Agent Framework 或 CLI。

P158 只收紧 Agent Memory 内部有限集合与生命周期行为：每个 project/user target 最多 512 个 active + pending item、最近 2048 个 rejected tombstone，单次 extraction/curation 最多 32 条，pending ledger page 最多 128 条；显式 Add 可原地恢复同 digest 的 pending/rejected proposal。`agentMemory.*` request/result shape、operation/feature/topic catalog、generated Desktop binding、Protocol `2026-08-24`、Artifact v23 与 SQLite epoch 82 均不改变。

P159 将 Runtime internal Skill Proposal storage 从 `_proposals/<revision>/SKILL.md` 一次性切换为 `_proposals/<name>/SKILL.md`，每个 project/user scope 最多 128 个 current proposal，完整 authored `SKILL.md` 最多 1 MiB；revision 仍是 scope/name/完整文档的 SHA-256 CAS，旧 handle 不兼容地失效。`skills.proposals.list/approve/reject` request/result shape、operation/feature/topic catalog、generated Desktop binding、Protocol `2026-08-24`、Artifact v23、SQLite epoch 82、公共 Go API 与 CLI 均不改变；`propose_skill.instructions` 的 internal Agent Tool schema 同步增加 1 MiB ceiling。

P160 只收紧 Runtime internal LSP document synchronization：进入 digest、`didOpen/didChange` 与 client open-state 的单文件最多 8 MiB，读取同时服从 caller cancellation 与 `limit+1` growth detection。LSP operation request/result shape、operation/feature/topic catalog、generated Desktop binding、Protocol `2026-08-24`、Artifact v23、SQLite epoch 82、公共 Go API、Desktop source、Agent Framework 与 CLI 均不改变。

P161 只收紧 Runtime internal MCP remote catalog admission：每 connected server 最多 2048 个 tools，每个 description 最多 64 KiB 且为有效 UTF-8，每个 encoded input schema 最多 1 MiB；模型目录和 `mcp.tools.list` 管理目录都 fail closed，不返回截断前缀。MCP operation request/result shape、operation/feature/topic catalog、generated Desktop binding、Protocol `2026-08-24`、Artifact v23、SQLite epoch 82、公共 Go API、Desktop source、Agent Framework 与 CLI 均不改变。

P162 只收紧 Knowledge 完整文档准入：单份 home/projectRoot/cwd `FLAME.md` 最多 1 MiB，`knowledge.update` 在 persistence port 前拒绝超限内容并投影为 `invalid_params`，filesystem store 的 direct write 与外部文件 read 复用同一 Domain 上限；完整 cascade 不截断或跳过越界文档。Knowledge operation request/result shape、content-revision 格式、CAS/atomic-replace/recovery 语义、operation/feature/topic catalog、generated Desktop binding、Protocol `2026-08-24`、Artifact v23、SQLite epoch 82、公共 Go API、Desktop source、Agent Framework 与 CLI 均不改变。

P163 只收紧 Lifecycle Hook 配置准入：单份 `hooks.json` 最多 256 KiB/128 条，global + project 完整级联最多 256 条；matcher 最多 256 bytes、command/inject 最多 8 KiB、command timeout 最多 5 分钟，配置文本必须是有效 UTF-8。`hooks.list` 与 fresh Run binding 对任一超限文件或级联整体失败，不截断、不跳过、不发布部分策略。Hook operation/request/result shape、trust key/active 语义、event/scope vocabulary、Protocol `2026-08-24`、Artifact v23、SQLite epoch 82、generated Desktop binding、公共 Go API、Desktop source、Agent Framework 与 CLI 均不改变。

P164 收紧 Hook command 私有进程合同：stdout/stderr 各最多保留 64 KiB 且继续 drain；stdout 只能为空或一个 UTF-8 JSON object，只接受 `decision/reason/injectContext/rewriteArguments` 与 `allow/deny/ask`，unknown/trailing/malformed/overflow 输出作为可观察的 broken-hook failure，不贡献 decision。既有非阻断错误策略保持，exit code 2 即使 stdout 失效也继续 deny；Unix timeout/cancellation 终止整个 process group，返回时再次清理后代。该私有 shell contract 的严格化不改变 `hooks.*` Protocol shape、trust/event/scope 语义、Artifact v23、SQLite epoch 82、generated Desktop binding、公共 Go API、Desktop source、Agent Framework 与 CLI。

P165 收紧同一私有 Hook command stdin 合同：Domain command projection 将 prompt/arguments/result/reason 类 material 分别限制在 256/256/128/8 KiB，prompt 与 result 只发布 marked UTF-8 prefix，arguments 必须 lossless；Shell 在进程创建前同时要求 raw material 与最终 JSON stdin 不超过 512 KiB。新增 `promptTruncated`、`tool.resultTruncated` 与 Subagent 对应 marker 只属于 private process JSON，不进入 Flame Protocol。超界或非法 material 作为可观察的 broken-hook failure，不执行 command；declarative hook 仍独立生效。`hooks.*` Protocol shape、Artifact v23、SQLite epoch 82、generated Desktop binding、公共 Go API、Desktop source、Agent Framework 与 CLI 均不改变。

P166 收紧现有 `agentDocs.list` / fresh Run AGENTS.md 与 `recipes.list` 的 authored-source 准入，不改变 wire shape：Application 统一规定每份完整文档最多 1 MiB/valid UTF-8，Agent document cascade 最多 64 份/4 MiB，Recipe 每 scope 128 份且完整级联最多 256 份/8 MiB；filesystem adapter 在 parse/materialize 前以 stat + cancellation-aware `limit+1` 复验，并以 1024-entry sentinel 限制 Recipe directory scan。现存 invalid/oversized source 整体失败；AGENTS.md 模型 projection 仍按 32 KiB 选择完整的 most-specific tail，但单份文档放不进预算时拒绝 Run，不静默省略。Protocol `2026-08-24`、operation/feature/topic catalog、Artifact v23、SQLite epoch 82、generated Desktop binding、公共 Go API、Desktop source、Agent Framework 与 CLI 均不改变。

P167 收紧既有 Workspace VCS read semantics，不改变 wire shape。`workspace.changes.list` 的完整 catalog 最多 10,000 项；`workspace.diff.get` structured 结果最多 5,000 个完整文件、默认/最高 5,000 行与 64 MiB retained string material，`limit=0` 现在明确选择默认 5,000 行，超过预算只在完整文件边界返回 `truncated=true`，第一文件与 binary/zero-row 文件没有例外。Raw diff aggregate 最多 64 MiB，超界 changes/raw/process output 投影既有 `invalid_params` 并要求缩小 workspace/path；Git-backed workspace file listing 的既有 20,000-entry 合同现在在保留第 20,001 个 path 前失败。所有 Git stdout 限 64 MiB、stderr 只保留 64 KiB prefix 且继续 drain，watch fingerprint 有 10 秒 lifetime；external diff/textconv/pager 不参与事实生成，untracked symlink 不跟随 referent，binary 与 quoted path 保持无损。Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、generated Desktop binding、公共 Go API、Desktop source、Agent Framework 与 CLI 均不改变。

P168 收紧既有 model-facing `read`/`apply_patch` 内部 Tool semantics，不改变 tool definition 或 wire shape。read-before-mutation stamp 现在对完整 regular file 做 cancellation-aware streaming SHA-256；删除撤销 stamp，创建/修改刷新 stamp，fingerprint 失败不再跳过 guard。Auto-format 只处理最多 8 MiB 的完整 input/output；Go/JSON 在进程内完成，Prettier 通过 stdin/stdout 运行且 stderr 只保留 64 KiB prefix 并继续 drain，只有验证成功后才 atomic replace。Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、generated Desktop binding、公共 Go API、Desktop source、Agent Framework 与 CLI 均不改变。

P169 收紧既有 model/direct `read` 内部语义，不改变 Tool name、`path/start_line/max_lines` request 或 `content/start_line/end_line/total_lines/truncated` response shape。Runtime 最多准入 8 MiB regular file与 1 MiB line，默认一次结果最多 1 MiB 且只在完整行后停止；`EndLine` 是最后返回行，`Truncated` 对省略 prefix/suffix/result budget 均为 true。完整扫描验证 UTF-8/NUL、BOM/CRLF、读取增长与 caller cancellation；mutation stamp 只有在 Tool call 前后 8 MiB-capped streaming digest 一致时提交。Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、generated Desktop binding、公共 Go API、Desktop source、Agent Framework、`tools/fs` 与 CLI 均不改变。

P170 收紧既有 `workspace.files.read/head` 语义，不改变 `ReadFileRequest`、`FileContent`、`GetFileHeadRequest`、`FileHead` 或 operation shape。`maxBytes=0` 现在选择 1 MiB 默认值，显式值最高 8 MiB；完整 `TotalLines` 扫描只接纳最多 64 MiB regular file，单行最多 8 MiB，并在 64 KiB buffer 上验证 caller cancellation、UTF-8/NUL、BOM/CRLF、trailing empty line 与读取期间增长。Application 对 port result 再验证 output bytes、text、window/content correspondence 与 truncation honesty。编辑器 read 可在有效 UTF-8 边界截断最后一行并设置 `truncated=true`；越界 range/资源返回 `invalid_params`，invalid text 返回 `unsupported_mime`。Head 默认 200、最高 400 行且不发布无标记 partial result。Desktop file view 现在对带行号导航请求目标前后各 200 行并消费响应 `startLine`；这是既有字段的真实消费，不是新 wire。Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、generated binding shape、公共 Go API、Agent Framework、`tools/fs` 与 CLI 均不改变。

P171 收紧既有 `workspace.files.search` 行为，不改变 `GrepRequest` / `GrepResult` shape。`query` 是最多 64 KiB 且必须编译成功的 Go/RE2-compatible regex；`limit=0` 选择 100，显式值最高 1000。Searchable corpus 使用既有 ignore-aware 20,000-candidate file catalog，单个完整 UTF-8 regular file/line 最多 8/1 MiB，一次请求实际扫描最多 512 MiB；binary/invalid/oversized individual source 不属于 corpus，catalog/aggregate 超限映射 `invalid_params`。`Matches` 是最多 8 MiB 的稳定 whole-row prefix，`Total` 在同一次 complete admitted-corpus scan 上精确产生并可大于 prefix；Application 复验 direct port 的 count/material/path/line/order/text/query correspondence。Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、generated binding shape、公共 Go API、Desktop source、Agent Framework、`tools/fs` 与 CLI 均不改变。

P172 breaking 收敛 model/direct filesystem search Tool contract；这不是 Flame Protocol method。Tool 名 `glob`/`grep` 保持。两种 request 当前唯一 shape 都是 `pattern/path?/max_results?`，pattern/path 分别由 strict schema 限制，default/max result count 统一为 100/1000；glob pattern 是相对 selected path 的 case-sensitive segment-doublestar，grep pattern 是逐完整行匹配的 Go/RE2 regex。旧 glob `ignore_case` 与 grep `file_glob/file_type/ignore_case/multiline/before_context_lines/after_context_lines/output_mode` 完整删除，不接受 alias；case-insensitive regex 用 inline flag，文件集合与上下文分别组合 glob/read。Glob response 唯一 shape 为 `paths/total/truncated`，grep 为 `matches/total/truncated`，其中 total 来自完整 admitted corpus，truncated 精确表示 retained prefix。两者使用 canonical root confinement、ignore-aware 20,000-candidate catalog，并逐 row 计算 JSON representation，使最终 encoded Tool result 最多 1 MiB；grep 前置 scanner 仍服从 P171 的 8 MiB row result、8/1/512 MiB file/line/scan。Runtime 不再调用共享 `tools/fs` Glob/Grep 或宿主 `find/rg/grep`。Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、generated binding、公共 Go API、Desktop source、Agent Framework、共享 `tools/fs` 与 CLI 均不改变。

P173 收紧 Runtime internal Skill source/lifecycle 行为，不改变 `skills.discovered.list`、`skills.library.list/archive/restore`、`skills.proposals.*`、`Skill`/`ManagedSkill`/`Page` wire shape 或 model Tool 名/schema。每个 project/user complete-list 当前最多 256 个 valid-name candidate、272 个 raw top-level entries；完整 `SKILL.md` 与 `read_skill_resource` 文件各最多 1 MiB。用户托管 active+archived 总量、approval、完整 list 与 idle sweep 共用 256-entry strict snapshot；`.usage.json` 最多 64 KiB/256 records。越界使用稳定 internal capacity/size error 整体失败，不分页、不截断、不回退共享 SDK 的 unbounded reader。Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、generated binding、公共 Go API、Desktop source、Agent Framework、共享 `skills`/`tools/skills` 与 CLI 均不改变。

P174 breaking 扩展公共 Go surface，新增 `runtime/localruntime` deployment handoff package；`contract/go-api.json` 同批冻结 `ErrInvalidToken`、`Token.Value/Path`、`OpenToken` 与 `ReadToken`。Durable token 文件唯一合法内容为 43-byte canonical RawURL encoding of exactly 32 bytes，必须是 0600 regular file；reader 用 path/open `SameFile` identity 与固定 44-byte probe 拒绝 symlink、替换、增长、空白、padding 和 non-canonical encoding。Runtime executable 与 Desktop 共用该 package；internal HTTP `LocalToken/OpenLocalToken` 与 Desktop private parser 已删除，不保留兼容入口。Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、generated TypeScript binding、Wails binding、Agent Framework 与 CLI 不变。

P175 只收紧 Runtime internal Knowledge crash-recovery 的资源语义：原子 stage sweep 从无界 `ReadDir(-1)` 改为 128-entry、caller-cancellable 的完整流式枚举，并由 architecture gate 禁止所有 Runtime production `os.ReadDir` 与 non-positive `File.ReadDir`。它不改变 `knowledge.list/get/update` wire、1 MiB document/CAS/revision、Protocol `2026-08-24`、公共 Go API、Artifact v23、SQLite epoch 82、generated binding、Desktop/Wails、Agent Framework 或 CLI。

P176 只收紧 Runtime internal Workspace Checkpoint：私有 Git command 统一进入既有 64 MiB stdout/64 KiB stderr `gitprocess.Run`，snapshot selection 固定为 20,000 paths/512 MiB current material，source alternates/index 分别为 64 KiB/64 MiB；raw `ls-files -z` path 不再 trim。内部新增稳定 `ErrSnapshotTooLarge` 供 owner 测试与错误链识别，但不进入公共 Go/Protocol。Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、public Go/generated binding、Desktop/Wails、Agent Framework 与 CLI 不变。

P177 只收紧 Runtime internal Sandbox command writer：stdout/stderr 各自继续完整 drain、最多保留 256 KiB 并使用既有 truncation marker；私有 storage 不再通过匿名嵌入暴露 `io.ReaderFrom`，因此 `os/exec`/`io.Copy` 不能绕过 bounded `Write`。Sandbox Tool output shape、Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、public Go/generated binding、Desktop/Wails、Agent Framework 与 CLI 不变。

P178 只收紧 Runtime internal MCP stdio session teardown：`dial` 的 lifecycle release 现在是可报告错误的 cleanup；Unix command 在 Start 前独占 process group，context cancellation 与 session Close 后 cleanup 都终止整组后代，handshake failure、probe、replacement、detach 与 Host shutdown 共用该 owner。非 Unix 保持 direct-process termination。MCP config/operation/tool shape、Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、public Go/generated binding、Desktop/Wails、Agent Framework 与 CLI 不变。

P179 只替换 Runtime internal Sandbox working-tree materialization：删除 in-memory tar pack/unpack，改为 source/destination `os.Root` 间的 64 KiB chunk copy；既有 100,000 entries、128 MiB/file、512 MiB aggregate 与 relative-symlink/mode 语义保留，并新增 opened identity/size、growth、cancellation 和 destination-inside-source fail-closed guard。Sandbox constructor/Tool output shape、Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、public Go/generated binding、Desktop/Wails、Agent Framework 与 CLI 不变。

P180 breaking 收紧 Runtime internal `infra/exec.Shell.Outcome`，增加 terminal process-tree cleanup error；唯一 Tool consumer 同批迁移，不保留旧三返回值 method。Unix Model Shell 在 Start 前独占 process group，timeout/stop/foreground cancel/natural leader exit/Host shutdown 都 group-stop 并 join；successful leader 的 descendant-held pipe 不改写其 exit code。Model-facing `shell/read_shell_output/stop_shell` name、request/result JSON shape、Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、public Go/generated binding、Desktop/Wails、Agent Framework 与 CLI 不变。

`sessions.snapshot` 是挂载 Session material view 的命名用例，不是通用展开机制：Application 校验
Session/Item/Run/open Interrupt/Plan/Goal 的跨投影关系，并与启动恢复复用唯一 Pending projection closure；每个 waiting
Run 必须由 root Pending 拥有，每个 Interrupt 必须精确解析到同 Session/Run/Item/occurrence 与匹配的 Question/Approval
payload，running Item 必须由 active continuation 唯一认领，terminal Run 不得保留 running Item。Persistence 在一个 SQLite transaction 内读取全部事实，Delivery
按调用方 capability 原样投影或整体拒绝，不能裁剪 waiting set。Desktop 只走这一路恢复已挂载 Session 的 HITL、Plan、Goal、Run/Tool，
并且只有赢得当前 view generation 的响应可以提交整份 material；独立分页资源接口继续存在，未挂载 Goal 才继续由 `goals.get` 读取。该 additive method 不改变
`protocolVersion`、Artifact version 或 SQLite epoch，也不授权旧四读 fallback。
`manifest.methods[].materializes` 只声明复合 query 原子承载的独立事实族，供合同审计和 consumer gate 区分
服务端组合读取与孤儿能力；它不继承目标 query 的筛选/分页语义，也不建立 alias 或客户端 fallback。

Goal read model 的 `status:"completing"` 精确表示模型已声明 objective 成功、但 owning Run 的最终记账与条件清除尚未完成。它保持目标占位且不可 stop/resume/start；下一次 `goals.changed` 后读取 `null` 才表示 settlement owner 已释放。Domain `complete`、Application drive 与公共 `completing` 分属各层，不互相泄露类型。

Goal 管理面 additive 增加 `goals.update` 与 `goals.clear`，不改变 `protocolVersion`、Artifact version 或 SQLite epoch。
update 在 Application drive quiescence 与 Goal CAS 边界内替换 objective，并通过 fresh incarnation 隔离旧 Run provenance；
status/reason、model/capabilities、budget/usage 与 createdAt 不重置。clear 在相同 owner 边界内条件清除，目标已不存在时幂等成功。
两者都不建立 Frontend standing writer：挂载 Session 仍只用 `sessions.snapshot` 修复整份 material。

Knowledge 条目以内容摘要作为 opaque revision。`knowledge.list/get` 即使文件尚不存在也返回可用于首次条件创建的 revision；
`knowledge.update` 必须携带 `expectedRevision`，在 Infra 的同路径原子替换边界比较并返回 committed Entry，不匹配以
`revision_conflict` fail closed。Application 只在成功提交后发布 `knowledge.changed`；Hook trust 同理发布 `hooks.changed`。
三条 Knowledge operation 都将 physical document 越过 semantic scope root 投影为 `path_outside_root`。Infra 解析唯一 physical
identity 后才读写，域内 symlink 的 alias 本身保持不变；跨进程 directory lease 包围 revision compare、权限继承、临时文件 fsync、
原子 rename 和父目录 sync，cold read 回收严格命名的 pre-publish staging。进程崩溃后的可见内容只能是上一 committed revision 或完整
新 revision。
这些 topic 是失效事实，不携带配置值。Provider/model role、approval policy 与 agent-memory review 同样在所属 Application use case 提交后发布专用失效事实；Delivery 才将中性 notice 映射为 wire topic，Desktop Workspace events Adapter 再映射到各 context 公开 query identity，Agent Framework 零感知。

公共 Go surface 只有模块根 `runtime`、`runtime/protocol` 与 `runtime/localruntime`，由生成的 `contract/go-api.json` 完整冻结。`protocol` 只公开 binding-neutral values、strict validation、版本、稳定错误 identity 与 `ProblemError`；模块根只公开 concrete Runtime lifecycle、准确 options 和类型化 operation methods；`localruntime` 只公开 durable token 的 validated `Token`、`OpenToken`、`ReadToken` 与稳定 `ErrInvalidToken`，不公开 transport/server 或 host-directory discovery。同一 canonical data directory 可由另一个 Go/HTTP Runtime 同时打开；实际冲突在对应 Session operation 上投影既有 `session_busy`。服务端 method interface、request context plumbing、numeric JSON-RPC code、reflection shape walker、artifact catalogue、Host、Store、Engine 和 Router 均属于 `internal`，不构成公共 Go surface。P113 对 Assembly、operation、Interaction、Toolset、LSP、MCP 以及 Runs/Sessions/Runsegment constructor 的 breaking correction 只收紧 internal valid construction 与 lifetime ownership；P148/P149 先分离 terminal diagnostic、再按 SDK 合同纠正 MCP close，P150 删除失去生产消费者的 Retryable/settlement 双态并让 terminal Sequence 在失败 Assembly timeout 后继续完成逆序资源图，P151 让 Host 整体 shutdown generation 独立于 caller wait，P152 再让 Instance 以同一 owner 规则从 operation Endpoint 穿过 workers 加入 Host；公共/CLI Close timeout 不再遗弃下层图。P174 breaking 增加唯一 deployment handoff package 并删除 HTTP internal token owner；Protocol method/event、Artifact 与 SQLite shape 不变。

P286 将 `internal/delivery/operation` 与 `internal/delivery/server` 的人工阶段分包收回内聚的 `internal/delivery` 根 package。`Endpoint` 现在同时拥有唯一 binding-neutral 进入点与 Delivery shutdown admission；`Handler` 只拥有 Application/protocol 翻译，不再向 Bootstrap 提供平行 lifecycle。`dispatch` 和 `transport` 仍保留为 JSON-RPC 与 HTTP/SSE 的独立 mechanism。这是 Runtime internal API 的 breaking 替换，不保留 alias、shim 或 forwarding package；公共 Go surface、Protocol method/event、Artifact 和 SQLite shape 不变。

## 3. 持久化 Baseline 1

### 3.1 SQLite

- 当前 `schemaEpoch = 95`；Provider credential/Base URL以NULL表达缺省且拒绝空文本，Session、Run、Goal、Schedule、Schedule occurrence及usage snapshot中的provider/model/reasoning identity都经同一bounded printable value object严格恢复，Agent Memory item identity由`mem_`加32位lowercase hex精确CHECK并经Domain codec恢复，MCP server name以1–32位canonical lowercase ASCII slug精确CHECK并经Domain codec恢复，MCP remote tool name以1–128位协议ASCII grammar恢复且策略存入规范化关系表，Session/Plan/Schedule revision受正数且JSON exact上界CHECK保护，Session catalog Unicode search material 由 Application canonicalizer 写入并严格复验，Transcript compaction 持久化 canonical 非空摘要，MCP 握手超时以 NULL/positive duration 区分 unbounded/bounded，Goal budget 与 Run limits 持久化显式 presence 语义；不允许 provider primitive sentinel、identity/effort 分裂、identity截断/normalization、search material 漂移、摘要缺失、预算哨兵或旧 epoch 迁移；
- P183 将 Runtime 稳定领域枚举直接持久化为其命名文本，并把 `session_permission_modes.mode/restore_mode` 从 INTEGER 改为 TEXT；旧 ordinal/数字字符串与新领域值不兼容，故一次性提升 epoch，不迁移、不双读、不保留 codec 映射表；
- P184 把 operation 注册/typed invocation 的 Go 1.27 泛型行为归还 `Registry` / `Endpoint`，让就近声明的 `operation.Name` 成为注册与模块根 binding 共用的唯一 method identity，并收紧 Hook verdict 与 Tool mutation scope 的 internal zero-value 边界；不改变 Protocol method text、Artifact 或 SQLite shape；
- P185 迁移到 Agent Baseline 31 的 `SchemaFor` owner并整体升级 Runtime/Desktop/Frontend 依赖图；不改变 Protocol method text、Artifact、SQLite、checkpoint 或公共 Go surface；
- `sessions.workspace_path` 是非空列；strict codec 先重建 Domain `Workspace`，相对、非 lexical-clean 或空路径均拒绝，旧 `sessions.cwd` 不读取；
- `sessions.provider` / `sessions.model` 是非空列；strict codec 只恢复 configured exact pair，Runtime 默认只在 Session admission 时安装，不在 reader/Run 层补写；
- `agent_memory_items.embedding_space` 与 `embedding` 是成对为空或成对有效的 search-derived cache；strict reader 拒绝空/半对、非 4-byte vector encoding 与非有限值。cache write 只在 exact item 仍 active 且 content digest 未变时提交；内容编辑同时清空 space/vector。epoch 80 的无空间裸 BLOB 不读取、不迁移；
- `agent_memory_items.content` 与 `agent_memory_ledger.fact` 各自最多 4096 个 Unicode code point；Domain constructor/normalizer、strict reader 与 fresh-schema CHECK 共用同一 owner constant。epoch 81 的无界 shape 不读取、不迁移；
- 数据目录为 `0700` 私有目录，可由少量同版本 Runtime 进程共享；schema/config setup 使用短期跨进程 lease，Runtime lifecycle 不拥有目录全局独占权；
- SQLite 事务与既有 uniqueness/CAS 继续拥有 durable winner。活跃 Session writer、physical working-tree shared/exclusive operation、Goal drive 与 ordered recovery sweep 使用 OS advisory lease；进程死亡由内核释放。单一 recovery winner 固定 Run-before-Goal 并只清理成功接管的 Session，不使用 TTL、heartbeat、全局 checkpoint/callback sweep 或兼容双路径；
- 其他 SQLite connection 的 commit 只触发全量 read-model resync，细粒度本地 invalidation 仍由提交用例发布；该同步机制不拥有 SQLite epoch、Artifact、checkpoint 或 protocol wire shape；
- `runtime_identity` 的单例 opaque namespace 与同一 durable idempotency replay store 共存亡；保留数据库重启不变，删除/重建同路径数据库必须变化，且不暴露数据库路径；
- Goal aggregate 与 Goal terminal ledger 使用 `incarnation_id`，Run/Interrupt provenance 使用 `goal_incarnation_id`；已退休的 `lease_id`/`goal_lease_id` 列不存在且不双读；
- Goal aggregate 还持久化 fresh Start 时协商并冻结的 canonical Run capabilities；Goal Resume 的调用方能力必须覆盖该集合，自治 Run 与 Goal 内 `create_goal` 都继承相同集合；
- executor checkpoint 与 pending interrupt 的技术身份列为 `root_member_id`；continuation/input-request binding JSON 使用 `memberId`/`requestId`，approval binding 额外持有 exact `toolCallId`，使 edited-arguments replay 不按 name/args 猜 ToolCall identity；
- `model_invocations` 与 `tool_invocations` 是 operational attempt journals，只保存 exact Run/Segment/call identity、state 与 started/finished time；semantic assistant final、Tool result、累计 usage 与最新权威 prompt footprint 仍只由 Transcript/Run owners 保存；`runs.context_tokens` 以 `0 = unknown` 保存该可回落的 footprint，不能从累计 input usage 推算；
- `runs.commit_segment_id` / `runs.commit_id` 保存当前 Run 最近一次完整 Application command write-set 的 opaque 技术回执，覆盖 fresh/resume opening、顶层 `EventCommit`、HITL answer claim、HITL tree barrier、waiting-child cancellation 与 terminal boundary；单 Run pump/command owner 在收到结算前不会发出下一笔 canonical command，因此 latest marker 足以核验 SQLite 已 COMMIT 但 success receipt 丢失的完整事务。Running marker 必须属于 exact active Segment，Waiting barrier 与 terminal 保留生产它们的 Segment；尚未打开 continuation Segment 的 answer claim，以及已经 Waiting 且不打开新 Segment 的 child cancellation，都以 empty Segment + unique command identity 表示，不能伪造 Segment。普通 Suspend、Resume、Restore 与 recovery 不沿用旧代 marker；
- `interrupts.state` 只有 `open`/`resuming`：`open` 不得携带 answer/claimedAt，`resuming` 必须携带两者；普通列表/读取只返回 `open`，continuation opening 必须在事务内证明 exact root 的 `resuming` claim；
- `child_run_start_reservations.payload` 是 `adapter/runsegment` 显式拥有的 canonical JSON，只保存没有独立列的 reservation facts；SQLite 不解释 payload，Application Go 结构体布局不是 durable wire。reservation/conclusion 只在 owning root tree 与当前进程仍可回调时保留；root terminal、parked terminal、rollback/restore/delete 在原 write-set 内按 Session 回收，boot recovery 在公共 Run 修复同一事务内清空上个进程的 callback ledger；
- 下一 quiescent barrier 只能由相同 Session/executor/root-member owner 替换 `resuming` row；terminal 与 recovery write-set 删除该 row。不存在 open row overwrite、answer rollback、dual state codec 或兼容列；
- Tool start 不占用 Transcript insertion order；同一 model Tool batch 的 completed Items 与 invocation terminals 按模型声明位置形成一个 canonical write-set；
- 一个 build 只接受一个精确 epoch；
- 没有运行时 migration chain、dual schema read 或 compatibility column；
- 重构产生 shape 变化时直接提升 epoch，并同步 fresh-schema tests、store codec、contract expectation 和本基线。

### 3.2 Executor checkpoint

当前 checkpoint 的产品语义是 Host envelope + opaque Agent Framework complete-tree payload。生产 Bootstrap 只保存 Agent Framework public TreeSnapshot v4 JSON，由 `adapter/agentexec` 唯一解释；Application/Store 不分支解析。

目标合同：

- Application owns checkpoint identity、BuildID、Session/Run identity、model selection、limits、capabilities、accounting 和 child Run binding；
- `adapter/agentexec` owns Agent Framework TreeSnapshot encode/decode/restore；
- payload 对 Application、Domain、Delivery、SQLite 和 Protocol 不透明；
- root Process tree 是 executor payload 的不可拆分恢复单位；
- checkpoint replacement 只能推进 frozen identity/limits 和 monotonic usage；
- terminalization 与 checkpoint deletion 由 Application write-set 原子决定。

P7 延续的 continuation payload baseline 是 Agent Framework TreeSnapshot v4 本身，不再包一层 Runtime 自创 payload version。Agent Framework public parser 校验 snapshot version/shape，exact DeploymentRef 校验策略实现与配置，Host BuildID 校验当前二进制/adapter expectation；任一不一致都 fail closed。Host envelope 的技术 codec 仍由 Runtime 当前唯一 SQLite epoch 拥有；当前 executor checkpoint policy schema 为 v2，并只接受 `goal_incarnation_id`。

### 3.3 Artifact 与 Transcript

Artifact、Transcript Item 和 ToolCall timing 的当前机器 shape 仍由 Runtime contract/store codec 拥有。Session Artifact 当前唯一版本为 27；其他版本在任何写入前确定性拒绝，不从旧 artifact 猜测缺失事实或改写版本号。Artifact 以显式 `plan` steps 携带 Plan 语义，并以必填bounded canonical provider/model 与可选 `reasoningEffort` 保存 Session 的精确模型选择；每条 Artifact Run 也保存自己的 frozen exact selection，并只在有限策略存在时携带至少一个严格正的 limit 轴；usage map的model property name受同一model identity约束。它不携带源 Runtime 的 revision/timestamp。Artifact Run 保留最新权威 prompt footprint，因此导入前后 Context gauge 的事实一致；每个 compaction Item 保留 Runtime 生成的 canonical 非空摘要，不由消费者重建。AgentMessage 的 `phase` 是 Runtime 在模型调用边界写入的过程说明 / 最终回答语义，并随 Transcript、Artifact 与客户端恢复保持一致。Question Item 的 `answers` 是唯一已接受响应；未回答或取消保持字段缺失，claim 成功时与 pending/checkpoint 变更同事务写入 Transcript。ToolCall 的 `approvalDecision` 是该调用实际接受的人类决定，和 Pending consume/checkpoint invalidation/commit receipt 同事务写入，并随续跑终态与 artifact 保留；自动放行不伪造。ToolCall lifecycle 与可选 exact execution duration 是两个事实：后者排除审批等待，无法证明时保持 unknown。Tool failure taxonomy 将工具所属 Run 的取消导致的在途终止表示为 `toolCanceled`，不与执行失败、审批拒绝或父 Run 上的 `childRunCanceled` 合并。

## 4. Agent Framework 消费 Baseline

Runtime 使用 Agent Framework [`API_BASELINE.md`](../../../agent/doc/API_BASELINE.md) 的 Baseline 33 canonical `agent/v0.2.0` module。P8 已把 P4–P7 验证的 root start/result、authoritative model/tool、waiting/restore/answer/steer、managed Delegate child 和 prepared waiting-subtree合同切为生产 Bootstrap 唯一 owner，P11 完成 canonical module path 替换；Core 六个模型模态的产物词汇现统一为 `Output`，其中 Interaction 消费唯一 `chat.Response.Output`。P188 进一步消费调用前 `ModelContextReducer` 与普通 Tool 的 exact `ToolInvocation.ModelResult`；durable store、transaction、模型窗口、产品事件和 client presentation 仍由 Runtime owner持有：

- root Kernel、Agenttest、Interaction、Planning、Planning/GOAP、Workflow、Platform 七个 public package 已冻结；OpenTelemetry adapter 由集成层 `otel/agent` 拥有；
- Process Snapshot v6、TreeSnapshot v4；
- Interaction state/protocol v8/v8；
- context-aware ProcessAdmitter、conclusive ProcessStartOutcome、提交式 `RequestCancellation`、带 exact applied-steer Signal identity 的 ModelInvocation、ToolInvocation、DelegateChildKey、ActiveDelegateChild inspector、DeferredTools/AdvertiseTools 与 contextless PreparedWaitingSubtreeCancellation Apply 已存在；
- Agent Framework Event 是 Framework 已发生事实，Delta 是 best-effort 临时输出；
- Strategy payload 和 TreeSnapshot private state 对 Runtime 不透明。

Runtime 只把 Agent Framework public API 当合同。原框架实现、Agent Framework tests/private wire、当前 `agentexec` API 都不是兼容基线。

P7 的两个前置缺口已经由真实 Runtime consumer 在 Agent Framework 中以 Framework-neutral 合同关闭：accepted admission 通过 prospective identity 的 started/aborted outcome闭合；waiting subtree 通过 one-shot prepared capability 持有同一 safe cut，全部 fallible staging 位于 Prepare，durable commit 后只调用 contextless Apply。Run、Store、transaction、产品 ID 和 private tree wire均未进入 Agent Framework。

`PreparedStepAcknowledger` 仍只回调单 Process Snapshot，Runtime 初版不启用。durable recovery baseline 只有已提交 quiescent complete-tree checkpoint；active-step crash 不伪装为可恢复。

Runtime 的 executor-owned opaque checkpoint envelope 当前 schema 为 v5：除完整 TreeSnapshot、指令上下文、accounting、exact model/reasoning selection 与 Agent steer Signal identity 到产品消息内容的精确映射外，还保存每个已accounted Process最近一次完整主请求的provider-reported/Runtime-estimated token pair，并为 Resume input 保存已由 opening 创建的 exact projected Item identity；恢复后 Conversation 仍会在安全边界追加一次，但 Transcript 不会重复创建 Item。Application 仍只持有 opaque bytes；Agent Framework 不见 Transcript 内容、token估算策略或 Runtime persistence。旧 envelope 不双读，恢复时确定性 fail closed。

### 4.1 允许的 import 边界

目标 production allowlist：

```text
internal/adapter/agentexec/** -> agent, agent/interaction
```

只有真实接入 Planning/Workflow/OTel/Platform 时，才分别通过 ADR 增加精确 package edge。默认禁止：

```text
internal/domain/**      -> agent/**
internal/application/** -> agent/**
internal/delivery/**    -> agent/**
internal/infra/**       -> agent/**
internal/adapter/toolset/** -> agent/**
```

临时 Agent module import 已永久禁止；不存在迁移 allowlist。

## 5. Application/Agent 防腐基线

### 5.1 Application 可以表达

- Run execution start/observe/cancel；
- opaque executor root/member identity；
- opaque checkpoint payload 和 Host expectation；
- product Interrupt/answer、SteerRun；
- child Run admission facts；
- waiting subtree 的应用事务输入/结果；
- executor lifecycle facts 和稳定 product outcome。

### 5.2 Application 不得表达

- Agent Framework `Process`、`Execution`、`Deployment`、`Signal`、`Effect`、`WaitID` concrete types；
- `TreeSnapshot` field、ExecutionState payload、Interaction phase 或 mailbox；
- arbitrary Signal submission；
- model/tool lifecycle 从 Delta 推断；
- Agent Framework Engine/Dispatcher/Resolver handle；
- arbitrary EffectID/Settlement/ResolveEffect endpoint；
- Framework Store、transaction 或 product metadata extension。

Unknown Effect 的产品合同是 live/recovery 一致的 fail closed：Application/Delivery 不得到 Settlement payload 构造权；agentexec 只向 Application 投影 indeterminate executor fact/identity。RunLost write-set 提交前 Process 保持 unknown wait，提交后才 Kill/release。

P8 已冻结生产 executor port：`RootExecutionStarter` 负责 validate/stage/begin，`ExecutionObserver` 负责只读事实流，`RunningRootCancellationRequester` 只提交 Framework cancel request，`ExecutionReleaser` 只负责 resource lifecycle；`WorkingContextComposer` 在 Application 边界组装完整 fresh-root context。Application opening durable 后 Begin 才 Start Process；cancel intent durable 后请求停止，pump 继续观察到确定终态才 release。

P8 已冻结 authoritative model/tool 合同：executor producer 只能通过同一有序 observation stream 提交 Application-owned closed fact 并等待 receipt；它不取得 Store、transaction 或 reducer。Application Run pump 在 speculative reducer 上计算 write-set，只有 persistence 全部成功才替换 live reducer并完成 receipt。model/tool post-call receipt failure 必须返回 Agent Framework Dispatcher 形成 unknown；pre-call failure 禁止外呼。Toolset 的唯一 visibility value 是 framework-neutral `toolset.Manifest`，通用 Toolset 对 Agent Framework 零 import。

P8 已冻结 continuation 合同：`WaitingExecutionContinuer.StageContinuation` 只 stage 一棵 exact live waiting tree，或按 opaque TreeSnapshot + exact Deployment/BuildID/Host scope 恢复；它不读取 Conversation，也不重算 WorkingContext。Application 先原子记录 exact answers、隐藏 interrupt row 并删除旧 checkpoint，再 stage/restore；next-Segment opening transaction 必须证明 durable `resuming` claim，成功后 `BeginContinuation` 才投递 WaitID-addressed semantic Signal。claim 后到下一 quiescent checkpoint 前没有 fallback recovery point，crash/boot recovery 一律 `RunLost`。

Product Interrupt/prompt/answer 使用 framework-neutral strict codec；`interactioninput` ACL 是唯一把它映射到 Agent Framework pending-input/Signal 的 owner。旧 private suspension adapter 已删除。真实 Runtime `ask_user`、interactive approval、deferred advertisement restore 与 steer 均走唯一生产路径。

P8 已冻结 child/subtree 合同：Delegate ToolCall authoritative commit 先于不可见 child start reservation；Agent Framework conclusive started 后才公开 child Run，aborted 只闭合 reservation。多 child、嵌套 child与乱序 sibling completion 使用稳定 parent/model-call/tool-index 因果顺序；恢复归因只调用 Interaction owner 的 typed inspector。waiting child cancellation 执行 prepare → application transaction → contextless Apply/Discard；移除最后边界时，Apply 只安装 resulting state，独立 Continue 才激活已提交 Segment。Apply 异常先释放旧 owner并由 `WaitingExecutionRestorer` 从 committed resulting checkpoint 精确恢复，恢复失败才 RunLost。

P8 已冻结这些内部消费端口及防腐语义：Application 单写者、operational journal 与 semantic Transcript 分离、final 独立于 Delta、并发 Tool canonical prefix 原子提交、unknown 在 release 前 durable `RunLost` 收口、answer claim → stage/restore → durable opening → semantic Signal，以及 Delegate reservation → conclusive start → public child Run 的唯一顺序。

Fresh root input 的防腐合同是 Application `WorkingContextComposer` 读取 Host Conversation 并追加当前 user message，再组装 Knowledge、Plan、Memory 与 hooks，形成完整 `WorkingContext` seed；agentexec 不读取产品 Store。成功 assistant final 由 Agent Framework Result 投影 `AssistantMessageCompleted`，不从 Delta 拼接。

WorkingContext 的来源归因属于 `adapter/agentexec` 私有合同：base prompt、Knowledge、pinned/recalled Memory、AGENTS.md、Plan 与 lifecycle hook 只在实际注入后以 versioned JSON-safe Message/Part metadata 标记，并区分 instruction/data purpose。该 metadata 随 opaque Interaction checkpoint 自包含恢复，但不进入 Runtime Protocol、Artifact/SQLite schema、Application port 或 Agent Framework 公共类型；公共诊断若出现必须另行设计安全投影。

P23 进一步冻结该私有合同的行为所有权：context source kind 唯一决定 purpose 并在 metadata 写入前验证；预算后的 Memory/Agent document prompt fragment 同时持有可见文本与来源；hook result 负责 block/injection/provenance 应用；`WorkingContextComposer` 负责完整 system/Plan/recall/hook 编排。该内部重构不改变 metadata JSON shape、prompt 文本或任何公共/持久合同。

Application executor tree identity 统一为 `ExecutorMember`/`MemberID`。Framework `ProcessID` 只能由 execution adapter 在边界内映射，不能重新进入 Application field、port 参数、持久化 technical field 或 Runtime Protocol。

## 6. Clean Architecture 边界基线

目标允许边：

```text
domain      -> stdlib + approved stable values/pure domain strategy contracts
application -> domain + consumer-owned ports + approved observation API
infra       -> domain immutable values + technical SDKs/mechanisms
adapter     -> application + domain + infra + capability SDKs
delivery    -> application + domain projection values + protocol/transport
bootstrap   -> config + application + adapter + infra + delivery
cmd         -> bootstrap + config
```

目标禁止边：

```text
domain      -/> application|adapter|infra|delivery|bootstrap
application -/> adapter|infra|delivery|bootstrap|Agent Framework|driver SDKs
infra       -/> application|adapter|delivery|bootstrap
adapter     -/> delivery|bootstrap
delivery    -/> adapter|infra|bootstrap|Agent Framework
all rings   -/> bootstrap composition objects
```

同一 ring 内的 package 仍必须形成 DAG；不能用同层身份为循环或 god package 辩护。

## 7. 自动守卫

P1 已建立：

- production target package DAG，以及会被稳定拒绝的 Delivery → Adapter 反例 fixture；
- Agent Framework importing leaf 与 imported public package 的双重 exact allowlist；
- 临时 Agent module import 的永久 absence guard；
- Domain、Application 与 Delivery 既有 external SDK denylist；
- context-based Domain I/O port、旧 private snapshot decoder 和旧 lifecycle owner 的永久禁止守卫；
- compatibility/legacy/versioned source directory 禁令。

机器 owner 是 `internal/arch/target_architecture_test.go`、`internal/arch/framework_boundary_test.go` 与各专项 architecture fitness test；不存在 temporary exception 台账，本文件不复制易漂移的逐文件集合。

P2–P10 已建立：

- protocol artifact digest/drift test，以及 canonical sample 同类型 strict `ValidateWire` gate；
- SQLite schema epoch 和 prior-version rejection test；
- checkpoint envelope strict codec、size、copy、round-trip 和 prior-version rejection（P6 已覆盖 Agent Framework TreeSnapshot parser、copy、corrupt/wrong-build/deployment；P8 随 production owner 收口剩余 envelope guard）；
- Agent Framework type/name leakage AST guard；
- no `component/common/core/utils` package guard（P18 将根级 cross-ring purity allowlist 与 Application mechanism owner guard 分开冻结）；
- no alias/dual codec/legacy path guard；
- exported contract GoDoc/parameter/error wrapping guard where the contract is intentionally frozen。

## 8. 不在 Baseline 1 中

- Runtime 对 Agent Framework Platform 的接入；
- 前端/TUI/CLI 新 consumer API；
- 新 binding 或 transport 必须有真实消费者与独立语义，不为未来客户端预留 package、factory 或 interface；
- 未来数据库 epoch、artifact version 或 Agent Framework TreeSnapshot version。

这些内容不能以 placeholder、预留字段或空接口提前进入代码；真实阶段完成后再冻结。
