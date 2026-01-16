# BCIndex 查询模式（mode）设计草案

> 目的：借鉴 repotalk 的“工具分层”思路，定义 BCIndex 的 mode 输出契约，并约束 CLI 使用复杂度。

## 1. 设计目标

- 将查询从“命中列表”升级为“回答任务类型”。
- 默认命令保持简单：`bcindex query --q "..."`（auto 自动推荐 mode）。
- 输出可直接喂给 LLM，减少二次拼接。
- mode 行为清晰、可扩展，但不强制新增复杂参数。

## 2. 模式定义（对齐 repotalk 思路）

| Mode | 任务类型 | 类比 repotalk | 结果目标 |
| --- | --- | --- | --- |
| `auto` | 自动推荐 | - | 根据 query 自动选择 mode |
| `search` | 纯检索 | `searchNodes` | 命中列表 |
| `context` | 上下文组织 | `searchNodes` + 文档/节点详情 | 适合回答“是什么/怎么做” |
| `impact` | 影响范围 | `getNodesDetail(needRelatedCodes=true)` | 依赖/引用摘要 |
| `architecture` | 系统概览 | `getReposDetail` + 包关系 | 包结构与依赖统计 |
| `quality` | 索引质量 | 统计类 | 覆盖度/规模 |

## 3. 多步检索思维（内部流程，CLI 保持不变）

- 将 mode 视为“检索策略”，其本质是固定的多步流水线。
- 用户只需指定 `--mode`，步骤在内部完成，不新增复杂参数。
- 基础步骤统一为：召回（doc/符号/向量）→ 扩展（relations/doc_links）→ 重排去重 → 预算截断。

**各 mode 的默认流水线：**
- `context`：文档优先召回 → 补充 doc_links/relations → 重排/去重/截断。
- `impact`：锁定符号或文件 → 依赖扩散（imports/depends_on）→ 影响摘要。
- `architecture`：关系图统计 → 关键边抽取 → 结构化概览。
- `quality`：覆盖度统计 → 低覆盖点定位 → 改进提示。

## 4. CLI 形态（保持简单）

**默认：**
```
bcindex query --q "..."
```

**建议用法：**
```
bcindex query --q "这个项目是干什么的" --mode context
bcindex query --q "某函数改动影响" --mode impact
bcindex query --q "整体结构" --mode architecture
bcindex query --q "索引质量" --mode quality
```

**约束：**
- 不强制增加新参数。
- `--type` 仅在 `search` 下有意义；其他 mode 内部统一使用 mixed。
- `--json` 可输出结构化结果，保持与现有 CLI 一致。
- `auto` 使用规则推荐，用户显式 `--mode` 时覆盖自动选择。
- `search` 输出默认做紧凑化（减少 snippet 行数与关系噪声）。
- `auto` 优先按问句意图选择 `context`，仅在明确“影响/依赖/位置”时切换到 `impact/search`。
- `context` 遇到“实现/逻辑/源码”类问句时，会优先识别函数名并偏向代码片段。
- `context` 对常见命令词做轻量同义扩展（如 index→索引），提升中文文档命中率。
- 查询会对自然语言做轻量分词与变体检索，并融合重复命中结果。

## 5. 输出契约（摘要优先）

### 4.1 context
- 文档优先（README/文档段落），再补少量代码片段。
- 同文件去重，避免一屏都是 README 标题。

### 4.2 impact
- 输出依赖/引用摘要（当前以 imports/depends_on 为主）。
- 结果稳定、简短，适合 LLM 快速判断影响面。

### 4.3 architecture
- 输出包依赖统计与头部依赖关系（top edges）。
- 不依赖 `--q`，但保留 `--q` 入口保持命令一致。

### 4.4 quality
- 输出索引覆盖统计（symbols/relations/doc_links/text_docs）。
- 不依赖 `--q`，但保留 `--q` 入口保持命令一致。

## 6. 逐步落地计划

1) **确定输出契约**（本阶段文档）  
2) **增强 context**（文档段落优先 + 去重 + 章节权重）  
3) **完善 impact**（关系摘要 + 文件级关联）  
4) **完善 architecture/quality**（统计项与可读性）  

## 7. 控制复杂度的原则

- 仅保留一个“入口命令”：`bcindex query`
- mode 不超过 6 个（含 auto）
- 可选参数保持最少（`--mode` / `--json` / `--top`）
