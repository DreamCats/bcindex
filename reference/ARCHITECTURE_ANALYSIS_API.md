# Live ShopAPI 架构分析文档

> **项目名称**: live_shopapi (TikTok 直播电商后端服务)
> **生成时间**: 2026-01-15
> **分析目的**: 为本地仓库索引架构提供技术基础

---

## 目录

1. [项目概述](#项目概述)
2. [技术栈](#技术栈)
3. [项目结构](#项目结构)
4. [分层架构](#分层架构)
5. [调用链路](#调用链路)
6. [核心设计模式](#核心设计模式)
7. [中间件体系](#中间件体系)
8. [依赖管理](#依赖管理)
9. [代码生成机制](#代码生成机制)
10. [配置管理](#配置管理)
11. [缓存策略](#缓存策略)
12. [测试体系](#测试体系)
13. [架构特点总结](#架构特点总结)
14. [注意事项](#注意事项)

---

## 项目概述

**live_shopapi** 是字节跳动 TikTok 直播电商的核心后端服务，负责处理直播间商品相关的 HTTP 请求，包括商品展示、购物车、优惠券、拍卖等功能。

### 核心职责

- 直播间商品列表查询与展示
- 商品添加/删除/置顶管理
- 购物车状态同步
- 优惠券领取与展示
- 拍卖功能支持
- 商品搜索与筛选

### 技术定位

- **服务类型**: HTTP API 服务
- **部署方式**: 微服务架构
- **通信协议**: HTTP/HTTPS + RPC (Kitex)
- **数据存储**: Redis (缓存) + 远程 RPC 服务

---

## 技术栈

### 核心框架

| 技术       | 版本    | 用途                     |
| ---------- | ------- | ------------------------ |
| **Go**     | 1.22.0  | 编程语言                 |
| **Hertz**  | v0.9.7  | HTTP 框架 (字节跳动开源) |
| **Kitex**  | v1.18.4 | RPC 框架 (字节跳动开源)  |
| **Thrift** | v1.14.2 | RPC 序列化协议           |

### 数据存储

| 技术        | 版本                | 用途          |
| ----------- | ------------------- | ------------- |
| **goredis** | v5.7.7+incompatible | Redis 客户端  |
| **gcache**  | v0.0.2              | 本地 LRU 缓存 |

### 序列化

| 技术         | 版本    | 用途               |
| ------------ | ------- | ------------------ |
| **sonic**    | v1.14.0 | 高性能 JSON 序列化 |
| **protobuf** | v1.36.5 | Protobuf 序列化    |

### 配置管理

| 技术      | 版本    | 用途                            |
| --------- | ------- | ------------------------------- |
| **TCC**   | v1.6.8  | TikTok Config Center (配置中心) |
| **viper** | v1.14.0 | 本地配置文件管理                |

### 日志与监控

| 技术           | 版本    | 用途           |
| -------------- | ------- | -------------- |
| **logs/v2**    | v2.2.2  | 结构化日志     |
| **metrics/v4** | v4.1.5  | 指标监控       |
| **bytedtrace** | v1.0.22 | 分布式链路追踪 |

### 限流与熔断

| 技术                  | 版本    | 用途       |
| --------------------- | ------- | ---------- |
| **arch_limiter**      | v1.13.6 | 架构级限流 |
| **simple_rate_limit** | v2.0.16 | 简单限流器 |

### 测试框架

| 技术         | 版本   | 用途         |
| ------------ | ------ | ------------ |
| **testify**  | v1.9.0 | 断言库       |
| **mockey**   | v1.3.0 | Mock 框架    |
| **goconvey** | -      | BDD 风格测试 |

---

## 项目结构

### 顶层目录结构

```
live_shopapi/
├── main.go                    # 应用入口
├── router.go                  # 自定义路由中间件配置
├── router_gen.go              # 代码生成的路由注册
├── go.mod / go.sum            # Go 依赖管理
├── Makefile                   # 构建脚本
├── atum.yaml                  # CI/CD 配置
├── conf/                      # 配置文件目录
│   ├── config.yaml
│   └── oec_live_shopapi.yaml
├── biz/                       # 业务逻辑层
│   ├── handler/               # HTTP 请求处理器
│   ├── service/               # 业务服务层
│   ├── middleware/            # 中间件
│   ├── dal/                  # 数据访问层
│   ├── config/               # 配置定义
│   ├── constant/              # 常量定义
│   ├── tcc/                  # TCC 配置客户端
│   ├── tools/                 # 工具函数
│   ├── model/                 # 数据模型
│   ├── types/                 # 类型定义
│   ├── helper/                # 辅助函数
│   ├── oec_common/           # OEC 通用组件
│   ├── event/                # 事件处理
│   ├── ttec_cache/           # TTEC 缓存
│   └── debug/                # 调试接口
├── repositories/              # 依赖注入容器
│   ├── container.go           # DI 容器定义
│   ├── sdk/                 # SDK 封装
│   ├── lib/                 # 通用库封装
│   └── tcc/                 # TCC 客户端
└── script/                   # 脚本目录
```

### 目录职责详解

#### `biz/handler/` - HTTP 处理器层

**职责**: 处理 HTTP 请求，参数解析，调用 Service 层

**特点**:

- 每个接口对应一个 Handler 文件
- 使用 Hertz 框架的 `RequestContext`
- 通过 `@router` 注解定义路由（代码生成）

**示例文件**:

- `get_live_bag.go` - 获取购物车
- `add_product.go` - 添加商品
- `del_product.go` - 删除商品
- `search_live_bag_products.go` - 搜索商品

**代码示例**:

```go
// @router /aweme/v1/oec/live/bag [GET]
func GetLiveBag(ctx context.Context, c *app.RequestContext) {
    GetLiveProducts(ctx, c)
}
```

#### `biz/service/` - 业务逻辑层

**职责**: 实现核心业务逻辑，调用 RPC 服务，数据组装

**特点**:

- 包含业务规则实现
- RPC 客户端调用
- 数据转换与组装
- 缓存策略实现

**示例**:

- `live_products.go` - 商品相关业务逻辑
- `live_room.go` - 直播间相关业务
- `toggle/manager.go` - 特性开关管理

**代码示例**:

```go
func GetLiveProducts(requestContext *types.RequestContext, authorId int64, ...) (resp *shop.GetLiveProductsResponse, status *Status.Status) {
    req := &shop.GetLiveProductsRequest{
        AuthorId: authorId,
        AppId: oec_base.AppId(appId),
        // ...
    }
    resp, err := oec_live_shop.RawCall.GetLiveProducts(requestContext.Context, req)
    // ...
}
```

#### `biz/middleware/` - 中间件层

**职责**: 请求拦截、预处理、后处理

**主要中间件**:

- `traffic_tag.go` - 流量标签处理
- `arch_limiter.go` - 限流中间件
- `error_code.go` - 错误码处理
- `high_priority_traffic.go` - 高优先级流量处理

#### `biz/dal/` - 数据访问层

**职责**: 数据库/缓存访问

**实现**:

- `redis.go` - Redis 客户端初始化
- 使用字节跳动内部 `goredis` 客户端
- 支持服务发现 (Consul)

**代码示例**:

```go
func initLiveRedisClient() {
    opt := goredis.NewOptionWithTimeout(50*time.Millisecond, ...)
    opt.SetServiceDiscoveryWithConsul()
    LiveRedisClient, err = goredis.NewClientWithOption(constant.LiveRedisPsm, opt)
}
```

#### `biz/model/` - 数据模型层

**职责**: 数据模型定义和转换

**子目录**:

- `live_serv/` - Protobuf 生成的模型
- `converter/` - Goverter 生成的转换器
- `client/` - RPC 客户端封装
- `cache/` - 缓存模型

#### `biz/config/` - 配置定义

**职责**: 业务配置常量和 AB 测试命名空间

**内容**:

- AB 测试命名空间定义
- 功能开关配置
- 业务常量

#### `biz/constant/` - 常量定义

**职责**: 全局常量定义

**内容**:

- 缓存键
- 日志键
- 限流键
- PSM 服务名

#### `biz/tools/` - 工具函数

**职责**: 通用工具函数

**功能**:

- `format.go` - 格式化工具
- `filter.go` - 过滤工具
- `convert.go` - 转换工具

#### `repositories/` - 依赖注入容器

**职责**: 管理全局依赖，实现依赖注入

**设计模式**: 手动实现的容器模式

**结构**:

```
repositories/
├── container.go          # 容器接口和实现
├── sdk/                 # SDK 封装
│   ├── metrics4.go      # 指标 SDK
│   └── ...
├── lib/                 # 通用库封装
│   ├── json.go          # JSON 库
│   └── limiter.go       # 限流库
└── tcc/                 # TCC 客户端封装
    ├── ttec_live_conf.go
    └── oec_live_shop_api.go
```

**代码示例**:

```go
type IContainer interface {
    GetJSON() lib.IJSON
    GetMetricsClient() sdk.IMetrics4
    GetTTecLiveConfTCC() tcc.ITTecLiveConf
    GetOecLiveShopAPITCC() tcc.IOecLiveShopAPI
    GetLimiter() lib.IGecLimiterLib
}

func InitContainer() {
    once.Do(func() {
        json := lib.NewJSON()
        mtr := sdk.NewMetrics4()
        ttecLiveConfTCC := tcc.NewTTecLiveConf(json)
        // ...
        c = &Container{...}
    })
}
```

---

## 分层架构

### 整体架构图

```
┌─────────────────────────────────────────────────────────────────┐
│                    HTTP Client (Mobile/Web)                   │
└────────────────────────────┬────────────────────────────────────┘
                             │ HTTP Request
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│              HTTP Layer (Hertz Framework)                      │
│                  router_gen.go (代码生成)                      │
│                  router.go (中间件配置)                        │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│              Middleware Chain (biz/middleware/)                │
│  1. HertzInterfaceLimitMw         - 接口限流                   │
│  2. LatencyRecorderHertz          - 延迟记录                   │
│  3. UnzipBodyHertz                - 请求解压                   │
│  4. Gzip                          - 响应压缩                   │
│  5. HertzSessionProcessor          - Session 处理               │
│  6. OdinProcessHertz               - Odin 认证                 │
│  7. TrafficTagProcessor            - 流量标签处理               │
│  8. TrafficTagPluginProcessor      - 流量标签插件               │
│  9. HighPriorityTrafficMW          - 高优先级流量               │
│  10. ErrorCodeMW                   - 错误码处理                 │
│  11. SetClientRegionInfoMW         - 客户端区域信息            │
│  12. MarkResponseCode              - 响应码标记                │
│  13. InitFeatureManager             - 特性管理                  │
│  14. AlphaSDK Middleware            - Alpha SDK                │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│              Handler Layer (biz/handler/)                     │
│  - 请求参数解析与验证                                          │
│  - 调用 Service 层                                             │
│  - 响应封装与返回                                              │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│              Service Layer (biz/service/)                     │
│  - 业务逻辑实现                                                │
│  - RPC 客户端调用                                              │
│  - 数据组装与转换                                              │
│  - 缓存策略执行                                                │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│              DAL Layer (biz/dal/)                             │
│  - Redis 客户端操作                                            │
│  - 本地缓存操作                                                │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│              External RPC Services                            │
│  - oec_live_shop              (直播商品服务)                   │
│  - live_room_goroom_tiktok   (直播间服务)                      │
│  - ttec_live_pack             (直播打包服务)                    │
│  - oec_product_product_detail (商品详情服务)                    │
│  - oec_promotion_voucher_center (优惠券中心)                   │
│  - overpass services           (跨服务调用)                     │
└─────────────────────────────────────────────────────────────────┘
```

### 各层职责详解

#### 1. HTTP Layer (路由层)

**文件**: `router_gen.go`, `router.go`

**职责**:

- 路由注册
- 中间件配置
- 请求分发

**特点**:

- 使用 Hertz 框架
- 路由代码由 `hertztool` 自动生成
- 支持版本化路由 (v1, v2, v3)

#### 2. Middleware Layer (中间件层)

**执行顺序**: 从上到下，洋葱模型

**关键中间件**:

| 中间件                  | 位置                                      | 功能                       |
| ----------------------- | ----------------------------------------- | -------------------------- |
| `HertzInterfaceLimitMw` | `biz/middleware/arch_limiter.go`          | 接口级限流，支持自定义 key |
| `TrafficTagProcessor`   | `biz/middleware/traffic_tag.go`           | 流量标签注入               |
| `ErrorCodeMW`           | `biz/middleware/error_code.go`            | 错误码处理和 SLI 标记      |
| `HighPriorityTrafficMW` | `biz/middleware/high_priority_traffic.go` | 高优先级流量处理           |

#### 3. Handler Layer (处理器层)

**职责**:

- HTTP 请求参数解析
- 参数验证
- 调用 Service 层
- 响应封装

**代码模式**:

```go
func GetLiveBag(ctx context.Context, c *app.RequestContext) {
    // 1. 参数解析
    req := &GetLiveBagReqDto{}
    if err := c.BindAndValidate(&req); err != nil {
        // 错误处理
        return
    }

    // 2. 构建 RequestContext
    requestContext := types.NewRequestContext(ctx, c, req)

    // 3. 调用 Service 层
    resp, status := service.GetLiveProducts(requestContext, ...)

    // 4. 响应封装
    tools.GetHertzResponseData(c, resp, status)
}
```

#### 4. Service Layer (服务层)

**职责**:

- 业务逻辑实现
- RPC 调用
- 数据组装
- 缓存策略

**代码模式**:

```go
func GetLiveProducts(requestContext *types.RequestContext, ...) (*shop.GetLiveProductsResponse, *Status.Status) {
    // 1. 构建请求
    req := &shop.GetLiveProductsRequest{...}

    // 2. 调用 RPC
    resp, err := oec_live_shop.RawCall.GetLiveProducts(requestContext.Context, req)
    if err != nil {
        logs.CtxError(requestContext.Context, "error: %v", err)
        return nil, Status.LiveApiServerRPCError
    }

    // 3. 数据处理
    products := tools.FormatProductsPack(requestContext, resp.Products, ...)

    // 4. 组装响应
    return &shop.GetLiveProductsResponse{
        Products: products,
        Total: resp.Total,
    }, nil
}
```

#### 5. DAL Layer (数据访问层)

**职责**:

- Redis 操作
- 本地缓存操作

**特点**:

- 使用字节跳动内部 `goredis` 客户端
- 支持服务发现
- 统一错误处理

---

## 调用链路

### 典型请求处理流程

以 **获取直播间商品列表** (`GET /aweme/v1/oec/live/bag`) 为例：

```
1. HTTP Request
   ↓
2. Hertz Router (router_gen.go)
   ↓
3. Middleware Chain
   ├─ HertzInterfaceLimitMw (限流检查)
   ├─ LatencyRecorderHertz (开始计时)
   ├─ UnzipBodyHertz (解压)
   ├─ Gzip (设置压缩)
   ├─ HertzSessionProcessor (Session 处理)
   ├─ OdinProcessHertz (Odin 认证)
   ├─ TrafficTagProcessor (流量标签)
   ├─ TrafficTagPluginProcessor (流量标签插件)
   ├─ HighPriorityTrafficMW (高优先级检查)
   ├─ ErrorCodeMW (错误码中间件)
   ├─ SetClientRegionInfoMW (区域信息)
   ├─ MarkResponseCode (响应码标记)
   ├─ InitFeatureManager (特性管理)
   └─ AlphaSDK Middleware (Alpha SDK)
   ↓
4. Handler: GetLiveBag (biz/handler/get_live_bag.go)
   ├─ 参数解析与验证
   ├─ 构建 RequestContext
   └─ 调用 service.GetLiveProducts()
   ↓
5. Service: GetLiveProducts (biz/service/live_products.go)
   ├─ 构建 RPC 请求
   ├─ 调用 oec_live_shop.RawCall.GetLiveProducts()
   │   ↓
   │   RPC Call (Kitex)
   │   ├─ 序列化 (Thrift)
   │   ├─ 网络传输
   │   └─ 反序列化
   │   ↓
   │   Return: *shop.GetLiveProductsResponse
   │   ↓
   ├─ 错误处理
   ├─ 数据格式化 (tools.FormatProductsPack)
   ├─ 缓存更新 (可选)
   └─ 返回响应
   ↓
6. Handler: 响应封装
   └─ tools.GetHertzResponseData()
   ↓
7. Middleware Chain (后处理)
   ├─ ErrorCodeMW (错误码处理)
   ├─ LatencyRecorderHertz (记录延迟)
   └─ Gzip (响应压缩)
   ↓
8. HTTP Response
```

### 跨服务调用链路

```
live_shopapi (HTTP Service)
    │
    ├─→ oec_live_shop (RPC) - 直播商品服务
    │       ├─ GetLiveProducts
    │       ├─ GetLiveProductIds
    │       └─ AddLiveProduct
    │
    ├─→ live_room_goroom_tiktok (RPC) - 直播间服务
    │       ├─ GetRoomData
    │       └─ GetRoomInfo
    │
    ├─→ ttec_live_pack (RPC) - 直播打包服务
    │       └─ GetLiveProductsPack
    │
    ├─→ oec_product_product_detail (RPC) - 商品详情服务
    │       └─ GetProductDetail
    │
    ├─→ oec_promotion_voucher_center (RPC) - 优惠券中心
    │       ├─ GetVoucherList
    │       └─ ClaimVoucher
    │
    └─→ overpass services (RPC) - 跨服务调用
            └─ 各种通用服务
```

### 并发调用模式

使用 `sync.WaitGroup` 实现并发 RPC 调用：

```go
func GetLiveProductsWgWrapper(wg *sync.WaitGroup, requestContext *types.RequestContext, ...) {
    defer wg.Done()

    // 调用 RPC
    resp, err := GetLiveProducts(requestContext, ...)

    // 数据处理
    ret := &LiveProductsCache{
        Products: tools.FormatProductsPack(...),
        Total:    int32(resp.Total),
    }

    // 更新响应
    productsResp.Data = &live_serv.GetLiveProductsResponse_Data{
        Products: ret.Products,
        Total:    ret.Total,
    }
}

// 调用方
wg := sync.WaitGroup{}
wg.Add(1)
go GetLiveProductsWgWrapper(&wg, requestContext, req, productsResp, ...)
wg.Wait()
```

---

## 核心设计模式

### 1. 依赖注入模式 (Dependency Injection)

**实现位置**: `repositories/container.go`

**设计思想**: 使用接口抽象，手动实现容器

**优点**:

- 解耦依赖关系
- 便于测试（可替换实现）
- 统一管理全局依赖

**代码结构**:

```go
// 接口定义
type IContainer interface {
    GetJSON() lib.IJSON
    GetMetricsClient() sdk.IMetrics4
    GetTTecLiveConfTCC() tcc.ITTecLiveConf
    GetOecLiveShopAPITCC() tcc.IOecLiveShopAPI
    GetLimiter() lib.IGecLimiterLib
}

// 具体实现
type Container struct {
    json              lib.IJSON
    metrics           sdk.IMetrics4
    ttecLiveConfTCC   tcc.ITTecLiveConf
    oecLiveShopAPITCC tcc.IOecLiveShopAPI
    limiter           lib.IGecLimiterLib
}

// 单例初始化
var c *Container
var once sync.Once

func InitContainer() {
    once.Do(func() {
        json := lib.NewJSON()
        mtr := sdk.NewMetrics4()
        ttecLiveConfTCC := tcc.NewTTecLiveConf(json)
        oecLiveShopAPITCC := tcc.NewOecLiveShopApi(json)
        limiter := lib.NewGecLimiter()
        c = &Container{...}
    })
}
```

### 2. 工厂模式 (Factory Pattern)

**应用场景**: 创建复杂对象

**示例**: Toggle Manager 创建

```go
func NewManager(
    enableToggleComplianceV2 bool,
    regionToggleProcessorMap map[string]processor.Processor,
) *Manager {
    return &Manager{
        enableToggleComplianceV2:   enableToggleComplianceV2,
        regionToggleProcessorMap:  regionToggleProcessorMap,
    }
}
```

### 3. 策略模式 (Strategy Pattern)

**应用场景**: 不同地区使用不同的合规处理策略

**代码示例**:

```go
type Manager struct {
    regionToggleProcessorMap map[string]processor.Processor
}

func (m Manager) GetToggleCompliance(productIds []int64) *live_serv.ToggleCompliance {
    if m.enableToggleComplianceV2 {
        // 使用 V2 策略
        toggleCompliance = m.getToggleComplianceV2(productIds)
    } else {
        // 使用地区策略
        regionToggleProcessor := m.regionToggleProcessorMap[region]
        toggleCompliance = regionToggleProcessor.GetToggleCompliance(...)
    }
    return toggleCompliance
}
```

### 4. 单例模式 (Singleton Pattern)

**应用场景**: 全局缓存、客户端连接

**示例**: 本地 LRU 缓存

```go
var (
    PopItemLRUCache = gcache.New(20000).LRU().Build()
    RoomFeatureLRUCache = gcache.New(50000).LRU().Build()
)
```

### 5. 责任链模式 (Chain of Responsibility)

**应用场景**: 中间件链

**实现**: Hertz 框架的中间件机制

```go
r.Use(
    bizmiddleware.HertzInterfaceLimitMw,
    middleware.LatencyRecorderHertz,
    middleware.UnzipBodyHertz,
    gzip.Gzip(),
    tiktok_session_lib.HertzSessionProcessor.ProcessRequest,
    // ... 更多中间件
)
```

### 6. 装饰器模式 (Decorator Pattern)

**应用场景**: 流量标签插件

**代码示例**:

```go
type TrafficTagPluginChain struct {
    plugins []ITrafficTagPlugin
}

func (c *TrafficTagPluginChain) Execute(ctx context.Context, reqCtx *app.RequestContext) {
    for _, plugin := range c.plugins {
        if plugin.IsMatch(ctx, reqCtx) {
            plugin.Execute(ctx, reqCtx)
        }
    }
}
```

---

## 中间件体系

### 中间件分类

#### 1. 认证与授权中间件

| 中间件                  | 文件                 | 功能         |
| ----------------------- | -------------------- | ------------ |
| `HertzSessionProcessor` | `tiktok_session_lib` | Session 处理 |
| `OdinProcessHertz`      | `tiktok_odinlib`     | Odin 认证    |

#### 2. 限流中间件

| 中间件                  | 文件                                      | 功能         |
| ----------------------- | ----------------------------------------- | ------------ |
| `HertzInterfaceLimitMw` | `biz/middleware/arch_limiter.go`          | 接口级限流   |
| `HighPriorityTrafficMW` | `biz/middleware/high_priority_traffic.go` | 高优先级流量 |

**限流策略**:

```go
// 第一层：自定义 key 限流 (接口名 + roomId)
if gslice.Contains(gecLimiterConfig.GetCustomLimitPaths(), path) {
    if isLimitWithDefaultVal(ctx, path, conv.Int64ToStr(roomId), false) {
        processIsDenied(ctx, c, path)
        return
    }
}

// 第二层：共享式限流
if isDenied, err = limiter.InterfaceMWLimiter.IsLimitMw(ctx, resourceName); err != nil {
    // 错误处理
}
```

#### 3. 流量标签中间件

| 中间件                      | 文件                            | 功能         |
| --------------------------- | ------------------------------- | ------------ |
| `TrafficTagProcessor`       | `biz/middleware/traffic_tag.go` | 流量标签注入 |
| `TrafficTagPluginProcessor` | `biz/middleware/traffic_tag.go` | 流量标签插件 |

**流量标签插件链**:

```go
type TrafficTagPluginChain struct {
    plugins []ITrafficTagPlugin
}

// 插件接口
type ITrafficTagPlugin interface {
    IsMatch(ctx context.Context, reqCtx *app.RequestContext) bool
    Execute(ctx context.Context, reqCtx *app.RequestContext)
}
```

#### 4. 错误处理中间件

| 中间件        | 文件                           | 功能                  |
| ------------- | ------------------------------ | --------------------- |
| `ErrorCodeMW` | `biz/middleware/error_code.go` | 错误码处理和 SLI 标记 |

**错误码处理逻辑**:

```go
func ErrorCodeMW(c context.Context, ctx *app.RequestContext) {
    ctx.Next(c)
    errorCodeMetrics(c, ctx)
}

func errorCodeMetrics(c context.Context, ctx *app.RequestContext) {
    // 从 TCC 加载错误码正则配置
    errorCodeRegexpConfig, err := repo.GetContainer().GetOecLiveShopAPITCC().GetErrorCodeRegexpConfig(c)

    // 记录指标
    metricsTags.Emit(m.Incr(1))

    // 根据配置修改响应头
    if affectSli {
        ctx.Response.Header.Set("tt_stable", "0")
    }
}
```

#### 5. 监控中间件

| 中间件                 | 文件                                   | 功能       |
| ---------------------- | -------------------------------------- | ---------- |
| `LatencyRecorderHertz` | `request_lib/v2/middleware`            | 延迟记录   |
| `MarkResponseCode`     | `content_basic_support_lib/middleware` | 响应码标记 |

#### 6. 数据处理中间件

| 中间件           | 文件             | 功能     |
| ---------------- | ---------------- | -------- |
| `UnzipBodyHertz` | `request_lib/v2` | 请求解压 |
| `Gzip`           | `hertz_ext/v2`   | 响应压缩 |

#### 7. 特性管理中间件

| 中间件                | 文件                                    | 功能         |
| --------------------- | --------------------------------------- | ------------ |
| `InitFeatureManager`  | `content_selection_lib/feature_manager` | 特性开关管理 |
| `AlphaSDK Middleware` | `alphasdk`                              | Alpha SDK    |

### 中间件执行执行顺序

```
请求 → HertzInterfaceLimitMw
     → LatencyRecorderHertz (开始计时)
     → UnzipBodyHertz
     → Gzip (设置压缩)
     → HertzSessionProcessor
     → OdinProcessHertz
     → TrafficTagProcessor
     → TrafficTagPluginProcessor
     → HighPriorityTrafficMW
     → ErrorCodeMW (注册后处理)
     → SetClientRegionInfoMW
     → MarkResponseCode
     → InitFeatureManager
     → AlphaSDK Middleware
     → Handler
     → ErrorCodeMW (后处理)
     → LatencyRecorderHertz (记录延迟)
     → Gzip (压缩响应)
     → 响应
```

---

## 依赖管理

### Go Module 依赖结构

```
live_shopapi
├── 字节跳动内部库 (code.byted.org)
│   ├── oec/                    # 电商相关
│   │   ├── rpcv2_*            # RPC 客户端
│   │   ├── live_common/       # 直播通用
│   │   ├── request_lib/       # 请求库
│   │   ├── affiliate_*_lib/   # 联盟营销
│   │   ├── content_*_lib/     # 内容相关
│   │   ├── schema_lib/        # Schema SDK
│   │   ├── status_code/       # 状态码
│   │   └── ...
│   ├── tikcast/               # 直播服务
│   ├── ttec/                  # TikTok 电商
│   │   ├── cart_lib/          # 购物车
│   │   ├── delivery_sdk/      # 配送
│   │   ├── promotion_c/       # 营销
│   │   ├── trade_lib/         # 交易
│   │   └── alphasdk/          # Alpha SDK
│   Tiktok/                   # TikTok 核心服务
│   │   ├── tiktok_odin_go_lib # Odin 认证
│   │   ├── tiktok_session_lib # Session
│   │   └── ...
│   ├── overpass/              # 跨服务调用
│   ├── bric_shark/            # 安全相关
│   ├── middleware/            # 中间件
│   │   ├── hertz/            # Hertz 中间件
│   │   └── hertz_ext/        # Hertz 扩展
│   ├── gopkg/                # 通用包
│   │   ├── logs/v2           # 日志
│   │   ├── metrics/v4        # 指标
│   │   ├── tccclient         # 配置中心
│   │   ├── env               # 环境变量
│   │   ├── jsonx             # JSON
│   │   ├── lang/conv         # 类型转换
│   │   └── ...
│   ├── kv/                   # 键值存储
│   │   ├── goredis           # Redis
│   │   └── redis-v6          # Redis v6
│   ├── kite/                 # Kitex RPC
│   └── ...
└── 第三方库
    ├── github.com/cloudwego/
    │   ├── hertz              # HTTP 框架
    │   └── kitex              # RPC 框架
    ├── github.com/bytedance/
    │   ├── sonic              # JSON 序列化
    │   └── mockey             # Mock 框架
    ├── google.golang.org/
    │   └── protobuf           # Protobuf
    ├── github.com/spf13/
    │   └── viper              # 配置管理
    ├── github.com/shopspring/
    │   └── decimal            # 高精度计算
    ├── github.com/bluele/
    │   └── gcache             # LRU 缓存
    ├── github.com/google/
    │   └── uuid               # UUID
    ├── github.com/stretchr/
    │   └── testify            # 测试框架
    └── ...
```

### 主要依赖说明

#### Web 框架

```go
github.com/cloudwego/hertz v0.9.7
code.byted.org/middleware/hertz v1.13.8
code.byted.org/middleware/hertz_ext/v2 v2.1.10
```

#### RPC 框架

```go
code.byted.org/kite/kitex v1.18.4
github.com/cloudwego/kitex v0.12.4
code.byted.org/gopkg/thrift v1.14.2
```

#### 配置中心

```go
code.byted.org/gopkg/tccclient v1.6.8
```

#### 缓存

```go
code.byted.org/kv/goredis v5.7.7+incompatible
code.byted.org/kv/redis-v6 v1.1.6
github.com/bluele/gcache v0.0.2
```

#### 日志与监控

```go
code.byted.org/gopkg/logs/v2 v2.2.2
code.byted.org/gopkg/metrics/v4 v4.1.5
code.byted.org/bytedtrace/interface-go v1.0.22
```

#### 序列化

```go
github.com/bytedance/sonic v1.14.0  // 高性能 JSON
google.golang.org/protobuf v1.36.5
```

#### 限流

```go
code.byted.org/oec/arch_limiter v1.13.6
code.byted.org/iesarch/simple_rate_limit/v2 v2.0.16
```

---

## 代码生成机制

### 1. HertzTool (路由代码生成)

**工具**: `hertztool`

**用途**: 根据 IDL 定义生成路由注册代码和 Handler 模板

**配置**: Makefile

```makefile
make:
    hertztool update --unset_omitempty=true -I=$(PROTO_PATH) -idl=$(IDL_PATH)
```

**生成文件**:

- `router_gen.go` - 路由注册代码
- `biz/handler/*.go` - Handler 模板文件

**IDL 示例**:

```thrift
service OecLiveShopApi {
    GetLiveProductsResponse GetLiveProducts(1: GetLiveProductsRequest req)
    GetLiveProductIdsResponse GetLiveProductIds(1: GetLiveProductIdsRequest req)
}
```

**生成代码示例**:

```go
// router_gen.go
func register(r *server.Hertz) {
    customizeRegister(r)
    r.GET("/aweme/v1/oec/live/bag", _GetLiveBag_mws(), handler.GetLiveBag)
    r.GET("/aweme/v1/oec/live/products", _GetLiveProducts_mws(), handler.GetLiveProducts)
    // ...
}
```

### 2. Goverter (模型转换器生成)

**工具**: `goverter`

**用途**: 自动生成模型转换代码

**配置**: Makefile

```makefile
generate_converter:
    cd biz/model/converter && go run github.com/jmattheis/goverter/cmd/goverter gen ...
```

**生成文件**: `biz/model/converter/*.go`

### 3. Protobuf 代码生成

**工具**: `http_idl_gen`

**用途**: 生成 Protobuf Go 代码

**生成文件**:

- `biz/model/live_serv/oec_live_shopapi.pb.go` - Protobuf 消息类型
- `biz/model/live_serv/*.pb.go` - 其他 Protobuf 文件

### 4. Kitex 代码生成

**工具**: `kitex`

**用途**: 生成 RPC 客户端代码

**生成位置**: `code.byted.org/oec/rpcv2_*/kitex_gen/`

---

## 配置管理

### 配置架构

```
配置中心 (TCC)
    │
    ├─→ oec.live.shopapi (主配置)
    │       ├─ 错误码配置
    │       ├─ 限流配置
    │       ├─ 功能开关
    │       └─ AB 测试配置
    │
    ├─→ ttec.live.conf (TTEC 配置)
    │       ├─ AB 测试参数
    │       └─ 灰度配置
    │
    └─→ oec.content.selection (内容选择配置)
            └─ 特性配置
```

### TCC 配置客户端

**初始化**: `biz/tcc/configs.go`

```go
var (
    liveShopApiTccClient       *tccclient.ClientV2
    VideoToggleHalfLoopClient  *tccclient.ClientV2
    LiveToggleAllowListClient  *tccclient.ClientV2
)

func initTcc() {
    cf := tccclient.NewConfigV2()
    liveShopApiTccClient, err = tccclient.NewClientV2("oec.live.shopapi", cf)
    VideoToggleHalfLoopClient, err = tccclient.NewClientV2("video.toggle.half.loop", cf)
    LiveToggleAllowListClient, err = tccclient.NewClientV2("live.toggle.allow.list", cf)
}
```

### 配置使用示例

```go
// 获取错误码配置
errorCodeRegexpConfig, err := repo.GetContainer().GetOecLiveShopAPITCC().GetErrorCodeRegexpConfig(c)

// 获取限流配置
gecLimiterConfig := repo.GetContainer().GetLimiter().GetGecLimiterConfig()

// 获取 AB 测试配置
abResult := abtest.GetAbParamV2(requestContext.Context)
```

### AB 测试集成

**初始化**: `main.go`

```go
abtest.InitAbAgent()
abtest.InitTccABParamReplacerByFunc(repo.GetContainer().GetTTecLiveConfTCC().GetTccAbParamConfigGetter())
abtest.InitGrayScaleProvider(&abtest.DefaultAgentProvider{}, &abtest.DefaultRpcProvider{}, func() (rate int64, whiteList []int64) {
    if cfg := config.GetAbTestAgentGrayScaleConfig(context.Background()); cfg != nil {
        return cfg.Rate, cfg.WhiteList
    }
    return
})
```

**使用示例**:

```go
abResult := abtest.GetAbParamV2(requestContext.Context)
if abResult != nil && abResult.TTEcom.LiveInfoEnrichPatternId {
    // 命中实验
} else {
    // 未命中实验
}
```

---

## 缓存策略

### 缓存层级

```
┌─────────────────────────────────────────────────────────────┐
│                    Handler Layer                            │
└────────────────────────────┬────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│              Local Cache (LRU)                             │
│  - PopItemLRUCache (20,000 items)                          │
│  - RoomFeatureLRUCache (50,000 items)                      │
└────────────────────────────┬────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│              TTEC Cache (Redis)                            │
│  - GetCacheProductInfo()                                    │
│  - GetCacheRoomInfo()                                       │
└────────────────────────────┬────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│              RPC Services                                   │
└─────────────────────────────────────────────────────────────┘
```

### 本地缓存

**实现**: `biz/model/cache/local_cache.go`

```go
var (
    PopItemLRUCache = gcache.New(20000).LRU().Build()
    RoomFeatureLRUCache = gcache.New(50000).LR(LocalCache).Build()
)
```

**特点**:

- 使用 LRU 淘汰策略
- 固定容量
- 进程级别缓存

### Redis 缓存

**实现**: `biz/ttec_cache/`

**主要方法**:

```go
// 获取商品缓存
func GetCacheProductInfo(ctx context.Context, roomId int64, region string, productIds []int64, deviceId int64) ([]*shop.Product, error)

// 获取直播间缓存
func GetCacheRoomInfo(ctx context.Context, roomId int64) (*room.GetRoomDataResponse, error)
```

**缓存策略**:

- 缓存键: `ttec:live:product:{roomId}:{region}:{productId}`
- 缓存时间: 根据业务配置
- 缓存更新: 主动更新 + 过期淘汰

### 缓存使用示例

```go
// 尝试从缓存获取
products, err := ttec_cache.GetCacheProductInfo(requestContext.Context, roomId, region, productIds, deviceId)
if err != nil {
    // 缓存未命中，调用 RPC
    resp, err := oec_live_shop.RawCall.GetLiveProducts(requestContext.Context, req)
    if err != nil {
        return nil, Status.LiveApiServerRPCError
    }
    products = resp.Products
}

// 使用缓存数据
return &shop.GetLiveProductsResponse{
    Products: products,
    Total:    int64(len(products)),
}, nil
```

---

## 测试体系

### 测试框架

| 框架       | 用途         |
| ---------- | ------------ |
| `testify`  | 断言库       |
| `mockey`   | Mock 框架    |
| `goconvey` | BDD 风格测试 |

### 测试结构

```
biz/
├── handler/
│   ├── add_product_test.go
│   ├── auction_check_test.go
│   ├── get_live_bag_test.go
│   └── ...
├── service/
│   ├── live_products_test.go
│     toggle/
│   │   └── manager_test.go
│   └── ...
└── ...
```

### 测试示例

#### BDD 风格测试

```go
func TestAuctionCheck(t *testing.T) {
    convey.Convey("TestAuctionCheck", t, func() {
        convey.Convey("normal case", func() {
            // 测试逻辑
        })
    })
}
```

#### Mock 测试

```go
func TestGetLiveProducts(t *testing.T) {
    mockey.PatchConvey("TestGetLiveProducts", t, func() {
        // Mock RPC 调用
        mockey.Mock(oec_live_shop.RawCall.GetLiveProducts).To(
            func(ctx context.Context, req *shop.GetLiveProductsRequest) (*shop.GetLiveProductsResponse, error) {
                return &shop.GetLiveProductsResponse{
                    Products: []*shop.Product{...},
                    Total:    10,
                }, nil
            },
        ).Build()

        // 调用测试函数
        resp, status := GetLiveProducts(requestContext, ...)

        // 断言
        assert.NotNil(t, resp)
        assert.Nil(t, status)
    })
}
```

### 测试覆盖率

- 总文件数: 325
- 测试文件数: 75
- 测试覆盖率: 约 23% (文件数比例)

---

## 架构特点总结

### 1. 清晰的分层架构

**优点**:

- 职责明确，易于维护
- 层间依赖单向，避免循环依赖
- 便于测试和扩展

**分层**: Handler → Service → DAL → External RPC

### 2. 丰富的中间件体系

**特点**:

- 洋葱模型，易于扩展和组合
- 统一的请求处理流程
- 跨切面关注点（限流、认证、监控等）集中管理

### 3. 依赖注入容器

**优点**:

- 解耦依赖关系
- 便于测试（可替换实现）
- 统一管理全局资源

**实现**: 手动实现的容器模式，非使用第三方 DI 框架

### 4. 代码生成驱动

**特点**:

- 减少重复代码
- 保证代码一致性
- 提高开发效率

**工具**: HertzTool, Goverter, Protobuf, Kitex

### 5. 配置中心驱动

**特点**:

- 配置集中管理
- 支持动态更新
- AB 测试集成

**实现**: TCC (TikTok Config Center)

### 6. 多级缓存策略

**层级**: Local Cache → Redis Cache → RPC

**优点**:

- 减少网络调用
- 提高响应速度
- 降低后端压力

### 7. 并发调用优化

**实现**: 使用 `sync.WaitGroup` 实现并发 RPC 调用

**优点**:

- 减少总响应时间
- 提高吞吐量

### 8. 微服务架构

**特点**:

- 服务拆分细粒度
- 通过 RPC 通信
- 独立部署和扩展

### 9. 完善的监控体系

**组件**:

- 日志: `logs/v2`
- 指标: `metrics/v4`
- 链路追踪: `bytedtrace`

### 10. 测试驱动开发

**特点**:

- 较完善的测试覆盖
- 使用 Mock 框架
- BDD 风格测试

---

## 注意事项

### 1. 内部依赖限制

**问题**: 大量依赖字节跳动内部库 (`code.byted.org`)

**影响**:

- 无法在开源环境直接编译
- 需要内部网络环境
- 依赖更新需要内部流程

**建议**:

- 对于本地索引，需要处理内部依赖的占位或模拟
- 重点关注业务逻辑层，而非内部库实现

### 2. 代码生成文件

**问题**: 部分文件由工具自动生成

**影响**:

- 手动修改会被覆盖
- 需要了解生成工具的使用

**建议**:

- 标记代码生成文件
- 索引时跳过或特殊处理

### 3. 配置依赖

**问题**: 依赖 TCC 配置中心

**影响**:

- 离线环境无法获取配置
- 配置变化影响行为

**建议**:

- 提供默认配置
- 支持本地配置文件

### 4. RPC 服务依赖

**问题**: 依赖多个外部 RPC 服务

**影响**:

- 需要服务发现机制
- 网络故障影响可用性

**建议**:

- 提供降级策略
- 实现熔断机制

### 5. 并发安全性

**注意点**:

- 全局变量需要线程安全
- 缓存访问需要加锁
- Context 传递要正确

**示例**:

```go
// 使用 sync.Once 确保单例初始化
var once sync.Once
func InitContainer() {
    once.Do(func() {
        // 初始化逻辑
    })
}
```

### 6. 错误处理

**模式**:

- 使用统一的 `Status.Status` 错误码
- 记录详细日志
- 返回用户友好的错误信息

**示例**:

```go
resp, err := oec_live_shop.RawCall.GetLiveProducts(ctx, req)
if err != nil {
    logs.CtxError(ctx, "GetLiveProducts error: %v", err)
    return nil, Status.LiveApiServerRPCError
}
```

### 7. 性能优化

**策略**:

- 使用并发调用
- 多级缓存
- 连接池复用
- 响应压缩

### 8. 安全性

**注意点**:

- 所有接口都需要认证（Odin）
- 敏感数据需要加密
- 输入参数需要验证
- 限流防止滥用

### 9. 可观测性

**实践**:

- 记录关键指标
- 链路追踪贯穿全链路
- 结构化日志
- 错误码标准化

### 10. 版本兼容性

**策略**:

- API 版本化 (v1, v2, v3)
- 向后兼容
- 渐进式升级

---

## 附录

### A. 关键文件索引

| 文件                             | 说明               |
| -------------------------------- | ------------------ |
| `main.go`                        | 应用入口           |
| `router.go`                      | 路由中间件配置     |
| `router_gen.go`                  | 代码生成的路由注册 |
| `repositories/container.go`      | 依赖注入容器       |
| `biz/dal/redis.go`               | Redis 客户端初始化 |
| `biz/middleware/arch_limiter.go` | 限流中间件         |
| `biz/middleware/traffic_tag.go`  | 流量标签中间件     |
| `biz/middleware/error_code.go`   | 错误码中间件       |

### B. 关键接口索引

| 接口       | 路由                                    | Handler                 | Service                 |
| ---------- | --------------------------------------- | ----------------------- | ----------------------- |
| 获取购物车 | `GET /aweme/v1/oec/live/bag`            | `GetLiveBag`            | `GetLiveProducts`       |
| 添加商品   | `POST /aweme/v1/oec/live/product/add`   | `AddProduct`            | `AddLiveProduct`        |
| 删除商品   | `POST /aweme/v1/oec/live/product/del`   | `DelProduct`            | `DelLiveProduct`        |
| 商品置顶   | `POST /aweme/v1/oec/live/product/top`   | `TopProduct`            | `TopLiveProduct`        |
| 搜索商品   | `GET /aweme/v1/oec/live/product/search` | `SearchLiveBagProducts` | `SearchLiveBagProducts` |

### C. RPC 服务索引

| 服务                           | 用途         |
| ------------------------------ | ------------ |
| `oec_live_shop`                | 直播商品服务 |
| `live_room_goroom_tiktok`      | 直播间服务   |
| `ttec_live_pack`               | 直播打包服务 |
| `oec_product_product_detail`   | 商品详情服务 |
| `oec_promotion_voucher_center` | 优惠券中心   |

### D. 常用工具函数

| 函数                   | 位置                           | 功能            |
| ---------------------- | ------------------------------ | --------------- |
| `FormatProductsPack`   | `biz/tools/format.go`          | 格式化商品列表  |
| `GetHertzResponseData` | `biz/tools/response.go`        | 封装 Hertz 响应 |
| `NewRequestContext`    | `biz/types/request_context.go` | 创建请求上下文  |

---

**文档结束**

如需进一步了解某个模块的详细信息，请参考对应源码文件。
