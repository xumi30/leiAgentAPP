function shortConnectionLabel(loading, status) {
  if (loading) return '检测中…';
  if (!status) return '未知';
  if (status.ok) {
    if (status.phase === 'connected') return '已连接';
    return '配置正常';
  }
  if (status.phase === 'no_config') return '未配置';
  return '不可用';
}

export default function Header({
  onOpenMemo,
  onOpenDocLibrary,
  onOpenLocalMemory,
  connectionLoading,
  connectionStatus,
  onRefreshConnection,
  onOpenSettings,
  thinkingDisabled,
  onThinkingDisabledChange,
}) {
  const ok = !connectionLoading && connectionStatus?.ok;
  const label = shortConnectionLabel(connectionLoading, connectionStatus);
  const detail = connectionStatus?.message ?? '';

  return (
    <header className="app-header">
      <div className="header-logo">
        <div className="logo-icon clay-card">
          <span className="logo-text">L</span>
        </div>
        <h1 className="logo-title">LeiAgent</h1>
      </div>

      <div className="header-controls">
        <label className="header-thinking-toggle clay-card" title="勾选后隐藏推理面板，且请求不再携带思考/推理参数">
          <input
            type="checkbox"
            checked={!!thinkingDisabled}
            onChange={(e) => onThinkingDisabledChange?.(e.target.checked)}
          />
          <span className="header-thinking-toggle__text">关闭思考</span>
        </label>

        {typeof onOpenSettings === 'function' ? (
          <button
            type="button"
            className="header-settings-btn clay-card"
            onClick={onOpenSettings}
            title="编辑 LLM 配置"
          >
            <span className="header-settings-btn__icon" aria-hidden>
              ⚙
            </span>
            <span className="header-settings-btn__text">设置</span>
          </button>
        ) : null}

        {typeof onOpenDocLibrary === 'function' ? (
          <button
            type="button"
            className="header-library-btn clay-card"
            onClick={onOpenDocLibrary}
            title="查看助手写入与对话中出现的文档"
          >
            <span className="header-library-btn__icon" aria-hidden>
              📚
            </span>
            <span className="header-library-btn__text">文库</span>
          </button>
        ) : null}

        {typeof onOpenLocalMemory === 'function' ? (
          <button
            type="button"
            className="header-localmemory-btn clay-card"
            onClick={onOpenLocalMemory}
            title="查看当前对话的 localMemory（LLM 上下文）"
          >
            <span className="header-localmemory-btn__icon" aria-hidden>
              🧠
            </span>
            <span className="header-localmemory-btn__text">本地记忆</span>
          </button>
        ) : null}

        {typeof onOpenMemo === 'function' ? (
          <button type="button" className="header-memo-btn clay-card" onClick={onOpenMemo} title="与 memo_write 共用 data/memo.md">
            <span className="header-memo-btn__icon" aria-hidden>
              📝
            </span>
            <span className="header-memo-btn__text">备忘录</span>
          </button>
        ) : null}

        <div className="header-connection">
          <div
            className={`status-indicator clay-card ${connectionLoading ? 'checking' : ok ? 'connected' : 'disconnected'}`}
            title={detail || label}
          >
            <span className={`status-dot ${connectionLoading ? 'pending' : ok ? 'active' : 'error'}`} />
            <span className="status-text">{label}</span>
          </div>
          {typeof onRefreshConnection === 'function' ? (
            <button
              type="button"
              className="header-refresh-btn"
              onClick={onRefreshConnection}
              disabled={connectionLoading}
              title="重新检测连接"
              aria-label="重新检测连接"
            >
              ↻
            </button>
          ) : null}
        </div>
      </div>
    </header>
  );
}
