# cmd_exec - AI Agent 命令执行模块

## 项目概述

**cmd_exec** 是一个为 AI Agent 提供安全、可靠系统命令执行能力的基础设施模块。它解决了 AI Agent 在执行系统命令时面临的核心挑战：

- **超时控制**：防止命令无限执行导致资源占用
- **内存保护**：防止大输出导致内存溢出（双端环形缓冲）
- **进程管理**：支持后台进程启动、监控、终止
- **跨平台兼容**：统一接口，Windows/Linux/macOS 无缝切换

---

## 功能特性

### 核心能力

| 特性 | 说明 |
|------|------|
| 同步执行 | 执行命令并等待结果，支持超时控制 |
| 后台执行 | 启动后台进程，支持状态查询和日志获取 |
| 监控窗口 | 后台启动后观察 N 秒，判断是否成功启动 |
| 双端缓冲 | Head-Tail 环形缓冲，最大 8KB，防止 OOM |
| 环境变量 | 继承宿主机环境变量，支持自定义覆盖 |
| 进程组管理 | 杀死进程时连带终止所有子进程 |

### 运行模式

- **CLI 模式**：从标准输入读取 JSON 请求，输出 JSON 响应
- **HTTP 服务模式**：提供 RESTful API，支持远程调用

---

## 安装与构建

### 环境要求

- Go 1.20+

### 构建命令

```bash
# Windows
go build -o cmd_exec.exe

# Linux/macOS
go build -o cmd_exec

# 交叉编译 Linux amd64（在 Windows 上）
set GOOS=linux&& set GOARCH=amd64&& go build -ldflags=-s -ldflags=-w -o cmd_exec_linux .

# 交叉编译 Windows（在 Linux 上）
GOOS=windows GOARCH=amd64 go build -o cmd_exec.exe .
```

### 运行测试

```bash
go test ./... -v
```

---

## 使用方式

### CLI 模式

从标准输入读取 JSON 请求，执行命令后输出 JSON 响应：

```bash
# 基础用法
echo '{"command": "echo hello"}' | ./cmd_exec

# 带超时
echo '{"command": "sleep 5", "timeout_seconds": 3}' | ./cmd_exec

# 后台执行
echo '{"command": "long_task", "run_in_background": true}' | ./cmd_exec

# 自定义环境变量
echo '{"command": "node app.js", "env": {"NODE_ENV": "production"}}' | ./cmd_exec
```

### HTTP 服务模式

启动 HTTP 服务器，提供 RESTful API：

```bash
# 默认端口 8080
./cmd_exec -server

# 自定义端口
./cmd_exec -server -port 9000

# 限制最大后台进程数
./cmd_exec -server -port 8080 -max-processes 50
```

---

## API 文档

### 执行命令

**POST** `/exec`

执行系统命令，支持同步和后台两种模式。

#### 请求参数

```json
{
  "command": "string，必填 - 要执行的系统命令",
  "working_dir": "string，可选 - 工作目录，默认当前目录",
  "timeout_seconds": "number，可选 - 超时时间（秒），默认 30",
  "run_in_background": "boolean，可选 - 是否后台运行，默认 false",
  "watch_duration_seconds": "number，可选 - 监控窗口时长（秒），仅后台模式",
  "env": "object，可选 - 自定义环境变量"
}
```

#### 响应结构

```json
{
  "status": "string - success/failed/timeout/killed/running",
  "exit_code": "number - 进程退出码，-1 表示异常或运行中",
  "stdout": "string - 标准输出（Head-Tail 双端缓冲）",
  "stderr": "string - 标准错误（Head-Tail 双端缓冲）",
  "system_message": "string - 系统消息/错误详情"
}
```

#### 示例

**同步执行**

```bash
curl -X POST http://localhost:8080/exec \
  -H "Content-Type: application/json" \
  -d '{"command": "ls -la", "timeout_seconds": 10}'
```

响应：
```json
{
  "status": "success",
  "exit_code": 0,
  "stdout": "total 32\n...",
  "stderr": "",
  "system_message": "command executed successfully"
}
```

**后台执行**

```bash
curl -X POST http://localhost:8080/exec \
  -H "Content-Type: application/json" \
  -d '{"command": "python train.py", "run_in_background": true}'
```

响应：
```json
{
  "status": "running",
  "exit_code": -1,
  "stdout": "",
  "stderr": "",
  "system_message": "process started with PID 12345"
}
```

