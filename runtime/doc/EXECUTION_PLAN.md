# Flame Runtime and CLI quality plan

> 状态：当前质量 Goal 进行中
>
> 范围：`runtime`、`runtime/localruntime`、`cli` 与它们拥有的仓库文档

本文只记录当前授权、工作流、批次状态和下一道门槛。稳定架构由 [`ARCHITECTURE.md`](ARCHITECTURE.md) 拥有，裁决历史由 [`DECISIONS.md`](DECISIONS.md) 拥有，当前能力由 [`CAPABILITY_LEDGER.md`](CAPABILITY_LEDGER.md) 拥有，精确版本与 digest 由 [`CONTRACT_BASELINE.md`](CONTRACT_BASELINE.md) 拥有。旧实施批次和逐条命令输出由 Git 历史保留，不再追加到本文。

## 当前目标

持续深度打磨 Runtime 与 CLI 的代码质量，直到用户叫停。每个批次必须减少真实维护成本、修复可复现产品缺陷，或收紧一个已有合同；纯风格改写、指标驱动拆分和形式化分层不构成批次。

目标方向如下：

- 优先使用行为充足的具体对象和值对象，减少过程函数围绕公开字段和原始值拼装状态
- 采用领域驱动设计与整洁依赖方向，但不按模式名称制造 package、interface、service、repository 或 manager
- 收敛过度分包、转发包装、重复表示和泄露边界，同时保留真正的领域、防腐、生命周期与技术机制 owner
- 允许 breaking change，并在同批删除旧 API、wire、storage、codec、test、doc 和 consumer 路径
- 用真实 DeepSeek 配置验证 Goal、Plan、steer、HITL、compaction、长运行和长上下文
- 让 TUI 的布局、交互密度与细节以 Grok Build 为主要参考，让 provider 处理以 OpenCode 为证据，让协议机制以 Codex Server 为证据

## 明确不做

- 不修改、格式化、生成、暂存或提交 `desktop`
- 不为一个 CLI 和一个逻辑 Runtime 设计通用多客户端、多服务端或分布式协调模型
- 不以文件行数、package 数量、圈复杂度、重复扫描或 lint 数量作为单独改动理由
- 不为 OOP 或 DDD 制造装饰性方法、空聚合、基类、通用 Repository、事件总线或依赖注入框架
- 不为看似完整的测试矩阵补 race、fuzz、跨平台、多连接或压力测试；只有改动边界需要时才运行
- 不复制参考项目的 daemon、cloud、plugin、compatibility 或进程拓扑
- 不打印、复制或固化 `runtime/config/config.yaml` 中的凭据

## 外部参考的使用边界

| 参考 | 只采纳的证据 | 禁止照搬 |
| --- | --- | --- |
| `/Users/tangerg/Desktop/scope` | 两条修复法则、充血模型、一个语义一个 owner、扁平 package、最新发布合同 | Scope 的 framework/module 发布拓扑 |
| `/Users/tangerg/Desktop/study/codex-server` | 协议分层、turn control、steer、interrupt、compaction、recovery | 多 client、daemon、云任务与兼容协议 |
| `/Users/tangerg/Desktop/grok-build` | TUI 信息层级、composer、Goal/Plan、流式稳定性、键鼠与 resize | Rust crate 边界和后台服务结构 |
| `/Users/tangerg/Desktop/opencode` | provider 目录、exact identity、credential precedence、custom endpoint、capability projection | JavaScript plugin runtime 与动态 provider 装载架构 |

采用参考设计前，必须写明 Flame 的 owner、真实消费者、现有反例和被删除的复杂度。相似实现不自动构成迁移理由。

## 工作流

### 1. 建立规则基线

- 让根 `AGENTS.md` 成为唯一仓库规则源，`CLAUDE.md` 只引用它
- 给 CLI 建立局部规则与稳定架构文档
- 删除点时审计、完成清单和重复事实；Git 保存历史，owner 文档保存当前合同
- 修正文档中与当前 Protocol、Artifact、SQLite、共享目录和 CLI 授权相冲突的事实

### 2. 升级依赖

- 查询 `runtime`、`runtime/localruntime` 与 `cli` 的全部 direct update
- 把所有 Scope module 升级到最新发布且相互一致的版本
- 升级其他 direct dependency，再由 `go mod tidy` 收敛 indirect graph
- 不使用本地 `replace`，不借 `go.work` 偶然解析未声明依赖
- 运行 workspace 与 `GOWORK=off` standalone 门禁
- 依赖升级形成独立提交，不混入架构重构

