**补丁已生成：**

:chatgpt-content-reference{index="20"}[下载 Git patch](sandbox:/mnt/data/flame-audit-delivery/flame-audit-partial-f5621459.patch)

:chatgpt-content-reference{index="21"}[下载完整交付包：补丁、审计报告、验证记录与依赖验证脚本](sandbox:/mnt/data/flame-audit-delivery/flame-audit-delivery.zip) · :chatgpt-content-reference{index="22"}[单独查看审计报告](sandbox:/mnt/data/flame-audit-delivery/AUDIT.md)

**必须明确：本轮没有完成你要求的“全仓审计、全部重复能力清理、依赖升级及完整验证”的闭环。交付的是包含实际修复和回归用例的局部候选补丁，不是可以直接发布的完整升级包。** 仍有已经确认但未修复的问题；锁文件也没有重新生成，具体情况如下。

## 一、补丁实际修改了什么

补丁基于：

```text
Tangerg/flame
f5621459a9a55f70e06ea272bebaf994c62e9223
```

共 **9 个文件，新增 298 行、删除 73 行**。本轮通过 GitHub 插件读取源码，以 AGENTS.md、PROJECT_RULES.md 和 DEVELOPMENT.md 中的所有权、治本、依赖及验证要求作为评判依据。

### 1. 删除 Flame 自建的树提交身份与摘要算法，改用 Scope 的权威实现

原来的 `interaction_durability.go` 自己声明 `treeCommit`，拼接 activation/effect/checkpoint ID，并对本地提交封装重新计算摘要。Scope 0.40 已经为这些边界提供 `Identity()`、`ContentDigest()`，继续保留本地算法就是两个实现同时定义同一个执行提交事实。

补丁中已经：

- 删除本地 `treeCommit` 及对应的 ID、摘要计算。
- 直接保存 Scope 返回的身份和摘要，没有旧算法 fallback 或双轨兼容。
- 保留 Runtime 应当拥有的产品事务、持久化 head、writer fencing 和结果收据。

同时更新了 SQLite TreeCommitter conformance 测试，在**实际写入入口**核对存储值与 Scope 的权威返回值是否一致。**这部分完整集成测试尚未运行。**

### 2. 给树提交和提交结果确认读取设置独立期限

原来的 `publicationContext` 移除了调用者的取消和 deadline，只绑定 release。保护持久化不受观察者取消影响是合理的，但树提交也因此缺少独立存储期限。

补丁在原 lifecycle owner 上增加树提交专用上下文：保留上下文值、隔离观察者取消、响应 owner release，并设置 **15 秒独立期限**。写入失败后的确认读取获得新的有限期限，而不是继承已经失效的写入上下文。

这里修复的是上下文策略，**不是保证强制终止不合作的 I/O**。完整恢复路径仍需验证，尤其要确认上层不会把存储 deadline 错当成普通观察取消。

### 3. 修复已经取消的执行 owner 仍能暂时返回活动上下文的竞态

原来的 `bind` 只通过 `context.AfterFunc` 传播取消。当 owner 在绑定前就已经结束时，回调仍可能尚未执行，使返回上下文短暂表现为有效。

补丁在注册回调之后同步检查 owner 状态，保证**绑定前已经结束的 owner，不会给新调用提供一个看起来仍有效的上下文**。没有额外维护第二个 retired 标志。

### 4. 修复任务同步触发退役后，已启动 Promise 丢失错误观察者的问题

`RetirableTaskCohort` 原来的路径是：

```text
检查当前状态 → operation() → 再次检查当前状态 → 安装 Promise handlers
```

如果 `operation()` 在返回 Promise 前同步触发 cohort 退役，第二次检查会先抛错，已经启动的操作就可能没有 rejection 观察者。

补丁把两个责任分开：`run` 拒绝退役后的新操作；`settle` 必须接管已经启动的操作，再决定等待者是否还能接收结果。