**监控窗口模式**

```bash
curl -X POST http://localhost:8080/exec \
  -H "Content-Type: application/json" \
  -d '{
    "command": "java -jar app.jar",
    "run_in_background": true,
    "watch_duration_seconds": 5
  }'
```

- 如果 5 秒内进程退出 → 返回 `status: "failed"` + 退出码 + 输出
- 如果 5 秒后仍在运行 → 返回 `status: "running"` + 初始化日志

---

### 查询进程状态

**GET** `/process/{pid}`

查询后台进程的当前状态。

#### 响应结构

```json
{
  "pid": 12345,
  "status": "running/completed/failed",
  "exit_code": 0,
  "command": "python train.py",
  "start_time": "2024-01-15T10:30:00Z",
  "end_time": "",
  "watch_duration_seconds": 0,
  "watch_completed": false
}
```

#### 示例

```bash
curl http://localhost:8080/process/12345
```

---

### 获取进程日志

**GET** `/process/{pid}/logs`

获取后台进程的完整信息，包含输出日志。

#### 响应结构

```json
{
  "pid": 12345,
  "status": "running",
  "exit_code": -1,
  "command": "python train.py",
  "start_time": "2024-01-15T10:30:00Z",
  "stdout": "Training started...\nEpoch 1: loss=0.5",
  "stderr": ""
}
```

#### 示例

```bash
curl http://localhost:8080/process/12345/logs
```

---

### 终止进程

**DELETE** `/process/{pid}`

终止指定的后台进程及其所有子进程。

#### 响应结构

```json
{
  "status": "success",
  "message": "process 12345 terminated"
}
```

#### 示例

```bash
curl -X DELETE http://localhost:8080/process/12345
```

---

### 健康检查

**GET** `/health`

检查服务是否正常运行。

#### 响应结构

```json
{
  "status": "healthy"
}
```

---

## 核心设计详解

### 双端环形缓冲（Head-Tail Ring Buffer）

**问题背景**：某些程序会无限输出日志（如 Java 异常堆栈刷屏），一次性读取会导致内存溢出。

**解决方案**：使用 Head-Tail 双端缓冲策略，严格限制内存使用 ≤ 8KB。

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

**设计原理**：

| 区域 | 大小 | 作用 |
|------|------|------|
| Head Buffer | 4KB 固定 | 保留程序启动时的初始输出，用于诊断根因 |
| Tail Buffer | 4KB 环形 | 循环覆盖保留最新输出，用于监控当前状态 |
| 中间数据 | 自动丢弃 | 不占用内存，超过 8KB 时自动截断 |

**截断提示**：总输出 > 8KB 时，拼接处插入：

```
... [TRUNCATED: 10240 bytes omitted] ...
```

---

### 监控窗口模式（watch_duration_seconds）

**适用场景**：启动后台服务（如 Java 应用、Web 服务器）时，需要确认是否成功启动。

**工作流程**：

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

**优势**：

- 避免启动失败但无法感知的问题
- 获取初始化阶段的日志（如启动错误、配置问题）
- 确认服务正常运行后再返回

---

### 超时控制与进程组管理

**超时机制**：

- 使用 `context.WithTimeout` 设置超时
- 超时后彻底终止进程及其所有子进程

**进程组管理**：

| 平台 | 实现方式 |
|------|----------|
| Linux/macOS | `setpgid` 创建进程组，`kill(-pgid)` 杀死整个组 |
| Windows | `CREATE_NEW_PROCESS_GROUP` 标志 |

**目的**：防止孤儿进程残留。

---

### 环境变量继承与覆盖

**默认行为**：子进程继承宿主机所有环境变量（`os.Environ()`）。

**覆盖规则**：传入的 `env` 对象中的变量会覆盖宿主机同名变量。

```json
{
  "command": "node app.js",
  "env": {
    "NODE_ENV": "production",
    "API_KEY": "secret123",
    "PATH": "/usr/local/bin:/usr/bin"
  }
}
```

**注意事项**：

- PATH 变量需谨慎覆盖，可能导致基础命令找不到
- 建议追加而非完全替换：`"PATH": "/custom/bin:$PATH"`

---

## 配置参数

### 常量配置

