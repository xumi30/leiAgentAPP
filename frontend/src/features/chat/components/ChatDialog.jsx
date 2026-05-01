import React, { useMemo, useState, useEffect, useRef, useCallback } from 'react';
import { useChatStore } from '../../../stores';
import { useStreaming } from '../hooks/useStreaming';
import { MessageBubble, ChatInput } from '../../../components';
import { ChatService } from '../../../services/chatService';
import { AddAgentToConversation, AddConversation, GetConversationAgents, GetMessages, ListAgents, SwitchChat } from '../../../services/api';
import { classifyUserMessage } from '../../../utils/messageClassify';
import { GetMCPConfigFormState, GetOpenClawSkillState, RespondShellApproval } from '../../../../wailsjs/go/main/App';
import { EventsOn } from '../../../../wailsjs/runtime/runtime';
import assistantAvatar from '../../../assets/images/aitx.png';
import Tooltip from './tooltip/Tooltip';
import MemoStrip from './MemoStrip';
import { useMemoComposer } from '../hooks/useMemoComposer';
import '../../../styles/chat.css';

const DEFAULT_ASSISTANT_AGENT_ID = 'agentid_0';

const AUTO_TO_TALK_LS_KEY = 'leiAgent.chatAutoToTalk';

function readAutoToTalkFromLS() {
  const raw = localStorage.getItem(AUTO_TO_TALK_LS_KEY);
  if (raw === 'true') return true;
  if (raw === 'false') return false;
  return false;
}

// 消息类型定义
const MESSAGE_FIELDS = {
  CHAT_ID: 'chatID',
  MESSAGE_ID: 'messageID',
  CONTENT: 'content',
  ROLE: 'role',
  TIMESTAMP: 'timestamp',
  TOTAL_TOKENS: 'total_tokens',
  AGENT_ID: 'agentID'
};

// 规范化消息对象
const normalizeMessage = (raw) => {
  return {
    [MESSAGE_FIELDS.CHAT_ID]: String(raw?.chatID ?? raw?.chat_id ?? '').trim(),
    [MESSAGE_FIELDS.MESSAGE_ID]: String(raw?.messageID ?? raw?.message_id ?? raw?.id ?? '').trim(),
    [MESSAGE_FIELDS.CONTENT]: String(raw?.content ?? '').trim(),
    [MESSAGE_FIELDS.ROLE]: String(raw?.role ?? '').trim(),
    [MESSAGE_FIELDS.TIMESTAMP]: raw?.timestamp ? new Date(raw.timestamp).getTime() : Date.now(),
    [MESSAGE_FIELDS.TOTAL_TOKENS]: Number(raw?.total_tokens ?? raw?.totalTokens ?? 0),
    [MESSAGE_FIELDS.AGENT_ID]: String(raw?.agentID ?? raw?.agent_id ?? '').trim()
  };
};

// 提取 Agent ID 的辅助函数
const getAgentID = (agent) => String(agent?.agentID ?? agent?.agent_id ?? '').trim();

const messageTokenCount = (message) => {
  const raw = message?.total_tokens ?? message?.totalTokens;
  const value = typeof raw === 'number' ? raw : Number.parseInt(String(raw ?? ''), 10);
  return Number.isFinite(value) && value > 0 ? value : 0;
};

