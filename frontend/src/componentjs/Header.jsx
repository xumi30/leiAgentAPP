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
        <button 
          className={`reasoning-toggle glass-button ${showReasoningPanel ? 'active' : ''}`}
          onClick={onToggleReasoning}
        >
          <span className="toggle-icon">{showReasoningPanel ? '📖' : '📝'}</span>
          <span className="toggle-text">{showReasoningPanel ? '隐藏推理' : '显示推理'}</span>
        </button>
        
        <div className={`status-indicator clay-card ${isConnected ? 'connected' : 'disconnected'}`}>
          <span className={`status-dot ${isConnected ? 'active' : ''}`}></span>
          <span className="status-text">{isConnected ? '已连接' : '已断开'}</span>
        </div>
      </div>
    </header>
  );
};
