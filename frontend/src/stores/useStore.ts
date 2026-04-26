import { EventsOff, EventsOn } from '../../wailsjs/runtime/runtime';
import { GetConversationAgents, GetMessages, ListAgents } from '../services/api';
import { create } from 'zustand';
import { persist, subscribeWithSelector } from 'zustand/middleware';

type AnyRecord = Record<string, any>;

export type ChatMessage = {
  role: 'user' | 'assistant' | string;
  content?: string;
  timestamp?: unknown;
  agentID?: string;
  agent_id?: string;
  messageID?: string;
  messageId?: string;
  id?: string;
  chatID?: string;
  chatId?: string;
  total_tokens?: number;
  totalTokens?: number;
} & AnyRecord;

type AgentLike = AnyRecord;

function toStringSafe(v: unknown) {
  return String(v ?? '');
}

function parseAssistantAgentMeta(rawContent: unknown): { agentID: string; content: string } {
  const raw = toStringSafe(rawContent);
  const trimmed = raw.trim();
  if (!trimmed.startsWith('{agentID:') || !trimmed.includes('}')) {
    return { agentID: '', content: raw };
  }
  const idx = trimmed.indexOf('}');
  if (idx < 0) return { agentID: '', content: raw };
  const agentID = trimmed.slice('{agentID:'.length, idx).trim();
  const content = trimmed.slice(idx + 1).trim();
  return { agentID, content };
}

function normalizeLoadedMessages(raw: unknown): ChatMessage[] {
  if (!Array.isArray(raw)) return [];
  return raw.map((m: any) => {
    const role = toStringSafe(m?.role).trim() || 'assistant';
    const rawContent = toStringSafe(m?.content);
    const messageID = toStringSafe(m?.messageID ?? m?.messageId ?? m?.id).trim();
    const chatID = toStringSafe(m?.chatID ?? m?.chatId).trim();
    const directAgentID = toStringSafe(m?.agentID ?? m?.agent_id).trim();
    const tokRaw = m?.total_tokens ?? m?.totalTokens;
    const tokValue = typeof tokRaw === 'number' ? tokRaw : Number.parseInt(toStringSafe(tokRaw), 10);

    if (role === 'assistant') {
      const meta = parseAssistantAgentMeta(rawContent);
      return {
        ...m,
        role,
        chatID: chatID || undefined,
        messageID: messageID || undefined,
        agentID: (meta.agentID || directAgentID) || undefined,
        content: meta.content,
        total_tokens: Number.isFinite(tokValue) ? tokValue : undefined,
      };
    }

    return {
      ...m,
      role,
      chatID: chatID || undefined,
      messageID: messageID || undefined,
      agentID: directAgentID || undefined,
      content: rawContent,
      total_tokens: Number.isFinite(tokValue) ? tokValue : undefined,
    };
  });
}

export type ChatSlice = {
  currentChatId: string;
  messages: ChatMessage[];
  isStreaming: boolean;
  streamPulse: { chatID: string; messageID: string } | null;
  taskBusy: boolean;
  stopVisible: boolean;
  queuedInputs: { id: string; content: string }[];

  setCurrentChatId: (chatId: string) => void;
  setMessages: (next: ChatMessage[] | ((prev: ChatMessage[]) => ChatMessage[])) => void;
  clearMessages: () => void;
  setIsStreaming: (v: boolean) => void;
  setStreamPulse: (p: { chatID: string; messageID: string } | null) => void;
  setTaskBusy: (v: boolean) => void;
  setStopVisible: (v: boolean) => void;
  enqueueInput: (content: string) => void;
  dropQueuedInput: (id: string) => void;
  shiftQueuedInput: () => { id: string; content: string } | null;
  clearQueuedInputs: () => void;
};

export type AgentSlice = {
  allAgents: AgentLike[];
  currentChatAgents: AgentLike[];
  setAllAgents: (list: AgentLike[]) => void;
  setCurrentChatAgents: (list: AgentLike[]) => void;
  ensureAllAgentsLoaded: () => Promise<void>;
};

export type UISlice = {
  memoStripOpen: boolean;
  sidebarOpen: boolean;
  setMemoStripOpen: (v: boolean) => void;
  setSidebarOpen: (v: boolean) => void;
  toggleSidebar: () => void;
};

export type StoreSlice = ChatSlice &
  AgentSlice &
  UISlice & {
    initWailsEventBridge: () => () => void;
  };

let bridgeInited = false;
let bridgeCleanup: null | (() => void) = null;

