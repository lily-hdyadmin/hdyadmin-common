# 公共国际化组件

`i18n` 包统一提供 hdyadmin 各 Go 服务的国际化能力，包括：

- `Accept-Language` 语言匹配和默认语言回退；
- 带参数消息的延迟编码与翻译；
- Kratos HTTP/gRPC 错误翻译；
- protobuf 响应中 `message`、`msg`、`error_message` 字段的递归翻译；
- unary/stream 中间件和 HTTP 错误编码器；
- 公共成功、失败提示及 HTTP 状态提示词典。

## 词典覆盖顺序

翻译器按以下顺序合并词典，后加载的内容优先：

1. `hdyadmin-common/i18n/locale` 公共基础词典；
2. 具体服务通过 `CatalogSource` 提供的内嵌词典；
3. `Config.Dir` 指定的外部词典目录。

服务词典只需要维护自己的 `response`、`reason` 和 `error` 等业务文案，公共 `common`、`http` 文案由本包统一维护。

## 服务接入

```go
translator, err := i18n.New(
    i18n.Config{DefaultLanguage: "zh-CN"},
    i18n.CatalogSource{
        FS: serviceLocaleFS,
        Root: "locale",
    },
)
```

无参数提示直接保存稳定消息键：

```go
Message: i18n.Success
```

带参数提示使用 `Message`，在 API 边界再根据请求语言渲染：

```go
Message: i18n.Message("response.taskCompleted", taskID)
```

公共中间件只翻译错误和提示字段：`message`、`msg`、`error_message`。它不会改写
`name`、`description` 等业务数据。

需要国际化的系统业务数据应在数据库保存稳定键，并由业务服务在响应边界填充独立展示字段，
例如同时返回 `name: database.role.platformAdministrator` 与
`displayName: 平台管理员`。用户输入的自由文本按原值保存和返回。
