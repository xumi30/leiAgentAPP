import { useState, useEffect, useCallback } from 'react';

import './App.css';

import ConversationList from './componentjs/ConversationList.jsx';
import { ChatDialog } from './features/chat';
import Header from './componentjs/Header.jsx';
import Reasoning from './componentjs/Reasonging.jsx';
import MemoModal from './componentjs/MemoModal.jsx';
import DocLibraryModal from './componentjs/DocLibraryModal.jsx';
import SettingsModal from './componentjs/SettingsModal.jsx';
import LocalMemoryModal from './componentjs/LocalMemoryModal.jsx';
import UserProfileModal from './componentjs/UserProfileModal.jsx';
import ScheduledTasksModal from './componentjs/ScheduledTasksModal.jsx';
import {
  GetLLMConfigFormState,
  GetMemoCalendarDates,
  GetLLMConnectionStatus,
  ProxyAuthRequest,
  SetLLMThinkingDisabled,
} from '../wailsjs/go/main/App';
import { EventsOff, EventsOn } from '../wailsjs/runtime/runtime';

const THINKING_LS_KEY = 'leiAgent.llmThinkingDisabled';
const CONNECTION_POLL_MS = 45000;

function readThinkingDisabledFromLS() {
  const raw = localStorage.getItem(THINKING_LS_KEY);
  if (raw === 'true') return true;
  if (raw === 'false') return false;
  return null;
}

function configNeedsAuthModal(llmState) {
  if (!llmState || llmState.usingExample) return true;
  const rows = Array.isArray(llmState.backends) ? llmState.backends : [];
  if (rows.length === 0) return true;
  return rows.some((row) => {
    const baseUrl = String(row?.baseUrl ?? '').trim();
    const model = String(row?.model ?? '').trim();
    const apiKey = String(row?.apiKey ?? '').trim();
    return !baseUrl || !model || !apiKey;
  });
}

function connectionNeedsAuthModal(status) {
  if (!status || status.ok !== true) return true;
  if (!String(status.configPath ?? '').trim()) return true;
  return false;
}

function parseAPIError(payload, fallback) {
  if (payload && typeof payload === 'object') {
    const message = payload.message
      || (payload.error && typeof payload.error === 'object' ? payload.error.message : null)
      || (typeof payload.error === 'string' ? payload.error : null)
      || payload.detail;
    if (typeof message === 'string' && message.trim()) return message.trim();
  }
  return fallback;
}

