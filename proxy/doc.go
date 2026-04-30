// Package proxy provides a small, self-contained, production-grade
// multi-backend LLM HTTP client.
//
// Design goals:
// - Read merged LLM backends from config/config.yaml when the file contains llm / llm_backends.
// - Support llm (single backend) and llm_backends (ordered failover)
// - Provide environment variable overrides for single-backend mode
// - Require config/config.yaml with LLM enabled when loading configuration
// - Expose only New(...) and (*Client).DoRequest(...)
package proxy
