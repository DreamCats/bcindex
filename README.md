# BCIndex

<div align="center">

**语义代码搜索工具 - 为 Go 项目设计的 AI 友好型代码索引**

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

</div>

BCIndex 是一个为 Go 项目设计的语义代码搜索工具，通过 AST 解析、向量检索和图分析，提供比传统关键词搜索更智能的代码查找体验。特别适合与 Claude Code、Cursor、Copilot Chat等 AI 编程助手配合使用。

## ✨ 特性

### 🔍 智能语义搜索
- **混合检索**: 结合向量相似度、关键词匹配和调用图分析
- **自然语言查询**: 用自然语言描述功能，找到相关代码
- **意图理解**: 自动识别查询意图（设计/实现/扩展点），调整结果排序

### 🧠 AI 友好设计
- **证据包生成**: 为 LLM 提供结构化、精简的上下文（<200行代码）
- **可解释推荐**: 每个结果都包含"为什么推荐"的理由
- **图关系提示**: 展示调用链、共同调用者、入口点等架构信息

### 📊 完整索引能力
- **语义单元索引**: package、interface、struct、func、method
- **调用图构建**: 自动分析函数调用关系
- **依赖关系**: 包导入、接口实现等
- **语义描述生成**: 自动生成包和符号的职责描述

### 🚀 高性能
- **增量索引**: 只重新索引变更的文件
- **批量嵌入**: 高效的向量生成
- **SQLite 存储**: 轻量级、无需额外数据库服务

## 🎯 适用场景

| 场景 | 传统方案 (rg/grep) | BCIndex |
|------|-------------------|---------|
| 找函数名 | ✅ 精确 | ✅ 精确 |
| 按功能找代码 | ❌ 需要知道关键词 | ✅ 自然语言查询 |
| 理解架构 | ❌ 需要手动追踪 | ✅ 调用图可视化 |
| 生成技术方案 | ❌ token 消耗大 | ✅ 证据包精简 |
| 扩展点定位 | ❌ 难以发现 | ✅ 图分析识别 |

## 📦 安装

### 从源码安装

**GitHub:**
```bash
# 克隆仓库
git clone https://github.com/DreamCats/bcindex.git
cd bcindex

# 编译
go build -o bcindex ./cmd/bcindex

# 安装到 PATH
sudo mv bcindex /usr/local/bin/
```

**GitLab (字节内部):**
```bash
# 克隆仓库
git clone git@code.byted.org:maifeng/bcindex.git
cd bcindex

# 编译
go build -o bcindex ./cmd/bcindex

# 安装到 PATH
sudo mv bcindex /usr/local/bin/
```

### 使用 go install

```bash
# 从 GitHub 安装
go install github.com/DreamCats/bcindex/cmd/bcindex@latest

# 从 GitLab 安装 (字节内部)
go install git@code.byted.org:maifeng/bcindex/cmd/bcindex@latest
```

## ⚙️ 配置

### 快速开始

1. 创建配置文件目录：
```bash
mkdir -p ~/.bcindex/config
```

2. 创建配置文件 `~/.bcindex/config/bcindex.yaml`:

```yaml
# 向量服务配置（必需）
embedding:
  provider: volcengine
  api_key: your-api-key
  endpoint: https://ark.cn-beijing.volces.com/api/v3
  model: doubao-embedding-vision-250615
  dimensions: 2048
  batch_size: 10

# 数据库配置（可选，不配置则使用默认路径）
# 默认按仓库生成独立数据库：
# ~/.bcindex/data/<repo-name>-<hash>.db
```

详细配置示例: [config.example.yaml](./config.example.yaml)

### 获取 API Key

**VolcEngine (火山引擎)**:
- 访问: https://console.volcark.com/
- 创建 API Key
- 支持的模型: `doubao-embedding-vision-250615` (2048维)

**OpenAI** (可选):
- 在配置文件中设置 `provider: openai`
- 配置 `openai_api_key` 和 `openai_model`

## 🚀 使用

### 工作流程

