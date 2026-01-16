# Live Pack 服务架构分析文档

## 一、技术栈概览

### 1.1 核心技术

- **编程语言**: Go 1.24
- **RPC 框架**: Kitex (字节跳动自研微服务框架)
- **IDL 定义**: Thrift
- **协议**: Thrift RPC

### 1.2 依赖库分类

#### 基础框架

- `code.byted.org/kite/kitex v1.19.4` - RPC 框架
- `code.byted.org/gopkg/tccclient v1.6.8` - 配置中心客户端
- `code.byted.org/kv/redis-v6 v1.1.5` - Redis 客户端
- `code.byted.org/oec/easecache_lib v0.1.18` - 缓存库

#### 业务 SDK

- `code.byted.org/ttec/cart_lib/v2` - 购物车 SDK
- `code.byted.org/ttec/trade_lib` - 交易 SDK
- `code.byted.org/ttec/multiverse_sdk v1.3.0` - 多宇宙 SDK
- `code.byted.org/ttec/delivery_sdk v0.0.74` - 配送 SDK
- `code.byted.org/oec/imagesdk v1.14.0` - 图片 SDK
- `code.byted.org/ttec/starling_client_proxy v1.1.8` - 国际化 SDK

#### 下游 RPC 客户端

- `code.byted.org/oec/rpcv2_ttec_live_pack` - Live Pack RPC
- `code.byted.org/oec/rpcv2_oec_product_*` - 商品相关 RPC
- `code.byted.org/oec/rpcv2_oec_promotion_*` - 营销相关 RPC
- `code.byted.org/oec/rpcv2_oec_trade_*` - 交易相关 RPC
- `code.byted.org/oec/rpcv2_ttec_content_*` - 内容相关 RPC

#### 工具库

- `code.byted.org/gopkg/logs/v2` - 日志库
- `code.byted.org/gopkg/metrics/v4` - 监控指标
- `code.byted.org/gopkg/jsonx` - JSON 处理
- `github.com/bytedance/sonic v1.14.1` - 高性能 JSON 库
- `github.com/shopspring/decimal v1.3.1` - 精确数值计算

---

## 二、架构特点

### 2.1 整体架构模式

**Pipeline + 策略模式 + 工厂模式**

采用自研的 Engine 框架，实现了一个五阶段的数据处理流水线：

1. **Providers** - 数据提供阶段（数据召回）
2. **Filters** - 数据过滤阶段
3. **Sorters** - 数据排序阶段
4. **Loaders** - 数据加载阶段（补充信息）
5. **Converters** - 数据转换阶段（DTO 构建）

### 2.2 核心设计理念

- **组件化**: 每个阶段由多个独立的 Task 组成
- **可配置**: 通过 TCC 配置中心动态控制各组件的开关和参数
- **并行执行**: 同阶段内的 Task 并行执行，不同阶段串行执行
- **依赖管理**: 支持强依赖和弱依赖，支持跨阶段依赖
- **超时控制**: Engine 级别和 Task 级别的超时控制
- **容错机制**: Panic 恢复、错误降级、空结果保护

### 2.3 执行模式

1. **内部阶段 DAG 模式** (默认): 阶段串行，阶段内任务并行
2. **跨阶段 DAG 模式**: 所有任务按依赖关系并行执行

---

## 三、目录结构与分层

### 3.1 目录组织

