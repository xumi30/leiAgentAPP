import { useState, useEffect, useCallback } from 'react';
import './App.css';

// 新架构组件
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

// 状态管理（待重构为stores）
const THINKING_LS_KEY = 'leiAgent.llmThinkingDisabled';

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

function App() {
  // 模态框状态
  const [modalState, setModalState] = useState({
    isMemoModalOpen: false,
    isDocLibraryModalOpen: false,
    isSettingsModalOpen: false,
    isLocalMemoryModalOpen: false,
    isUserProfileModalOpen: false,
    isScheduledTasksModalOpen: false,
  });

  // 应用状态
  const [llmState, setLlmState] = useState(null);
  const [status, setStatus] = useState(null);
  const [memoCalendarDates, setMemoCalendarDates] = useState([]);
  const [llmThinkingDisabled, setLlmThinkingDisabled] = useState(readThinkingDisabledFromLS());
  const [conversations, setConversations] = useState([]);
  const [reasoningVisible, setReasoningVisible] = useState(false);

  // 配置和状态检查
  const needsAuthModal = configNeedsAuthModal(llmState) || connectionNeedsAuthModal(status);

  // 模态框控制函数
  const openModal = useCallback((modalName) => {
    setModalState(prev => ({ ...prev, [modalName]: true }));
  }, []);

  const closeModal = useCallback((modalName) => {
    setModalState(prev => ({ ...prev, [modalName]: false }));
  }, []);

  // 数据加载
  const loadData = useCallback(async () => {
    try {
      const [newLlmState, newStatus, newMemoDates] = await Promise.all([
        GetLLMConfigFormState(),
        GetLLMConnectionStatus(),
        GetMemoCalendarDates(),
      ]);
      setLlmState(newLlmState);
      setStatus(newStatus);
      setMemoCalendarDates(newMemoDates);
    } catch (error) {
      console.error('加载数据失败:', error);
    }
  }, []);

  // 初始化
  useEffect(() => {
    loadData();
  }, [loadData]);

  // 保存禁用思考状态
  useEffect(() => {
    if (llmThinkingDisabled !== null) {
      localStorage.setItem(THINKING_LS_KEY, llmThinkingDisabled.toString());
      SetLLMThinkingDisabled(llmThinkingDisabled);
    }
  }, [llmThinkingDisabled]);

  return (
    <div className="app">
      {/* 主容器 */}
      <div className="app-container">
        {/* 头部导航 - 暂时保留原有的Header组件 */}
        <Header 
          onOpenModal={openModal}
          reasoningVisible={reasoningVisible}
          setReasoningVisible={setReasoningVisible}
        />
        
        {/* 三栏布局 */}
        <div className="app-content">
          {/* 左侧：对话列表 */}
          <div className="sidebar-left">
            <ConversationList 
              conversations={conversations}
              setConversations={setConversations}
            />
          </div>
          
          {/* 中间：对话区域 - 使用新架构的ChatDialog */}
          <div className="main-content">
            <ChatDialog />
          </div>
          
          {/* 右侧：推理栏（可选） */}
          {reasoningVisible && (
            <div className="sidebar-right">
              <Reasoning />
            </div>
          )}
        </div>
      </div>

      {/* 模态框组件 - 暂时保留原有的 */}
      {modalState.isMemoModalOpen && (
        <MemoModal 
          onClose={() => closeModal('isMemoModalOpen')}
          memoCalendarDates={memoCalendarDates}
        />
      )}
      
      {modalState.isDocLibraryModalOpen && (
        <DocLibraryModal onClose={() => closeModal('isDocLibraryModalOpen')} />
      )}
      
      {modalState.isSettingsModalOpen && (
        <SettingsModal 
          onClose={() => closeModal('isSettingsModalOpen')}
          llmState={llmState}
          status={status}
          llmThinkingDisabled={llmThinkingDisabled}
          setLlmThinkingDisabled={setLlmThinkingDisabled}
        />
      )}
      
      {modalState.isLocalMemoryModalOpen && (
        <LocalMemoryModal onClose={() => closeModal('isLocalMemoryModalOpen')} />
      )}
      
      {modalState.isUserProfileModalOpen && (
        <UserProfileModal onClose={() => closeModal('isUserProfileModalOpen')} />
      )}
      
      {modalState.isScheduledTasksModalOpen && (
        <ScheduledTasksModal onClose={() => closeModal('isScheduledTasksModalOpen')} />
      )}

      {/* 认证模态框 */}
      {needsAuthModal && (
        <div className="auth-modal-overlay">
          <div className="auth-modal">
            <form
              onSubmit={async (e) => {
                e.preventDefault();
                const formData = new FormData(e.target);
                const response = await ProxyAuthRequest({
                  username: String(formData.get('username')),
                  password: String(formData.get('password')),
                });
                if (response.valid) {
                  loadData();
                } else {
                  alert('认证失败: ' + (response.message || '未知错误'));
                }
              }}
              className="auth-form"
            >
              <h2>登录</h2>
              <input 
                type="text" 
                name="username" 
                placeholder="用户名" 
                required 
                className="auth-input"
              />
              <input 
                type="password" 
                name="password" 
                placeholder="密码" 
                required 
                className="auth-input"
              />
              <button type="submit" className="auth-submit">登录</button>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}

export default App;