BCIndex 的使用方式非常简单 - 就在你的项目目录中使用：

```bash
# 1. 进入你的 Go 项目目录
cd /path/to/your/go/project

# 2. 构建索引（首次使用或代码更新后）
bcindex index

# 3. 搜索代码
bcindex search "your query"

# 4. 生成证据包（给 AI 用）
bcindex evidence "implementation details"
```

### 1. 索引你的代码

```bash
# 索引当前目录（最常用）
cd /path/to/your/go/project
bcindex index

# 从任意位置索引指定项目
bcindex index -repo /path/to/project

# 强制重建索引
bcindex index -force
```

**说明：**
- 默认在当前工作目录查找 Go 项目
- 自动检测 `go.mod` 文件
- 如果没有 `go.mod` 会警告但继续索引

索引过程会：
1. 使用 AST 解析所有 Go 文件
2. 提取符号（函数、类型、接口等）
3. 构建调用图和依赖关系
4. 生成语义描述
5. 创建向量嵌入

### 2. 搜索代码

```bash
# 自然语言搜索
bcindex search "处理订单状态的函数"

# 关键词搜索
bcindex search "UpdateOrder" -keyword-only

# 向量搜索
bcindex search "database connection" -vector-only

# 获取更多结果
bcindex search "error handling" -k 20

# JSON 输出（脚本集成）
bcindex search "cache" -json

# 详细输出（包含评分和理由）
bcindex search "order status" -v
```

### 3. 生成证据包 (AI 辅助)

证据包是为 LLM 优化的结构化上下文，包含：
- 包卡片（职责、角色、关键符号）
- 符号卡片（签名、位置、推荐理由）
- 代码片段（严格控制在 200 行以内）
- 图提示（调用链、入口点等）

```bash
# 生成证据包到标准输出
bcindex evidence "如何实现幂等性"

# 保存到文件
bcindex evidence "支付流程" -output payment_evidence.json

# 自定义证据包大小
bcindex evidence "database migration" \
  -max-packages 5 \
  -max-symbols 20 \
  -max-snippets 10 \
  -max-lines 500
```

### 4. MCP (stdio) 集成

在需要与支持 MCP 的客户端集成时，可启动 stdio server：

```bash
bcindex mcp
```

该模式提供三个工具：
- `bcindex_locate`：快速定位符号/文件/定义（适合“在哪里/是什么”）
- `bcindex_context`：上下文证据包（适合“怎么实现/调用链/模块关系”）
- `bcindex_refs`：引用/调用/依赖关系（适合“被谁引用/谁调用/外部依赖”）

客户端配置（stdio）：
- 在客户端的 MCP 设置中新增一个 stdio server，命令为 `bcindex`，参数为 `mcp`
- 注意全局参数必须放在子命令前面（如 `-repo`、`-config`）

示例（JSON 形式，具体字段以客户端为准）：
```json
{
  "name": "bcindex",
  "command": "bcindex",
  "args": ["mcp"]
}
```

固定仓库路径示例：
```json
{
  "name": "bcindex",
  "command": "bcindex",
  "args": ["-repo", "/path/to/your/repo", "mcp"]
}
```

示例输入（MCP tool arguments）：
```json
{
  "query": "如何生成证据包",
  "top_k": 10,
  "include_unexported": false
}
```

`bcindex_refs` 输入示例（按符号 ID）：
```json
{
  "symbol_id": "func:myapp/service/payment.ProcessPayment",
  "edge_type": "calls",
  "direction": "incoming",
  "top_k": 20
}
```

`bcindex_refs` 输入示例（按符号名 + 包过滤）：
```json
{
  "symbol_name": "ProcessPayment",
  "package_path": "myapp/service/payment",
  "direction": "both",
  "top_k": 20
}
```