**排序仍然由 Dougong 的 `SerialQueue` 拥有**，没有在 Flame 重写 Promise 队列。

## 二、三个依赖的处理状态

| 依赖 | 核实与改动 | 当前状态 |
|---|---|---|
| **Scope** | Runtime 的 26 个直接 requirement 从 `v0.38.0` 调整为 `v0.40.0`，并采用上述新 API | **源码迁移候选；模块图和校验和未完成** |
| **Dougong** | frontend manifest 从 `^0.7.0` 改为明确目标 `0.7.1` | **manifest 已改；npm 产物及 lock 未验证** |
| **Oolong** | CLI 的七个直接依赖已经是 `v0.21.0`，保持不动 | **未伪造版本升级；完整 CLI 能力去重审计未完成** |

上述原始依赖和标签内版本均读取了实际文件；Oolong 的 Release “latest”页面仍返回更旧版本，不能据此把现有依赖降级。

**`go.sum`、`go.work.sum`、`package-lock.json` 没有重新生成。** 因此不能把这部分称为依赖升级完成；应用补丁后直接运行 `npm ci`，可能因 manifest/lock 不一致失败。

## 三、已确认但没有修复的关键问题

**资源退役与资源释放的错误所有权仍不完整。** 已实际复现：`settleBeforeAbort` 的晚到资源 disposer 抛错，会形成未观察的 rejection；`disposeAsyncIterator` 的 `return()` 抛错，却仍然返回成功。这两个行为都来自 `asyncOwnership.ts`，本补丁没有修改它。

这不能靠补一个空 `catch` 或日志治本。需要迁移真实调用方，让退役 generation 结束等待，而独立 operation lifetime 继续拥有获取、晚到释放和错误报告。Dougong 0.7.1 的真实示例已经展示了 `operations.spawn` 与 `generation.cleanup` 的组合，但它不是一个新增的 facade API，不能假装直接导入就完成迁移。

**ToolCallRef 的统一迁移也未完成。** Scope 已提供权威逻辑引用，但 Flame 的工具身份、Delegate 关联、结果元数据和恢复路径仍消费本地推导。只替换一个入口会形成两套身份，所以本次没有提交这种不完整替换。

因此，**目前不能声称整个 Flame 已经做到 one fact, one owner。**

## 四、实际验证结果与应用方式

| 检查 | 实际结果 |
|---|---|
| 原始文件校验 | 8 个重建原始文件与 GitHub blob SHA 一致 |
| TypeScript 叶子实码隔离回归 | 原代码 **7/10**；修改后 **10/10** |
| Go 上下文方法隔离回归 | 原代码 **4/7**；修改后 **7/7** |
| Go 上下文隔离 race 检查 | `go test -race -count=10` 通过 |
| 补丁检查 | apply check、实际应用、反向检查通过；9 个文件逐字节一致 |
| 完整 Runtime／SQLite／CLI／前端／IDE／原生门禁 | **未运行** |

以上隔离测试**不是全应用测试**。执行容器无法联网安装依赖，本机 Go 为 1.23.2，而仓库要求 1.27.0；没有降低项目要求，也没有用上游或上一轮的测试结果冒充本轮验证。

在基线对应的独立工作分支中应用：

```bash
git apply --check /path/to/flame-audit-partial-f5621459.patch
git apply /path/to/flame-audit-partial-f5621459.patch
```

完整交付包内的脚本用于在有网络及符合要求的工具链环境中解析依赖、生成真实锁文件并运行门禁：

```bash
bash /path/to/complete-and-verify.sh /path/to/flame
```

**该脚本只通过了语法检查，未实际执行。** 它不会提交、推送或清理你的改动；即使全部门禁通过，也不会自动解决上述未修复项。

完整报告已经区分了修复、证据、未覆盖范围与剩余风险。**在依赖锁定、完整集成验证和开放问题得到处理之前，这份补丁应作为审查与继续修复的候选，而不是直接用于生产发布。**