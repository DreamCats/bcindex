# BCIndex 向量化与混合检索技术方案

> 目标：为 LLM 提供更高质量的语义搜索能力，并与现有文本/符号索引融合，实现“理解搜 + 精准搜”。

## 1. 为什么要做向量化

### 1.1 仅关键词/符号检索的局限
- 对自然语言提问不友好（用户不知道函数名/路径）。
- 同义词、缩写、业务术语不一致时召回不足。
- LLM 需要“语义相关”的上下文，而非单纯字符串匹配。

### 1.2 向量化的价值
- 支持“语义检索”：用自然语言也能命中代码/文档段落。
- 更适合 LLM：召回“相关实现”，便于生成回答。
- 对文档 + 代码统一召回，降低上下文缺失。

## 2. 为什么要“混合检索”，而不是只做向量

向量检索擅长“理解”，但对“精确定位”较弱。  
混合检索可以兼顾两者：

- **符号/路径命中**：用于精确定位与强约束。
- **向量召回**：用于语义召回，补齐“关键词不知道”的场景。
- **融合排序**：让精确结果排前，语义结果补全上下文。

解决的问题：
- 避免“向量误召回”。
- 避免“关键词漏召回”。
- 提升 LLM 端答案完整度与准确性。

## 3. 现有架构与向量化的融合点

当前架构已有：
- Go 符号索引（SQLite）
- 文本索引（Bleve）
- 分块规则（Markdown）

向量化新增模块即可接入：
1) **索引阶段**：为 chunk 生成 embedding，并写入向量库。  
2) **查询阶段**：将用户 query 向量化，检索向量库。  
3) **融合阶段**：与 text/symbol 结果合并排序。

## 4. 向量存储：Qdrant

选择理由：
- 本地/单机部署成本低，读写性能好。
- 支持 payload 过滤（repo_id / kind / path）。
- 便于未来扩展跨仓检索与混合排序。

### 4.1 Collection 组织策略

推荐默认：**单一 collection + repo_id 过滤**  
- 优点：支持跨仓检索，管理简单。  
- 缺点：payload 过滤必须可靠。  
- 适合：LLM 统一查询入口场景。

可选：每仓一个 collection  
- 优点：物理隔离，易排查。  
- 缺点：跨仓检索复杂。  
- 适合：强调隔离/权限的场景。

### 4.2 向量结构（payload）

```
id: <hash or uuid>
vector: [float]
payload:
  repo_id: string
  path: string
  kind: "go_func" | "md_section"
  name: string        # 函数名或标题
  title: string       # Markdown 标题路径
  line_start: int
  line_end: int
  hash: string        # chunk hash
  updated_at: int64
```

## 5. 向量生成：Volces Embeddings

使用接口（参考 `reference/volces_embeddings.md`）：

```
POST https://ark.cn-beijing.volces.com/api/v3/embeddings/multimodal
```

### 5.1 请求参数建议

- `input`: 仅使用 `type="text"`  
- `encoding_format`: `float`  
- `dimensions`: `1024`（默认推荐）

理由：
- 1024 维度在效果/成本/存储之间平衡更好。  
- 如果对召回要求更高，可切到 2048（成本更高）。  

### 5.2 为什么选择 `float`
- Qdrant 原生支持 float 向量。  
- 方便做 L2/Cosine 计算，不需解码。  

## 6. 分块策略（Chunking）

### 6.1 Go 代码
- 单元：**函数/方法级别**  
- 内容：`签名 + 注释 + 函数体片段`  
- 上限：约 1200-1800 字符（防止超过 embedding 限制）

好处：
- 语义完整（函数是最小语义单元）  
- 易定位（有符号名与行号）  

### 6.2 Markdown
- 单元：**标题层级块**  
- 超长段落按段落拆分，可加 50-100 字符重叠  

好处：
- 保留文档结构  
- 方便 LLM 拼接上下文  

## 7. 增量索引与删除策略

问题：增量更新会导致旧向量残留。  
解决策略：
- 在 SQLite 记录 `file -> vector_ids` 映射。
- 文件变更时先删除旧向量，再写入新向量。
- 删除/重命名文件时，先清理旧向量。

## 8. 混合检索策略

### 8.1 召回
- text: top 20  
- symbol: top 20  
- vector: top 30  

### 8.2 融合排序（默认）
```
final_score = 0.5 * vector_score
            + 0.3 * text_score
            + 0.2 * symbol_boost
```

策略好处：
- 确保符号/路径命中排前  
- 向量用于补充召回  
- 降低“语义误召回”影响  

## 9. 解决的问题与收益

- 解决 LLM “不知道关键词就搜不到”的问题  
- 减少人工定位成本与 token 消耗  
- 为后续 MCP/Skill 提供更强的语义检索能力  

## 10. 风险与规避

- **敏感代码外发**：提供 per-repo 开关，默认开启向量化，但允许显式关闭  
- **成本**：控制维度与 batch 大小（建议 batch=8/16）  
- **噪声**：混合检索 + 结构化排序减少误召回  

## 11. 性能与并发策略

索引阶段建议并发处理文件与向量化请求，避免串行导致速度瓶颈：
- **文件处理并发**：按 CPU 核心数启用 worker（如 `min(8, CPU)`）。
- **向量化并发**：按 API 限流配置并发与 batch，避免触发限流。
- **好处**：大仓库全量索引时间显著降低，同时不影响索引一致性。

## 12. 参数默认值建议

| 参数 | 默认值 | 原因 |
| --- | --- | --- |
| dimensions | 1024 | 成本/存储/效果均衡 |
| encoding_format | float | 与 Qdrant 兼容 |
| chunk_max_chars | 1500 | 防止过长输入 |
| overlap_chars | 80 | 保留上下文连续性 |
| vector_top_k | 30 | 语义补充召回 |
| index_workers | min(8, CPU) | 降低全量索引耗时 |

## 13. 下一步落地建议

1) 新增向量索引目录与 Qdrant 配置  
2) 实现 embedding 客户端（Volces）  
3) 扩展 index pipeline 生成向量  
4) 扩展 query pipeline 混合检索  

## 14. 分阶段实现计划

### Phase 1：基础向量链路（最小可用）
- 接入 Qdrant（本地/服务地址配置）  
- Volces embedding 客户端（仅 text）  
- Chunk 规则：Go 函数块 + Markdown 标题块  
- 向量写入与删除（file -> vector_ids 映射）  

### Phase 2：索引与更新
- 全量索引写向量库  
- `--diff`/`watch` 增量同步向量  
- 批量请求与并发控制  

### Phase 3：检索与融合
- `vector` 查询  
- `mixed` 融合排序（text/symbol/vector）  
- 输出结构化字段（score/source/line）  

### Phase 4：调优与保护
- chunk 参数可配置（长度/重叠）  
- per-repo 向量开关  
- 召回质量评估与调权  
