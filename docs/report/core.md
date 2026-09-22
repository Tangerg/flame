**两份报告已经完成，按你指定的“核心流式协议 / 旁路 API”划分，每份都以 Flame 为主体，同时对照 OpenCode v2 和 Codex main。** 报告包含源码证据、设计取舍、最小改进范围、实施顺序和验收场景。

## 两份报告

### 1. 核心 AI 流式协议

[Flame_Core_Streaming_Protocol_Analysis_2026-09-21.md](sandbox:/workspace/scratch/406683280e4a/reports/Flame_Core_Streaming_Protocol_Analysis_2026-09-21.md)

覆盖 Session / Run / Segment / Item 的语义，start / steer / resume / cancel 的成功边界，权威事件与预览、游标、快照与订阅衔接、断线恢复、进程重启、幂等与未知结果、人工中断、子任务、背压和前端投影。

### 2. 旁路 API

[Flame_Auxiliary_API_Analysis_2026-09-21.md](sandbox:/workspace/scratch/406683280e4a/reports/Flame_Auxiliary_API_Analysis_2026-09-21.md)

覆盖工作区与文件系统、搜索、附件、工作区与运行范围的 diff、回滚、skills、配置、模型、MCP、授权尝试、资源读取、缓存失效、终端与进程，以及 hooks、授权规则、目标、调度、知识、记忆等相邻领域。附有 API 索引，便于后续拆任务时核对。

### 分析基线

所有源码链接均固定到本次读取的提交，避免分支更新后证据漂移。

