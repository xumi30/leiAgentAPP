export default function Header({ isConnected, showReasoningPanel, onToggleReasoning, onOpenMemo }) {
  return (
    <header className="app-header">
      <div className="header-logo">
        <div className="logo-icon clay-card">
          <span className="logo-text">L</span>
        </div>
        <h1 className="logo-title">LeiAgent</h1>
      </div>

      <div className="header-controls">
        {typeof onOpenMemo === 'function' ? (
          <button type="button" className="header-memo-btn clay-card" onClick={onOpenMemo} title="与 memo_write 共用 data/memo.md">
            <span className="header-memo-btn__icon" aria-hidden>
              📝
            </span>
            <span className="header-memo-btn__text">备忘录</span>
          </button>
        ) : null}

        <div className={`status-indicator clay-card ${isConnected ? 'connected' : 'disconnected'}`}>
          <span className={`status-dot ${isConnected ? 'active' : ''}`}></span>
          <span className="status-text">{isConnected ? '已连接' : '已断开'}</span>
        </div>
      </div>
    </header>
  );
}
