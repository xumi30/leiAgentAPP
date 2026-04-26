import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { EventsOff, EventsOn } from '../../wailsjs/runtime/runtime';
import { GetMessages } from '../services/api';
import type { Message, Sheet } from '../types/chat';

type RuntimeErrorLike = unknown;

export interface UseChatMessagesOptions {
  chatId: string;
  onRuntimeError?: (message: string, error?: RuntimeErrorLike) => void;
}

export interface StreamPulse {
  chatID: string;
  messageID: string;
}

export interface ChatMessage extends Message {
  chatID?: string;
  messageID?: string;
  total_tokens?: number;
  totalTokens?: number;
}

function toStringSafe(v: unknown) {
  return String(v ?? '');
}

function parseAssistantAgentMeta(rawContent: string): { agentID: string; content: string } {
  const raw = toStringSafe(rawContent);
  const trimmed = raw.trim();
  if (!trimmed.startsWith('{agentID:') || !trimmed.includes('}')) {
    return { agentID: '', content: raw };
  }
  const closingBraceIndex = trimmed.indexOf('}');
  if (closingBraceIndex < 0) return { agentID: '', content: raw };
  const agentID = trimmed.slice('{agentID:'.length, closingBraceIndex).trim();
  const content = trimmed.slice(closingBraceIndex + 1).trim();
  return { agentID, content };
}

function normalizeLoadedMessages(raw: unknown): ChatMessage[] {
  if (!Array.isArray(raw)) return [];
  return raw.map((m) => {
    const role = (toStringSafe((m as any)?.role).trim() || 'assistant') as ChatMessage['role'];
    const content = toStringSafe((m as any)?.content);
    const messageID = toStringSafe((m as any)?.messageID ?? (m as any)?.messageId ?? (m as any)?.id);
    const chatID = toStringSafe((m as any)?.chatID ?? (m as any)?.chatId);
    const timestamp = (m as any)?.timestamp;
    const tokRaw = (m as any)?.total_tokens ?? (m as any)?.totalTokens;
    const total_tokens =
      typeof tokRaw === 'number' ? tokRaw : Number.parseInt(toStringSafe(tokRaw), 10);

    if (role === 'assistant') {
      const meta = parseAssistantAgentMeta(content);
      return {
        role,
        content: meta.content,
        agentID: meta.agentID || toStringSafe((m as any)?.agentID ?? (m as any)?.agent_id).trim(),
        messageID,
        chatID,
        timestamp,
        total_tokens: Number.isFinite(total_tokens) ? total_tokens : undefined,
      };
    }

    return {
      role: role === 'user' ? 'user' : 'assistant',
      content,
      agentID: toStringSafe((m as any)?.agentID ?? (m as any)?.agent_id).trim() || undefined,
      messageID,
      chatID,
      timestamp,
      total_tokens: Number.isFinite(total_tokens) ? total_tokens : undefined,
    };
  });
}

/**
 * useChatMessages
 * - 负责：GetMessages(chatId) + 监听 Wails EventsOn("dialogAppend"/"dialogStreamEnd"/"chatTaskState"/...)
 * - 不负责：UI 滚动、消息列表分 sheet 切片（由上层组件决定）
 */