| 仓库                 | 分支   | 固定提交                                                                                             |
| ------------------ | ---- | ------------------------------------------------------------------------------------------------ |
| Tangerg/flame      | main | [5989445](https://github.com/Tangerg/flame/commit/5989445dbd787c0914a72013bb511b9a08399138)      |
| anomalyco/opencode | v2   | [2f0c861](https://github.com/anomalyco/opencode/commit/2f0c861af098ec914a19751fe62d715448f6bf84) |
| openai/codex       | main | [40eeb6e8](https://github.com/openai/codex/commit/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f)      |

## 总体判断

**Flame 值得保留现有协议骨架，优先补齐少数语义和异常处理边界。**

你现在已经有明确的 Runtime 所有权、Session / Run / Segment / Item 分层、命令幂等、权威事件与临时预览分离、有限回放、快照恢复和协议生成链。旁路也已有工作区约束、有界文件读取、结构化 diff、技能管理、MCP 状态与资源重同步。这些都是实际实现，不能再列成“参考两个项目后才需要建设”的基础设施。[Flame Run 协议](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/runs.go)、[事件契约](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/events.go)、[方法 manifest](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/contract/manifest.json)

两家的主要启发在于：**把输入接纳、事实提交、物料完整度、实际生效状态和过程生命周期表达得更清楚。** 下面是最影响改进顺序的发现。

## 一、核心流式协议：建议优先处理的三个点

### 1. 核心权威事件处理失败时，需要明确停止确认游标并进入恢复

这是本次最值得优先核查和修正的一条路径。

Flame 的核心事件折叠器也通过插件机制注册。当前 reducer 会捕获 handler 异常并记录诊断，然后继续返回；`agentStore` 遍历结束后仍可能将批次标为已接纳，batcher 随后确认批次末尾位置。因此，**“批次被接纳”目前不能完全等同于“其中每个权威事实都成功应用”。** [异常捕获](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/desktop/frontend/src/plugins/builtin/agent/application/fold/reducer.ts#L8-L20)、[批次接纳](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/desktop/frontend/src/plugins/builtin/agent/adapters/agentStore.ts#L163-L180)、[游标确认](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/desktop/frontend/src/plugins/builtin/agent/adapters/runEventBatcher.ts#L47-L59)

建议在现有投影 owner 与 pump 之间补明确的失败信号：

* 核心权威处理失败，不确认该批次。
* 结束当前观察 generation，防止后续批次越过失败位置。
* 进入已有 snapshot-tail 恢复。
* 可选插件的附加展示失败，继续保留隔离和诊断。

这里不能只简单地重新抛异常：batcher 有 rAF、容量触发、尾部 flush 等执行位置，异常未必能到达 pump 的消费循环。报告已把这些路径列入验收矩阵。

**这是静态源码确认的异常控制流边界，本次没有运行故障注入，不能据此声称正常流已经发生数据丢失。**

### 2. Steer 值得补稳定输入关联，解决前端按内容猜对应关系

Flame 已经有 `expectedSegmentId` 和后续真实应用输入的机制，当前不足更具体地落在前后端关联上。

前端先建立本地 steer 占位消息；成功 ACK 不会制造权威 Item。后续权威用户消息到达时，当前 fold 按提取后的文字查找同文占位。这个匹配没有比较附件内容，因此同文多次追加、相同文字配不同图片、纯图片输入都缺少精确对应依据。[发送与 ACK](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/desktop/frontend/src/plugins/builtin/agent/application/input/chatSend.ts#L76-L105)、[当前匹配逻辑](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/desktop/frontend/src/plugins/builtin/agent/application/fold/fold.ts#L188-L232)

建议由现有输入 owner 提供稳定的接纳标识，或预留未来使用的 `userItemId`，并随真实应用事实带回。要保持两个边界：

* 有身份、已接受，不代表 Item 已提交。
* Item 已进入实际应用边界，也不能被界面夸成“模型已经理解”。

OpenCode 的持久 Inbox 对“输入接纳与执行分开”很有启发，但 Flame 可以先补精确关联；只有确实要做可见排队、撤回或跨重启输入对账时，再扩展持久生命周期。[OpenCode Inbox 交付](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/core/src/session/projector.ts#L601-L628)

### 3. 保留现有幂等防重，专项验证“业务提交后、回执提交前”的崩溃窗口

Flame 当前顺序是 Claim → 执行业务 → 编码结果 → 完成幂等回执。同进程内有 pending 结果补写和优雅关闭处理；SQLite 未决 reservation 不会仅因为时间经过而释放。[回执提交顺序](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/delivery/replay.go#L76-L125)、[SQLite 保留逻辑](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/infra/sqlite/idempotency.go#L37-L88)

从这个顺序推断，如果进程在业务提交后、回执完成前退出，重启后原 key **可能长期处于 `idempotency_in_progress`**。这是保护副作用不被重复执行的保守行为，需要改善的是可核对性和用户解释。

建议先做隔离故障验证，再决定哪些操作可以通过明确的持久关联恢复回执。前端已有未知结果身份保留机制，应复用；不能靠超时换 key、删除 reservation 或相似文本匹配来“恢复成功”。[现有客户端未决处理](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/desktop/frontend/src/rpc/mutationJournal.ts#L129-L164)

## 二、对两家核心协议，应吸收哪些设计

### OpenCode v2：输入生命周期和执行屏障值得学，默认日志保证要看清

OpenCode 将输入接纳、交付到历史、执行推进分别建模，这对排队、追加输入和取消 pending 输入很有价值。但 `delivered` 只证明输入进入了历史，不能解释成 provider 已接收或模型已经使用。

此外，本次 v2 的普通 event 流与实验 Session log 是两套契约。Bus 默认 `persist=false`：持久投影仍更新，但事件正文只在开启保留时写入。因此，不能把“有日志 API”直接理解成默认全量事件溯源。[默认保留开关](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/core/src/bus.ts#L200-L205)、[事件正文写入条件](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/core/src/bus.ts#L395-L430)

对 Flame，我建议保留当前有限回放与一致快照；只有明确需要审计或历史事件重播时，再评估持久日志。

### Codex main：物料完整度和生命周期表达值得学，控制语义要自行取舍

Codex 的 Thread / Turn / Item、`itemsView`、服务端交互请求与恢复组织方式，都能帮助检验客户端是否准确理解“未加载、摘要、完整”“仍待答、已解决、执行结束”。

不过，Codex 的 `turn/start` 可以进入 start-or-steer 路径。Flame 当前显式区分 start、steer、resume，并对目标 Segment 做保护，这种语义适合精确运行历史，值得保留。[Codex start-or-steer](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server/src/request_processors/turn_processor.rs#L647-L714)

另一个应保留的 Flame 细节是：**`runs.cancel` 成功已经具有结算屏障**；如果自然完成或失败先获胜，取消不能覆盖它。不能为了套用另一家的异步 ACK 模型，把现有成功响应降格为“只是收到停止请求”。[Flame 取消结算](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/application/agent/runs/cancellation.go#L51-L103)

## 三、旁路 API：最有价值的改进集中在四个方面

### 1. Diff 先让比较对象可核对

需要区分：

| 用户意图         | 对应物料                       |
| ------------ | -------------------------- |
| 当前工作区改了什么    | Git 基线到当前工作区的差异            |
| 这个分支准备合并什么   | merge-base 到 HEAD 或工作区     |
| 这一轮任务期间发生了什么 | Run / Segment 范围、快照区间或操作来源 |
| 批准或撤回后会怎样    | 带前置版本的动作预览                 |

Flame 现有 diff 已支持结构化行、rename、binary、计数和截断，应保留。优先增量是让响应和界面明确**实际解析的基线**；随后再按需求考虑运行范围变化集和分片读取。[Flame Diff 结构](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/workspace_diff.go#L3-L65)

OpenCode 的 working / branch / committed 比较有启发；Codex 则同时存在 Turn 聚合 diff、FileChange Item 和废弃的 Git 旁路查询，说明这些物料本就承担不同职责。[OpenCode 比较范围](https://github.com/anomalyco/opencode/blob/2f0c861af098ec914a19751fe62d715448f6bf84/packages/protocol/src/groups/vcs.ts#L1-L104)、[Codex Turn diff](https://github.com/openai/codex/blob/40eeb6e8a89ef421c25d4c40e06fa1d40ce66b4f/codex-rs/app-server-protocol/src/protocol/v2/turn.rs#L553-L563)

### 2. Skill 增加详情与来源，让已有 revision 产生可操作冲突

建议保持 discovered 列表轻量，由实际 resolver 提供按需正文、来源、scope、revision 和必要诊断。前端不应重新实现同名覆盖规则。

一个可以较快落地的点是 proposal 错误：Flame 已有 exact revision，但部分冲突被映为 `invalid_params`。可以复用 `revision_conflict` 或明确的 proposal-state error，让界面知道需要重新审阅，并保留用户位置。[Proposal 契约](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/skillproposals.go#L11-L33)、[当前错误映射](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/delivery/handler_skills.go#L122-L149)

OpenCode 全文放入 skill list 的方式无需照搬；Codex 的元信息、局部错误和详情分离更有参考价值，但其部分详情接口有 marketplace 范围限制，报告已明确标出。

### 3. 配置先说明什么时候生效

Flame 各领域已经有不同且合理的生命周期：

* Utility role：下个 Run boundary。
* Embedding role：后续 search。
* MCP update：保存后进入 connecting，再由后台连接推进状态。
* Hook trust：下个 Run 重读。
* Skill archive：影响后续解析，不能抹去已注入的上下文。

这些事实首先应该进入权威文档和设置界面，不必统一增加一套 `appliedAt / restartRequired` 状态机。[Role 生效边界](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/application/integration/models/roles.go#L11-L80)、[MCP 更新与连接](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/application/integration/mcp/servers.go#L93-L148)、[Hook trust 边界](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/internal/delivery/handler_hooks.go#L81-L85)

Codex 的配置来源、有效值和 override 解释值得吸收；它的整套层级配置和热更新体系则未必适合 Flame。

### 4. 资源失效先用好已有字段，再决定优化

Flame wire 已带 path、workspace、watchId、names、serverIds 等信息，前端目前简化后较多按整类 query 失效。这里有减少重复 IO 的空间，但当前粗粒度重读是可靠策略，不能直接定性为性能故障。[Wire 资源提示](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/runtime/protocol/runtime_events.go#L131-L162)、[前端失效映射](https://github.com/Tangerg/flame/blob/5989445dbd787c0914a72013bb511b9a08399138/desktop/frontend/src/plugins/builtin/workspace/domain/eventInvalidation.ts#L55-L84)

建议测量后再细化，并保留 resync、旧 generation 拒绝，以及 ignore/config 变化引起的广域失效。OpenCode 的刷新合并和按消费者读取值得借鉴，Flame 无需新建事件总线。

## 建议的实施顺序

| 顺序         | 工作                                                    |
| ---------- | ----------------------------------------------------- |
| **优先处理**   | 核心权威 fold 异常恢复；steer 精确关联；已有 ACK、模型选择与配置生效文档校准        |
| **随后推进**   | Skill proposal typed conflict；diff 基线可见；skill 按需详情与来源 |
| **验证后决定**  | 幂等崩溃窗口对账；长会话读取成本；资源失效粒度；大 diff 分片                     |
| **产品需求触发** | 文件写入、二进制与 artifact、交互终端、远端环境、多工作树、marketplace         |

这次通过 GitHub 插件完成的是**静态源码与测试代码分析**，没有修改三个仓库，没有运行应用、真实模型或上游测试套件。报告中的已实现行为、静态推断与新增建议已分别说明，便于你据此安排后续验证和改进。
