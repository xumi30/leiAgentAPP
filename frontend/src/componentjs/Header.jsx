export default function Header({ isConnected, showReasoningPanel, onToggleReasoning }) {
  return (
    <header className="app-header">
      <div className="header-logo">
        <div className="logo-icon clay-card">
          <span className="logo-text">L</span>
        </div>
        <h1 className="logo-title">LeiAgent</h1>
      </div>
      
      <div className="header-controls">

        <div className={`status-indicator clay-card ${isConnected ? 'connected' : 'disconnected'}`}>
          <span className={`status-dot ${isConnected ? 'active' : ''}`}></span>
          <span className="status-text">{isConnected ? '已连接' : '已断开'}</span>
        </div>
      </div>
    </header>
  );
};
