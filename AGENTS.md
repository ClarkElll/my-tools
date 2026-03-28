# AGENTS.md

本文件为进入本仓库工作的代码代理提供上下文与执行约定。

## 仓库定位

- 这是一个 Go 工具集合仓库，每个工具都是独立的可执行程序。
- 当前模块名是 `github.com/ClarkElll/my-tools`。
- Go 版本以 `go.mod` 为准，目前是 `go 1.26.1`。

## 目录约定

- `app/<tool-name>`: 单个工具的入口目录，通常从 `main.go` 启动。
- `internal/`: 多个工具共享的内部代码，不对仓库外暴露。
- `scripts/`: 开发辅助脚本，目前包含新工具脚手架。
- `.cache/`: 本地构建与模块缓存目录，由 `Makefile` 使用。

## 开发命令

- `make fmt`: 格式化全部 Go 代码。
- `make test`: 运行全部测试。
- `make build`: 编译全部工具。
- `make run-example`: 运行示例工具。
- `make new-tool TOOL=<name>`: 生成一个新的工具目录。

## 新增工具时的要求

- 新工具必须放在 `app/<tool-name>` 下。
- 工具名应匹配 `^[a-z0-9][a-z0-9-]*$`，与 `scripts/new-tool.sh` 保持一致。
- 命令行入口优先沿用当前模式：
  - 使用标准库 `flag`。
  - 使用 `flag.NewFlagSet(...)` 创建独立 flag 集。
  - 将帮助输出接入 `internal/cliutil.NewUsage`，保持仓库内 CLI 风格一致。
- 如果逻辑会被多个工具复用，抽到 `internal/`，不要在多个 `app/` 目录复制实现。

## 修改代码时的约定

- 保持工具之间相互独立，避免把某个工具的业务逻辑直接耦合到另一个工具目录。
- 优先继续使用标准库；引入第三方依赖前，先确认标准库无法合理满足需求。
- 新增用户可见行为时，同步更新 `README.md` 中的使用方式或示例命令。
- 保持帮助文案、工具描述和 `Invocation` 字段准确，避免复制脚手架后遗留占位文本。

## 日志与指标约定

- 运行期日志统一使用 `internal/logutil` 创建基于 `log/slog` 的结构化 logger；不要在运行路径里用 `fmt.Print*` 充当日志。
- CLI 工具入口创建 logger 后，至少附带 `tool=<tool-name>` 字段，并把 logger 传给内部运行逻辑复用。
- Prometheus / VictoriaMetrics 指标统一使用 `github.com/VictoriaMetrics/metrics`，公共暴露逻辑放在 `internal/metricsutil`，避免各工具重复手写 exporter。
- 指标名统一使用 `my_tools_<tool-name-with-underscores>_<metric>` 前缀；累计量优先使用 `_total` 后缀，时延优先使用 `_seconds` 命名。
- 工具暴露指标时，HTTP 路径默认使用 `/metrics`；新增相关 flag、端口或路径后，需要同步更新 `README.md` 与帮助文案。

## 验证要求

- 提交前至少运行 `make fmt` 和 `make test`。
- 新增或修改 CLI 工具后，至少补一次命令行冒烟验证，例如：
  - `go run ./app/example -help`
  - `go run ./app/<tool-name> ...`

## 当前代码现状

- `app/example` 是参考实现，展示了参数解析、默认值处理和统一帮助输出。
- `app/remote-write-bench` 是一个 Prometheus remote write 压测工具，支持 mock 时序数据并按配置持续写入。
- `internal/cliutil` 已有测试，新增公共 CLI 行为时优先在这里扩展并补测试。
