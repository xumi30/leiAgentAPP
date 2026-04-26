import React, { useMemo, useState, useEffect, useRef } from 'react';
import { useChatStore } from '../../../stores';
import { useStreaming } from '../hooks/useStreaming';
import { MessageBubble, ChatInput } from '../../../components';
import { ChatService } from '../../../services/chatService';
import { AddAgentToConversation, AddConversation, GetConversationAgents, GetMessages, ListAgents, SwitchChat } from '../../../services/api';
import { classifyUserMessage } from '../../../utils/messageClassify';
import { EventsOn, EventsOff } from '../../../../wailsjs/runtime/runtime';
import assistantAvatar from '../../../assets/images/aitx.png';

const DEFAULT_ASSISTANT_AGENT_ID = 'agentid_0';

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

  const mentionOptions = useMemo(() => {
    const list = Array.isArray(conversationAgents) ? conversationAgents : [];
    return list
      .map((a) => ({
        agent_id: getAgentID(a),
        agent_name: String(a?.agent_name ?? '').trim(),
        avatar_image: String(a?.avatar_image ?? '').trim(),
      }))
      .filter((a) => a.agent_id && a.agent_name);
  }, [conversationAgents]);



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

    EventsOn('dialogAppend', onAppend);
    EventsOn('dialogStreamEnd', onStreamEnd);
    EventsOn('chatTaskState', onTaskState);
    EventsOn('GetMessagesByMessageID', onGetMessagesByMessageID);
    return () => {
      EventsOff('dialogAppend');
      EventsOff('dialogStreamEnd');
      EventsOff('chatTaskState');
      EventsOff('GetMessagesByMessageID');
    };
  }, [chatId, setMessages, startStreaming, stopStreaming]);

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
    const result = await ChatService.sendMessage(userMessage, { chatId: String(chatId ?? ''), aite: Array.from(aiteAgentIds) });

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

  // 停止生成按钮
  const handleStopGenerating = () => {
    ChatService.stopChat(String(chatId ?? ''));
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
            className="dialog__tab dialog__tab--active dialog__tab--main dialog__tab--convo-tint"
            title={(conversationTitle || '主对话').trim() || '主对话'}
          >
            <span className="dialog__tab-inline" dir="auto">
              <span className="dialog__tab-main-title-inline">
                {(conversationTitle || '主对话').trim() || '主对话'}
              </span>
            </span>
          </button>
        </div>

        {Array.isArray(conversationAgents) && conversationAgents.length > 0 ? (
          <div className="dialog__agents" aria-label="当前聊天已加入的 agents">
            {conversationAgents.map((agent) => (
              <div
                key={getAgentID(agent)}
                className="dialog__agent-chip"
                title={String(agent?.description ?? '')}
              >
                <img
                  className="dialog__agent-chip-avatar"
                  src={String(agent?.avatar_image ?? '')}
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

      <div className="dialog__messages">
        {currentMessages.map((message, index) => (
          <MessageBubble
            key={message?.messageID ?? `${message?.role ?? 'msg'}_${message?.timestamp ?? index}_${index}`}
            message={message}
            index={index}
            messages={currentMessages}
            isStreaming={
              Boolean(streamPulse)
              && String(streamPulse?.chatID ?? '') === String(chatId ?? '')
              && String(streamPulse?.messageID ?? '') === String(message?.messageID ?? '')
            }
          />
        ))}

        <div ref={messagesEndRef} />
      </div>

      <div className="dialog__input">
        <div className="dialog__input-row">
          <button
            type="button"
            className={`dialog__btn-stop${stopVisible ? ' dialog__btn-stop--visible' : ''}`}
            onClick={handleStopGenerating}
            aria-label="停止生成"
            title="停止生成"
          >
            <span className="dialog__btn-stop-icon" aria-hidden>⏹</span>
            <span className="dialog__btn-stop-text">停止</span>
          </button>

          <ChatInput
            value={inputValue}
            onChange={handleInputChange}
            onSend={handleSendMessage}
            mentionOptions={mentionOptions}
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
            placeholder={taskBusy ? "正在生成回复..." : "输入您的消息..."}
          />
        </div>
      </div>
    </div>
  );
};

export default ChatDialog;