| 常量 | 默认值 | 说明 |
|------|--------|------|
| `DefaultTimeoutSeconds` | 30 | 默认超时时间（秒） |
| `MaxOutputBytes` | 8192 (8KB) | 最大输出字节数 |
| `TruncateHeadBytes` | 4096 (4KB) | Head 缓冲区大小 |
| `TruncateTailBytes` | 4096 (4KB) | Tail 缓冲区大小 |
| `DefaultHTTPPort` | 8080 | HTTP 服务默认端口 |

### 命令行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-server` | 启动 HTTP 服务模式 | false |
| `-port` | HTTP 服务端口 | 8080 |
| `-max-processes` | 最大后台进程数 | 100 |
| `-h, -help` | 显示帮助信息 | - |

---

## 状态码说明

### 执行状态 (status)

| 状态 | 说明 |
|------|------|
| `success` | 命令成功执行，退出码为 0 |
| `failed` | 命令执行失败，退出码非 0 |
| `timeout` | 命令执行超时，进程被终止 |
| `killed` | 进程被手动终止 |
| `running` | 进程正在后台运行 |

### 进程状态

| 状态 | 说明 |
|------|------|
| `running` | 进程正在运行 |
| `completed` | 进程正常完成 |
| `failed` | 进程异常退出或被终止 |

---

## 错误处理

### 预定义错误

| 错误 | 说明 |
|------|------|
| `ErrEmptyCommand` | 命令为空 |
| `ErrProcessNotFound` | 进程未找到 |
| `ErrTimeoutExceeded` | 执行超时 |
| `ErrProcessAlreadyExists` | 进程已存在（PID 冲突） |
| `ErrInvalidWorkingDir` | 工作目录无效 |

### HTTP 错误响应

```json
{
  "error": "process not found"
}
```

| HTTP 状态码 | 说明 |
|-------------|------|
| 400 | 请求参数错误 |
| 404 | 进程未找到 |
| 405 | 方法不允许 |
| 500 | 内部错误 |

---

## 最佳实践

### 1. 选择合适的执行模式

| 场景 | 推荐模式 |
|------|----------|
| 快速命令（ls, echo） | 同步执行 |
| 长时间任务（训练模型） | 后台执行 |
| 服务启动（Web Server） | 后台 + 监控窗口 |

### 2. 设置合理的超时

```json
{
  "command": "npm install",
  "timeout_seconds": 300  // 5 分钟，依赖安装可能较慢
}
```

### 3. 使用监控窗口确认启动

```json
{
  "command": "java -jar app.jar",
  "run_in_background": true,
  "watch_duration_seconds": 10  // 等待 10 秒确认启动
}
```

### 4. 定期清理已完成进程

```bash
# 通过 API 清理（需自行实现清理接口）
# 或设置进程数限制自动拒绝新进程
./cmd_exec -server -max-processes 50
```

### 5. 环境变量谨慎覆盖

```json
{
  "env": {
    "NODE_ENV": "production",
    "LOG_LEVEL": "info"
    // 不要完全覆盖 PATH
  }
}
```

---

## 跨平台兼容性

### Shell 命令包装

| 平台 | 包装方式 |
|------|----------|
| Windows | `cmd.exe /c <command>` |
| Linux/macOS | `bash -c "<command>"` |

### 进程终止方式

| 平台 | 实现方式 |
|------|----------|
| Windows | `CREATE_NEW_PROCESS_GROUP` + `Process.Kill()` |
| Linux/macOS | `setpgid` + `kill(-pgid, SIGKILL)` |

---

## 项目结构

```
cmd_exec/
├── main.go              # 程序入口、CLI 参数、HTTP 服务
├── types.go             # 输入输出结构体定义
├── constants.go         # 常量定义
├── ringbuf.go           # 双端环形缓冲区实现
├── stream.go            # 安全流读取器
├── platform.go          # 跨平台命令构建（通用代码）
├── platform_unix.go     # Unix/Linux/macOS 特定实现
├── platform_windows.go  # Windows 特定实现
├── background.go        # 后台进程管理
├── executor.go          # 核心执行逻辑
├── go.mod               # Go 模块定义
├── cmd_exec_test.go     # 单元测试
├── README.md            # 使用文档
└── QWEN.md              # 项目规格文档
```

---

## 版本历史

| 版本 | 日期 | 变更说明 |
|------|------|----------|
| 0.2.0 | - | 新增监控窗口模式、双端环形缓冲 |
| 0.1.0 | - | 初始版本，核心功能实现 |

---

## 许可证

MIT License