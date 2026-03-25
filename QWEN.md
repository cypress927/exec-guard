# cmd_exec - AI Agent 核心命令执行模块

## 项目概述

**定位**：为 AI Agent 提供安全、可靠的系统命令执行能力的基础设施模块。

**角色**：资深系统级研发工程师实现的生产级工具。

**核心特性**：
- 严谨的超时控制与进程管理
- 安全的流读取与内存保护（双端环形缓冲）
- 监控窗口模式（watch_duration_seconds）
- 后台进程模式支持
- 跨平台兼容（Windows / Linux / macOS）

---

## 功能规格

### 输入结构（JSON）

```json
{
  "command": "string，必填 - 要执行的系统命令",
  "working_dir": "string，可选 - 工作目录，默认为当前目录",
  "timeout_seconds": "number，可选 - 超时时间（秒），默认 30 秒",
  "run_in_background": "boolean，可选 - 是否后台运行，默认 false",
  "watch_duration_seconds": "number，可选 - 监控窗口时长（秒），仅后台模式有效",
  "env": "object，可选 - 自定义环境变量（覆盖宿主机同名变量）"
}
```

### 输出结构（JSON）

```json
{
  "status": "string - 执行状态：success/failed/timeout/killed/running",
  "exit_code": "number - 进程退出码，-1 表示异常或运行中",
  "stdout": "string - 标准输出（Head-Tail 双端缓冲，最大 8KB）",
  "stderr": "string - 标准错误（Head-Tail 双端缓冲，最大 8KB）",
  "system_message": "string - 系统消息/错误详情"
}
```

---

## 核心行为要求

### 1. 超时控制

- 超过 `timeout_seconds` 必须彻底终止进程
- **Linux/Unix**：使用进程组管理，杀死整个进程组避免孤儿进程
- **Windows**：使用 `CREATE_NEW_PROCESS_GROUP` 标志

### 2. 双端环形缓冲（Head-Tail Ring Buffer）

**内存保护机制**：严禁一次性读取大文件导致内存泄漏。

```
┌─────────────────────────────────────────────────────────────┐
│                    双端缓冲结构 (最大 8KB)                    │
├──────────────────┬─────────────────────┬───────────────────┤
│   Head Buffer    │   丢弃区域           │   Tail Buffer     │
│   (前 4KB)        │   (中间数据)         │   (后 4KB)         │
│                  │                     │                   │
│  保留根因证据     │   自动覆盖丢弃       │   保留最新状态     │
└──────────────────┴─────────────────────┴───────────────────┘
```

- **Head**：固定 4KB 缓冲区，保留程序启动时的初始输出（用于诊断根因）
- **Tail**：4KB 环形队列，循环覆盖保留最新输出（用于监控当前状态）
- **中间数据**：自动丢弃，不占用内存
- **截断提示**：总输出 > 8KB 时，拼接处插入 `\n... [TRUNCATED: XXX bytes omitted] ...\n`

**适用场景**：处理 Java 程序无限抛出异常刷屏、日志风暴等场景。

### 3. 监控窗口模式（watch_duration_seconds）

当 `run_in_background = true` 且设置了 `watch_duration_seconds` 时：

```
进程启动 ──────────────┬──────────────────────> 时间
                       │
                       ▼
              等待 watch_duration_seconds
                       │
           ┌───────────┴───────────┐
           ▼                       ▼
      进程已退出              进程仍在运行
           │                       │
           ▼                       ▼
    返回 status: failed      返回 status: running
         exit_code: X            + 初始化输出日志
         + 完整输出               进程继续后台运行
```

- **N 秒内退出**：视作同步执行失败，返回 `status: "failed"` 及退出码
- **N 秒后仍在运行**：返回 `status: "running"`，附带这 N 秒内收集到的初始化输出日志，进程安全地留在后台运行

### 4. 环境变量继承与覆盖

**默认行为**：子进程继承宿主机（Host）所有环境变量（`os.Environ()`）。

**覆盖规则**：传入的 `env` 对象中的变量会覆盖宿主机同名变量。

```json
{
  "command": "java -jar app.jar",
  "env": {
    "NODE_ENV": "production",
    "PATH": "/usr/local/bin:$PATH",
    "CUSTOM_VAR": "my_value"
  }
}
```

**实现逻辑**：
```
1. 读取宿主机环境变量 → envMap
2. 遍历传入的 env 对象 → 覆盖 envMap 中的键
3. 转换 envMap → []string 赋值给 cmd.Env
```

**注意**：PATH 丢失会导致基础命令（ls, java, go）找不到，建议仅在必要时追加而非完全替换。

### 5. 后台模式支持

当 `run_in_background = true` 时：

- 启动后立即返回进程 PID
- 后台持续捕获输出流（使用 RingBuffer）
- 提供查询接口：
  - `GET /process/{pid}` - 查询进程状态
  - `GET /process/{pid}/logs` - 获取最新日志（Head-Tail 拼接结果）
  - `DELETE /process/{pid}` - 终止进程

### 6. 跨平台兼容

