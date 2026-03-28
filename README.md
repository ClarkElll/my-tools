# my-tools

一个使用 Go 编写的常用工具集合。每个工具都是独立程序，统一放在 `app/<tool-name>` 目录下。

## 要求

- Go 1.26.1+

## 目录约定

- `app/<tool-name>`: 每个工具自己的入口与实现
- `internal/`: 工具之间共享、但不对外暴露的公共代码
- `scripts/`: 开发辅助脚本

## 快速开始

运行示例工具：

```bash
go run ./app/example -name Clark
```

查看帮助：

```bash
go run ./app/example -help
```

常用命令：

```bash
make fmt
make test
make build
make build-remote-write-bench
make docker-build-remote-write-bench
make run-example
```

## remote-write-bench

`remote-write-bench` 用于按固定节奏生成时序点并写入 Prometheus remote write 接口，适合验证 `/api/v1/write` 的写入链路与时序调度行为。

运行帮助：

```bash
go run ./app/remote-write-bench -help
```

示例：

```bash
go run ./app/remote-write-bench \
  -url http://127.0.0.1:8480/insert/0/prometheus/api/v1/write \
  -series 1000 \
  -sample-interval 15s \
  -request-interval 1s \
  -concurrency 2 \
  -max-series-per-request 500 \
  -timeout 3s \
  -duration 1m \
  -utf8-label
```

常用参数：

- `-url`: remote write 地址，必须显式提供
- `-series`: 总共要写入的时序数量
- `-sample-interval`: 同一条时序相邻样本的时间戳间隔；remote write 样本时间戳精度是毫秒，所以最小支持 `1ms`
- `-request-interval`: 真正发送 remote write 请求的调度间隔
- `-concurrency`: 调度保底 worker 数量；当上一次请求还没完成时，用于保证新的调度任务仍能按时开始执行
- `-max-series-per-request`: 单个 write 请求允许携带的最大时序数；默认一次请求发送全部时序
- `-timeout`: 单个 HTTP 请求超时时间
- `-duration`: 运行总时长；`0` 表示一直运行直到中断
- `-utf8-label`: 打开后，会在每条时序上追加固定 UTF-8 label `中文label=value`

行为说明：

- 程序启动后会立刻发送第一轮请求，之后按 `-request-interval` 固定调度
- 程序会按 `-sample-interval` 为每条时序生成样本点，并在每个 `-request-interval` 调度时把到期样本打包发送
- 如果 `series` 大于 `max-series-per-request`，同一个调度时刻会拆成多个 write 请求顺序发送
- 当 `request-interval` 大于 `sample-interval` 时，单个 write 请求会携带多个 sample；当 `request-interval` 小于 `sample-interval` 时，部分调度轮次可能没有新 sample 可发
- 每条时序都会自动带上零填充的 `series_id` label，例如 `series_id-00`、`series_id-01`，便于按字符串顺序区分不同 series
- 当请求执行时间超过 `request-interval` 时，后续调度任务会进入 worker 队列等待执行，不会因为慢请求直接丢弃
- 每个 remote write 请求都会输出一条结构化日志，标明成功或失败，并附带序号、series 数、payload 大小、耗时和 HTTP 状态

构建二进制：

```bash
make build-remote-write-bench
```

默认输出当前主机平台的二进制；也可以显式指定：

```bash
make build-remote-write-bench REMOTE_WRITE_BENCH_GOOS=linux REMOTE_WRITE_BENCH_GOARCH=amd64
```

构建 Docker 镜像：

```bash
make docker-build-remote-write-bench IMAGE=my-tools/remote-write-bench TAG=latest
```

`docker-build-remote-write-bench` 会先按 `PLATFORM` 交叉编译二进制，再执行镜像封装。

## 新增工具

使用脚手架生成一个新工具：

```bash
make new-tool TOOL=demo
```

这会创建 `app/demo/main.go`，你可以在这个目录里继续扩展自己的逻辑。共享逻辑优先抽到 `internal/`，避免在多个工具之间复制代码。