export function useChatMessages(opts: UseChatMessagesOptions) {
  const { chatId, onRuntimeError } = opts;

  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [streamPulse, setStreamPulse] = useState<StreamPulse | null>(null);
  const [taskBusy, setTaskBusy] = useState(false);
  const [stopVisible, setStopVisible] = useState(false);

  const chatIdRef = useRef<string>(toStringSafe(chatId));
  chatIdRef.current = toStringSafe(chatId);

  // sheets 仍保留：Dialog.jsx 目前依赖这个结构（即便暂时禁用新建/切换）
  const [sheets, setSheets] = useState<Sheet[]>([{ id: 'main', title: '主对话', startIdx: 0 }]);
  const [activeSheetId, setActiveSheetId] = useState<string>('main');

  const resetForChat = useCallback(() => {
    setMessages([]);
    setStreamPulse(null);
    setTaskBusy(false);
    setStopVisible(false);
    setSheets([{ id: 'main', title: '主对话', startIdx: 0 }]);
    setActiveSheetId('main');
  }, []);

  // chatId 变化时加载历史消息
  useEffect(() => {
    const cid = toStringSafe(chatId).trim();
    resetForChat();
    if (!cid) return;

    let cancelled = false;
    (async () => {
      try {
        const loaded = await GetMessages(cid);
        if (cancelled) return;
        setMessages(normalizeLoadedMessages(loaded));
      } catch (e) {
        if (cancelled) return;
        onRuntimeError?.(`加载对话失败：${(e as any)?.message ?? String(e)}`, e);
        setMessages([]);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [chatId, onRuntimeError, resetForChat]);

  // 事件监听：只挂一次，内部用 ref 对齐当前 chatId
  useEffect(() => {
    const handleDialogAppend = (payload: any) => {
      const nextChatID = toStringSafe(payload?.chatID ?? payload?.chatId).trim();
      const nextMsgID = toStringSafe(payload?.messageID ?? payload?.messageId ?? payload?.id).trim();

      setStreamPulse({
        chatID: nextChatID,
        messageID: nextMsgID,
      });

      const role = toStringSafe(payload?.role).trim();
      const rawContent = toStringSafe(payload?.content);
      const timestamp = payload?.timestamp;

      const tokRaw = payload?.total_tokens ?? payload?.totalTokens;
      const tokValue =
        typeof tokRaw === 'number' ? tokRaw : Number.parseInt(toStringSafe(tokRaw), 10);

      let content = rawContent;
      let agentID = toStringSafe(payload?.agentID ?? payload?.agent_id).trim();

      if (role === 'assistant') {
        const meta = parseAssistantAgentMeta(rawContent);
        if (meta.agentID) agentID = meta.agentID;
        content = meta.content;
      }

      setMessages((prev) => {
        const list = Array.isArray(prev) ? prev : [];
        if (!nextMsgID) {
          return [
            ...list,
            {
              role: role === 'user' ? 'user' : 'assistant',
              content,
              timestamp,
              agentID: agentID || undefined,
              chatID: nextChatID || undefined,
              messageID: undefined,
              total_tokens: Number.isFinite(tokValue) && tokValue > 0 ? tokValue : undefined,
            },
          ];
        }

        const exists = list.some((m) => toStringSafe(m.messageID) === nextMsgID);
        if (!exists) {
          return [
            ...list,
            {
              role: role === 'user' ? 'user' : 'assistant',
              content,
              timestamp,
              agentID: agentID || undefined,
              chatID: nextChatID || undefined,
              messageID: nextMsgID,
              total_tokens: Number.isFinite(tokValue) && tokValue > 0 ? tokValue : undefined,
            },
          ];
        }

        // 复刻 Dialog.jsx 行为：assistant 流式时做拼接；user/非流式做覆盖
        return list.map((m) => {
          if (toStringSafe(m.messageID) !== nextMsgID) return m;
          const isAssistant = (m.role ?? role) === 'assistant';
          if (isAssistant) {
            return {
              ...m,
              agentID: agentID || m.agentID,
              content: toStringSafe(m.content) + content,
              total_tokens:
                Number.isFinite(tokValue) && tokValue > 0
                  ? tokValue
                  : (m.total_tokens ?? m.totalTokens),
            };
          }
          return {
            ...m,
            content,
            agentID: agentID || m.agentID,
            total_tokens:
              Number.isFinite(tokValue) && tokValue > 0
                ? tokValue
                : (m.total_tokens ?? m.totalTokens),
          };
        });
      });
    };

    const handleDialogStreamEnd = (payload: any) => {
      const cid = toStringSafe(payload?.chatID ?? payload?.chatId);
      const mid = toStringSafe(payload?.messageID ?? payload?.messageId);
      setStreamPulse((prev) => {
        if (!prev) return prev;
        if (prev.chatID === cid && prev.messageID === mid) return null;
        return prev;
      });
    };

    const handleChatTaskState = (payload: any) => {
      const cid = toStringSafe(payload?.chatID ?? payload?.chatId);
      if (cid && cid !== chatIdRef.current) return;
      const busy = Boolean(payload?.busy);
      setTaskBusy(busy);
      setStopVisible(busy);
    };

    const handleGetMessagesByMessageID = (payload: any) => {
      const nextMsgID = toStringSafe(payload?.messageID ?? payload?.messageId ?? payload?.id).trim();
      if (!nextMsgID) return;
      setMessages((prev) => {
        const list = Array.isArray(prev) ? prev : [];
        const exists = list.some((m) => toStringSafe(m.messageID) === nextMsgID);
        if (!exists) return list;
        const merged = normalizeLoadedMessages([payload])[0];
        return list.map((m) => (toStringSafe(m.messageID) === nextMsgID ? { ...m, ...merged } : m));
      });
    };

    const handleSendError = (error: any) => {
      const msg = toStringSafe(error?.message ?? error ?? '未知错误');
      onRuntimeError?.(`发送消息失败：${msg}`, error);
      setTaskBusy(false);
      setStopVisible(false);
      setStreamPulse(null);
    };

    const handleDispatcherError = (error: any) => {
      const msg = toStringSafe(error?.message ?? error ?? '未知错误');
      onRuntimeError?.(`无法启动对话引擎：${msg}`, error);
      setTaskBusy(false);
      setStopVisible(false);
      setStreamPulse(null);
    };

    EventsOn('dialogAppend', handleDialogAppend);
    EventsOn('dialogStreamEnd', handleDialogStreamEnd);
    EventsOn('chatTaskState', handleChatTaskState);
    EventsOn('GetMessagesByMessageID', handleGetMessagesByMessageID);
    EventsOn('sendMessageError', handleSendError);
    EventsOn('dispatcherError', handleDispatcherError);

    return () => {
      EventsOff('dialogAppend');
      EventsOff('dialogStreamEnd');
      EventsOff('chatTaskState');
      EventsOff('GetMessagesByMessageID');
      EventsOff('sendMessageError');
      EventsOff('dispatcherError');
    };
  }, [onRuntimeError]);

  const activeSheet = useMemo(
    () => sheets.find((s) => s.id === activeSheetId) ?? sheets[0],
    [sheets, activeSheetId],
  );

  const activeSheetMessages = useMemo(() => {
    const sh = activeSheet;
    if (!sh) return messages;
    return messages.slice(sh.startIdx);
  }, [messages, activeSheet]);

  return {
    messages,
    setMessages,
    streamPulse,
    taskBusy,
    stopVisible,
    sheets,
    setSheets,
    activeSheetId,
    setActiveSheetId,
    activeSheetMessages,
  };
}