### 3. 建立真实场景基线

- 运行现有 Runtime 与 CLI offline suites，记录原有失败
- 使用生产配置路径运行最小 live DeepSeek smoke
- 建立或修复真实 E2E driver，覆盖普通 Run、Tool、Goal、Plan、steer、HITL resume、compaction 与长上下文
- 只有真实场景暴露缺陷时才增加新的持久化、协议或生命周期机制

### 4. 审计所有权与 package

- 从 composition root、Runtime operation、Cobra root 和 TUI state 追踪真实调用图
- 为每个候选证明 consumer、dynamic entrypoint、public obligation、persisted shape、lifecycle 与 history
- 优先删除 forwarding wrapper、single-owner micro-package、duplicate DTO、primitive sentinel 和 mirrored state
- 优先在现有 owner package 内用多个职责文件重组；只有独立变化轴存在时才保留 package
- 不把微包合并为 god backend，不把过程代码分散为更多装饰性对象

### 5. 修复领域与应用边界

- 让 Domain entity/value object 拥有自身不变量、合法转换和派生语义
- 让 Application use case 拥有跨 aggregate write-set、durable winner 和 external apply 顺序
- 保持 config、wire、request、response、storage record 和 external fact 为数据
- 删除 `validate → mutate → cleanup` 由调用方记忆的隐式协议
- 对每个 breaking change 一次迁移 Runtime、CLI consumer、生成合同与文档

### 6. 修复 provider 与协议边界

- 以 exact provider/model/options 为唯一身份
- 统一 credential、endpoint、catalog 与 capability 的准入 owner
- 对 HTTP 和 embedded 运行同一 operation/Application 路径
- 对缺失凭据、错误 endpoint、能力不匹配、重复 model、stream 中断与 provider error 建立真实反例
- 必要协议能力先参考 Codex Server，再按 Flame 领域语言设计，不泄露 Framework 或 transport 类型

### 7. 打磨 TUI

- 先对齐 Grok Build 的信息层级、spacing、composer、Goal/Plan、stream block、overlay、focus 与 resize
- 复用 Oolong 原语，不在 Flame 内 fork editor、terminal mode 或 cell-width 机制
- 收敛散落的 key/mouse condition 与 presentation state 到真实 owner
- 用 deterministic render/application tests 验证状态，用 PTY 验证终端协议
- 不因像素对齐重写无关业务或建立 design-system package

### 8. 验收与持续迭代

- 运行本批最小决定性测试，再运行受影响模块 test、vet、build、tidy、generate 和 architecture gates
- 只有并发 owner 改动才运行 targeted race；只有 strict arbitrary-input codec 改动才运行 targeted fuzz
- 运行适用的真实 DeepSeek E2E，并确认凭据未出现在输出或 diff
- 检查完整 staged diff，确保没有 `desktop`
- 更新本表，形成一个可独立 revert 的提交并推送
- 从新的真实场景或高置信审计候选进入下一批

## 批次记录

本表只保留当前 Goal 的批次结论。详细文件、命令和失败输出属于对应提交。