function AuthModal({
  open,
  mode,
  onModeChange,
  onClose,
  authBusy,
  authError,
  authNotice,
  loginUsername,
  setLoginUsername,
  loginPassword,
  setLoginPassword,
  registerUsername,
  setRegisterUsername,
  registerEmail,
  setRegisterEmail,
  registerPassword,
  setRegisterPassword,
  registerCode,
  setRegisterCode,
  onSendCode,
  onLogin,
  onRegister,
}) {
  if (!open) return null;

  return (
    <div
      className="auth-modal-overlay"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div
        className="auth-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="auth-modal-title"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <div className="auth-modal__head">
          <div>
            <p id="auth-modal-title" className="auth-modal__title">连接 Proxy-LB</p>
            <p className="auth-modal__desc">连接失败或配置缺失时，可以在这里完成注册/登录并写入当前 LLM 配置。</p>
          </div>
          <button type="button" className="auth-modal__close" onClick={onClose} aria-label="关闭">×</button>
        </div>

        <div className="auth-modal__switch">
          <button type="button" className={`auth-modal__switch-btn${mode === 'login' ? ' auth-modal__switch-btn--active' : ''}`} onClick={() => onModeChange('login')}>
            登录
          </button>
          <button type="button" className={`auth-modal__switch-btn${mode === 'register' ? ' auth-modal__switch-btn--active' : ''}`} onClick={() => onModeChange('register')}>
            注册
          </button>
        </div>

        {mode === 'login' ? (
          <div className="auth-modal__form">
            <label className="auth-modal__field">
              <span className="auth-modal__label">用户名</span>
              <input className="auth-modal__input" value={loginUsername} onChange={(e) => setLoginUsername(e.target.value)} autoComplete="username" />
            </label>
            <label className="auth-modal__field">
              <span className="auth-modal__label">密码</span>
              <input className="auth-modal__input" type="password" value={loginPassword} onChange={(e) => setLoginPassword(e.target.value)} autoComplete="current-password" />
              <span className="auth-modal__hint">至少 8 个字符</span>
            </label>
          </div>
        ) : (
          <div className="auth-modal__form">
            <label className="auth-modal__field">
              <span className="auth-modal__label">用户名</span>
              <input className="auth-modal__input" value={registerUsername} onChange={(e) => setRegisterUsername(e.target.value)} autoComplete="username" />
            </label>
            <label className="auth-modal__field">
              <span className="auth-modal__label">邮箱</span>
              <input className="auth-modal__input" type="email" value={registerEmail} onChange={(e) => setRegisterEmail(e.target.value)} autoComplete="email" />
            </label>
            <label className="auth-modal__field">
              <span className="auth-modal__label">密码</span>
              <input className="auth-modal__input" type="password" value={registerPassword} onChange={(e) => setRegisterPassword(e.target.value)} autoComplete="new-password" />
              <span className="auth-modal__hint">至少 8 个字符</span>
            </label>
            <div className="auth-modal__grid auth-modal__grid--code">
              <label className="auth-modal__field">
                <span className="auth-modal__label">验证码</span>
                <input className="auth-modal__input" value={registerCode} onChange={(e) => setRegisterCode(e.target.value)} autoComplete="one-time-code" />
              </label>
              <button type="button" className="auth-modal__ghost-btn" onClick={onSendCode} disabled={authBusy}>
                发送验证码
              </button>
            </div>
          </div>
        )}

        {authError ? <div className="auth-modal__error">{authError}</div> : null}
        {authNotice ? <div className="auth-modal__notice">{authNotice}</div> : null}

        <div className="auth-modal__actions">
          <button type="button" className="auth-modal__secondary-btn" onClick={onClose}>取消</button>
          <button
            type="button"
            className="auth-modal__primary-btn"
            onClick={mode === 'login' ? onLogin : onRegister}
            disabled={authBusy || (mode === 'login' ? !loginUsername.trim() || !loginPassword.trim() : !registerUsername.trim() || !registerEmail.trim() || !registerPassword.trim() || !registerCode.trim())}
          >
            {authBusy ? '处理中…' : mode === 'login' ? '登录并写入配置' : '注册并写入配置'}
          </button>
        </div>
      </div>
    </div>
  );
}

