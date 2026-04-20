# LeiAgent 功能说明（项目总览）

> 本文档汇总桌面端 **LeiAgent** 当前主要能力，含界面、Agent、LLM、文库、工具、**本地记忆 / 用户画像** 与**备忘录**专章，便于产品/开发对照与维护。  
> **最后更新**：2026-04-20

---

## 1. 项目概览

- **形态**：基于 **Wails v2** 的桌面应用（Go 后端 + Web 前端），本地运行。
- **布局**：顶栏全局操作；左侧**会话列表 + 日历**；中间**对话**；右侧可选**推理/思考**面板（可关闭）；多个模态层（设置、备忘录、文库等）。
- **分栏**：左/右栏宽度可拖拽调整；关闭「思考」时右侧推理槽隐藏。

---

## 2. 顶栏（Header）

- **关闭思考**：勾选后隐藏右侧推理面板，且对 LLM 关闭思考/推理类参数（与 `localStorage` 同步并调用后端 `SetLLMThinkingDisabled`）。
- **设置**：打开 **LLM 配置（YAML）** 编辑器模态框。
- **文库**：打开文档库模态框（见 §7）。
- **本地记忆**：打开当前会话的 `localMemory` 调试视图。
- **画像**：打开当前会话的结构化用户画像面板，可手动刷新生成。
- **备忘录**：打开备忘录窗口（见 §10、§11）。
- **连接状态**：展示 `GetLLMConnectionStatus` 探测结果（如已连接 / 未配置 / 不可用）；支持手动刷新；启动后定时轮询。

---

## 3. 会话列表与日历（ConversationList）

- **列表**：拉取 `ListConversation`，切换 `SwitchChat`，重命名 `UpdateConversationTitle`，删除 `DeleteConversation`（成功后可整页刷新）。
- **日历**：按日期筛选会话（`created_at` / `updated_at` 与选中 YYYY-MM-DD 对齐）。
- **备忘标记**：`GetMemoCalendarDates` 提供的日期在日历上标识「当日备忘中有日期串」类提示（与备忘录正文中的日期解析一致）。
- **切换会话**：派发 `conversationChanged`（含 `conversationId`、`title` 等），中间对话区与右侧推理等联动。

---

## 4. 对话区（Dialog）

- **消息**：展示用户 / 助手 / 流式更新；监听 Wails 事件如 `dialogAppend`、`dialogStreamEnd`、`GetMessagesByMessageID` 等。
- **发送**：`SendMessage`；空内容不发。
- **停止**：`StopChat` 中断当前任务；后端在保留 Dispatcher 前提下重启监听（意图等内存态保留）。
- **便签页（Sheets）**：同一会话内多「便签」分段浏览消息（主对话 + 用户新开主题）；新主题发送前可归类为 `newTopic` 等（`classifyUserMessage`）。
- **控制类输入**：如暂停等可走 `SendUserDisplayOnly`，仅展示不入 Agent，并 `StopChat`。
- **生成备忘**：见 §11。

---

## 5. Agent 与运行模式（Dispatcher）

- **按会话调度**：每 `chatID` 对应输入/输出/推理通道；用户消息触发 `dispatcher`，进入 **意图识别**（`ConfirmIntention`），再按模式分支。
- **模式**（与配置/提示词一致前提下）：
  - **CHAT**：常规对话，`proxy.Communicate` + 记忆。
  - **PLAN**：任务规划，`planner.GeneratePlan` 与执行。
  - **TOOL**：走 `agent.HandleChat`，工具调用循环。
  - **切换/重置意图**：特定意图下清空 `Intention` 并提示用户重新输入。
- **意图刷新**：部分规则下对新一轮用户话语文本触发重新分类（`ShouldReclassifyIntent`）。
- **并发池**：`agentPool` 有上限（满则淘汰最旧 Dispatcher）；停止对话会 `Shutdown` 再以新 context 恢复监听。
- **流式输出**：助手与推理内容经事件推送到前端；结束符写入 DB，支持仅展示不落库的 ephemeral 结束标记。

---

## 6. 推理侧栏（Reasoning）

- 展示当前会话 `GetReasoningMessage` 返回的推理类消息；依赖顶栏「关闭思考」是否显示整块 UI。

---

## 7. 文库（DocLibraryModal）

- **登记列表**：`ListDocumentLibrary` — 工具写入与历史消息中出现过的现存文件路径等。
- **Workspace**：浏览 `workspace/` 下目录（`ListLibraryWorkspaceDir`），支持查看/编辑文本、新建目录、写文件、删除、重命名；根路径由 `GetLibraryWorkspaceRoot` 提供。
- **阅读与系统打开**：`ReadDocumentForViewer`（有大小上限）、`RevealDocumentInExplorer`。
- **深度链接**：应用内可派发 `leiagent-open-document`（`CustomEvent`，`detail.path`）打开文库并聚焦路径。

---

## 8. 设置（SettingsModal）

- **模型与连接**：加载 `GetLLMConfigEditorState`（YAML 全文、保存路径、是否示例）；保存 `SaveLLMConfigText`（校验后写入）。
- 保存成功后可触发顶栏连接状态刷新（`onSaved` → `refreshConnection`）。

---

## 9. 内置工具（Dispatcher 注册清单）