`bcindex_refs` 输出示例：
```json
{
  "symbol_id": "func:myapp/service/payment.ProcessPayment",
  "direction": "incoming",
  "edge_type": "calls",
  "count": 2,
  "symbols": [
    {
      "id": "func:myapp/service/payment.ProcessPayment",
      "name": "ProcessPayment",
      "kind": "func",
      "package_path": "myapp/service/payment",
      "file_path": "service/payment/process.go",
      "line": 42,
      "signature": "func ProcessPayment(ctx context.Context, req *PayRequest) error"
    }
  ],
  "edges": [
    {
      "edge_type": "calls",
      "from": {
        "id": "method:myapp/handler.PaymentHandler.Handle",
        "name": "Handle",
        "kind": "method",
        "package_path": "myapp/handler",
        "file_path": "handler/payment.go",
        "line": 88
      },
      "to": {
        "id": "func:myapp/service/payment.ProcessPayment",
        "name": "ProcessPayment",
        "kind": "func",
        "package_path": "myapp/service/payment",
        "file_path": "service/payment/process.go",
        "line": 42
      }
    },
    {
      "edge_type": "calls",
      "from": {
        "id": "func:myapp/job.RetryPaymentJob",
        "name": "RetryPaymentJob",
        "kind": "func",
        "package_path": "myapp/job",
        "file_path": "job/retry_payment.go",
        "line": 25
      },
      "to": {
        "id": "func:myapp/service/payment.ProcessPayment",
        "name": "ProcessPayment",
        "kind": "func",
        "package_path": "myapp/service/payment",
        "file_path": "service/payment/process.go",
        "line": 42
      }
    }
  ]
}
```

**证据包输出示例**:
```json
{
  "query": "如何实现幂等性",
  "top_packages": [
    {
      "path": "myapp/service/payment",
      "role": "application/business",
      "summary": "支付服务 - 处理支付逻辑和幂等性",
      "why": [
        "包含 ProcessPayment 函数",
        "实现了幂等中间件"
      ],
      "key_symbols": ["ProcessPayment", "IdempotencyMiddleware"]
    }
  ],
  "top_symbols": [
    {
      "id": "sym_123",
      "name": "ProcessPayment",
      "kind": "func",
      "signature": "func (s *Service) ProcessPayment(ctx context.Context, req PaymentRequest) (*PaymentResponse, error)",
      "file": "service/payment.go:45",
      "why": [
        "匹配 '幂等性实现'",
        "使用了唯一键去重",
        "被 HTTP handler 调用"
      ]
    }
  ],
  "snippets": [
    {
      "file_path": "service/payment.go",
      "start_line": 45,
      "end_line": 89,
      "content": "...",
      "reason": "Symbol: ProcessPayment (func)"
    }
  ],
  "graph_hints": [
    "HTTP handler -> service.ProcessPayment -> repo.Save -> outbox.Publish",
    "Entry points: ProcessPayment"
  ],
  "metadata": {
    "total_symbols": 5,
    "total_packages": 2,
    "total_lines": 156,
    "generated_at": "2025-01-15T10:30:00Z"
  }
}
```

### 4. 查看统计信息

```bash
# 人类可读格式
bcindex stats

# JSON 格式
bcindex stats -json
```

输出示例：
```
📊 Index Statistics

Packages:        42
Symbols:         387
Edges:           1523
Embeddings:      387
```

### 5. 生成文档注释 (docgen)

使用 LLM 自动为缺少文档的 Go 代码生成符合 Go Doc 规范的注释。

```bash
# 预览模式 - 查看将要生成的文档（推荐先运行）
bcindex docgen --dry-run

# 显示差异
bcindex docgen --diff

# 限制生成数量
bcindex docgen --max 50 --max-per-file 10

# 只处理特定路径
bcindex docgen --include internal/service --include internal/handler

# 排除某些目录
bcindex docgen --exclude vendor --exclude testdata

# 实际生成文档
bcindex docgen

# 覆盖已有文档
bcindex docgen --overwrite
```

**说明**：
- 扫描范围为函数、方法、类型（struct/interface），不包括 const/var
- 生成的注释遵循 Go Doc 规范：
  - 首句以符号名开头
  - 一句话摘要 + 可选的关键约束/副作用/错误条件
  - 中文为主 + 英文技术术语
