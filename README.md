# hdyadmin-common

# 基础来源

从 go-tangra-common 的 标签 v1.20.0 拷贝而来 进行二开

## 公共统计协议

`protos/common/service/v1/statistics.proto` 定义了各业务模块向 Core 提供统计数据时使用的稳定接口：

- `ModuleStatisticsService.GetModuleStatistics`：统一的模块统计查询入口；
- `GetModuleStatisticsRequest`：统一到期天数、最近错误数量和租户筛选条件；
- `GetModuleStatisticsResponse`：包含稳定模块标识和 `google.protobuf.Struct` 统计数据。

模块可以继续保留自己的强类型统计协议。公共协议只承担跨模块调用边界，避免 Core 直接依赖并重复生成各模块的完整 Proto。

`statistics.NewResponse` 可以将模块原有的 Protobuf 响应转换成公共响应，并保持原有 Proto JSON 字段名和 64 位整数的 JSON 表示。