function App() {
  const [leftWidth, setLeftWidth] = useState(260);
  const [rightWidth, setRightWidth] = useState(360);
  const [isDragging, setIsDragging] = useState(null);

  const [memoOpen, setMemoOpen] = useState(false);
  const [scheduledTasksOpen, setScheduledTasksOpen] = useState(false);
  const [docLibOpen, setDocLibOpen] = useState(false);
  const [docLibFocusPath, setDocLibFocusPath] = useState(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [localMemoryOpen, setLocalMemoryOpen] = useState(false);
  const [userProfileOpen, setUserProfileOpen] = useState(false);
  const [needLoginOpen, setNeedLoginOpen] = useState(false);
  const [activeChatId, setActiveChatId] = useState('');
  const [activeChatTitle, setActiveChatTitle] = useState('');
  /** 助手流式输出进行中（按 chatID），用于侧栏列表显示加载态 */
  const [streamingChatIds, setStreamingChatIds] = useState(() => new Set());
  const [memoDates, setMemoDates] = useState(() => new Set());

  const [thinkingDisabled, setThinkingDisabled] = useState(true);

  const [connectionLoading, setConnectionLoading] = useState(true);
  const [connectionStatus, setConnectionStatus] = useState(null);
  const [authModalOpen, setAuthModalOpen] = useState(false);
  const [authMode, setAuthMode] = useState('login');
  const [authBusy, setAuthBusy] = useState(false);
  const [authError, setAuthError] = useState('');
  const [authNotice, setAuthNotice] = useState('');
  const [loginUsername, setLoginUsername] = useState('');
  const [loginPassword, setLoginPassword] = useState('');
  const [registerUsername, setRegisterUsername] = useState('');
  const [registerEmail, setRegisterEmail] = useState('');
  const [registerPassword, setRegisterPassword] = useState('');
  const [registerCode, setRegisterCode] = useState('');

  const fetchConnectionStatus = useCallback(async () => {
    try {
      const s = await GetLLMConnectionStatus();
      return s && typeof s === 'object' ? { ...s } : s;
    } catch (e) {
      console.error('GetLLMConnectionStatus:', e);
      return {
        ok: false,
        phase: 'client_error',
        message: String(e?.message || e),
        configPath: '',
      };
    }
  }, []);

  const refreshConnection = useCallback(async () => {
    setConnectionLoading(true);
    try {
      const status = await fetchConnectionStatus();
      setConnectionStatus(status);
      return status;
    } finally {
      setConnectionLoading(false);
    }
  }, [fetchConnectionStatus]);

  const prepareAuthModal = useCallback(async (llmStateArg = null) => {
    setAuthError('');
    setAuthNotice('');
    setAuthModalOpen(true);
  }, []);

  const performAuthRequest = useCallback(async (path, payload, successText) => {
    setAuthBusy(true);
    setAuthError('');
    setAuthNotice('');
    try {
      const data = await ProxyAuthRequest(path, payload);
      const statusCode = Number(data?._statusCode ?? 0);
      if (statusCode < 200 || statusCode >= 300) {
        throw new Error(parseAPIError(data, `请求失败（HTTP ${statusCode}）`));
      }
      setAuthNotice(successText);
      setAuthModalOpen(false);
      await refreshConnection();
    } catch (e) {
      setAuthError(String(e?.message || e || '请求失败'));
    } finally {
      setAuthBusy(false);
    }
  }, [refreshConnection]);

  const handleSendRegisterCode = useCallback(async () => {
    setAuthBusy(true);
    setAuthError('');
    setAuthNotice('');
    try {
      const data = await ProxyAuthRequest('/auth/register/send-code', {
        username: String(registerUsername ?? '').trim(),
        email: String(registerEmail ?? '').trim(),
      });
      const statusCode = Number(data?._statusCode ?? 0);
      if (statusCode < 200 || statusCode >= 300) {
        throw new Error(parseAPIError(data, `发送验证码失败（HTTP ${statusCode}）`));
      }
      setAuthNotice(parseAPIError(data, '验证码已发送，请查收邮箱。'));
    } catch (e) {
      setAuthError(String(e?.message || e || '发送验证码失败'));
    } finally {
      setAuthBusy(false);
    }
  }, [registerUsername, registerEmail]);

  const handleLoginSubmit = useCallback(async () => {
    await performAuthRequest('/auth/login', {
      username: String(loginUsername ?? '').trim(),
      password: String(loginPassword ?? ''),
    }, '登录成功，已写入配置。');
  }, [loginUsername, loginPassword, performAuthRequest]);

  const handleRegisterSubmit = useCallback(async () => {
    await performAuthRequest('/auth/register', {
      username: String(registerUsername ?? '').trim(),
      password: String(registerPassword ?? ''),
      email: String(registerEmail ?? '').trim(),
      code: String(registerCode ?? '').trim(),
    }, '注册成功，已写入配置。');
  }, [registerUsername, registerPassword, registerEmail, registerCode, performAuthRequest]);

  const handleHeaderRefresh = useCallback(async () => {
    setConnectionLoading(true);
    try {
      const [status, llmState] = await Promise.all([
        fetchConnectionStatus(),
        GetLLMConfigFormState().catch(() => null),
      ]);
      setConnectionStatus(status);
      if (connectionNeedsAuthModal(status) || configNeedsAuthModal(llmState)) {
        await prepareAuthModal(llmState);
      }
    } finally {
      setConnectionLoading(false);
    }
  }, [fetchConnectionStatus, prepareAuthModal]);

  const refreshMemoDates = useCallback(async () => {
    try {
      const arr = await GetMemoCalendarDates();
      setMemoDates(new Set(Array.isArray(arr) ? arr : []));
    } catch (e) {
      console.error('GetMemoCalendarDates:', e);
    }
  }, []);

  useEffect(() => {
    refreshMemoDates();
  }, [refreshMemoDates]);

  useEffect(() => {
    refreshConnection();
  }, [refreshConnection]);

  useEffect(() => {
    const id = setInterval(() => {
      refreshConnection();
    }, CONNECTION_POLL_MS);
    return () => clearInterval(id);
  }, [refreshConnection]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        // 暂时停用“关闭思考”开关后，前端统一回落为默认关闭思考逻辑。
        if (!cancelled) {
          setThinkingDisabled(true);
          localStorage.setItem(THINKING_LS_KEY, 'true');
        }
        await SetLLMThinkingDisabled(true);
      } catch (e) {
        console.error('SetLLMThinkingDisabled:', e);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (thinkingDisabled && isDragging === 'right') {
      setIsDragging(null);
    }
  }, [thinkingDisabled, isDragging]);

  useEffect(() => {
    const onConv = (e) => {
      const d = e.detail || {};
      setActiveChatId(d.conversationId ?? '');
      setActiveChatTitle(d.title ?? '');
    };
    window.addEventListener('conversationChanged', onConv);
    return () => window.removeEventListener('conversationChanged', onConv);
  }, []);

  useEffect(() => {
    const onAppend = (message) => {
      const cid = String(message?.chatID ?? '');
      if (!cid) return;
      setStreamingChatIds((prev) => {
        if (prev.has(cid)) return prev;
        const next = new Set(prev);
        next.add(cid);
        return next;
      });
    };
    const onStreamEnd = (payload) => {
      const cid = String(payload?.chatID ?? '');
      if (!cid) return;
      setStreamingChatIds((prev) => {
        if (!prev.has(cid)) return prev;
        const next = new Set(prev);
        next.delete(cid);
        return next;
      });
    };
    EventsOn('dialogAppend', onAppend);
    EventsOn('dialogStreamEnd', onStreamEnd);
    return () => {
      EventsOff('dialogAppend');
      EventsOff('dialogStreamEnd');
    };
  }, []);

  useEffect(() => {
    const onMemoSaved = () => {
      refreshMemoDates();
      setMemoOpen(true);
    };
    window.addEventListener('memoSaved', onMemoSaved);
    return () => window.removeEventListener('memoSaved', onMemoSaved);
  }, [refreshMemoDates]);

  useEffect(() => {
    const onNeedLogin = () => { setNeedLoginOpen(true); };
    EventsOn('needLogin', onNeedLogin);
    return () => { EventsOff('needLogin'); };
  }, []);

  useEffect(() => {
    const onOpenDoc = (e) => {
      const ce = /** @type {CustomEvent<{ path?: string }>} */ (e);
      const p = ce.detail?.path;
      if (typeof p === 'string' && p.trim()) {
        setDocLibFocusPath(p.trim());
        setDocLibOpen(true);
      }
    };
    window.addEventListener('leiagent-open-document', onOpenDoc);
    return () => window.removeEventListener('leiagent-open-document', onOpenDoc);
  }, []);

  const handleMouseDown = (e, border) => {
    e.preventDefault();
    setIsDragging(border);
  };

  useEffect(() => {
    const handleMouseMove = (e) => {
      if (!isDragging) return;

      const container = document.querySelector('.main-content');
      const containerRect = container.getBoundingClientRect();

      if (isDragging === 'left') {
        const newLeftWidth = e.clientX - containerRect.left;
        setLeftWidth(Math.max(100, Math.min(400, newLeftWidth)));
      } else if (isDragging === 'right') {
        const newRightWidth = containerRect.right - e.clientX;
        setRightWidth(Math.max(110, Math.min(1500, newRightWidth)));
      }
    };

    const handleMouseUp = () => {
      setIsDragging(null);
    };

    if (isDragging) {
      document.addEventListener('mousemove', handleMouseMove);
      document.addEventListener('mouseup', handleMouseUp);
    }

    return () => {
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
    };
  }, [isDragging]);

  const handleThinkingDisabledChange = useCallback((v) => {
    setThinkingDisabled(v);
    localStorage.setItem(THINKING_LS_KEY, String(v));
    SetLLMThinkingDisabled(v).catch((e) => console.error('SetLLMThinkingDisabled:', e));
  }, []);

  const showReasoningChrome = false;

  return (
    <div id="App" className="board-column">
      <Header
        onOpenMemo={() => setMemoOpen(true)}
        onOpenScheduledTasks={() => setScheduledTasksOpen(true)}
        onOpenDocLibrary={() => {
          setDocLibFocusPath(null);
          setDocLibOpen(true);
        }}
        onOpenLocalMemory={() => setLocalMemoryOpen(true)}
        onOpenUserProfile={() => setUserProfileOpen(true)}
        connectionLoading={connectionLoading}
        connectionStatus={connectionStatus}
        onRefreshConnection={handleHeaderRefresh}
        onOpenSettings={() => setSettingsOpen(true)}
        thinkingDisabled={thinkingDisabled}
        onThinkingDisabledChange={handleThinkingDisabledChange}
      />
      <AuthModal
        open={authModalOpen}
        mode={authMode}
        onModeChange={setAuthMode}
        onClose={() => setAuthModalOpen(false)}
        authBusy={authBusy}
        authError={authError}
        authNotice={authNotice}
        loginUsername={loginUsername}
        setLoginUsername={setLoginUsername}
        loginPassword={loginPassword}
        setLoginPassword={setLoginPassword}
        registerUsername={registerUsername}
        setRegisterUsername={setRegisterUsername}
        registerEmail={registerEmail}
        setRegisterEmail={setRegisterEmail}
        registerPassword={registerPassword}
        setRegisterPassword={setRegisterPassword}
        registerCode={registerCode}
        setRegisterCode={setRegisterCode}
        onSendCode={() => void handleSendRegisterCode()}
        onLogin={() => void handleLoginSubmit()}
        onRegister={() => void handleRegisterSubmit()}
      />
      <SettingsModal
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        onSaved={refreshConnection}
      />
      <MemoModal open={memoOpen} onClose={() => setMemoOpen(false)} onMemoSaved={refreshMemoDates} />
      <ScheduledTasksModal open={scheduledTasksOpen} onClose={() => setScheduledTasksOpen(false)} />
      <DocLibraryModal
        open={docLibOpen}
        focusPath={docLibFocusPath}
        activeChatId={activeChatId}
        onClose={() => {
          setDocLibOpen(false);
          setDocLibFocusPath(null);
        }}
      />
      <LocalMemoryModal open={localMemoryOpen} chatId={activeChatId} onClose={() => setLocalMemoryOpen(false)} />
      <UserProfileModal open={userProfileOpen} chatId={activeChatId} onClose={() => setUserProfileOpen(false)} />
      {needLoginOpen ? (
        <div
          className="auth-modal-overlay"
          role="presentation"
          onMouseDown={(e) => { if (e.target === e.currentTarget) setNeedLoginOpen(false); }}
        >
          <div className="auth-modal" role="dialog" aria-modal="true">
            <div className="auth-modal__head">
              <div>
                <p className="auth-modal__title">未登录</p>
                <p className="auth-modal__desc">未登录状态，请登录后再使用。</p>
              </div>
            </div>
            <div className="auth-modal__actions">
              <button type="button" className="auth-modal__secondary-btn" onClick={() => setNeedLoginOpen(false)}>取消</button>
              <button type="button" className="auth-modal__primary-btn" onClick={() => { setNeedLoginOpen(false); prepareAuthModal(); }}>登录</button>
            </div>
          </div>
        </div>
      ) : null}
      <div className="main-content">
        <div
          className="main-content__sidebar-slot"
          style={{ width: `${leftWidth}px`, minWidth: '200px', maxWidth: '400px' }}
        >
          <ConversationList
            memoDates={memoDates}
            refreshMemoDates={refreshMemoDates}
            streamingChatIds={streamingChatIds}
          />
        </div>
        <div className="resizer left-resizer" onMouseDown={(e) => handleMouseDown(e, 'left')} />
        <div className="main-content__dialog-slot">
          <ChatDialog />
        </div>
        {showReasoningChrome ? (
          <>
            <div className="resizer right-resizer" onMouseDown={(e) => handleMouseDown(e, 'right')} />
            <div
              className="main-content__reasoning-slot"
              style={{
                width: `${rightWidth}px`,
                maxWidth: '1500px',
              }}
            >
              <Reasoning conversationId={activeChatId} />
            </div>
          </>
        ) : null}
      </div>
    </div>
  );
}

export default App;
