# P0 查询扩展总结与示例

## 结论摘要
- `domain_aliases.yaml` 由 `bcindex docgen` 初始化生成，用户手工维护同义词/中英别名。
- P0 查询扩展在检索侧完成，不改索引结构，不要求重建索引。
- **FTS（关键词）**：命中同义词组后，使用 OR 组装同义词，并在多组之间使用 AND 组合。
- **向量（embedding）**：低成本方案为“拼接一次”——将命中的同义词直接附加到原 query 后再做一次 embedding。
- **风险评估**：
  - 混合检索（FTS + 向量）：只选一个英文 alias 作为补召回，风险低，可接受。
  - 向量-only：可能漏召回（比如 alias 里有 `promotion` 但未选择），此风险暂缓处理。

## 具体示例

**词表**：
```yaml
synonyms:
  秒杀:
    - flash sale
    - seckill
    - promotion
  达人:
    - creator
    - influencer
    - koc
```

**用户 query**：  
`创建达人秒杀的业务逻辑是怎样的`

### FTS Query（AND of groups）
```
(达人 OR creator OR influencer OR koc)
AND
(秒杀 OR "flash sale" OR seckill OR promotion)
```

### 向量 Query（拼接一次）
```
创建达人秒杀的业务逻辑是怎样的 达人 creator influencer koc 秒杀 flash sale seckill promotion
```

### 预期效果
- `CreateCreatorPromotion`（“达人 + promotion”）优先命中
- `CreateSellerFlashsale`（“商家秒杀”）仍可能召回，但排序靠后
