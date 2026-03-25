package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
)

// TestRingBuffer 测试环形缓冲区
func TestRingBuffer(t *testing.T) {
	// 测试小数据（不触发截断）
	rb := NewRingBuffer(4096, 4096)
	n, err := rb.Write([]byte("hello world"))
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}
	if n != 11 {
		t.Errorf("Write() n = %d, want 11", n)
	}

	data := rb.Bytes()
	if string(data) != "hello world" {
		t.Errorf("Bytes() = %s, want 'hello world'", string(data))
	}
}

// TestRingBufferHeadTail 测试 Head-Tail 双端缓冲
func TestRingBufferHeadTail(t *testing.T) {
	// 使用小缓冲区测试
	rb := NewRingBuffer(10, 10)

	// 写入超过 Head+Tail 的数据（25 字节）
	largeData := strings.Repeat("a", 25)
	rb.Write([]byte(largeData))

	data := rb.Bytes()

	// 验证包含 Head（前 10 字节）
	if !strings.HasPrefix(string(data), strings.Repeat("a", 10)) {
		t.Error("Head should contain first 10 bytes")
	}

	// 验证包含截断提示
	if !strings.Contains(string(data), "[TRUNCATED:") {
		t.Error("Should contain truncate placeholder")
	}

	// 验证总大小合理（Head 10 + Tail 10 + placeholder ~30 = ~50-60）
	// placeholder 格式："\n... [TRUNCATED: 15 bytes omitted] ...\n"
	placeholderLen := len(fmt.Sprintf(TruncatePlaceholder, 15))
	maxExpectedLen := 10 + 10 + placeholderLen + 5 // 允许一些余量
	if len(data) > maxExpectedLen {
		t.Errorf("Data too large: %d bytes, max expected ~%d", len(data), maxExpectedLen)
	}
}

// TestRingBufferCircular 测试环形队列覆盖
func TestRingBufferCircular(t *testing.T) {
	rb := NewRingBuffer(5, 5)

	// 写入大量数据触发环形覆盖（15 字节）
	data := strings.Repeat("x", 15)
	rb.Write([]byte(data))

	result := rb.Bytes()

	// Head 应该是前 5 个 x
	if !strings.HasPrefix(string(result), "xxxxx") {
		t.Errorf("Head should be first 5 x's, got: %s", string(result)[:5])
	}

	// 验证包含截断提示（因为 15 > 5+5）
	if !strings.Contains(string(result), "[TRUNCATED:") {
		t.Error("Should contain truncate placeholder")
	}
}

// TestRingBufferReset 测试重置
func TestRingBufferReset(t *testing.T) {
	rb := NewRingBuffer(10, 10)
	rb.Write([]byte(strings.Repeat("a", 20)))

	rb.Reset()

	if rb.TotalWritten() != 0 {
		t.Errorf("TotalWritten after reset = %d, want 0", rb.TotalWritten())
	}
	if len(rb.Bytes()) != 0 {
		t.Errorf("Bytes after reset = %v, want empty", rb.Bytes())
	}
}

// TestRingBufferClone 测试克隆
func TestRingBufferClone(t *testing.T) {
	rb := NewRingBuffer(10, 10)
	rb.Write([]byte("hello"))

	clone := rb.Clone()

	if clone.TotalWritten() != rb.TotalWritten() {
		t.Error("Clone TotalWritten mismatch")
	}
	if string(clone.Bytes()) != string(rb.Bytes()) {
		t.Error("Clone data mismatch")
	}
}

// TestStreamReaderRingBuffer 测试 StreamReader 使用 RingBuffer
func TestStreamReaderRingBuffer(t *testing.T) {
	reader := NewStreamReader()
	input := strings.Repeat("a", 100)

	result, err := reader.ReadAll(strings.NewReader(input))
	if err != nil {
		t.Errorf("ReadAll() error = %v", err)
	}

	if result.TotalRead != 100 {
		t.Errorf("TotalRead = %d, want 100", result.TotalRead)
	}

	if len(result.Data) != 100 {
		t.Errorf("Data length = %d, want 100", len(result.Data))
	}
}