以下在 `dispatcher.toolsInfo()` 中注册并参与规划/工具模式（名称以代码为准）：

| 能力方向 | 代表工具 |
|----------|----------|
| Shell | `bash`（bashfunction） |
| 文件 | 分块写入、整文件写入（fileFunction） |
| 文库路径 | libraryfs |
| 备忘录 | memo_write（memotool） |
| 小说/长文 | noveltool |
| 时间 | 当前时间、日期信息、时间计算（timeFunctions） |
| 搜索/数据 | 天气、地理编码、行情（searchFunctions） |

另有 SerpAPI、百度搜索等实现于 `internal/tools/searchFuctions`，是否接入以 Agent/注册处为准。

---

## 10. 数据与存储（概要）

- **会话与消息**：`dataoperation` + SQLite（对话、消息、推理等，详见包内实现）。
- **Agent 记忆**：`internal/memory` 等与 `chatID` 绑定的上下文。
- **结构化画像**：`profiles/{chatID}.json`，保存 identity/preferences/personality/behavior/state/memory/predictions。
- **备忘录主存**：`data/memo_notes.db`（§11 详述）；旧 `data/memo.md` 可导入。
- **工作区文件**：`workspace/` 目录，与文库、工具写文件联动。
- **日志**：`logging` / `logs/`（如 `default.log`）。

---

## 11. 备忘录数据与后端 API

- **主存储**：SQLite（`data/memo_notes.db`），与 `memo_write` 等工具共用。
- **迁移**：旧版单文件 `data/memo.md` 可在首次需要时自动导入（以 `internal/memo` 为准）。
- **结构**：一级 ATX 标题 `# 标题` 分条；正文末尾可用 `<!--leiAgent-memo-src:id1,id2-->` 记录来源对话消息 ID。
- **Wails 能力**：读取/全量保存、路径查询、追加 Markdown、已引用 messageID 列表、LLM 整理备忘、日历日期提取等（见 `app.go` 与 `wailsjs` 绑定）。

---

## 12. 本地记忆与用户画像

- **本地记忆窗口（LocalMemoryModal）**：查看当前 `chatID` 的 `localMemory` 消息序列，便于调试 LLM 实际上下文。
- **画像窗口（UserProfileModal）**：展示结构化 `identity / preferences / personality / behavior / state / memory / predictions`。
- **画像生成**：`RefreshUserProfile(chatID)` 基于会话历史与 LLM 推断刷新 `profiles/{chatID}.json`。
- **运行时注入**：Dispatcher 在处理新用户消息时，会把用户画像摘要作为额外 system directive 注入请求上下文，用作软性个性化，不覆盖当前显式需求。
- **安全边界**：画像被当作“推测性记忆”使用；若和当前用户输入冲突，以当前输入为准。

---

## 13. 备忘录窗口（MemoModal）

- **预览与列表**：摘要与 Markdown 预览**去掉**末尾 `<!--leiAgent-memo-src:...-->`；编辑区保留完整切片。
- **窗口**：拖拽调整大小、最大化/还原。
- **页脚**：说明 SQLite 主存与旧 `memo.md` 导入。
- **写入后联动**：收到 `memoSaved` 且 `detail.focusLatest` 时自动打开并尽量选中**最新一条**；已打开时重新加载并同样选中最新条。

---

## 14. 对话区「生成备忘」

- **流程**：展开后在消息旁勾选（可多选）→ 直接写入或模型优化。
- **标题**：优先勾选中的**用户**消息首行，否则助手，再否则第一条。
- **重复引用**：已出现在备忘中的 `messageID` 再次勾选时 **confirm**。
- **直接写入**：`AppendMemoMarkdown`。
- **模型优化**：提示词 + `ComposeMemoWithLLM` 后再追加。
- **界面**：未勾选不显示提示词与两按钮；点消息列表空白收起；展开后主按钮为「取消生成」；写入中禁用；勾选框与用户/助手头像侧对齐。
- **快捷提示词**：内置若干 + 自定义（`+` 添加、自定义项可删）；自定义存 **`localStorage`**，清站点数据或换设备会丢。
- **完成写入**：派发 `CustomEvent('memoSaved', { detail: { focusLatest: true } })`。

---

## 15. 前端全局事件约定

| 事件 | 说明 |
|------|------|
| `memoSaved` | 备忘追加成功；建议 `detail.focusLatest` 控制是否打开备忘录并聚焦最新条。 |
| `conversationChanged` | 切换当前会话（`detail.conversationId`、`title` 等）。 |
| `leiagent-open-document` | `detail.path` 打开文库并聚焦文件。 |

Wails 运行时事件（如 `dialogAppend`、`reasoningAppend`、`dialogStreamEnd` 等）由后端 `EventsEmit`，前端 `EventsOn` 订阅，细节以代码为准。

---

## 16. 维护建议

- 行为、API 或工具注册有变时，同步更新**对应小节**与文首 **最后更新**日期。
- 新增模态入口、存储路径或自定义事件时，建议在本文件 **§14** 与相关功能节中写明，避免仅依赖代码检索。
- 根目录 `README.md` 若与本文不一致，以**仓库实现**为准时可优先更新本文或 README 中指向本文的链接。
