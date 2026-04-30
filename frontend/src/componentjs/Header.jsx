import appIcon from '../../../build/appicon.png';
import '../componentcss/Header.css';

function shortConnectionLabel(loading, status) {
  if (loading) return '检测中…';
  if (!status) return '未检测';
  if (status.ok) {
    if (status.phase === 'connected') return '已连接';
    return '配置正常';
  }
  if (status.phase === 'no_config') return '未配置';
  return '请登录';
}

export default function Header({
  onOpenMemo,
  onOpenScheduledTasks,
  onOpenDocLibrary,
  onOpenLocalMemory,
  onOpenUserProfile,
  connectionLoading,
  connectionStatus,
  onRefreshConnection,
  onOpenSettings,
  thinkingDisabled,
  onThinkingDisabledChange,
}) {
  const neverProbed = !connectionLoading && !connectionStatus;
  const ok = !connectionLoading && connectionStatus?.ok;
  const label = shortConnectionLabel(connectionLoading, connectionStatus);
  const detail = connectionStatus?.message ?? '';
  const badgeTitle = (() => {
    if (connectionLoading) return label;
    if (!connectionStatus) return '未检测 · 点击触发一次探测（以发消息时的实际结果为准）';
    if (connectionStatus.ok && connectionStatus.phase === 'connected') {
      const tail = '点击刷新连通性探测';
      return detail ? `${detail} · ${tail}` : tail;
    }
    return detail || label;
  })();

  return (
    <header className="app-header">
      <div className="header-logo">
        <div className="logo-icon clay-card">
          <img src={appIcon} alt="" className="logo-img" width={40} height={40} />
        </div>
        <h1 className="logo-title">LeiAgent</h1>
      </div>

      <div className="header-controls">
        {/* 暂时停用“关闭思考”开关，保留代码以便后续恢复。
        <label className="header-thinking-toggle clay-card" title="勾选后隐藏推理面板，且请求不再携带思考/推理参数">
          <input
            type="checkbox"
            checked={!!thinkingDisabled}
            onChange={(e) => onThinkingDisabledChange?.(e.target.checked)}
          />
          <span className="header-thinking-toggle__text">关闭思考</span>
        </label>
        */}

        {typeof onOpenSettings === 'function' ? (
          <button
            type="button"
            className="header-settings-btn clay-card"
            onClick={onOpenSettings}
            title="打开设置"
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

        {typeof onOpenUserProfile === 'function' ? (
          <button
            type="button"
            className="header-localmemory-btn clay-card"
            onClick={onOpenUserProfile}
            title="查看这台应用长期沉淀的用户画像，以及当前会话带来的补充证据"
          >
            <span className="header-localmemory-btn__icon" aria-hidden>
              👤
            </span>
            <span className="header-localmemory-btn__text">画像</span>
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

        {typeof onOpenScheduledTasks === 'function' ? (
          <button
            type="button"
            className="header-memo-btn clay-card"
            onClick={onOpenScheduledTasks}
            title="查看定时任务列表"
          >
            <span className="header-memo-btn__icon" aria-hidden>
              ⏰
            </span>
            <span className="header-memo-btn__text">定时任务</span>
          </button>
        ) : null}

        <div className="header-connection">
          <div
            className={`status-indicator clay-card ${connectionLoading ? 'checking' : neverProbed ? 'idle' : ok ? 'connected' : 'disconnected'}`}
            title={badgeTitle}
            role={typeof onRefreshConnection === 'function' ? 'button' : undefined}
            tabIndex={typeof onRefreshConnection === 'function' ? 0 : undefined}
            onClick={onRefreshConnection}
            onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onRefreshConnection?.(); } }}
          >
            <span className={`status-dot ${connectionLoading ? 'pending' : neverProbed ? 'neutral' : ok ? 'active' : 'error'}`} />
            <span className="status-text">{label}</span>
            <span className="status-refresh-icon" aria-hidden>↻</span>
          </div>
        </div>
      </div>
    </header>
  );
}
