package main

import (
	"fmt"
	"io"
)

// StreamReader 安全流读取器
// 使用 RingBuffer 实现双端缓冲，严格限制内存使用 ≤ 8KB
type StreamReader struct {
	// ringBuffer 双端环形缓冲区
	ringBuffer *RingBuffer
}

// NewStreamReader 创建流读取器实例
func NewStreamReader() *StreamReader {
	return &StreamReader{
		ringBuffer: NewRingBuffer(TruncateHeadBytes, TruncateTailBytes),
	}
}

// ReadResult 读取结果结构
type ReadResult struct {
	// Data 读取的数据（已拼接）
	Data []byte
	// TotalRead 总共读取的字节数（原始大小）
	TotalRead int
	// Truncated 是否被截断
	Truncated bool
	// OmittedBytes 被丢弃的字节数
	OmittedBytes int
}

// ReadAll 从 reader 读取全部数据到 RingBuffer
// 使用 io.Copy 流式传输，防止内存泄漏
func (r *StreamReader) ReadAll(reader io.Reader) (*ReadResult, error) {
	// 使用 io.Copy 流式写入 RingBuffer
	n, err := io.Copy(r.ringBuffer, reader)

	result := &ReadResult{
		TotalRead: int(n),
	}

	if err != nil && err != io.EOF {
		return result, fmt.Errorf("read error: %w", err)
	}

	// 获取拼接后的数据
	result.Data = r.ringBuffer.Bytes()
	result.TotalRead = r.ringBuffer.TotalWritten()

	// 判断是否截断
	if result.TotalRead > MaxOutputBytes {
		result.Truncated = true
		result.OmittedBytes = result.TotalRead - len(result.Data)
	}

	return result, nil
}

// ReadWithLimit 带限制的读取（兼容旧接口）
// 使用简单缓冲区，适合小数据量场景
func (r *StreamReader) ReadWithLimit(reader io.Reader, limit int) ([]byte, error) {
	limitedReader := &io.LimitedReader{
		R: reader,
		N: int64(limit),
	}

	var buf []byte
	chunk := make([]byte, 4096)
	for {
		n, err := limitedReader.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read error: %w", err)
		}
		if limitedReader.N <= 0 {
			break
		}
	}

	return buf, nil
}

// SafeReadString 安全读取并返回字符串
func (r *StreamReader) SafeReadString(reader io.Reader) (string, error) {
	result, err := r.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(result.Data), nil
}

// GetRingBuffer 获取底层环形缓冲区（用于后台模式持续写入）
func (r *StreamReader) GetRingBuffer() *RingBuffer {
	return r.ringBuffer
}

// Reset 重置读取器
func (r *StreamReader) Reset() {
	r.ringBuffer.Reset()
}

// NewStreamReaderWithSize 创建指定大小的流读取器
func NewStreamReaderWithSize(headSize, tailSize int) *StreamReader {
	return &StreamReader{
		ringBuffer: NewRingBuffer(headSize, tailSize),
	}
}