```
live_pack/
├── main.go                    # 服务入口
├── handler.go                 # RPC 服务实现
├── middleware.go              # 中间件（身份注入）
├── kx.yml                    # Kitex 配置
├── atum.yaml                 # 部署配置
│
├── handlers/                  # Handler 层（请求处理）
│   ├── get_live_bag_data_handler.go
│   ├── get_room_full_data_handler.go
│   └── ...
│
├── engines/                   # Engine 构建器
│   ├── live_bag_products.go
│   ├── pin_card_data.go
│   └── ...
│
├── engine_framework/          # 引擎框架核心
│   ├── component/            # 组件定义
│   │   ├── component.go     # Task 接口定义
│   │   ├── engine_conf.go   # 引擎配置
│   │   └── runtime_record.go # 运行时记录
│   ├── engine_model/         # 引擎模型
│   ├── schedule/            # 调度器
│   └── tool/               # 工具类
│
├── entities/                 # 实体处理层
│   ├── providers/           # 数据提供者（召回）
│   ├── filters/             # 过滤器
│   ├── sorters/             # 排序器
│   ├── loaders/             # 数据加载器
│   │   ├── product_loaders/
│   │   ├── room_loaders/
│   │   ├── promotion_display_info_loaders/
│   │   └── ...
│   ├── converters/           # 数据转换器
│   ├── dto_builders/        # DTO 构建器
│   └── req_params_builders/ # 请求参数构建器
│
├── dal/                     # 数据访问层
│   ├── rpc/                # RPC 调用
│   │   ├── ttec.content.product_model.go
│   │   ├── oec.product.product_detail.go
│   │   └── ...
│   ├── sdk/                # SDK 封装
│   │   ├── cart_lib.go
│   │   ├── trade_lib.go
│   │   ├── image_sdk/
│   │   ├── multi_region_sdk/
│   │   └── ...
│   ├── redis/              # Redis 操作
│   ├── ease_cache/         # 缓存操作
│   └── tcc/               # 配置中心
│
├── model/                   # 数据模型
│   ├── format/             # 格式化模型
│   └── rpc/                # RPC 模型
│
├── constdef/               # 常量定义
│   ├── errors.go           # 错误定义
│   ├── fcp.go              # FCP 常量
│   └── ...
│
├── utils/                  # 工具类
│   └── metric_v4/          # 指标工具
│
└── conf/                   # 配置文件
```

### 3.2 分层说明

#### Handler 层 (`handlers/`)

- **职责**: 接收 RPC 请求，构建 Engine，执行并返回响应
- **特点**:
  - 每个接口对应一个 Handler
  - 支持多 Engine 并行执行（如 GetLiveBagDataHandler）
  - 负责请求参数校验、响应格式化
  - 统一日志记录和错误处理

#### Engine 层 (`engines/`)

- **职责**: 构建具体的业务 Engine，配置各阶段的 Task
- **特点**:
  - 每个业务场景对应一个 Engine 构建函数
  - 声明式配置 Task 列表
  - 支持复用 Task

#### Entity 层 (`entities/`)

- **职责**: 实现具体的业务逻辑 Task
- **子目录**:
  - `providers/`: 数据召回（从 RPC、缓存等获取原始数据）
  - `filters/`: 业务规则过滤
  - `sorters/`: 排序逻辑
  - `loaders/`: 数据补充（补充商品详情、价格、促销等信息）
  - `converters/`: DTO 转换

#### DAL 层 (`dal/`)

- **职责**: 封装底层依赖
- **子目录**:
  - `rpc/`: 下游 RPC 调用封装
  - `sdk/`: 第三方 SDK 封装
  - `redis/`: Redis 操作
  - `ease_cache/`: 缓存操作
  - `tcc/`: 配置中心读取

---

## 四、调用链路

### 4.1 标准请求流程

