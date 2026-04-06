import { useState, useEffect, useCallback } from 'react';

import './App.css';

import ConversationList from './componentjs/ConversationList.jsx';
import Dialog from './componentjs/Dialog.jsx';
import Header from './componentjs/Header.jsx';
import Reasoning from './componentjs/Reasonging.jsx';
import MemoModal from './componentjs/MemoModal.jsx';
import DocLibraryModal from './componentjs/DocLibraryModal.jsx';
import SettingsModal from './componentjs/SettingsModal.jsx';
import {
  GetMemoCalendarDates,
  GetLLMConnectionStatus,
  GetLLMThinkingDisabled,
  SetLLMThinkingDisabled,
} from '../wailsjs/go/main/App';

const THINKING_LS_KEY = 'leiAgent.llmThinkingDisabled';
const CONNECTION_POLL_MS = 45000;

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
  const [docLibOpen, setDocLibOpen] = useState(false);
  const [docLibFocusPath, setDocLibFocusPath] = useState(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [activeChatId, setActiveChatId] = useState('');
  const [activeChatTitle, setActiveChatTitle] = useState('');
  const [memoDates, setMemoDates] = useState(() => new Set());

  const [thinkingDisabled, setThinkingDisabled] = useState(() => {
    const fromLs = readThinkingDisabledFromLS();
    return fromLs === null ? false : fromLs;
  });

  const [connectionLoading, setConnectionLoading] = useState(true);
  const [connectionStatus, setConnectionStatus] = useState(null);

  const refreshConnection = useCallback(async () => {
    setConnectionLoading(true);
    try {
      const s = await GetLLMConnectionStatus();
      setConnectionStatus(s && typeof s === 'object' ? { ...s } : s);
    } catch (e) {
      console.error('GetLLMConnectionStatus:', e);
      setConnectionStatus({
        ok: false,
        phase: 'client_error',
        message: String(e?.message || e),
        configPath: '',
      });
    } finally {
      setConnectionLoading(false);
    }
  }, []);

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
      const fromLs = readThinkingDisabledFromLS();
      if (fromLs !== null) {
        try {
          await SetLLMThinkingDisabled(fromLs);
        } catch (e) {
          console.error('SetLLMThinkingDisabled:', e);
        }
        return;
      }
      try {
        const v = await GetLLMThinkingDisabled();
        if (!cancelled) {
          setThinkingDisabled(!!v);
          localStorage.setItem(THINKING_LS_KEY, String(!!v));
        }
      } catch (e) {
        console.error('GetLLMThinkingDisabled:', e);
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
    const onMemoSaved = () => {
      refreshMemoDates();
      setMemoOpen(true);
    };
    window.addEventListener('memoSaved', onMemoSaved);
    return () => window.removeEventListener('memoSaved', onMemoSaved);
  }, [refreshMemoDates]);

  useEffect(() => {
    /** @param {Event} e */
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

  const showReasoningChrome = !thinkingDisabled;

  return (
    <div id="App" className="board-column">
      <Header
        onOpenMemo={() => setMemoOpen(true)}
        onOpenDocLibrary={() => {
          setDocLibFocusPath(null);
          setDocLibOpen(true);
        }}
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
      <DocLibraryModal
        open={docLibOpen}
        focusPath={docLibFocusPath}
        onClose={() => {
          setDocLibOpen(false);
          setDocLibFocusPath(null);
        }}
      />
      <div className="main-content">
        <div
          className="main-content__sidebar-slot"
          style={{ width: `${leftWidth}px`, minWidth: '200px', maxWidth: '400px' }}
        >
          <ConversationList memoDates={memoDates} refreshMemoDates={refreshMemoDates} />
        </div>
        <div className="resizer left-resizer" onMouseDown={(e) => handleMouseDown(e, 'left')} />
        <div className="main-content__dialog-slot">
          <Dialog />
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
