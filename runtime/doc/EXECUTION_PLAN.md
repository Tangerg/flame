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
| Q0 | 建立根与模块规则，收敛文档 owner，删除 CLI 点时台账 | 已完成 | 链接检查；Runtime 文档/SQLite 事实门禁；CLI 架构门禁 | 本批提交 |
| Q1 | 升级 Scope 与全部 direct dependency | 待开始 | workspace + standalone test/vet/build/tidy | 待提交 |
| Q2 | 建立真实 DeepSeek E2E 场景基线 | 待开始 | Run/Tool/Goal/Plan/steer/HITL/compaction/long-context | 待提交 |
| Q3 | 收敛 CLI 过度分包与贫血模型 | 待开始 | consumer proof、architecture gates、真实 CLI flow | 待提交 |
| Q4 | 收敛 Runtime owner、抽象与 corner cases | 待开始 | Domain/Application/Protocol/SQLite + real E2E | 待提交 |
| Q5 | 对齐 Grok Build 的 TUI 体验 | 待开始 | render + PTY + live Runtime flow | 待提交 |

## 当前下一步

开始 Q1：查询并升级 Scope 与其他 direct dependency，收敛 indirect graph，运行 workspace 与 standalone 门禁，形成独立提交。Q1 完成前不开始大规模 package 或领域重构。

## Goal 完成条件

本 Goal 只有在用户叫停，或以下事实同时成立时才可能完成：

- Runtime 与 CLI 的高置信坏味道已按真实 owner 修复，没有已知 forwarding layer、重复 truth 或 primitive sentinel
- package 图由真实职责解释，CLI 不再以微包模拟架构，Runtime 不以环目录模拟 DDD
- Scope 与其他 direct dependency 保持最新发布基线
- 真实 DeepSeek E2E 覆盖核心长期场景并稳定通过
- Protocol、storage、provider 与 TUI 的已知 corner case 已有 owner 和决定性回归证据
- 所有批次已经提交并推送，`desktop` 保持用户自己的并行修改