```
┌─────────────────────────────────────────────────────────────────┐
│                         Client                               │
└────────────────────────┬──────────────────────────────────────┘
                         │ Thrift RPC
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Kitex Server                              │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │          Middleware (身份注入)                          │  │
│  └─────────────────────────────────────────────────────────┘  │
└────────────────────────┬──────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Handler Layer                             │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │  1. BuildReqParams (构建请求参数)                   │  │
│  │  2. BuildEngine (构建 Engine)                        │  │
│  │  3. Engine.Do (执行 Engine)                          │  │
│  │  4. Format Response (格式化响应)                     │  │
│  └─────────────────────────────────────────────────────────┘  │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Engine Framework                           │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │  Stage 1: Providers (数据召回)                      │  │
│  │    ├─ ProductListProvider                            │  │
│  │    └─ PopCardProvider                               │  │
│  ├─────────────────────────────────────────────────────────┤  │
│  │  Stage 2: Filters (数据过滤)                        │  │
│  │    └─ LiveBagProductFilter                          │  │
│  ├─────────────────────────────────────────────────────────┤  │
│  │  Stage 3: Sorters (数据排序)                        │  │
│  │    └─ TopProductsSorter                             │  │
│  ├─────────────────────────────────────────────────────────┤  │
│  │  Stage 4: Loaders (数据补充)                        │  │
│  │    ├─ ProductModelLoader                            │  │
│  │    ├─ ProductAtmosphereLoader                       │  │
│  │    ├─ PromotionDisplayInfoLoader                     │  │
│  │    └─ ... (多个 Loader 并行执行)                    │  │
│  ├─────────────────────────────────────────────────────────┤  │
│  │  Stage 5: Converters (数据转换)                     │  │
│  │    ├─ ProductConverter                              │  │
│  │    └─ LiveBagProductsConverter                      │  │
│  └─────────────────────────────────────────────────────────┘  │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                    DAL Layer                                 │
│  ┌──────────┬──────────┬──────────┬──────────┬──────────┐ │
│  │   RPC     │   SDK    │  Redis   │   TCC    │  Cache   │ │
│  └──────────┴──────────┴──────────┴──────────┴──────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

### 4.2 Engine 执行细节

#### 并行执行策略

- **阶段内并行**: 同一阶段的多个 Task 使用 `sync.WaitGroup` 并行执行
- **阶段间串行**: 前一阶段完成后才执行下一阶段
- **共享数据**: 使用 `ISharedTask` 接口实现跨 Engine 的数据共享（`Once.Do`）

#### 错误处理

- **强依赖错误**: 直接中断 Engine 执行
- **弱依赖错误**: 记录错误但继续执行
- **Panic 恢复**: 每个 Task 都有 recover 保护

#### 超时控制

- Engine 级别超时：默认 `MaxEngineTimeoutLimit`
- Task 级别超时：通过 TCC 配置
- 超时后设置 `Terminated` 标志，后续 Task 跳过执行

---

## 五、核心组件详解

### 5.1 Engine 框架核心接口

#### ITask 接口 (`engine_framework/component/component.go`)

```go
type ITask interface {
    GetName() string                              // Task 名称
    GetType() TaskTypeEnum                        // Task 类型
    NeedRun(*engine_model.RequestContext) (bool, string)  // 是否需要执行
    DoRun(*engine_model.RequestContext) (res *engine_model.InProcessResult, err error)  // 执行逻辑
    IsStrongDependency(*engine_model.RequestContext) bool  // 是否强依赖
    InitInProcessResult() *engine_model.InProcessResult  // 初始化结果
}
```

#### Task 类型枚举

```go
const (
    Providers  TaskTypeEnum = 1  // 数据提供
    Filters    TaskTypeEnum = 2  // 数据过滤
    Sorters    TaskTypeEnum = 3  // 数据排序
    Loaders    TaskTypeEnum = 4  // 数据加载
    Converters TaskTypeEnum = 5  // 数据转换
)
```

### 5.2 RequestContext

- 封装请求上下文
- 包含：`ctx`, `method`, `reqParams`, `reqAssemble`, `inProcessResults`
- 提供方法获取和设置中间结果

### 5.3 EngineConf

- 从 TCC 读取的动态配置
- 包含：超时配置、Task 开关配置、依赖关系配置

---

## 六、配置管理

### 6.1 TCC 配置中心

- **用途**: 动态配置管理
- **配置项**:
  - `LivePackEngineConf`: Engine 级别配置
  - `LiveBagEntryConfig`: 入口配置
  - `FlashSaleAllConfigs`: 闪购配置
  - `BuyButtonConfig`: 购买按钮配置
  - 等等...

### 6.2 配置读取示例

```go
// dal/tcc/tcc.go
livePackEngineConfGetter = LivePackTcc.NewGetter(
    KeyLivePackEngineConf,
    jsonx.Unmarshal,
    &LivePackEngineConf{}
)
```

### 6.3 配置应用

- Engine 启动时从 TCC 拉取配置
- 配置匹配规则：`Method + EngineName`
- Task 级别配置通过 `TaskConfs` 映射

---

## 七、监控与日志

### 7.1 日志

- **库**: `code.byted.org/gopkg/logs/v2`
- **日志级别**: Info, Error
- **日志内容**:
  - 请求/响应参数
  - Task 执行状态
  - Panic 堆栈
  - 超时信息

### 7.2 监控指标

- **库**: `code.byted.org/gopkg/metrics/v4`
- **指标类型**:
  - Engine 执行耗时
  - Task 执行耗时
  - 成功率/失败率
  - 超时次数

### 7.3 RuntimeRecord

- 记录 Engine 运行时信息
- 包含：各阶段耗时、Task 执行状态、错误信息
- 用于日志输出和问题排查

---

## 八、特殊特性

### 8.1 多区域支持

- **中间件**: `CalcurateAndInjectIdentity`
- **SDK**: `multi_region_sdk.CalcIdentity`
- **用途**: 根据用户区域路由到对应的后端服务

### 8.2 缓存策略

- **EaseCache**: 二级缓存（本地 + Redis）
- **缓存 Key**: 基于业务场景构建
- **缓存失效**: TCC 配置控制

### 8.3 实验分支

- **AB 测试**: `code.byted.org/oec/live_common/abtest`
- **配置控制**: 通过 TCC 配置开关
- **用途**: 灰度发布、功能实验

### 8.4 Mock 机制

- **配置**: `livePackDownstreamRpcMock`
- **用途**: 测试环境模拟下游 RPC 响应

---

## 九、接口列表

### 9.1 RPC 接口

| 接口名                    | Handler                                   | Engine                       | 说明                  |
| ------------------------- | ----------------------------------------- | ---------------------------- | --------------------- |
| MGetRoomLiteData          | mget_room_lite_data_handler.go            | -                            | 批量获取房间轻量数据  |
| GetRoomFullData           | get_room_full_data_handler.go             | -                            | 获取房间完整数据      |
| GetPinCardData            | get_pin_card_data_handler.go              | -                            | 获取 Pin 卡片数据     |
| GetLiveBagData            | get_live_bag_data_handler.go              | LiveBagProductsEngine        | 获取购物袋数据        |
| GetLiveBagPreviewData     | get_live_bag_preview_data_handler.go      | -                            | 获取购物袋预览数据    |
| GetLiveBagAssemble        | get_live_bag_assemble_handler.go          | -                            | 获取购物袋组装数据    |
| GetLiveBagRefresh         | get_live_bag_refresh_handler.go           | LiveBagProductsRefreshEngine | 刷新购物袋数据        |
| GetPreviewPinCardData     | get_preview_pin_card_data_handler.go      | -                            | 获取预览 Pin 卡片数据 |
| GetLiveBagTab             | get_live_bag_tab_handler.go               | -                            | 获取购物袋 Tab        |
| GetLiveBagTopBanner       | get_live_bag_top_banner_handler.go        | -                            | 获取顶部 Banner       |
| GetLiveBagPromotionBanner | get_live_bag_promotion_banner_handler.go  | -                            | 获取促销 Banner       |
| GetLiveBagCreatorShopInfo | get_live_bag_creator_shop_info_handler.go | -                            | 获取创作者店铺信息    |
| GetLiveRoomCommonInfo     | get_live_room_common_info_handler.go      | -                            | 获取直播间公共信息    |
| GetSurpriseSetDetail      | get_surprise_set_detail_handler.go        | -                            | 获取惊喜套装详情      |
| MGetFcpEcomComponent      | mget_fcp_ecom_component_handler.go        | -                            | 批量获取 FCP 电商组件 |

---

## 十、设计模式总结

### 10.1 使用的设计模式

1. **策略模式**: Task 接口定义统一，不同实现策略不同
2. **工厂模式**: Engine 和 Task 的构建函数
3. **模板方法模式**: Engine 的执行流程固定，Task 实现细节不同
4. **责任链模式**: 五阶段流水线处理
5. **单例模式**: Once.Do 实现共享数据
6. **建造者模式**: EngineConf.WithConf 链式配置
7. **适配器模式**: DAL 层适配不同的下游服务

### 10.2 架构优势

- **高扩展性**: 新增业务场景只需添加 Engine 和 Task
- **高可维护性**: 分层清晰，职责单一
- **高可配置性**: TCC 动态配置，无需重启
- **高性能**: 并行执行、缓存优化
- **高可靠性**: 容错机制、降级策略

### 10.3 注意事项

1. **Task 依赖**: 注意 Task 之间的依赖关系，避免死锁
2. **共享数据**: 使用 `ISharedTask` 时注意并发安全
3. **超时配置**: 合理设置超时时间，避免级联超时
4. **错误处理**: 区分强依赖和弱依赖，合理降级
5. **日志记录**: 关键路径记录详细日志，便于排查

---

## 十一、开发指南

### 11.1 新增接口流程

1. 在 IDL 中定义接口
2. 在 `handler.go` 中实现接口方法
3. 在 `handlers/` 中创建 Handler 文件
4. 在 `engines/` 中创建 Engine 构建函数
5. 在 `entities/` 中实现所需的 Task
6. 在 `dal/` 中添加必要的 RPC/SDK 调用

### 11.2 新增 Task 流程

1. 确定 Task 类型（Provider/Filter/Sorter/Loader/Converter）
2. 在对应目录创建文件
3. 实现 `ITask` 接口
4. 在 Engine 构建函数中注册 Task
5. 在 TCC 中添加配置（如需要）

### 11.3 调试技巧

1. 查看 PPE 环境日志（env.IsPPE()）
2. 检查 RuntimeRecord 输出
3. 使用 TCC 配置开关调试
4. 查看监控指标定位性能瓶颈

---

## 十二、依赖关系图

### 12.1 服务依赖

```
live_pack
├── cmp_ecom_global_relation (商品关系)
├── ttec_content_product_model (商品模型)
├── oec_product_product_detail (商品详情)
├── oec_product_product_price (商品价格)
├── ttec_promotion_renderer (营销渲染)
├── ttec_promotion_delivery (营销投放)
├── oec_trade_auction (拍卖)
├── oec_trade_cart (购物车)
├── ttec_trade_all_chain (交易链路)
├── ttec_content_live_creator (创作者)
├── ttec_live_data (直播数据)
└── ttec_seller_core (卖家核心)
```

### 12.2 基础设施依赖

```
live_pack
├── TCC (配置中心)
├── Redis (缓存)
├── EaseCache (二级缓存)
├── ImageSDK (图片服务)
└── Starling (国际化)
```

---

## 附录

### A. 关键文件索引

| 文件                                        | 说明               |
| ------------------------------------------- | ------------------ |
| main.go                                     | 服务入口           |
| handler.go                                  | RPC 服务实现       |
| middleware.go                               | 中间件             |
| engine_framework/schedule/engine.go         | Engine 核心逻辑    |
| engine_framework/component/component.go     | Task 接口定义      |
| dal/tcc/tcc.go                              | TCC 配置读取       |
| handlers/get_live_bag_data_handler.go       | 典型 Handler 示例  |
| engines/live_bag_products.go                | 典型 Engine 示例   |
| entities/providers/product_list_provider.go | 典型 Provider 示例 |

### B. 常见错误码

| 错误码                 | 说明            |
| ---------------------- | --------------- |
| EngineErrFmt_Timeout   | Engine 执行超时 |
| TaskErrFmt_Panic       | Task 执行 Panic |
| BizErrFmt_LiveClose    | 直播间已关闭    |
| BizErrFmt_EcLiveBanned | 电商权限被封禁  |

### C. 性能优化建议

1. 合理使用缓存（EaseCache）
2. 批量 RPC 调用减少网络开销
3. 并行执行充分利用 CPU
4. 避免不必要的 Task 执行（NeedRun 判断）
5. 监控慢 Task 并优化

---

**文档版本**: v1.0
**生成时间**: 2026-01-15
**分析工具**: Claude Code
