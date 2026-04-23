package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

type ctxKeyRequestID struct{}

const ctxCallerSkip = 3

// DebugfCtx 在日志消息中附带请求 ID（若 context 中有）。
func DebugfCtx(ctx context.Context, format string, args ...interface{}) {
	if id := RequestIDFromContext(ctx); id != "" {
		debugWithCallerSkip(ctxCallerSkip, "[req-"+id+"] "+format, args...)
		return
	}
	debugWithCallerSkip(ctxCallerSkip, "[-] "+format, args...)
}

// ContextWithRequestID 把请求 ID 写入 context，供 InfofCtx 等使用。
func ContextWithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyRequestID{}, id)
}

// RequestIDFromContext 读取 ContextWithRequestID 写入的请求 ID。
func RequestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyRequestID{}).(string)
	return v
}

// NextRequestID 生成用于 X-Request-Id 与日志关联的短 ID。
func NextRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}

// InfofCtx 在日志消息中附带请求 ID（若 context 中有）。
func InfofCtx(ctx context.Context, format string, args ...interface{}) {
	if id := RequestIDFromContext(ctx); id != "" {
		infoWithCallerSkip(ctxCallerSkip, "[req-"+id+"] "+format, args...)
		return
	}
	infoWithCallerSkip(ctxCallerSkip, "[-] "+format, args...)
}

// WarnfCtx 在日志消息中附带请求 ID（若 context 中有）。
func WarnfCtx(ctx context.Context, format string, args ...interface{}) {
	if id := RequestIDFromContext(ctx); id != "" {
		warnWithCallerSkip(ctxCallerSkip, "[req-"+id+"] "+format, args...)
		return
	}
	warnWithCallerSkip(ctxCallerSkip, "[-] "+format, args...)
}

// ErrorfCtx 在日志消息中附带请求 ID（若 context 中有）。
func ErrorfCtx(ctx context.Context, format string, args ...interface{}) {
	if id := RequestIDFromContext(ctx); id != "" {
		errorWithCallerSkip(ctxCallerSkip, "[req-"+id+"] "+format, args...)
		return
	}
	errorWithCallerSkip(ctxCallerSkip, "[-] "+format, args...)
}
