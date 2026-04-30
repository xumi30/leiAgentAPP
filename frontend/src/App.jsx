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
  GetMemoCalendarDates,
  GetLLMConnectionStatus,
  SetLLMThinkingDisabled,
} from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';

const THINKING_LS_KEY = 'leiAgent.llmThinkingDisabled';

function readThinkingDisabledFromLS() {
  const raw = localStorage.getItem(THINKING_LS_KEY);
  if (raw === 'true') return true;
  if (raw === 'false') return false;
  return null;
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
  const [llmConfigPrompt, setLlmConfigPrompt] = useState(null);
  const [activeChatId, setActiveChatId] = useState('');
  const [activeChatTitle, setActiveChatTitle] = useState('');
  /** 助手流式输出进行中（按 chatID），用于侧栏列表显示加载态 */
  const [streamingChatIds, setStreamingChatIds] = useState(() => new Set());
  const [memoDates, setMemoDates] = useState(() => new Set());

  const [thinkingDisabled, setThinkingDisabled] = useState(true);

  const [connectionLoading, setConnectionLoading] = useState(false);
  const [connectionStatus, setConnectionStatus] = useState(null);

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
    const offAppend = EventsOn('dialogAppend', onAppend);
    const offStreamEnd = EventsOn('dialogStreamEnd', onStreamEnd);
    return () => {
      offAppend();
      offStreamEnd();
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
    const openPrompt = (payload) => {
      const detail = payload && typeof payload === 'object' ? payload : {};
      const fallback = typeof payload === 'string' ? payload : '';
      setLlmConfigPrompt({
        title: String(detail.title ?? '需要可用的 LLM'),
        message: String(detail.message ?? fallback ?? '请在设置中进行 LLM 配置'),
        configCreated: Boolean(detail.configCreated),
        configPath: String(detail.configPath ?? ''),
      });
    };
    const onNeedLogin = (payload) => openPrompt(payload);
    const onLLMConfigRequired = (payload) => openPrompt(payload);
    const offNeedLogin = EventsOn('needLogin', onNeedLogin);
    const offLLMConfigRequired = EventsOn('llmConfigRequired', onLLMConfigRequired);
    return () => {
      offNeedLogin();
      offLLMConfigRequired();
    };
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
        onRefreshConnection={refreshConnection}
        onOpenSettings={() => setSettingsOpen(true)}
        thinkingDisabled={thinkingDisabled}
        onThinkingDisabledChange={handleThinkingDisabledChange}
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
      {llmConfigPrompt ? (
        <div
          className="auth-modal-overlay"
          role="presentation"
          onMouseDown={(e) => { if (e.target === e.currentTarget) setLlmConfigPrompt(null); }}
        >
          <div className="auth-modal" role="dialog" aria-modal="true">
            <div className="auth-modal__head">
              <div>
                <p className="auth-modal__title">{llmConfigPrompt.title}</p>
                <p className="auth-modal__desc">
                  {llmConfigPrompt.configCreated ? '已自动创建配置文件。' : ''}
                  {llmConfigPrompt.message || '请在设置中进行 LLM 配置'}
                </p>
                {llmConfigPrompt.configPath ? (
                  <p className="auth-modal__desc auth-modal__desc--path">{llmConfigPrompt.configPath}</p>
                ) : null}
              </div>
            </div>
            <div className="auth-modal__actions">
              <button type="button" className="auth-modal__secondary-btn" onClick={() => setLlmConfigPrompt(null)}>取消</button>
              <button
                type="button"
                className="auth-modal__primary-btn"
                onClick={() => {
                  setLlmConfigPrompt(null);
                  setSettingsOpen(true);
                }}
              >
                打开设置
              </button>
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
