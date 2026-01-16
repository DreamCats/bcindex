# BCIndex 快速参考

## 一分钟开始

```bash
# 1. 安装
go install github.com/DreamCats/bcindex/cmd/bcindex@latest

# 2. 配置
mkdir -p ~/.bcindex/config
cat > ~/.bcindex/config/bcindex.yaml << 'EOF'
embedding:
  provider: volcengine
  api_key: your-api-key
  endpoint: https://ark.cn-beijing.volces.com/api/v3
  model: doubao-embedding-vision-250615
  dimensions: 2048
  batch_size: 10
EOF

# 3. 使用
cd /path/to/your/go/project
bcindex index
bcindex search "处理订单状态"
```

## 常用命令

### 索引
```bash
bcindex index                    # 索引配置文件中的项目
bcindex -repo . index           # 索引当前目录
bcindex index -force            # 强制重建索引
```

### 搜索
```bash
bcindex search "query"              # 自然语言搜索
bcindex search "FunctionName" -keyword-only  # 关键词搜索
bcindex search "query" -k 20        # 获取更多结果
bcindex search "query" -v           # 详细输出
bcindex search "query" -json        # JSON 输出
```

### 证据包
```bash
bcindex evidence "query"                              # 标准证据包
bcindex evidence "query" -output evidence.json        # 保存到文件
bcindex evidence "query" -max-lines 500               # 增加代码行数
```

### 统计
```bash
bcindex stats                 # 查看索引统计
bcindex stats -json           # JSON 格式
```

## 查询技巧

### 1. 功能描述查询
```
"处理订单状态的函数"
"数据库连接池"
"错误处理中间件"
```

### 2. 架构层次查询
```
"HTTP handlers"
"service layer"
"repository implementations"
```

### 3. 技术特性查询
```
"idempotent operations"
"retry logic"
"circuit breaker"
"rate limiting"
```

### 4. 设计模式查询
```
"factory pattern"
"observer pattern"
"dependency injection"
```

## 证据包使用场景

### 给 Claude Code
```bash
# 在 Claude Code 中调用
bcindex evidence "如何实现幂等API" > context.json

# Claude 读取 context.json 并生成方案
```

### 给 Cursor
```json
{
  "name": "get_code_context",
  "command": "bcindex",
  "args": ["evidence", "{query}", "-max-lines", "200"]
}
```

### 给脚本
```bash
# 在 CI/CD 中使用
bcindex evidence "database migration" -output migration_context.json
```

## 配置文件位置

| 平台 | 配置路径 |
|------|---------|
| Linux/macOS | `~/.bcindex/config/bcindex.yaml` |
| Windows | `%USERPROFILE%\.bcindex\config\bcindex.yaml` |

## 数据库位置

| 平台 | 数据库路径 |
|------|-----------|
| Linux/macOS | `~/.bcindex/data/<repo-name>-<hash>.db` |
| Windows | `%USERPROFILE%\.bcindex\data\<repo-name>-<hash>.db` |

## 故障排查

### 问题: 配置文件找不到
```bash
mkdir -p ~/.bcindex/config
cp config.example.yaml ~/.bcindex/config/bcindex.yaml
vim ~/.bcindex/config/bcindex.yaml  # 编辑配置
```

### 问题: API 认证失败
1. 检查 API Key 是否正确
2. 确认 endpoint URL 正确
3. 验证账户配额

### 问题: 索引很慢
1. 检查网络连接
2. 调整 `batch_size` 参数
3. 考虑使用更快的模型

### 问题: 搜索结果不准
1. 尝试 `-vector-only` 或 `-keyword-only`
2. 使用 `-v` 查看评分详情
3. 调整搜索权重（需修改配置）

## 性能优化

### 减少索引时间
```yaml
embedding:
  batch_size: 50  # 增加批量大小
```

### 提高搜索精度
```yaml
search:
  vector_weight: 0.7    # 增加向量权重
  keyword_weight: 0.1
  graph_weight: 0.2
```

### 减少 token 使用
```bash
bcindex evidence "query" -max-lines 100  # 减少代码行数
```

## 更多信息

- 完整文档: [README.md](./README.md)
- 配置示例: [config.example.yaml](./config.example.yaml)
- 架构设计: [reference/NEW_SOLUTION.md](./reference/NEW_SOLUTION.md)
- 问题反馈: [GitHub Issues](https://github.com/DreamCats/bcindex/issues)
