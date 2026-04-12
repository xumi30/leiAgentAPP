package memory

import (
	"context"
	"strings"
	"sync"

	"leiAgent/logging"
)

// AutoCompressEveryAssistantTurns 每累计多少次「助手带正文的回复」后触发一次记忆压缩（与 AddAssistantContentMessage 对齐）。
const AutoCompressEveryAssistantTurns = 10

// AutoCompressYAMLMessageThreshold 从 YAML 载入记忆时，若消息条数 **超过** 该值（即 >= 阈值+1）则触发一次压缩。
// 例如设为 20 表示超过 20 条（21 条及以上）时压缩。
const AutoCompressYAMLMessageThreshold = 20

var (
	autoCompressHook   func(context.Context, string)
	autoCompressHookMu sync.RWMutex
	autoCompressState  sync.Map // map[string]*compressRunState
)

type compressRunState struct {
	mu      sync.Mutex
	running bool
	rerun   bool
}

// SetAutoCompressHook 注册满轮次后的压缩逻辑（由上层注入，避免 memory 包依赖 proxy/memoryagent）。
// 典型实现：调用 memoryagent.Compress。可为 nil 表示关闭自动压缩。
func SetAutoCompressHook(h func(context.Context, string)) {
	autoCompressHookMu.Lock()
	defer autoCompressHookMu.Unlock()
	autoCompressHook = h
}

func getCompressRunState(chatID string) *compressRunState {
	if v, ok := autoCompressState.Load(chatID); ok {
		return v.(*compressRunState)
	}
	state := &compressRunState{}
	actual, _ := autoCompressState.LoadOrStore(chatID, state)
	return actual.(*compressRunState)
}

// invokeAutoCompressHook 异步调用已注册的压缩逻辑，并对同一 chatID 做去重/补跑控制。
func invokeAutoCompressHook(chatID string) {
	cid := strings.TrimSpace(chatID)
	if cid == "" {
		return
	}
	autoCompressHookMu.RLock()
	h := autoCompressHook
	autoCompressHookMu.RUnlock()
	if h == nil {
		return
	}

	state := getCompressRunState(cid)
	state.mu.Lock()
	if state.running {
		state.rerun = true
		state.mu.Unlock()
		logging.Info("自动记忆压缩已在进行，标记补跑 chatID=%s", cid)
		return
	}
	state.running = true
	state.rerun = false
	state.mu.Unlock()

	go func() {
		for {
			func() {
				defer func() {
					if r := recover(); r != nil {
						logging.Error("自动记忆压缩 panic chatID=%s: %v", cid, r)
					}
				}()
				h(context.Background(), cid)
			}()

			state.mu.Lock()
			if state.rerun {
				state.rerun = false
				state.mu.Unlock()
				logging.Info("自动记忆压缩补跑 chatID=%s", cid)
				continue
			}
			state.running = false
			state.mu.Unlock()
			return
		}
	}()
}

// afterAssistantContentTurn 在成功追加一条 assistant 正文消息后调用（异步触发压缩，避免阻塞用户链路）。
func (m *localMemory) afterAssistantContentTurn(chatID string) {
	cid := strings.TrimSpace(chatID)
	if cid == "" {
		return
	}
	m.compressTurnMu.Lock()
	m.assistantReplyTurns[cid]++
	n := m.assistantReplyTurns[cid]
	if n < AutoCompressEveryAssistantTurns {
		m.compressTurnMu.Unlock()
		return
	}
	m.assistantReplyTurns[cid] = 0
	m.compressTurnMu.Unlock()

	invokeAutoCompressHook(cid)
}