| 批次 | 目的 | 状态 | 验收 | 提交 |
| --- | --- | --- | --- | --- |
| Q0 | 建立根与模块规则，收敛文档 owner，删除 CLI 点时台账 | 已完成 | 链接检查；Runtime 文档/SQLite 事实门禁；CLI 架构门禁；CLI 随附配置示例由自身严格 loader 验证，Runtime 示例不再复制 provider catalog；CLI 入口文档明确分离 consumer preferences 与 Runtime 执行配置 | `f9cab5a`、`94e45e7`、`6c9ebc3` |
| Q1 | 升级 Scope 与全部 direct dependency | 已完成 | Scope v0.12.0；Runtime 实际构建图同步前移 AWS SDK 与 pprof，其他 direct dependency 复核无更新，未消费的上游子图继续由 `go mod tidy` 收敛；Runtime 与 CLI 的 standalone `localruntime` 发布依赖同步前移到 `92fa7f3`；workspace + standalone test/vet/build/tidy/verify 全绿，CLI Staticcheck 全绿；真实 DeepSeek Goal+Plan | `290ea4a`、`09f0ae8`、`a8a02b8`、`bff76b6`、`459a851`、`ba21200`、`bf6bce2`、`6e7b3e0`、`759e0d3`、`02968a5`、`9aac9cc`、`c48c52c`、`1236ee3`、`1203902`、`8f4704d` |
| Q2 | 建立真实 DeepSeek E2E 场景基线 | 已完成 | Goal/Plan、steer、13-turn long-context compaction、question/HITL restart-resume、进程强杀 long-running Tool 后的 lost recovery 与同 Session 再运行均已通过；真实 TUI 已验证运行中 steer 留在同一 Run，free-text `ask_user` 的答复、Tool result、Question 与最终文本能一致冷读；真实 one-shot Question 退出后，另一进程的 TUI 用同一 Session 立即恢复弹窗、回答并续接原 Run 第二个 Segment；真实 CLI JSON 冷启动与同 Session 续接也已经 DeepSeek 验证；CLI 按 Plan revision/content 幂等折叠 Runtime 的终段 latest-value fence，真实两轮 Goal 与跨进程 Plan 恢复不再被合法重复通知翻成失败 | `e93eab5`、`61ce5d7`、`ceb2e3f`、`7e8880b` |
| Q3 | 收敛 CLI 过度分包与贫血模型 | 进行中 | composition manifest 已收敛；provider 环境凭据覆盖、exact key 与源码 worktree 的真实配置发现已验证；Provider rich model 由 credential/endpoint policy 派生 readiness，wire flag 只用于拒绝双向矛盾而不再成为第二 truth；CLI 缺省 Run 省略 provider/model 并继承 active Session durable selection，不再硬编码 DeepSeek 覆盖 Runtime 默认，TUI 只合成显示 label；CLI `FLAME_CLI_*`、Runtime `FLAME_*` 与 subcommand-local flags 各自拥有配置边界；隔离临时 home 的真实 DeepSeek CLI one-shot 以空 `options:{}` 完成，实际 usage 归属 Runtime 默认 model；one-shot 只由 Conversation root 终态结算，child 结束后断流会续接原 segment 而不是提前退出；`run` stdin 使用 4 MiB `limit+1` 外部字节流包络并对最终组合值复验 UTF-8/大小，不再在任何 Session/Run admission 前无界 `ReadAll`；Approval 以 Tool invocation 同值关联 running Item，Item 独有的 safety/timestamp 不再把合法 HITL 翻成 stream failure；真实 DeepSeek unattended Approval 已验证默认拒绝与 `--approve-all` 执行均正确完成并可跨进程冷读；Workbench 以同一 16 MiB complete-document 合同约束读写，并闭合 versioned envelope、value 与 pending HITL rich codec 的字段词汇，不再接受合法 JSON 前缀后的超限、尾随或未知数据，也不能写出下一进程无法重开的状态；Stash/Workspace rich value 在写前和恢复后共同验证 canonical identity/path、时间、prompt 与集合唯一性，损坏 catalog 不再经容量裁剪混入进程状态；Session transfer 的 immutable Document 统一拥有格式、内容与 64 MiB encoded limit，Publish 写 exact bytes，不再写出 Load 拒绝的 `limit+1` artifact；附件保持 dispatch-time 动态 path reference，但 image rich value 先闭合 MIME，文本完整读取后拒绝 invalid UTF-8/NUL，不再由 JSON marshal 静默替换字节；真实 DeepSeek shell 审批、执行、跨进程冷读，以及 CLI/Runtime 同进程强杀后恢复等待弹窗并续接原 Run 均已验证；继续按 consumer proof 审计 | `7875a20`、`8170949`、`4224eff`、`d538ff4`、`24b51b1`、`c4caae8`、`cb0f728`、`f881868`、`3b307a5`、`ca45962`、`663427c`、`3d7329f`、`b094727`、`bbabd59` |
| Q4 | 收敛 Runtime owner、抽象与 corner cases | 进行中 | endpoint provider 环境凭据、URL 校验与单消费者假边界已根治；Google chat 与 embedding 共用同一 custom endpoint truth；Runtime YAML 与 `hooks.json` 使用闭合输入词汇，Hook 拼错 matcher 不再静默扩大为全工具策略；idempotency replay 的 versioned outcome 与 exact typed response 共用闭合解码，value/problem 必须唯一，已执行命令的权威结果不再吞掉未知字段后降级重放；Conversation 完整读取拒绝 malformed durable row，不再让 SQLite count 与 Run/compaction message coordinate 因静默跳行分叉；Plan current 与 Run boundary 共用 closed-field/single-document 持久 decoder，不再把未知 step 字段删掉后继续注入模型；Artifact v27 内嵌 Scope message 以 canonical round-trip 证明 typed decode 不丢语义，RawMessage 不再让未知结构字段静默消失，开放 metadata 继续保真；JSON-RPC typed params 以字段缺席唯一表达 omission，显式 `null` 不再被 Go pointer 静默折叠为 preserve，开放 Tool arguments 仍保留 null；provider credential 来源在 Viper 解析后保持为 rich value，`FLAME_APIKEY` 与 provider-native env 都只进入 immutable overlay；只有 durable row 整体不存在时 YAML values 才可 first-run seed，显式 clear/update 经重启仍权威，所有聚合格式化固定脱敏；required-key provider 可先以 unconfigured 状态启动再通过 `providers.update` 建立 stored truth，stored-only 配置无需 YAML/env secret 即可重启；`providers.test` 由 Models Application 以十秒 deadline 有界结算，慢 endpoint 不再永久占住配置操作；endpoint model discovery 由 provider profile 按真实 OpenAI/Anthropic wire protocol 选择路径与鉴权，不再把 Anthropic-compatible 误探测为 Bearer `/models`；Anthropic listing 请求最大 1000 条单页并拒绝 `has_more`，不再把默认 20 条第一页冒充完整目录，也不为未出现的千级 catalog 建分页状态机；`models.list` 在 probe/fallback 前由 catalog 拒绝未知 provider 并映射 `invalid_params`，不再用成功空页制造第二份支持集合；endpoint probe 在网络前构造真实 chat adapter，Azure 等 provider-specific URL 约束不再出现 test 成功、Run 必败；child capability 只由 root row 持久化，恢复时再物化继承值；Delegate ToolResult 原样持久化模型可见的结构化/失败结果，避免后续上下文与 durable conversation 分叉；Host shutdown 在 producer 收束后有界 drain 已接纳维护，真实 DeepSeek one-shot 完成或等待 Question 时都能在退出前持久化 Session title，parked Run 不产生 workspace snapshot；相同二进制跨进程冷读仍保留 waiting Run、Question 与 checkpoint；合同生成器删除 6 个无消费者校验原语并拒绝未实现组合，Session Run observer 以非零类型表达 pointer key 唯一性，Runtime Staticcheck 重新全绿；本轮真实 DeepSeek Goal/Plan、tool-boundary steer、HITL restart、强杀长工具恢复及 13-turn 单次压缩均通过 | `c0cd320`、`18413c7`、`2a3c7de`、`2bdf819`、`5952684`、`af627aa`、`6338c15`、`2fcf4f5`、`d31297e`、`334e97b`、`2674af3`、`ea57fc4`、`9f02aca`、`319734a`、`c07e251`、`be545d6`、`db962c5`、`bb9637d`、`464ad68`、`20220b5`、`fc22266`、`92fa7f3`、`be3d192` |
| Q5 | 对齐 Grok Build 的 TUI 体验 | 进行中 | 已收敛 Goal/Plan/Run/composer 信息层级；真实 PTY 移除遗留旧品牌标记；slash completion 在 80×24、36×18 与 36×10 都只使用 composer 上方空间，窄屏截断候选而不污染输入区 | `f77bb98`、`0c45695`、`a788455`、`d57bf3d`、`f7f9c50` |

## 当前下一步

继续 Q3/Q4：按真实 consumer 与失败反例审计 CLI/Runtime owner、重复 truth 和 provider/application 边界；没有反例时不制造改动。Q5 只处理真实 TUI flow 暴露的信息层级或交互问题，不按文件大小、参考实现或像素差异机械重写。

## Goal 完成条件

本 Goal 只有在用户叫停，或以下事实同时成立时才可能完成：

- Runtime 与 CLI 的高置信坏味道已按真实 owner 修复，没有已知 forwarding layer、重复 truth 或 primitive sentinel
- package 图由真实职责解释，CLI 不再以微包模拟架构，Runtime 不以环目录模拟 DDD
- Scope 与其他 direct dependency 保持最新发布基线
- 真实 DeepSeek E2E 覆盖核心长期场景并稳定通过
- Protocol、storage、provider 与 TUI 的已知 corner case 已有 owner 和决定性回归证据
- 所有批次已经提交并推送，`desktop` 保持用户自己的并行修改
