import { useState, useEffect, useCallback } from 'react';

import './App.css';

import ConversationList from './features/conversations/ConversationList.jsx';
import { ChatDialog } from './features/chat';
import Header from './components/Header.jsx';
import MemoModal from './features/memo/MemoModal.jsx';
import DocLibraryModal from './features/documents/DocLibraryModal.jsx';
import SettingsModal from './features/settings/SettingsModal.jsx';
import LocalMemoryModal from './features/memory/LocalMemoryModal.jsx';
import UserProfileModal from './features/memory/UserProfileModal.jsx';
import ScheduledTasksModal from './features/scheduling/ScheduledTasksModal.jsx';
import {
  GetMemoCalendarDates,
  GetLLMConnectionStatus,
} from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';

function App() {
  const [leftWidth, setLeftWidth] = useState(260);
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
    const onLLMConfigRequired = (payload) => openPrompt(payload);
    const offLLMConfigRequired = EventsOn('llmConfigRequired', onLLMConfigRequired);
    return () => {
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
      </div>
    </div>
  );
}

export default App;