const ChatDialog = () => {
  // 状态管理
  const {
    chatId,
    messages,
    setMessages,
    addMessage,
    setChatId,
    setSheets,
    setActiveSheetId,
    setConversationAgents,
    conversationAgents,
    allAgents,
    setAllAgents,
    conversationTitle,
    setConversationTitle,
    setClassifyHint,
    setRuntimeError,
    sheets,
    activeSheetId
  } = useChatStore();

  const currentMessages = useMemo(() => {
    const activeSheet = (sheets || []).find((sheet) => sheet.id === activeSheetId);
    const startIdx = activeSheet?.startIdx ?? 0;
    return (messages || []).slice(startIdx);
  }, [messages, sheets, activeSheetId]);

  const {
    stopVisible,
    taskBusy,
    startStreaming,
    stopStreaming,
    addToQueue,
    processQueue
  } = useStreaming();

  const messagesEndRef = useRef(null);
  const [inputValue, setInputValue] = useState('');
  const [streamPulse, setStreamPulse] = useState(null); // { chatID, messageID }
  const [aiteAgentIds, setAiteAgentIds] = useState(() => new Set());
  const [isAutoToTalk, setIsAutoToTalk] = useState(() => readAutoToTalkFromLS());
  const [memoStripOpen, setMemoStripOpen] = useState(false);
  const [memoHint, setMemoHint] = useState('');
  const [memoError, setMemoError] = useState('');
  /** 待确认的 shell：后台阻塞直到用户 RespondShellApproval（仅当前会话显示横幅）*/
  const [shellApproval, setShellApproval] = useState(null);
  const [mcpOptions, setMcpOptions] = useState([]);
  const [skillOptions, setSkillOptions] = useState([]);
  const memoHintTimerRef = useRef(null);

  useEffect(() => {
    try {
      localStorage.setItem(AUTO_TO_TALK_LS_KEY, String(isAutoToTalk));
    } catch (e) {
      console.warn('persist chatAutoToTalk:', e);
    }
  }, [isAutoToTalk]);

  // conversations.agents 常为空；助手消息仍属 agentid_0，这里用于顶栏与 @ 提及
  const conversationAgentsForUI = useMemo(() => {
    const list = Array.isArray(conversationAgents) ? [...conversationAgents] : [];
    const hasDefault = list.some((a) => getAgentID(a) === DEFAULT_ASSISTANT_AGENT_ID);
    if (hasDefault) return list;

    const fromAll = (Array.isArray(allAgents) ? allAgents : []).find(
      (a) => String(a?.agent_id ?? a?.agentID ?? '').trim() === DEFAULT_ASSISTANT_AGENT_ID,
    );
    if (fromAll) {
      return [fromAll, ...list];
    }
    return [
      {
        agentID: DEFAULT_ASSISTANT_AGENT_ID,
        agent_id: DEFAULT_ASSISTANT_AGENT_ID,
        agent_name: '工具人',
        avatar_image: '',
        description: '',
      },
      ...list,
    ];
  }, [conversationAgents, allAgents]);

  const mentionOptions = useMemo(() => {
    return conversationAgentsForUI
      .map((a) => ({
        agent_id: getAgentID(a),
        agent_name: String(a?.agent_name ?? '').trim(),
        avatar_image: String(a?.avatar_image ?? '').trim(),
      }))
      .filter((a) => a.agent_id && a.agent_name);
  }, [conversationAgentsForUI]);

  const loadCommandOptions = useCallback(async () => {
    try {
      const [mcpState, skillState] = await Promise.all([
        GetMCPConfigFormState().catch(() => null),
        GetOpenClawSkillState().catch(() => null),
      ]);

      const servers = Array.isArray(mcpState?.servers) ? mcpState.servers : [];
      setMcpOptions(
        servers
          .map((row) => ({
            ...row,
            label: String(row?.label ?? '').trim(),
            cachedTools: Array.isArray(row?.cachedTools) ? row.cachedTools : [],
          }))
          .filter((row) => row.label),
      );

      const skills = Array.isArray(skillState?.skills) ? skillState.skills : [];
      setSkillOptions(
        skills
          .map((skill) => ({
            ...skill,
            name: String(skill?.name ?? '').trim(),
            description: String(skill?.description ?? skill?.statusDetail ?? '').trim(),
          }))
          .filter((skill) => skill.name && skill.supported !== false),
      );
    } catch (e) {
      console.error('load command picker options:', e);
      setMcpOptions([]);
      setSkillOptions([]);
    }
  }, []);

  useEffect(() => {
    void loadCommandOptions();
  }, [loadCommandOptions]);



  // 加载全量 agents（用于头像/名称兜底）
  useEffect(() => {
    void (async () => {
      try {
        const list = await ListAgents();
        setAllAgents(Array.isArray(list) ? list : []);
      } catch (e) {
        console.error('ListAgents:', e);
        setAllAgents([]);
      }
    })();
  }, [setAllAgents]);

  // 滚动到最新消息
  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  };

  useEffect(() => {
    scrollToBottom();
  }, [messages]);

  const showTransientHint = useCallback((text, ms = 2200) => {
    const t = String(text ?? '').trim();
    if (!t) return;
    setMemoHint(t);
    if (memoHintTimerRef.current) window.clearTimeout(memoHintTimerRef.current);
    memoHintTimerRef.current = window.setTimeout(() => {
      setMemoHint('');
      memoHintTimerRef.current = null;
    }, ms);
  }, []);

  const showRuntimeError = useCallback((text, ms = 2600) => {
    const t = String(text ?? '').trim();
    if (!t) return;
    setMemoError(t);
    if (memoHintTimerRef.current) window.clearTimeout(memoHintTimerRef.current);
    memoHintTimerRef.current = window.setTimeout(() => {
      setMemoError('');
      memoHintTimerRef.current = null;
    }, ms);
  }, []);

  // 流式输出：dialogAppend 追加、dialogStreamEnd 结束、chatTaskState 同步忙碌态
  useEffect(() => {
    const onAppend = (payload) => {
      const normalized = normalizeMessage(payload);

      // 验证必要字段
      if (!normalized[MESSAGE_FIELDS.CHAT_ID] || !normalized[MESSAGE_FIELDS.MESSAGE_ID]) return;
      if (String(chatId ?? '').trim() && normalized[MESSAGE_FIELDS.CHAT_ID] !== String(chatId ?? '').trim()) return;

      // 直接使用后端返回的 agentID，无需从内容中解析
      const nextMessage = {
        ...normalized,
        [MESSAGE_FIELDS.AGENT_ID]: normalized[MESSAGE_FIELDS.AGENT_ID] || DEFAULT_ASSISTANT_AGENT_ID
      };

      setStreamPulse({ chatID: normalized[MESSAGE_FIELDS.CHAT_ID], messageID: normalized[MESSAGE_FIELDS.MESSAGE_ID] });

      setMessages((prev) => {
        const list = Array.isArray(prev) ? prev : [];
        const idx = list.findIndex((m) => String(m?.messageID ?? m?.messageId ?? '') === normalized[MESSAGE_FIELDS.MESSAGE_ID]);

        if (idx < 0) return [...list, nextMessage];

        return list.map((m, i) => {
          if (i !== idx) return m;

          // 处理流式内容追加
          if (normalized[MESSAGE_FIELDS.ROLE] === 'assistant') {
            return { ...m, ...nextMessage, content: String(m?.content ?? '') + normalized[MESSAGE_FIELDS.CONTENT] };
          }

          return { ...m, ...nextMessage };
        });
      });
    };

    const onStreamEnd = (payload) => {
      const cid = String(payload?.chatID ?? '').trim();
      const mid = String(payload?.messageID ?? payload?.messageId ?? payload?.id ?? '').trim();
      const agentID = String(payload?.agentID ?? payload?.agent_id ?? payload?.agentId ?? '').trim();

      // 仅更新消息的 agentID，移除刷新 agents 列表的逻辑
      if (mid && agentID) {
        setMessages((prev) => {
          const list = Array.isArray(prev) ? prev : [];
          if (list.length === 0) return list;
          let changed = false;
          const next = list.map((m) => {
            const mId = String(m?.messageID ?? m?.messageId ?? m?.id ?? '').trim();
            if (mId !== mid) return m;
            const existing = String(m?.agentID ?? m?.agent_id ?? m?.agentId ?? '').trim();
            if (existing) return m;
            changed = true;
            return { ...m, agentID };
          });
          return changed ? next : list;
        });
      }

      setStreamPulse((prev) => {
        if (!prev) return prev;
        if (cid && prev.chatID !== cid) return prev;
        if (mid && prev.messageID !== mid) return prev;
        return null;
      });
      stopStreaming();
    };

    const onTaskState = (payload) => {
      const cid = String(payload?.chatID ?? '').trim();
      if (String(chatId ?? '').trim() && cid !== String(chatId ?? '').trim()) return;
      const busy = Boolean(payload?.busy);
      if (busy) startStreaming();
      else stopStreaming();
    };

    // 后端补全消息字段（如 agentID/agent_id/头像等）时会推送该事件
    const onGetMessagesByMessageID = (payload) => {
      const normalized = normalizeMessage(payload);

      if (String(chatId ?? '').trim() && normalized[MESSAGE_FIELDS.CHAT_ID] !== String(chatId ?? '').trim()) return;
      if (!normalized[MESSAGE_FIELDS.MESSAGE_ID]) return;

      setMessages((prev) => {
        const list = Array.isArray(prev) ? prev : [];
        const idx = list.findIndex((m) => String(m?.messageID ?? m?.messageId ?? '') === normalized[MESSAGE_FIELDS.MESSAGE_ID]);
        if (idx < 0) return list;

        const merged = {
          ...payload,
          ...normalized,
          [MESSAGE_FIELDS.AGENT_ID]: normalized[MESSAGE_FIELDS.AGENT_ID] || list[idx]?.agentID || DEFAULT_ASSISTANT_AGENT_ID
        };

        return list.map((m, i) => (i === idx ? { ...m, ...merged } : m));
      });
    };

    const offAppend = EventsOn('dialogAppend', onAppend);
    const offStreamEnd = EventsOn('dialogStreamEnd', onStreamEnd);
    const offTaskState = EventsOn('chatTaskState', onTaskState);
    const offGetMessagesByMessageID = EventsOn('GetMessagesByMessageID', onGetMessagesByMessageID);
    const offSendMessageError = EventsOn('sendMessageError', stopStreaming);
    const offDispatcherError = EventsOn('dispatcherError', stopStreaming);
    const offLLMConfigRequired = EventsOn('llmConfigRequired', stopStreaming);
    return () => {
      offAppend();
      offStreamEnd();
      offTaskState();
      offGetMessagesByMessageID();
      offSendMessageError();
      offDispatcherError();
      offLLMConfigRequired();
    };
  }, [chatId, setMessages, startStreaming, stopStreaming]);

  useEffect(() => {
    const onShellApprovalRequest = (data) => {
      const cid = String(data?.chatID ?? '').trim();
      const reqId = String(data?.requestId ?? '').trim();
      const cmd = String(data?.command ?? '');
      if (!cid || !reqId) return;
      setShellApproval({ chatID: cid, requestId: reqId, command: cmd });
    };
    const offShellApprovalRequest = EventsOn('shellApprovalRequest', onShellApprovalRequest);
    return () => {
      offShellApprovalRequest();
    };
  }, []);

  const shellApprovalVisible =
    shellApproval &&
    String(shellApproval.chatID ?? '').trim() !== '' &&
    String(shellApproval.chatID ?? '').trim() === String(chatId ?? '').trim();

  /** 与 MessageBubble 的 isStreaming 一致：有 append 数据流时也应显示「跑马中」并可停止 */
  const streamFlowingThisChat = useMemo(
    () =>
      Boolean(streamPulse)
      && String(streamPulse?.chatID ?? '') === String(chatId ?? ''),
    [streamPulse, chatId],
  );
  const stopButtonEngaged = stopVisible || streamFlowingThisChat;

  const resolveShellApproval = useCallback(async (approve) => {
    if (!shellApproval?.requestId) return;
    const cid = String(shellApproval.chatID ?? '').trim();
    const rid = String(shellApproval.requestId ?? '').trim();
    if (!cid || !rid) return;
    try {
      await RespondShellApproval(cid, rid, Boolean(approve));
    } catch (e) {
      console.error('RespondShellApproval:', e);
    }
    setShellApproval(null);
  }, [shellApproval]);

  const memoListMessages = useMemo(() => {
    const list = Array.isArray(currentMessages) ? currentMessages : [];
    return list.filter((msg) => {
      const hasText = String(msg?.content ?? '').trim() !== '';
      const streamingHere =
        Boolean(streamPulse)
        && String(streamPulse?.chatID ?? '') === String(chatId ?? '')
        && String(streamPulse?.messageID ?? '') === String(msg?.messageID ?? '');
      return hasText || streamingHere;
    });
  }, [currentMessages, streamPulse, chatId]);

  const conversationTokenTotal = useMemo(() => {
    return (Array.isArray(currentMessages) ? currentMessages : []).reduce(
      (sum, message) => sum + messageTokenCount(message),
      0,
    );
  }, [currentMessages]);

  const memoComposer = useMemoComposer({
    open: memoStripOpen,
    messages: memoListMessages,
    onClose: () => setMemoStripOpen(false),
    onHint: (t) => showTransientHint(t),
    onError: (t) => showRuntimeError(t),
  });

  const onMessagesMemoDismissMouseDown = useCallback((e) => {
    if (!memoStripOpen || memoComposer.memoCheckSaving) return;
    if (e.target !== e.currentTarget) return;
    setMemoStripOpen(false);
  }, [memoStripOpen, memoComposer.memoCheckSaving]);

  // 加载历史消息：侧栏切换会话时触发 conversationChanged
  useEffect(() => {
    const handleConversationChange = (event) => {
      const { conversationId, title } = event?.detail ?? {};
      const nextChatId = String(conversationId ?? '').trim();
      setChatId(nextChatId);
      setSheets([{ id: 'main', title: '主对话', startIdx: 0 }]);
      setActiveSheetId('main');
      setConversationTitle(String(title ?? '').trim());

      if (!nextChatId) {
        setMessages([]);
        setConversationAgents([]);
        return;
      }

      void (async () => {
        try {
          const [nextMessages, nextAgents] = await Promise.all([
            GetMessages(nextChatId),
            GetConversationAgents(nextChatId).catch(() => []),
          ]);
          const normalize = (msg) => {
            const normalized = normalizeMessage(msg);

            // 直接使用后端返回的 agentID，无需从内容中解析
            if (normalized[MESSAGE_FIELDS.ROLE] !== 'assistant') return normalized;

            return {
              ...normalized,
              [MESSAGE_FIELDS.AGENT_ID]: normalized[MESSAGE_FIELDS.AGENT_ID] || DEFAULT_ASSISTANT_AGENT_ID
            };
          };

          const normalizedMessages = Array.isArray(nextMessages) ? nextMessages.map(normalize) : [];
          setMessages(normalizedMessages);
          setConversationAgents(Array.isArray(nextAgents) ? nextAgents : []);
        } catch (e) {
          console.error('GetMessages:', e);
          setMessages([]);
          setConversationAgents([]);
        }
      })();
    };

    window.addEventListener('conversationChanged', handleConversationChange);
    return () => window.removeEventListener('conversationChanged', handleConversationChange);
  }, [setChatId, setMessages, setSheets, setActiveSheetId, setConversationAgents, setConversationTitle]);

  // 将 agent 加入当前会话（侧栏/Agent卡片触发）
  useEffect(() => {
    const handleAddAgent = async (event) => {
      const nextAgent = event?.detail?.agent;
      const agentId = getAgentID(nextAgent);
      if (!agentId) return;

      try {
        let currentChat = String(chatId ?? '').trim();
        if (!currentChat) {
          const title = '新对话';
          const newID = await AddConversation(title);
          const idStr = newID != null ? String(newID).trim() : '';
          if (!idStr) {
            setRuntimeError('新建对话失败：未返回会话ID');
            return;
          }
          SwitchChat(idStr);
          window.dispatchEvent(
            new CustomEvent('conversationChanged', {
              detail: { conversationId: idStr, title },
            }),
          );
          currentChat = idStr;
        }

        await AddAgentToConversation(currentChat, agentId);
        const nextAgents = await GetConversationAgents(currentChat);
        setConversationAgents(Array.isArray(nextAgents) ? nextAgents : []);
        setClassifyHint(`已加入当前聊天：${String(nextAgent?.agent_name || agentId)}`);
        setTimeout(() => setClassifyHint(''), 2200);
      } catch (e) {
        console.error('leiagent-add-agent-to-chat:', e);
        setRuntimeError(`加入当前聊天失败：${String(e?.message || e)}`);
      }
    };

    window.addEventListener('leiagent-add-agent-to-chat', handleAddAgent);
    return () => window.removeEventListener('leiagent-add-agent-to-chat', handleAddAgent);
  }, [chatId, setChatId, setConversationAgents, setClassifyHint, setRuntimeError]);

  // 处理用户消息发送
  const handleSendMessage = async (content) => {
    if (!ChatService.validateMessage(content)) return;

    const processedContent = ChatService.preprocessMessage(content);

    // 添加用户消息
    const userMessage = {
      role: 'user',
      content: processedContent,
      timestamp: Date.now()
    };
    addMessage(userMessage);

    // 消息分类处理
    const messageType = classifyUserMessage(content, { isStreaming: taskBusy });

    if (messageType === 'control') {
      // 控制指令：停止生成
      await ChatService.stopChat(String(chatId ?? ''));
      stopStreaming();
      return;
    }

    // 开始流式响应
    startStreaming();

    // 发送消息到后端
    // 如果开启了自动对话，则在发送消息后停止聊天
    if (isAutoToTalk) {
      try {
        await ChatService.stopChat(String(chatId ?? ''));
      } catch (e) {
        console.error('停止聊天失败:', e);
      }
    }
    let needNewChatName = false;
    if (conversationTitle === '新对话') {
      needNewChatName = true;
    }

    const result = await ChatService.sendMessage(
      String(chatId ?? ''),
      processedContent,
      'user',
      undefined,
      Array.from(aiteAgentIds),
      isAutoToTalk,
      needNewChatName,
    );

    if (!result.success) {
      console.error('消息发送失败:', result.error);
      stopStreaming();
      // 可以考虑添加错误提示
    }

    setInputValue('');
    setAiteAgentIds(new Set());
  };

  // 处理输入变化
  const handleInputChange = (value) => {
    setInputValue(value);
    if (!String(value ?? '').trim()) setAiteAgentIds(new Set());
  };
  const handleToggleAutoTalk = async () => {
    const newState = !isAutoToTalk;
    setIsAutoToTalk(newState);

    if (!newState) {
      try {
        await ChatService.stopChat(String(chatId ?? ''));
      } catch (e) {
        console.error('停止聊天失败:', e);
      }
    }
  };


  // 停止生成按钮
  const handleStopGenerating = async () => {
    try {
      await ChatService.stopChat(String(chatId ?? ''));
    } catch (e) {
      console.error('停止聊天失败:', e);
    }
    setStreamPulse(null);
    stopStreaming();
  };

  return (
    <div id={`dialog_${String(chatId ?? '')}`} className="dialog">
      <div className="dialog__header dialog__header--tabs">
        <div className="dialog__tabs" role="tablist" aria-label="同一会话便签页">
          <button
            type="button"
            role="tab"
            aria-selected
            className="dialogTitle"
            title={(conversationTitle || '主对话').trim() || '主对话'}
          >
            <span className="dialog__tab-inline" dir="auto">
              <span className="dialog__tab-main-title-inline">
                {(conversationTitle || '主对话').trim() || '主对话'}
              </span>
              <span className="dialog__tab-token-pill" aria-label="当前对话 token 数">
                {conversationTokenTotal.toLocaleString()} tokens
              </span>
            </span>
          </button>

          <Tooltip
            content={isAutoToTalk ? "关闭自动对话：群员将停止自动参与对话" : "开启自动对话：群员将自动参与对话"}
            position="top"
          >
            <button
              type="button"
              className={`dialog__tab dialog__toggle ${isAutoToTalk ? 'dialog__toggle--active' : ''}`}
              onClick={handleToggleAutoTalk}
              aria-pressed={isAutoToTalk}
            >
              <span className="dialog__toggle-track">
                <span className="dialog__toggle-thumb" />
              </span>
              <span className="dialog__toggle-label">
                {isAutoToTalk ? "群聊开启：开" : "群聊关闭：关"}
              </span>
            </button>
          </Tooltip>


        </div>

        {Array.isArray(conversationAgentsForUI) && conversationAgentsForUI.length > 0 ? (
          <div className="dialog__agents" aria-label="当前聊天已加入的 agents">
            {conversationAgentsForUI.map((agent) => (
              <div
                key={getAgentID(agent)}
                className="dialog__agent-chip"
                title={String(agent?.description ?? '')}
              >
                <img
                  className="dialog__agent-chip-avatar"
                  src={String(agent?.avatar_image ?? '').trim() || assistantAvatar}
                  onError={(e) => {
                    if (e.currentTarget?.src !== assistantAvatar) e.currentTarget.src = assistantAvatar;
                  }}
                  alt={String(agent?.agent_name ?? getAgentID(agent))}
                />
                <span className="dialog__agent-chip-label">
                  {String(agent?.agent_name ?? getAgentID(agent))}
                </span>
              </div>
            ))}
          </div>
        ) : null}
      </div>

      {memoHint ? (
        <div className="dialog__classify-hint" role="status">{memoHint}</div>
      ) : null}
      {memoError ? (
        <div className="dialog__classify-hint" role="alert">{memoError}</div>
      ) : null}

      {shellApprovalVisible ? (
        <div className="shell-approval-banner" role="dialog" aria-label="确认执行 shell 命令">
          <div className="shell-approval-banner__title">助手请求在本机执行命令，请确认</div>
          <pre className="shell-approval-banner__cmd">{shellApproval.command}</pre>
          <div className="shell-approval-banner__actions">
            <button
              type="button"
              className="shell-approval-banner__btn shell-approval-banner__btn--secondary"
              onClick={() => void resolveShellApproval(false)}
            >
              拒绝
            </button>
            <button
              type="button"
              className="shell-approval-banner__btn shell-approval-banner__btn--primary"
              onClick={() => void resolveShellApproval(true)}
            >
              允许执行
            </button>
          </div>
        </div>
      ) : null}

      <div className="dialog__messages">
        <div onMouseDown={onMessagesMemoDismissMouseDown}>
          {memoListMessages.map((message, index) => (
            <MessageBubble
              key={message?.messageID ?? `${message?.role ?? 'msg'}_${message?.timestamp ?? index}_${index}`}
              message={message}
              index={index}
              messages={memoListMessages}
              isStreaming={
                Boolean(streamPulse)
                && String(streamPulse?.chatID ?? '') === String(chatId ?? '')
                && String(streamPulse?.messageID ?? '')
                  === String(message?.messageID ?? message?.messageId ?? message?.id ?? '')
              }
              memoStripOpen={memoStripOpen}
              memoChecked={memoComposer.memoMarkedIds?.has?.(
                String(message?.messageID ?? message?.messageId ?? message?.id ?? '').trim(),
              )}
              onToggleMemo={memoComposer.tryToggleMemoMark}
            />
          ))}
        </div>

        <div ref={messagesEndRef} />
      </div>

      <div className="dialog__input">
        <div className="dialog__input-row">
          <button
            type="button"
            className="dialog__btn-stop dialog__btn-stop--visible"
            onClick={handleStopGenerating}
            disabled={!stopButtonEngaged}
          >
            <span className="dialog__btn-stop-icon" aria-hidden>⏹</span>
            <span className="dialog__btn-stop-text">{stopButtonEngaged ? '跑马中' : '摸鱼中'}</span>
          </button>

          <ChatInput
            value={inputValue}
            onChange={handleInputChange}
            onSend={handleSendMessage}
            mentionOptions={mentionOptions}
            mcpOptions={mcpOptions}
            skillOptions={skillOptions}
            onFocus={() => void loadCommandOptions()}
            onMentionPicked={(picked) => {
              const id = getAgentID(picked);
              if (!id) return;
              setAiteAgentIds((prev) => {
                const next = new Set(prev);
                next.add(id);
                return next;
              });
            }}
            disabled={taskBusy}
            placeholder={taskBusy ? "正在生成回复..." : "输入您的消息...@私聊..."}
          />
        </div>

        <MemoStrip
          open={memoStripOpen}
          busy={memoComposer.memoCheckSaving}
          markedCount={memoComposer.memoMarkedCount}
          presets={memoComposer.allMemoComposePresets}
          presetAddOpen={memoComposer.memoPresetAddOpen}
          draftLabel={memoComposer.memoPresetDraftLabel}
          draftText={memoComposer.memoPresetDraftText}
          composeHint={memoComposer.memoComposeHint}
          onToggleOpen={() => {
            if (memoComposer.memoCheckSaving) return;
            setMemoStripOpen((v) => !v);
          }}
          onSetComposeHint={memoComposer.setMemoComposeHint}
          onSaveDirect={() => void memoComposer.saveDirectMemo()}
          onSendLLM={() => void memoComposer.sendLLMMemo()}
          onTogglePresetAdd={() => memoComposer.setMemoPresetAddOpen((v) => !v)}
          onDraftLabel={memoComposer.setMemoPresetDraftLabel}
          onDraftText={memoComposer.setMemoPresetDraftText}
          onAddPreset={memoComposer.addCustomMemoPreset}
          onRemovePreset={memoComposer.removeCustomMemoPreset}
        />
      </div>
    </div>
  );
};

export default ChatDialog;
