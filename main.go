package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

// Server HTTP 服务器
type Server struct {
	executor *CommandExecutor
	port     int
}

// NewServer 创建服务器实例
func NewServer(port int, maxProcesses int) *Server {
	processManager := NewProcessManager(maxProcesses)
	return &Server{
		executor: NewCommandExecutor(processManager),
		port:     port,
	}
}

// registerRoutes 注册 HTTP 路由
func (s *Server) registerRoutes() {
	http.HandleFunc("/exec", s.handleExec)
	http.HandleFunc("/process/", s.handleProcess)
	http.HandleFunc("/health", s.handleHealth)
}

// handleExec 处理命令执行请求
// POST /exec
// Body: ExecRequest JSON
func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	resp, err := s.executor.Execute(&req)
	if err != nil {
		s.sendError(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.sendJSON(w, resp, http.StatusOK)
}

// handleProcess 处理进程管理请求
// GET /process/{pid} - 查询进程状态
// GET /process/{pid}/logs - 获取进程日志
// DELETE /process/{pid} - 终止进程
func (s *Server) handleProcess(w http.ResponseWriter, r *http.Request) {
	// 解析路径 /process/{pid}[/logs]
	path := r.URL.Path
	const prefix = "/process/"
	if len(path) <= len(prefix) {
		s.sendError(w, "invalid path", http.StatusBadRequest)
		return
	}

	remainder := path[len(prefix):]
	// 检查是否是 logs 请求
	isLogs := false
	if len(remainder) > 5 && remainder[len(remainder)-5:] == "/logs" {
		isLogs = true
		remainder = remainder[:len(remainder)-5]
	}

	// 解析 PID
	var pid int
	if _, err := fmt.Sscanf(remainder, "%d", &pid); err != nil {
		s.sendError(w, "invalid PID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if isLogs {
			s.handleGetProcessLogs(w, pid)
		} else {
			s.handleGetProcessStatus(w, pid)
		}
	case http.MethodDelete:
		s.handleDeleteProcess(w, pid)
	default:
		s.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetProcessStatus 获取进程状态
func (s *Server) handleGetProcessStatus(w http.ResponseWriter, pid int) {
	status, err := s.executor.GetProcessStatus(pid)
	if err != nil {
		if err == ErrProcessNotFound {
			s.sendError(w, "process not found", http.StatusNotFound)
			return
		}
		s.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.sendJSON(w, status, http.StatusOK)
}

// handleGetProcessLogs 获取进程日志
func (s *Server) handleGetProcessLogs(w http.ResponseWriter, pid int) {
	info, err := s.executor.GetProcessInfo(pid)
	if err != nil {
		if err == ErrProcessNotFound {
			s.sendError(w, "process not found", http.StatusNotFound)
			return
		}
		s.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.sendJSON(w, info, http.StatusOK)
}

// handleDeleteProcess 终止进程
func (s *Server) handleDeleteProcess(w http.ResponseWriter, pid int) {
	err := s.executor.TerminateProcess(pid)
	if err != nil {
		if err == ErrProcessNotFound {
			s.sendError(w, "process not found", http.StatusNotFound)
			return
		}
		s.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.sendJSON(w, map[string]string{
		"status":  "success",
		"message": fmt.Sprintf("process %d terminated", pid),
	}, http.StatusOK)
}

// handleHealth 健康检查端点
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.sendJSON(w, map[string]string{
		"status": "healthy",
	}, http.StatusOK)
}

// sendJSON 发送 JSON 响应
func (s *Server) sendJSON(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

// sendError 发送错误响应
func (s *Server) sendError(w http.ResponseWriter, message string, status int) {
	s.sendJSON(w, map[string]string{
		"error": message,
	}, status)
}

// Run 启动服务器
func (s *Server) Run() error {
	s.registerRoutes()

	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("starting cmd_exec server on %s", addr)

	return http.ListenAndServe(addr, nil)
}

// printUsage 打印帮助信息
func printUsage() {
	fmt.Println(`cmd_exec - AI Agent Command Execution Tool

Usage:
  cmd_exec [options]

Modes:
  CLI Mode (default): Read JSON from stdin, execute command, output JSON result
  Server Mode (-server): Start HTTP server for remote command execution

Options:
  -h, -help       Show this help message
  -server         Run in HTTP server mode
  -port int       HTTP server port (default: 8080)
  -max-processes  Maximum number of background processes (default: 100)

Examples:
  # CLI Mode - execute command from stdin
  echo '{"command": "echo hello"}' | cmd_exec

  # Server Mode - start HTTP server on port 8080
  cmd_exec -server -port 8080

API Endpoints (Server Mode):
  POST   /exec              Execute command
  GET    /process/{pid}     Get process status
  GET    /process/{pid}/logs Get process logs
  DELETE /process/{pid}     Terminate process
  GET    /health            Health check`)
}

// CLI 模式：直接执行命令并输出结果
func runCLI() error {
	// 从 stdin 读取 JSON 请求
	var req ExecRequest
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		return fmt.Errorf("failed to parse input JSON: %w", err)
	}

	// 创建执行器
	processManager := NewProcessManager(100)
	executor := NewCommandExecutor(processManager)

	// 执行命令
	resp, err := executor.Execute(&req)
	if err != nil {
		return err
	}

	// 输出 JSON 响应
	return json.NewEncoder(os.Stdout).Encode(resp)
}

func main() {
	// 命令行参数
	serverMode := flag.Bool("server", false, "run in HTTP server mode")
	port := flag.Int("port", DefaultHTTPPort, "HTTP server port")
	maxProcesses := flag.Int("max-processes", 100, "maximum number of background processes")
	help := flag.Bool("help", false, "show help message")
	helpShort := flag.Bool("h", false, "show help message")
	flag.Parse()

	// 检测帮助参数
	if *help || *helpShort {
		printUsage()
		return
	}

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("shutting down...")
		os.Exit(0)
	}()

	if *serverMode {
		// 服务器模式
		server := NewServer(*port, *maxProcesses)
		if err := server.Run(); err != nil {
			log.Fatalf("server error: %v", err)
		}
	} else {
		// CLI 模式
		if err := runCLI(); err != nil {
			log.Fatalf("execution error: %v", err)
			os.Exit(1)
		}
	}
}