- 默认不会覆盖已有文档，需要 `--overwrite` 参数

**domain_aliases.yaml 配置**：

首次运行 `bcindex docgen` 时，会在仓库根目录自动生成 `domain_aliases.yaml` 模板文件：

```yaml
# BCIndex 领域词映射配置文件
# 用于定义业务领域内的同义词、中英对照、别名等

version: 1

synonyms:
  # 示例: 电商/促销相关
  # 秒杀:
  #   - flash sale
  #   - promotion
  #   - seckill

  # 请根据你的业务领域添加更多同义词组
```

**使用场景**：
- **中英对照**: 秒杀 -> flash sale, promotion, seckill
- **业务别名**: 达人 -> creator, influencer, koc
- **缩写展开**: ID -> identifier, user_id, uid

**说明**：
- 文件不存在时自动生成，已存在时跳过
- 使用 `--init-aliases` 可强制重新生成模板
- 该文件用于后续的查询扩展功能（P0 方案）

**可选配置**（默认已使用 `domain_aliases.yaml`）：
```yaml
search:
  synonyms_file: domain_aliases.yaml  # 相对 repo root
```

**配置**：
需要在配置文件中设置 `docgen.api_key`，也可以复用 `embedding.api_key`：

```yaml
# DocGen 配置（可选，不配置则使用 embedding.api_key）
docgen:
  provider: volcengine
  api_key: your-docgen-api-key  # 或使用 embedding.api_key
  endpoint: https://ark.cn-beijing.volces.com/api/v3/chat/completions
  model: doubao-1-5-pro-32k-250115
```

## 📖 命令参考

### 全局选项

| 选项 | 说明 |
|------|------|
| `-config <path>` | 指定配置文件路径 |
| `-repo <path>` | 覆盖配置中的仓库路径 |
| `-v, -version` | 显示版本信息 |
| `-h, -help` | 显示帮助信息 |

### bcindex index

构建代码索引。

**选项**:
- `-force`: 强制重建索引
- `-v`: 详细输出

**示例**:
```bash
bcindex index
bcindex -repo /path/to/repo index -v
```

### bcindex search

搜索代码。

**选项**:
- `-k <num>`: 返回结果数量 (默认: 10)
- `-vector-only`: 仅使用向量搜索
- `-keyword-only`: 仅使用关键词搜索
- `-json`: JSON 格式输出
- `-v`: 详细输出（评分和理由）

**示例**:
```bash
bcindex search "order validation"
bcindex search "CreateOrder" -keyword-only -k 20
bcindex search "error handling" -json
```

### bcindex evidence

生成 LLM 友好的证据包。

**选项**:
- `-output <path>`: 输出文件路径（默认: stdout）
- `-max-packages <num>`: 最大包数量 (默认: 3)
- `-max-symbols <num>`: 最大符号数量 (默认: 10)
- `-max-snippets <num>`: 最大代码片段数 (默认: 5)
- `-max-lines <num>`: 最大总行数 (默认: 200)

**示例**:
```bash
bcindex evidence "implement retry logic"
bcindex evidence "payment flow" -output evidence.json
bcindex evidence "cache" -max-symbols 20 -max-lines 500
```

### bcindex stats

显示索引统计信息。

**选项**:
- `-json`: JSON 格式输出

**示例**:
```bash
bcindex stats
bcindex stats -json
```

### bcindex docgen

自动生成 Go 代码的文档注释。

**选项**:
- `--dry-run`: 预览模式，不实际修改文件
- `--diff`: 显示差异
- `--overwrite`: 覆盖已有文档
- `--init-aliases`: 强制重新生成 domain_aliases.yaml
- `--max <num>`: 最大总符号数 (默认: 200)
- `--max-per-file <num>`: 每个文件最大符号数 (默认: 50)
- `--include <pattern>`: 包含路径（可多次指定）
- `--exclude <pattern>`: 排除路径（可多次指定）