export const useStore = create<StoreSlice>()(
  subscribeWithSelector(
    persist(
      (set, get) => ({
        // chatSlice
        currentChatId: '',
        messages: [],
        isStreaming: false,
        streamPulse: null,
        taskBusy: false,
        stopVisible: false,
        queuedInputs: [],

        setCurrentChatId: (chatId) => set({ currentChatId: toStringSafe(chatId).trim() }),
        setMessages: (next) =>
          set((state) => {
            const value = typeof next === 'function' ? next(state.messages) : next;
            return { messages: Array.isArray(value) ? value : [] };
          }),
        clearMessages: () => set({ messages: [] }),
        setIsStreaming: (v) => set({ isStreaming: Boolean(v) }),
        setStreamPulse: (p) => set({ streamPulse: p }),
        setTaskBusy: (v) => set({ taskBusy: Boolean(v) }),
        setStopVisible: (v) => set({ stopVisible: Boolean(v) }),
        enqueueInput: (content) => {
          const text = toStringSafe(content).trim();
          if (!text) return;
          set((s) => ({
            queuedInputs: [
              ...(Array.isArray(s.queuedInputs) ? s.queuedInputs : []),
              {
                id: `queued_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`,
                content: text,
              },
            ],
          }));
        },
        dropQueuedInput: (id) =>
          set((s) => ({
            queuedInputs: (Array.isArray(s.queuedInputs) ? s.queuedInputs : []).filter(
              (x) => x.id !== id,
            ),
          })),
        shiftQueuedInput: () => {
          const list = get().queuedInputs;
          if (!Array.isArray(list) || list.length === 0) return null;
          const picked = list[0];
          set({ queuedInputs: list.slice(1) });
          return picked;
        },
        clearQueuedInputs: () => set({ queuedInputs: [] }),

        // agentSlice
        allAgents: [],
        currentChatAgents: [],
        setAllAgents: (list) => set({ allAgents: Array.isArray(list) ? list : [] }),
        setCurrentChatAgents: (list) => set({ currentChatAgents: Array.isArray(list) ? list : [] }),
        ensureAllAgentsLoaded: async () => {
          try {
            const list = await ListAgents();
            set({ allAgents: Array.isArray(list) ? list : [] });
          } catch (e) {
            console.error('ListAgents:', e);
            set({ allAgents: [] });
          }
        },

        // uiSlice (persist)
        memoStripOpen: false,
        sidebarOpen: true,
        setMemoStripOpen: (v) => set({ memoStripOpen: Boolean(v) }),
        setSidebarOpen: (v) => set({ sidebarOpen: Boolean(v) }),
        toggleSidebar: () => set((s) => ({ sidebarOpen: !s.sidebarOpen })),

        // bridge
        initWailsEventBridge: () => {
          if (bridgeInited && bridgeCleanup) return bridgeCleanup;
          bridgeInited = true;

          void get().ensureAllAgentsLoaded();

          const onConversationChanged = (event: any) => {
            const { conversationId } = event?.detail ?? {};
            const nextChatId = toStringSafe(conversationId).trim();

            set({
              currentChatId: nextChatId,
              messages: [],
              isStreaming: false,
              streamPulse: null,
              currentChatAgents: [],
              taskBusy: false,
              stopVisible: false,
              queuedInputs: [],
            });

            if (!nextChatId) return;

            void (async () => {
              try {
                const [rawMessages, rawAgents] = await Promise.all([
                  GetMessages(nextChatId),
                  GetConversationAgents(nextChatId).catch(() => []),
                ]);
                set({
                  messages: normalizeLoadedMessages(rawMessages),
                  currentChatAgents: Array.isArray(rawAgents) ? rawAgents : [],
                });
              } catch (e) {
                console.error('conversationChanged load:', e);
                set({ messages: [], currentChatAgents: [] });
              }
            })();
          };

          const onDialogAppend = async (payload: any) => {
            const cid = toStringSafe(payload?.chatID ?? payload?.chatId).trim();
            const mid = toStringSafe(payload?.messageID ?? payload?.messageId ?? payload?.id).trim();
            const role = toStringSafe(payload?.role).trim() || 'assistant';
            const timestamp = payload?.timestamp;
            const rawContent = payload?.content;

            if (cid && !get().currentChatId) set({ currentChatId: cid });
            if (cid && get().currentChatId && cid !== get().currentChatId) return;

            const meta = role === 'assistant' ? parseAssistantAgentMeta(rawContent) : { agentID: '', content: toStringSafe(rawContent) };
            const directAgentID = toStringSafe(payload?.agentID ?? payload?.agent_id).trim();
            const tokRaw = payload?.total_tokens ?? payload?.totalTokens;
            const tokValue = typeof tokRaw === 'number' ? tokRaw : Number.parseInt(toStringSafe(tokRaw), 10);

            // 如果消息中有新的 agentID，则尝试补齐 agents 数据（避免头像只能靠刷新才出现）
            const newAgentID = meta.agentID || directAgentID;
            let shouldReloadAgents = false;
            let shouldReloadAllAgents = false;
            if (newAgentID && cid) {
              const currentState = get();
              // 检查当前agents中是否已包含这个agentID
              const agentExists = currentState.currentChatAgents.some(
                (a: any) => toStringSafe(a?.agentID ?? a?.agent_id ?? a?.id).trim() === newAgentID
              );
              shouldReloadAgents = !agentExists;

              // 如果全量 agents 也没有该 ID，则刷新 allAgents（常见于运行中新增/更新 agent）
              const matchedAll = currentState.allAgents.find(
                (a: any) => toStringSafe(a?.agent_id ?? a?.agentID ?? a?.id).trim() === newAgentID
              );
              // 有些情况下 agent 已存在，但 avatar_image 仍为空（例如运行中更新头像）；此时也需要刷新 allAgents
              const hasAvatar = Boolean(toStringSafe((matchedAll as any)?.avatar_image ?? (matchedAll as any)?.avatar).trim());
              shouldReloadAllAgents = !matchedAll || !hasAvatar;
            }

            if (cid && mid) {
              set({
                isStreaming: true,
                streamPulse: { chatID: cid, messageID: mid },
                taskBusy: true,
                stopVisible: true,
              });
            } else {
              set({ isStreaming: true, taskBusy: true, stopVisible: true });
            }

            // 如果需要则重新加载agents
            if (shouldReloadAgents && cid) {
              try {
                const rawAgents = await GetConversationAgents(cid).catch(() => []);
                set({ currentChatAgents: Array.isArray(rawAgents) ? rawAgents : [] });
              } catch (e) {
                console.error('重新加载对话agents失败:', e);
              }
            }

            if (shouldReloadAllAgents) {
              try {
                await get().ensureAllAgentsLoaded();
              } catch (e) {
                console.error('重新加载全量agents失败:', e);
              }
            }

            set((state) => {
              const list = Array.isArray(state.messages) ? state.messages : [];
              if (!mid) {
                return {
                  messages: [
                    ...list,
                    {
                      ...payload,
                      role,
                      content: meta.content,
                      agentID: (meta.agentID || directAgentID) || undefined,
                      chatID: cid || undefined,
                      messageID: undefined,
                      timestamp,
                      total_tokens: Number.isFinite(tokValue) ? tokValue : undefined,
                    },
                  ],
                };
              }

              const idx = list.findIndex((m) => toStringSafe(m?.messageID ?? m?.messageId ?? m?.id).trim() === mid);
              if (idx < 0) {
                return {
                  messages: [
                    ...list,
                    {
                      ...payload,
                      role,
                      content: meta.content,
                      agentID: (meta.agentID || directAgentID) || undefined,
                      chatID: cid || undefined,
                      messageID: mid,
                      timestamp,
                      total_tokens: Number.isFinite(tokValue) ? tokValue : undefined,
                    },
                  ],
                };
              }

              return {
                messages: list.map((m, i) => {
                  if (i !== idx) return m;
                  if (role === 'assistant') {
                    // 流式：同 messageID 追加 chunk；但如果 chunk 带了 meta，则直接覆盖正文
                    const nextContent = meta.agentID ? meta.content : toStringSafe(m?.content) + meta.content;
                    return {
                      ...m,
                      ...payload,
                      role,
                      chatID: cid || m?.chatID,
                      messageID: mid,
                      agentID: (meta.agentID || directAgentID || toStringSafe(m?.agentID)).trim() || undefined,
                      content: nextContent,
                      timestamp: timestamp ?? m?.timestamp,
                      total_tokens:
                        Number.isFinite(tokValue) && tokValue > 0
                          ? tokValue
                          : (m?.total_tokens ?? m?.totalTokens),
                    };
                  }
                  return {
                    ...m,
                    ...payload,
                    role,
                    chatID: cid || m?.chatID,
                    messageID: mid,
                    agentID: (directAgentID || toStringSafe(m?.agentID)).trim() || undefined,
                    content: meta.content,
                    timestamp: timestamp ?? m?.timestamp,
                    total_tokens:
                      Number.isFinite(tokValue) && tokValue > 0 ? tokValue : (m?.total_tokens ?? m?.totalTokens),
                  };
                }),
              };
            });
          };

          const onDialogStreamEnd = (payload: any) => {
            const cid = toStringSafe(payload?.chatID ?? payload?.chatId).trim();
            const mid = toStringSafe(payload?.messageID ?? payload?.messageId ?? payload?.id).trim();
            const agentID = toStringSafe(payload?.agentID ?? payload?.agent_id ?? payload?.agentId).trim();
            const cur = get().streamPulse;
            if (cur && cid && cur.chatID !== cid) return;
            if (cur && mid && cur.messageID !== mid) return;

            // 后端在 dialogStreamEnd 才补齐 agentID 时，需要回写到对应消息，否则头像只能靠刷新才出现
            if (mid && agentID) {
              set((state) => {
                const list = Array.isArray(state.messages) ? state.messages : [];
                if (list.length === 0) return { messages: list };
                let changed = false;
                const next = list.map((m) => {
                  const mId = toStringSafe(m?.messageID ?? m?.messageId ?? m?.id).trim();
                  if (mId !== mid) return m;
                  const existing = toStringSafe((m as any)?.agentID ?? (m as any)?.agent_id ?? (m as any)?.agentId).trim();
                  if (existing) return m;
                  changed = true;
                  return { ...m, agentID };
                });
                return changed ? { messages: next } : { messages: list };
              });

              // 如果当前 agents 里还没有这个 agentID（或头像为空），立刻刷新 agents，避免头像卡在默认图
              // 注意：这里不 await，避免阻塞 UI
              try {
                const st = get();
                const inChat = Array.isArray(st.currentChatAgents) ? st.currentChatAgents : [];
                const all = Array.isArray(st.allAgents) ? st.allAgents : [];
                const inChatHit = inChat.find((a: any) => toStringSafe(a?.agentID ?? a?.agent_id ?? a?.id).trim() === agentID);
                const allHit = all.find((a: any) => toStringSafe(a?.agentID ?? a?.agent_id ?? a?.id).trim() === agentID);
                const avatar = toStringSafe((allHit as any)?.avatar_image ?? (allHit as any)?.avatar).trim();
                const needReloadAgents = !inChatHit;
                const needReloadAll = !allHit || !avatar;
                if (cid && needReloadAgents) {
                  void GetConversationAgents(cid)
                    .then((raw) => set({ currentChatAgents: Array.isArray(raw) ? raw : [] }))
                    .catch(() => {});
                }
                if (needReloadAll) {
                  void get().ensureAllAgentsLoaded();
                }
              } catch {
                // ignore
              }
            }
            set({ isStreaming: false, streamPulse: null, taskBusy: false, stopVisible: false });
          };

          const onChatTaskState = (payload: any) => {
            const cid = toStringSafe(payload?.chatID ?? payload?.chatId).trim();
            if (cid && get().currentChatId && cid !== get().currentChatId) return;
            const busy = Boolean(payload?.busy);
            set({ isStreaming: busy, taskBusy: busy, stopVisible: busy });
            if (!busy) set({ streamPulse: null, queuedInputs: [] });
          };

          const onGetMessagesByMessageID = (payload: any) => {
            const cid = toStringSafe(payload?.chatID ?? payload?.chatId).trim();
            if (cid && get().currentChatId && cid !== get().currentChatId) return;
            const nextMsgID = toStringSafe(payload?.messageID ?? payload?.messageId ?? payload?.id).trim();
            if (!nextMsgID) return;

            const merged = normalizeLoadedMessages([payload])[0];
            if (!merged) return;

            set((state) => {
              const list = Array.isArray(state.messages) ? state.messages : [];
              const exists = list.some((m) => toStringSafe(m?.messageID ?? m?.messageId ?? m?.id).trim() === nextMsgID);
              if (!exists) return { messages: list };
              return {
                messages: list.map((m) => {
                  const mid = toStringSafe(m?.messageID ?? m?.messageId ?? m?.id).trim();
                  if (mid !== nextMsgID) return m;
                  return { ...m, ...merged };
                }),
              };
            });
          };

          const onError = (payload: any) => {
            console.error('chat error:', payload);
            set({ isStreaming: false, streamPulse: null, taskBusy: false, stopVisible: false });
          };

          window.addEventListener('conversationChanged', onConversationChanged);
          EventsOn('dialogAppend', onDialogAppend);
          EventsOn('dialogStreamEnd', onDialogStreamEnd);
          EventsOn('chatTaskState', onChatTaskState);
          EventsOn('GetMessagesByMessageID', onGetMessagesByMessageID);
          EventsOn('sendMessageError', onError);
          EventsOn('dispatcherError', onError);

          bridgeCleanup = () => {
            window.removeEventListener('conversationChanged', onConversationChanged);
            EventsOff('dialogAppend');
            EventsOff('dialogStreamEnd');
            EventsOff('chatTaskState');
            EventsOff('GetMessagesByMessageID');
            EventsOff('sendMessageError');
            EventsOff('dispatcherError');
          };

          return bridgeCleanup;
        },
      }),
      {
        name: 'leiAgent.uiPrefs.v1',
        partialize: (state) => ({
          memoStripOpen: state.memoStripOpen,
          sidebarOpen: state.sidebarOpen,
        }),
      },
    ),
  ),
);

/**
 * 入口处调用一次即可（StrictMode 下也安全）。
 */
export function initStoreEventBridge() {
  return useStore.getState().initWailsEventBridge();
}

