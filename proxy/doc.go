// Package proxy provides a small, self-contained, production-grade
// multi-backend LLM HTTP client.
//
// Design goals:
// - Read config from config/config.yaml when enable_llm_config is true
// - Support llm (single backend) and llm_backends (ordered failover)
// - Provide environment variable overrides for single-backend mode
// - Fall back to config/defaultBackend.yaml when config is disabled/missing
// - Expose only New(...) and (*Client).DoRequest(...)
package proxy