**示例**:
```bash
bcindex docgen --dry-run
bcindex docgen --diff
bcindex docgen --max 100 --max-per-file 20
bcindex docgen --include internal/service --exclude vendor
bcindex docgen --init-aliases  # 重新生成 domain_aliases.yaml
```

## 🏗️ 架构

BCIndex 的设计参考了 [NEW_SOLUTION.md](./reference/NEW_SOLUTION.md) 中的最佳实践：

### 离线索引流程

```
Git Repo (Go Code)
       ↓
   Indexer
  ├─ AST 解析 (go/parser + go/types)
  ├─ 抽取语义单元 (symbols)
  ├─ 构建关系图 (edges)
  ├─ 生成语义描述 (semantic text)
  └─ 创建向量嵌入 (embeddings)
       ↓
   Storage
  ├─ SQLite (metadata)
  ├─ FTS5 (keywords)
  └─ Vector DB (embeddings)
```

### 在线查询流程

```
User Query
     ↓
RAG Orchestrator
  ├─ Query 解析/改写
  ├─ 混合检索
  │   ├─ 向量搜索 (semantic similarity)
  │   ├─ 关键词搜索 (BM25)
  │   └─ 图特征 (PageRank, layers)
  ├─ 结构化重排
  │   ├─ 意图识别 (design/implementation/extension)
  │   ├─ 层级排序 (handler → service → repo)
  │   └─ 中心性加权
  └─ 证据包组装
      ├─ 包卡片 (Package Cards)
      ├─ 符号卡片 (Symbol Cards)
      ├─ 代码片段 (Code Snippets)
      └─ 图提示 (Graph Hints)
     ↓
  Results / Evidence Pack
```

### 核心组件

| 组件 | 文件 | 功能 |
|------|------|------|
| **Indexer** | `internal/indexer/` | 索引流程编排 |
| **AST Pipeline** | `internal/ast/` | Go 代码解析 |
| **Embedding** | `internal/embedding/` | 向量生成 |
| **Hybrid Retriever** | `internal/retrieval/hybrid.go` | 混合检索 |
| **Graph Ranker** | `internal/retrieval/ranking.go` | 图排序 |
| **Evidence Builder** | `internal/retrieval/evidence.go` | 证据包生成 |
| **DocGen** | `internal/docgen/` | 文档生成 |
| **Store** | `internal/store/` | 数据持久化 |

## 🔧 开发

### 项目结构

```
bcindex/
├── cmd/
│   ├── bcindex/          # 主 CLI 工具
│   ├── extract/          # 旧版：符号提取工具
│   ├── relations/        # 旧版：关系提取工具
│   └── embed/            # 旧版：嵌入工具
├── internal/
│   ├── ast/              # AST 解析和符号抽取
│   ├── config/           # 配置管理
│   ├── docgen/           # 文档生成
│   ├── embedding/        # 向量嵌入服务
│   ├── indexer/          # 索引器
│   ├── mcpserver/        # MCP 服务器
│   ├── retrieval/        # 检索和排序
│   ├── semantic/         # 语义描述生成
│   └── store/            # 数据存储
├── reference/
│   ├── NEW_SOLUTION.md   # 架构设计文档
│   └── REFACTOR_PLAN.md  # 重构计划
├── config.example.yaml   # 配置示例
└── README.md
```

### 运行测试

```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test ./internal/retrieval/...

# 详细输出
go test -v ./internal/ast/...
```

### 代码质量

```bash
# 格式化代码
go fmt ./...

# 静态检查
go vet ./...

# 使用 golangci-lint
golangci-lint run
```

## 🤝 集成

### Claude Code

在 Claude Code 中使用 BCIndex 作为工具：

```json
{
  "tools": [
    {
      "name": "semantic_search",
      "description": "Search code using natural language",
      "command": "bcindex",
      "args": ["search", "{{query}}", "-json"]
    },
    {
      "name": "get_evidence",
      "description": "Get LLM-friendly evidence pack",
      "command": "bcindex",
      "args": ["evidence", "{{query}}", "-max-lines", "200"]
    }
  ]
}
```