| 平台 | 命令包装方式 | 进程组处理 |
|------|-------------|-----------|
| Windows | `cmd.exe /c <command>` | `CREATE_NEW_PROCESS_GROUP` |
| Linux/macOS | `bash -c "<command>"` | `setpgid` + `Kill(-pgid)` |

---

## 项目结构

```
cmd_exec/
├── main.go              # 程序入口、CLI 参数解析、HTTP 服务
├── types.go             # 输入输出结构体定义
├── constants.go         # 常量定义（8KB、4KB 等）
├── ringbuf.go           # 双端环形缓冲区实现（核心内存保护）
├── stream.go            # 安全流读取器（基于 RingBuffer）
├── platform.go          # 跨平台命令包装（通用代码）
├── platform_unix.go     # Unix/Linux/macOS 特定实现
├── platform_windows.go  # Windows 特定实现
├── background.go        # 后台进程管理、PID 追踪
├── executor.go          # 核心执行逻辑、超时控制、监控窗口
├── go.mod               # Go 模块定义
├── cmd_exec_test.go     # 单元测试套件
└── QWEN.md              # 项目文档
```

---

## 构建与运行

### 环境要求

- Go 1.20+

### 构建命令

```bash
# 编译
go build -o cmd_exec.exe    # Windows
go build -o cmd_exec        # Linux/macOS

# 运行
go run .

# 测试
go test ./... -v

# 格式化
go fmt ./...

# 静态检查
go vet ./...
```

### 运行模式

#### CLI 模式（默认）
从 stdin 读取 JSON 请求，输出 JSON 响应：
```bash
echo '{"command": "echo hello"}' | ./cmd_exec
```

#### HTTP 服务器模式
```bash
./cmd_exec -server -port 8080
```

---

## API 使用示例

### 同步执行

```bash
curl -X POST http://localhost:8080/exec \
  -H "Content-Type: application/json" \
  -d '{"command": "echo hello", "timeout_seconds": 10}'
```

### 后台执行（无监控窗口）

```bash
# 启动后台进程
curl -X POST http://localhost:8080/exec \
  -H "Content-Type: application/json" \
  -d '{"command": "long_running_task", "run_in_background": true}'

# 查询状态
curl http://localhost:8080/process/12345

# 获取日志（Head-Tail 拼接）
curl http://localhost:8080/process/12345/logs

# 终止进程
curl -X DELETE http://localhost:8080/process/12345
```

### 后台执行（带监控窗口）

```bash
# 启动并监控 5 秒
curl -X POST http://localhost:8080/exec \
  -H "Content-Type: application/json" \
  -d '{
    "command": "java -jar app.jar",
    "run_in_background": true,
    "watch_duration_seconds": 5
  }'

# 如果 5 秒内进程退出 → 返回 failed + exit_code
# 如果 5 秒后仍在运行 → 返回 running + 初始化日志
```

### 自定义环境变量

```bash
# 设置 NODE_ENV 和自定义变量
curl -X POST http://localhost:8080/exec \
  -H "Content-Type: application/json" \
  -d '{
    "command": "node app.js",
    "env": {
      "NODE_ENV": "production",
      "API_KEY": "secret123",
      "LOG_LEVEL": "debug"
    }
  }'

# 注意：默认继承宿主机所有环境变量
# 传入的变量会覆盖同名变量（如 PATH）
```

---

## 开发规范

### 代码风格

- 遵循 Go 官方代码风格（`go fmt`）
- 使用 `golint` 或 `golangci-lint` 进行代码审查
- 函数命名采用驼峰式（导出函数首字母大写）

### 错误处理

- 所有错误必须显式处理，禁止忽略
- 使用 `errors.Wrap` 或 `fmt.Errorf` 添加上下文
- 错误信息应清晰、可操作

### 测试要求

- 核心功能必须有单元测试覆盖
- 边界条件测试（超时、大输出、异常命令）
- 跨平台行为一致性验证

### 安全考虑

- 禁止命令注入风险（不直接拼接用户输入到命令）
- 资源泄漏防护（defer 关闭文件/管道）
- 并发安全（后台进程管理器使用互斥锁）
- **内存安全**：RingBuffer 严格限制 8KB，防止 OOM

---

## 核心设计详解

### RingBuffer 双端缓冲实现

```go
type RingBuffer struct {
    headBuffer   []byte  // 前 4KB 固定缓冲
    tailBuffer   []byte  // 后 4KB 环形队列
    tailStart    int     // 环形队列起始位置
    tailEnd      int     // 环形队列结束位置
    totalWritten int     // 总写入字节数
}
```

**写入流程**：
1. 优先写入 Head，直到填满 4KB
2. Head 满后，写入 Tail 环形队列
3. Tail 满后，循环覆盖（丢弃最旧数据）

**读取流程**：
1. 返回 `Head + [TRUNCATED 提示] + Tail`
2. 总大小 ≤ 8KB + 提示长度

---

## 版本历史

| 版本 | 日期 | 变更说明 |
|------|------|----------|
| 0.2.0 | - | 新增监控窗口模式、双端环形缓冲 |
| 0.1.0 | - | 初始版本，核心功能实现 |
