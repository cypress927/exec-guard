package main

import (
	"fmt"
	"sync"
)

// RingBuffer 双端环形缓冲区
// 实现 Head-Tail 双缓冲策略，防止内存溢出
// - Head: 固定 4KB，保留初始输出（根因证据）
// - Tail: 4KB 环形队列，保留最新输出（最新状态）
// - 中间数据：自动丢弃
type RingBuffer struct {
	mu sync.Mutex

	// headBuffer 头部固定缓冲区（前 4KB）
	headBuffer []byte
	// headFull headBuffer 是否已满
	headFull bool

	// tailBuffer 尾部环形缓冲区（后 4KB）
	tailBuffer []byte
	// tailStart 环形队列起始位置
	tailStart int
	// tailEnd 环形队列结束位置
	tailEnd int
	// tailFull tailBuffer 是否已满（循环覆盖过）
	tailFull bool

	// totalWritten 总共写入的字节数（用于计算丢弃量）
	totalWritten int
	// headSize 头部缓冲区大小
	headSize int
	// tailSize 尾部缓冲区大小
	tailSize int
}

// NewRingBuffer 创建双端环形缓冲区
func NewRingBuffer(headSize, tailSize int) *RingBuffer {
	return &RingBuffer{
		headBuffer: make([]byte, 0, headSize),
		tailBuffer: make([]byte, tailSize),
		headSize:   headSize,
		tailSize:   tailSize,
	}
}

// Write 实现 io.Writer 接口
// 流式写入数据，自动管理 Head-Tail 缓冲
func (rb *RingBuffer) Write(p []byte) (n int, err error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	n = len(p)
	rb.totalWritten += n

	// 逐字节写入以正确处理边界
	for _, b := range p {
		// 优先写入 Head
		if len(rb.headBuffer) < rb.headSize {
			rb.headBuffer = append(rb.headBuffer, b)
			if len(rb.headBuffer) == rb.headSize {
				rb.headFull = true
			}
		} else {
			// Head 已满，写入 Tail 环形队列
			rb.tailBuffer[rb.tailEnd] = b
			rb.tailEnd = (rb.tailEnd + 1) % rb.tailSize

			// 如果追上了起始位置，移动起始位置（丢弃最旧的数据）
			if rb.tailFull || rb.tailEnd == rb.tailStart {
				rb.tailStart = (rb.tailStart + 1) % rb.tailSize
				rb.tailFull = true
			}
		}
	}

	return n, nil
}

// Bytes 返回拼接后的完整数据
// Head + [截断提示] + Tail
func (rb *RingBuffer) Bytes() []byte {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	// 计算 Tail 中的有效数据
	var tailData []byte
	if rb.tailFull {
		// Tail 已满，从 tailStart 到 tailEnd 是有效数据
		if rb.tailEnd > rb.tailStart {
			tailData = append([]byte(nil), rb.tailBuffer[rb.tailStart:rb.tailEnd]...)
		} else if rb.tailEnd < rb.tailStart {
			// 环形跨越边界
			tailData = append([]byte(nil), rb.tailBuffer[rb.tailStart:]...)
			tailData = append(tailData, rb.tailBuffer[:rb.tailEnd]...)
		} else {
			// tailStart == tailEnd 且 full，说明整个 buffer 都是有效数据
			tailData = append([]byte(nil), rb.tailBuffer...)
		}
	} else {
		// Tail 未满，从 0 到 tailEnd 是有效数据
		tailData = append([]byte(nil), rb.tailBuffer[:rb.tailEnd]...)
	}

	// 如果 Head 未满且 Tail 为空，直接返回 Head
	if len(rb.headBuffer) < rb.headSize && len(tailData) == 0 {
		return append([]byte(nil), rb.headBuffer...)
	}

	// 需要拼接 Head 和 Tail
	// 计算被丢弃的字节数
	omittedBytes := rb.totalWritten - len(rb.headBuffer) - len(tailData)
	if omittedBytes < 0 {
		omittedBytes = 0
	}

	// 构建结果
	var result []byte
	result = append(result, rb.headBuffer...)

	if omittedBytes > 0 {
		placeholder := fmt.Sprintf(TruncatePlaceholder, omittedBytes)
		result = append(result, []byte(placeholder)...)
	}

	result = append(result, tailData...)

	return result
}

// String 返回字符串形式
func (rb *RingBuffer) String() string {
	return string(rb.Bytes())
}

// TotalWritten 返回总共写入的字节数
func (rb *RingBuffer) TotalWritten() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.totalWritten
}

// HeadLen 返回头部缓冲区长度
func (rb *RingBuffer) HeadLen() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return len(rb.headBuffer)
}

// TailLen 返回尾部缓冲区有效数据长度
func (rb *RingBuffer) TailLen() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.tailFull {
		if rb.tailEnd > rb.tailStart {
			return rb.tailEnd - rb.tailStart
		} else if rb.tailEnd < rb.tailStart {
			return rb.tailSize - rb.tailStart + rb.tailEnd
		} else {
			return rb.tailSize
		}
	}
	return rb.tailEnd
}

// Reset 重置缓冲区
func (rb *RingBuffer) Reset() {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.headBuffer = rb.headBuffer[:0]
	rb.headFull = false
	rb.tailStart = 0
	rb.tailEnd = 0
	rb.tailFull = false
	rb.totalWritten = 0
}

// Clone 克隆当前缓冲区内容
func (rb *RingBuffer) Clone() *RingBuffer {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	clone := &RingBuffer{
		headBuffer:   append([]byte(nil), rb.headBuffer...),
		tailBuffer:   append([]byte(nil), rb.tailBuffer...),
		headFull:     rb.headFull,
		tailStart:    rb.tailStart,
		tailEnd:      rb.tailEnd,
		tailFull:     rb.tailFull,
		totalWritten: rb.totalWritten,
		headSize:     rb.headSize,
		tailSize:     rb.tailSize,
	}

	return clone
}