### Cursor

添加到 Cursor 的 MCP 服务器：

```python
# cursor_mcp_server.py
import subprocess
import json

def semantic_search(query: str) -> dict:
    result = subprocess.run(
        ["bcindex", "search", query, "-json"],
        capture_output=True,
        text=True
    )
    return json.loads(result.stdout)

def get_evidence(query: str) -> dict:
    result = subprocess.run(
        ["bcindex", "evidence", query],
        capture_output=True,
        text=True
    )
    return json.loads(result.stdout)
```

## 📊 性能

### MCP 工具 Token 开销

BCIndex 提供 6 个 MCP 工具，每次 API 调用会携带工具定义（JSON Schema），预估 token 开销如下：

| Tool | Description | Fields | Total |
|------|-------------|--------|-------|
| `bcindex_locate` | 17 | 55 | 72 |
| `bcindex_context` | 154 | 143 | 297 |
| `bcindex_refs` | 90 | 71 | 161 |
| `bcindex_read` | 145 | 105 | 250 |
| `bcindex_status` | 74 | 15 | 89 |
| `bcindex_repos` | 79 | 0 | 79 |
| **Subtotal** | 559 | 389 | 948 |
| JSON Schema Overhead | - | - | 675 |
| **Total** | - | - | **~1,623** |

> **说明**: Token 估算使用 chars/4 近似方法，实际值可能有 ±20% 偏差。这是每次 API 调用的固定成本。

### 索引性能

| 项目规模 | 符号数 | 索引时间 | 数据库大小 |
|---------|--------|---------|-----------|
| 小型 (<1K 文件) | ~500 | ~30s | ~5MB |
| 中型 (1K-10K) | ~5K | ~2min | ~50MB |
| 大型 (>10K) | ~20K | ~5min | ~200MB |

### 查询性能

| 查询类型 | 平均延迟 |
|---------|---------|
| 关键词搜索 | <10ms |
| 向量搜索 | ~50ms |
| 混合检索 | ~100ms |
| 证据包生成 | ~200ms |

## 🐛 故障排查

### 配置文件找不到

**错误**:
```
Error: config file not found at: ~/.bcindex/config/bcindex.yaml
```

**解决**:
```bash
# 创建配置目录
mkdir -p ~/.bcindex/config

# 复制示例配置
cp config.example.yaml ~/.bcindex/config/bcindex.yaml

# 编辑配置
vim ~/.bcindex/config/bcindex.yaml
```

### API Key 无效

**错误**:
```
Failed to create embedding service: authentication failed
```

**解决**:
1. 检查 API Key 是否正确
2. 确认账户有足够的配额
3. 验证 endpoint URL

### 索引失败

**错误**:
```
Indexing failed: failed to parse package
```

**解决**:
1. 确保 Go 模块有 `go.mod` 文件
2. 检查代码是否有语法错误
3. 尝试使用 `-v` 选项查看详细日志

## 📝 路线图

- [ ] 支持更多编程语言 (TypeScript, Python, Rust)
- [ ] Web UI 界面
- [ ] 实时索引监控
- [ ] 分布式索引支持
- [ ] 更多嵌入模型支持
- [ ] VSCode 插件
- [ ] JetBrains 插件

## 📄 许可证

MIT License - 详见 [LICENSE](LICENSE) 文件

## 🙏 致谢

- 架构设计灵感来自 [NEW_SOLUTION.md](./reference/NEW_SOLUTION.md)
- 使用了 Go 官方的 `go/parser` 和 `go/types` 包
- 向量检索参考了现代 RAG 系统的最佳实践

## 📮 联系方式

- 作者: DreamCats
- GitHub: [@DreamCats](https://github.com/DreamCats)
- 问题反馈: [GitHub Issues](https://github.com/DreamCats/bcindex/issues)

---

**💡 提示**: 第一次使用前，请确保已经配置好向量服务的 API Key，并运行 `bcindex index` 构建索引。