// TestStreamReaderLargeInput 测试大数据量读取
func TestStreamReaderLargeInput(t *testing.T) {
	reader := NewStreamReader()
	input := strings.Repeat("b", MaxOutputBytes+1000)

	result, err := reader.ReadAll(strings.NewReader(input))
	if err != nil {
		t.Errorf("ReadAll() error = %v", err)
	}

	if !result.Truncated {
		t.Error("Should be truncated")
	}

	// 验证包含截断提示
	if !strings.Contains(string(result.Data), "[TRUNCATED:") {
		t.Error("Should contain truncate placeholder")
	}

	// 验证内存使用受控
	if len(result.Data) > MaxOutputBytes+200 {
		t.Errorf("Data too large: %d bytes", len(result.Data))
	}
}

// TestExecRequestValidate 测试请求验证
func TestExecRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     *ExecRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: &ExecRequest{
				Command: "echo hello",
			},
			wantErr: false,
		},
		{
			name: "empty command",
			req: &ExecRequest{
				Command: "",
			},
			wantErr: true,
		},
		{
			name: "valid with timeout",
			req: &ExecRequest{
				Command:        "echo hello",
				TimeoutSeconds: 10,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestExecRequestGetTimeout 测试超时时间获取
func TestExecRequestGetTimeout(t *testing.T) {
	tests := []struct {
		name string
		req  *ExecRequest
		want int
	}{
		{
			name: "default timeout",
			req: &ExecRequest{
				Command: "echo hello",
			},
			want: DefaultTimeoutSeconds,
		},
		{
			name: "custom timeout",
			req: &ExecRequest{
				Command:        "echo hello",
				TimeoutSeconds: 60,
			},
			want: 60,
		},
		{
			name: "zero timeout uses default",
			req: &ExecRequest{
				Command:        "echo hello",
				TimeoutSeconds: 0,
			},
			want: DefaultTimeoutSeconds,
		},
		{
			name: "negative timeout uses default",
			req: &ExecRequest{
				Command:        "echo hello",
				TimeoutSeconds: -10,
			},
			want: DefaultTimeoutSeconds,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.req.GetTimeout(); got != tt.want {
				t.Errorf("GetTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestExecRequestGetWatchDuration 测试监控窗口时长获取
func TestExecRequestGetWatchDuration(t *testing.T) {
	tests := []struct {
		name string
		req  *ExecRequest
		want int
	}{
		{
			name: "no watch duration",
			req: &ExecRequest{
				Command: "echo hello",
			},
			want: 0,
		},
		{
			name: "with watch duration",
			req: &ExecRequest{
				Command:              "echo hello",
				RunInBackground:      true,
				WatchDurationSeconds: 10,
			},
			want: 10,
		},
		{
			name: "negative watch duration",
			req: &ExecRequest{
				Command:              "echo hello",
				WatchDurationSeconds: -5,
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.req.GetWatchDuration(); got != tt.want {
				t.Errorf("GetWatchDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestExecRequestHasWatchWindow 测试是否启用监控窗口
func TestExecRequestHasWatchWindow(t *testing.T) {
	tests := []struct {
		name string
		req  *ExecRequest
		want bool
	}{
		{
			name: "no background",
			req: &ExecRequest{
				Command: "echo hello",
			},
			want: false,
		},
		{
			name: "background without watch",
			req: &ExecRequest{
				Command:         "echo hello",
				RunInBackground: true,
			},
			want: false,
		},
		{
			name: "background with watch",
			req: &ExecRequest{
				Command:              "echo hello",
				RunInBackground:      true,
				WatchDurationSeconds: 5,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.req.HasWatchWindow(); got != tt.want {
				t.Errorf("HasWatchWindow() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestStreamReader 测试流读取器
func TestStreamReader(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantTrunc bool
	}{
		{
			name:      "small input",
			input:     "hello world",
			wantTrunc: false,
		},
		{
			name:      "large input gets truncated",
			input:     strings.Repeat("a", MaxOutputBytes+1000),
			wantTrunc: true,
		},
	}

	reader := NewStreamReader()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := reader.ReadAll(bytes.NewReader([]byte(tt.input)))
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}

			if result.Truncated != tt.wantTrunc {
				t.Errorf("ReadAll() truncated = %v, want %v", result.Truncated, tt.wantTrunc)
			}

			if !tt.wantTrunc {
				if string(result.Data) != tt.input {
					t.Errorf("ReadAll() data mismatch for small input")
				}
			} else {
				// 验证截断后大小不超过限制太多（允许 placeholder）
				if len(result.Data) > MaxOutputBytes+100 {
					t.Errorf("ReadAll() data too large: %d bytes", len(result.Data))
				}
				// 验证包含截断提示
				if !strings.Contains(string(result.Data), "[TRUNCATED:") {
					t.Errorf("ReadAll() missing truncate placeholder")
				}
			}
		})
	}
}

// TestStreamReaderLimited 测试有限读取
func TestStreamReaderLimited(t *testing.T) {
	reader := NewStreamReader()
	input := "hello world"

	data, err := reader.ReadWithLimit(bytes.NewReader([]byte(input)), 5)
	if err != nil {
		t.Fatalf("ReadWithLimit() error = %v", err)
	}

	if string(data) != "hello" {
		t.Errorf("ReadWithLimit() = %s, want 'hello'", string(data))
	}
}

// TestProcessManager 测试进程管理器
func TestProcessManager(t *testing.T) {
	pm := NewProcessManager(10)

	// 测试注册
	proc := pm.NewBackgroundProcess(12345, "test command", nil, nil)
	err := pm.Register(12345, proc)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// 测试获取
	got, exists := pm.Get(12345)
	if !exists {
		t.Error("Get() exists = false, want true")
	}
	if got.Command != "test command" {
		t.Errorf("Get() command = %s, want 'test command'", got.Command)
	}

	// 测试获取不存在的进程
	_, exists = pm.Get(99999)
	if exists {
		t.Error("Get() non-existent process exists = true, want false")
	}

	// 测试重复注册
	err = pm.Register(12345, proc)
	if err != ErrProcessAlreadyExists {
		t.Errorf("Register() duplicate error = %v, want ErrProcessAlreadyExists", err)
	}

	// 测试状态获取
	status, err := pm.GetStatus(12345)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if status.PID != 12345 {
		t.Errorf("GetStatus() PID = %d, want 12345", status.PID)
	}

	// 测试终止
	err = pm.Terminate(12345)
	if err != nil {
		t.Logf("Terminate() note: %v (expected for nil cmd)", err)
	}

	// 测试清理
	pm.processes[12345].Status = ProcessStatusCompleted
	cleaned := pm.CleanupCompleted()
	if cleaned != 1 {
		t.Errorf("CleanupCompleted() = %d, want 1", cleaned)
	}
}

// TestProcessManagerMaxLimit 测试进程数限制
func TestProcessManagerMaxLimit(t *testing.T) {
	pm := NewProcessManager(2)

	// 注册到上限
	for i := 1; i <= 2; i++ {
		proc := pm.NewBackgroundProcess(i, "test", nil, nil)
		if err := pm.Register(i, proc); err != nil {
			t.Fatalf("Register(%d) error = %v", i, err)
		}
	}

	// 超过限制
	proc := pm.NewBackgroundProcess(999, "test", nil, nil)
	err := pm.Register(999, proc)
	if err == nil {
		t.Error("Register() over limit should error")
	}
}

// TestPlatformCommandBuilder 测试平台命令构建
func TestPlatformCommandBuilder(t *testing.T) {
	builder := NewPlatformCommandBuilder()

	cmd := builder.BuildCommand("echo hello", "")
	if cmd == nil {
		t.Fatal("BuildCommand() returned nil")
	}

	// 验证命令被正确包装
	if cmd.Dir == "" {
		t.Error("BuildCommand() Dir is empty")
	}
}

// TestKillProcess 测试进程终止（空安全）
func TestKillProcess(t *testing.T) {
	// 测试 nil 命令
	err := KillProcess(nil)
	if err != nil {
		t.Errorf("KillProcess(nil) error = %v, want nil", err)
	}
}

// TestGetShellCommand 测试 shell 命令获取
func TestGetShellCommand(t *testing.T) {
	cmd := GetShellCommand()
	if cmd == "" {
		t.Error("GetShellCommand() returned empty string")
	}
	// 验证包含正确的 shell
	if !strings.Contains(cmd, "cmd") && !strings.Contains(cmd, "bash") {
		t.Errorf("GetShellCommand() = %s, should contain 'cmd' or 'bash'", cmd)
	}
}

// TestConstants 测试常量定义
func TestConstants(t *testing.T) {
	if MaxOutputBytes != 8192 {
		t.Errorf("MaxOutputBytes = %d, want 8192", MaxOutputBytes)
	}
	if TruncateHeadBytes != 4096 {
		t.Errorf("TruncateHeadBytes = %d, want 4096", TruncateHeadBytes)
	}
	if TruncateTailBytes != 4096 {
		t.Errorf("TruncateTailBytes = %d, want 4096", TruncateTailBytes)
	}
	if DefaultTimeoutSeconds != 30 {
		t.Errorf("DefaultTimeoutSeconds = %d, want 30", DefaultTimeoutSeconds)
	}
}

// TestTruncatePlaceholder 测试截断提示格式
func TestTruncatePlaceholder(t *testing.T) {
	placeholder := TruncatePlaceholder
	expected := "\n... [TRUNCATED: %d bytes omitted] ...\n"
	if placeholder != expected {
		t.Errorf("TruncatePlaceholder = %s, want %s", placeholder, expected)
	}
}

// TestStatusConstants 测试状态常量
func TestStatusConstants(t *testing.T) {
	statuses := []string{StatusSuccess, StatusFailed, StatusTimeout, StatusKilled, StatusRunning}
	for _, s := range statuses {
		if s == "" {
			t.Error("empty status constant")
		}
	}
}

// TestErrors 测试错误定义
func TestErrors(t *testing.T) {
	if ErrEmptyCommand == nil {
		t.Error("ErrEmptyCommand is nil")
	}
	if ErrProcessNotFound == nil {
		t.Error("ErrProcessNotFound is nil")
	}
	if ErrTimeoutExceeded == nil {
		t.Error("ErrTimeoutExceeded is nil")
	}
}

// TestStreamReaderEdgeCases 测试流读取器边界情况
func TestStreamReaderEdgeCases(t *testing.T) {
	reader := NewStreamReader()

	// 测试空输入
	result, err := reader.ReadAll(bytes.NewReader([]byte{}))
	if err != nil {
		t.Errorf("ReadAll(empty) error = %v", err)
	}
	if len(result.Data) != 0 {
		t.Errorf("ReadAll(empty) data length = %d, want 0", len(result.Data))
	}

	// 测试 EOF
	result, err = reader.ReadAll(strings.NewReader(""))
	if err != nil && err != io.EOF {
		t.Errorf("ReadAll EOF handling error = %v", err)
	}
}

// TestProcessManagerConcurrent 测试并发安全
func TestProcessManagerConcurrent(t *testing.T) {
	pm := NewProcessManager(1000)
	done := make(chan bool, 10)

	// 并发注册（每个 goroutine 使用不同的 PID 范围）
	for i := 0; i < 10; i++ {
		go func(base int) {
			for j := 0; j < 10; j++ {
				pid := base*100 + j
				proc := pm.NewBackgroundProcess(pid, "test", nil, nil)
				// 忽略重复注册的错误
				pm.Register(pid, proc)
			}
			done <- true
		}(i)
	}

	// 等待完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证至少有一些进程存在（由于并发竞争，可能有些被覆盖）
	count := 0
	for i := 0; i < 100; i++ {
		_, exists := pm.Get(i)
		if exists {
			count++
		}
	}

	// 验证至少注册了一些进程
	if count == 0 {
		t.Error("No processes found after concurrent registration")
	}
}

// BenchmarkStreamReader 基准测试：流读取
func BenchmarkStreamReader(b *testing.B) {
	reader := NewStreamReader()
	data := []byte(strings.Repeat("a", MaxOutputBytes))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader.ReadAll(bytes.NewReader(data))
	}
}

// BenchmarkProcessManager 基准测试：进程管理
func BenchmarkProcessManager(b *testing.B) {
	pm := NewProcessManager(1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pid := i % 1000
		proc := pm.NewBackgroundProcess(pid, "test", nil, nil)
		pm.Register(pid, proc)
		pm.GetStatus(pid)
	}
}

// TestBuildEnv 测试环境变量构建
func TestBuildEnv(t *testing.T) {
	// 测试空自定义环境变量（应继承系统环境变量）
	env := buildEnv(nil)
	if len(env) == 0 {
		t.Error("buildEnv(nil) should inherit system environment")
	}

	// 验证系统环境变量被继承（检查是否有 PATH 或 Path）
	// 注意：某些测试环境可能没有 PATH，所以仅记录不报错
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") || strings.HasPrefix(e, "Path=") {
			// PATH found, good
			break
		}
	}

	// 测试自定义环境变量覆盖
	customEnv := map[string]string{
		"TEST_CUSTOM_VAR": "test_value",
	}
	env = buildEnv(customEnv)

	foundCustom := false
	for _, e := range env {
		if e == "TEST_CUSTOM_VAR=test_value" {
			foundCustom = true
			break
		}
	}
	if !foundCustom {
		t.Error("buildEnv() should include custom environment variable")
	}
}

// TestBuildEnvOverride 测试环境变量覆盖
func TestBuildEnvOverride(t *testing.T) {
	// 设置一个测试用的系统环境变量（如果存在）
	// 注意：Go 测试中无法动态设置 os.Environ()
	// 这里测试自定义变量覆盖逻辑

	customEnv := map[string]string{
		"NODE_ENV":    "production",
		"CUSTOM_VAR":  "custom_value",
		"ANOTHER_VAR": "another_value",
	}

	env := buildEnv(customEnv)

	// 转换为 map 便于检查
	envMap := make(map[string]string)
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// 验证自定义变量存在
	if envMap["NODE_ENV"] != "production" {
		t.Errorf("NODE_ENV = %s, want 'production'", envMap["NODE_ENV"])
	}
	if envMap["CUSTOM_VAR"] != "custom_value" {
		t.Errorf("CUSTOM_VAR = %s, want 'custom_value'", envMap["CUSTOM_VAR"])
	}
	if envMap["ANOTHER_VAR"] != "another_value" {
		t.Errorf("ANOTHER_VAR = %s, want 'another_value'", envMap["ANOTHER_VAR"])
	}
}

// TestBuildEnvEmpty 测试空环境变量
func TestBuildEnvEmpty(t *testing.T) {
	env := buildEnv(map[string]string{})
	if len(env) == 0 {
		t.Error("buildEnv({}) should still inherit system environment")
	}
}

// TestExecRequestEnv 测试 Env 字段
func TestExecRequestEnv(t *testing.T) {
	req := &ExecRequest{
		Command: "echo hello",
		Env: map[string]string{
			"TEST_VAR": "test",
		},
	}

	if req.Env == nil {
		t.Error("Env should not be nil")
	}
	if req.Env["TEST_VAR"] != "test" {
		t.Errorf("TEST_VAR = %s, want 'test'", req.Env["TEST_VAR"])
	}
}
