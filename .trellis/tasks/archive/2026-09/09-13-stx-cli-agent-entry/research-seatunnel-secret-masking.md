# Apache SeaTunnel 配置脱敏研究

## 官方实现

- Apache SeaTunnel 2.3.12 的 `ConfigShadeUtils.DEFAULT_SENSITIVE_KEYWORDS` 包含 `password`、`username`、`auth`、`token`、`access_key`、`secret_key`。任务可以通过 `env.shade.options` 增加敏感配置项。
- 2.3.12 的 `ConfigBuilder.configDesensitization` 会递归复制配置结构，命中敏感键时把标量或列表元素替换成 `******`，并在打印解析后的任务配置前调用。
- 当前开发分支扩充了匹配规则：`.`、`_`、`-` 视为同类分隔符，支持嵌套配置路径和多段配置名后缀，并额外处理 JDBC URL 与 `sasl.jaas.config`。
- 参考文件：
  - <https://github.com/apache/seatunnel/blob/2.3.12/seatunnel-core/seatunnel-core-starter/src/main/java/org/apache/seatunnel/core/starter/utils/ConfigShadeUtils.java>
  - <https://github.com/apache/seatunnel/blob/2.3.12/seatunnel-core/seatunnel-core-starter/src/main/java/org/apache/seatunnel/core/starter/utils/ConfigBuilder.java>
  - <https://github.com/apache/seatunnel/blob/dev/seatunnel-core/seatunnel-core-starter/src/main/java/org/apache/seatunnel/core/starter/utils/ConfigShadeUtils.java>
  - <https://github.com/apache/seatunnel/blob/dev/seatunnel-core/seatunnel-core-starter/src/main/java/org/apache/seatunnel/core/starter/utils/ConfigBuilder.java>
  - <https://github.com/apache/seatunnel/pull/7247>

## STX 当前情况

- 同步工作台的全局变量已经支持 `secret` 类型：返回时显示 `******`，更新时提交空值或原掩码会保留数据库中的旧值。
- 同步任务的 `content`、任务版本的 `content_snapshot`、运行实例 `submit_spec.submitted_content` 和部分预览内容当前会通过 API 原样返回。
- 前端编辑器读取任务正文后会再次提交完整正文；版本比较和运行详情也会读取这些字段。

## 不能直接照搬的部分

- SeaTunnel 的实现是 Java 代码，STX 控制端是 Go；应复用规则和测试样例，而不是增加 Java 运行时依赖。
- SeaTunnel 2.3.12 主要解决日志打印，不定义 API 读取、任务编辑往返、审计和诊断包的处理方式。
- 仅按固定键精确匹配会漏掉 `access-key`、`clientSecret`、嵌套路径等写法。STX 应采用当前开发分支的分隔符归一和递归路径规则，并允许管理员增加敏感键。
- 脱敏失败时不能返回原文。对于无法可靠解析的 HOCON/JSON，敏感响应应拒绝返回或只返回摘要。

## 建议用于 STX 的规则

- 服务端保留原始任务配置用于执行，但所有 API、CLI、审计、日志和诊断输出都调用同一个脱敏组件。
- HOCON/JSON 先解析成结构再递归处理，不依赖单个正则表达式替换完整配置。
- 默认词表继承 SeaTunnel，并补充常见变体、HTTP 认证 Header、Cookie、云厂商密钥和 STX 令牌字段。
- 响应携带 `secrets_masked: true` 与命中的配置路径，不返回原值。
- 编辑接口识别受控掩码占位符；未修改的占位符保留服务端旧值，新值才替换旧秘密。
- 对任务正文、版本快照、提交正文和诊断材料建立同一组测试样例，确保 `--output raw` 也不能绕过服务端脱敏。
