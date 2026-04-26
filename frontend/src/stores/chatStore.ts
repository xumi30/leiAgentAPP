import { create } from 'zustand';
import type { ChatState, Message, Sheet } from '../types/chat';

interface ChatActions {
  setChatId: (chatId: string) => void;
  setMessages: (messages: Message[] | ((prev: Message[]) => Message[])) => void;
  addMessage: (message: Message) => void;
  clearMessages: () => void;
  setStopVisible: (visible: boolean) => void;
  setTaskBusy: (busy: boolean) => void;
  setStreamPulse: (pulse: string | null) => void;
  setQueuedInputs: (inputs: string[]) => void;
  addToQueue: (input: string) => void;
  processQueue: (processor: (input: string) => void) => void;
  clearQueue: () => void;
  setSheets: (sheets: Sheet[]) => void;
  addSheet: (sheet: Sheet) => void;
  switchSheet: (sheetId: string) => void;
  removeSheet: (sheetId: string) => void;
  setActiveSheetId: (sheetId: string) => void;
  setConversationTitle: (title: string) => void;
  setConversationAgents: (agents: any[]) => void;
  setAllAgents: (agents: any[]) => void;
  setAgentsById: (agentsById: Map<string, any>) => void;
  updateAgentsById: () => void;
  setClassifyHint: (hint: string) => void;
  setRuntimeError: (error: string) => void;
}

export const useChatStore = create<ChatState & ChatActions>((set, get) => ({
  // 状态
  chatId: '',
  messages: [],
  stopVisible: false,
  taskBusy: false,
  streamPulse: null,
  queuedInputs: [],
  sheets: [{ id: 'main', title: '主对话', startIdx: 0 }],
  activeSheetId: 'main',
  conversationTitle: '',
  conversationAgents: [],
  allAgents: [],
  agentsById: new Map(),
  classifyHint: '',
  runtimeError: '',

  // 操作方法
  setChatId: (chatId) => set({ chatId }),
  
  setMessages: (messages) => set((state) => {
    const next = typeof messages === 'function' ? messages(state.messages) : messages;
    return { messages: Array.isArray(next) ? next : [] };
  }),
  
  addMessage: (message) => set((state) => ({
    messages: [...state.messages, message]
  })),
  
  clearMessages: () => set({ messages: [] }),
  
  setStopVisible: (stopVisible) => set({ stopVisible }),
  
  setTaskBusy: (taskBusy) => set({ taskBusy }),
  
  setStreamPulse: (streamPulse) => set({ streamPulse }),
  
  setQueuedInputs: (queuedInputs) => set({ queuedInputs }),
  
  addToQueue: (input) => set((state) => ({
    queuedInputs: [...state.queuedInputs, input]
  })),
  
  processQueue: (processor) => {
    const state = get();
    if (state.queuedInputs.length === 0) return;
    
    const nextInput = state.queuedInputs[0];
    processor(nextInput);
    set({ queuedInputs: state.queuedInputs.slice(1) });
  },
  
  clearQueue: () => set({ queuedInputs: [] }),
  
  setSheets: (sheets) => set({ sheets }),
  
  addSheet: (sheet) => set((state) => ({
    sheets: [...state.sheets, sheet],
    activeSheetId: sheet.id
  })),
  
  switchSheet: (sheetId) => set({ activeSheetId: sheetId }),
  
  removeSheet: (sheetId) => {
    if (sheetId === 'main') return;
    
    set((state) => {
      const newSheets = state.sheets.filter(sheet => sheet.id !== sheetId);
      const newActiveSheetId = state.activeSheetId === sheetId ? 'main' : state.activeSheetId;
      return { 
        sheets: newSheets, 
        activeSheetId: newActiveSheetId 
      };
    });
  },
  
  setActiveSheetId: (activeSheetId) => set({ activeSheetId }),
  
  setConversationTitle: (conversationTitle) => set({ conversationTitle }),
  
  setConversationAgents: (conversationAgents) => set({ conversationAgents }),
  
  setAllAgents: (allAgents) => set((state) => {
    const nextAllAgents = Array.isArray(allAgents) ? allAgents : [];
    
    // 自动更新 agentsById
    const DEFAULT_ASSISTANT_AGENT_ID = 'agentid_0';
    const m = new Map();
    
    // 遍历全量列表，构建 Map
    for (const a of nextAllAgents) {
      const id = String(a?.agentID ?? a?.agent_id ?? '').trim();
      if (!id) continue;
      m.set(id, a);
    }

    // 确保默认助手存在
    if (!m.has(DEFAULT_ASSISTANT_AGENT_ID)) {
      m.set(DEFAULT_ASSISTANT_AGENT_ID, {
        agentID: DEFAULT_ASSISTANT_AGENT_ID,
        agent_name: '工具人',
        avatar_image: '', // 使用默认头像
      });
    }
    
    return { allAgents: nextAllAgents, agentsById: m };
  }),

  setAgentsById: (agentsById) => set({ agentsById }),

  updateAgentsById: () => set((state) => {
    const DEFAULT_ASSISTANT_AGENT_ID = 'agentid_0';
    const m = new Map();
    const all = Array.isArray(state.allAgents) ? state.allAgents : [];
    
    // 遍历全量列表，构建 Map
    for (const a of all) {
      const id = String(a?.agentID ?? a?.agent_id ?? '').trim();
      if (!id) continue;
      m.set(id, a);
    }

    // 确保默认助手存在
    if (!m.has(DEFAULT_ASSISTANT_AGENT_ID)) {
      m.set(DEFAULT_ASSISTANT_AGENT_ID, {
        agentID: DEFAULT_ASSISTANT_AGENT_ID,
        agent_name: '工具人',
        avatar_image: '', // 使用默认头像
      });
    }
    return { agentsById: m };
  }),

  setClassifyHint: (classifyHint) => set({ classifyHint }),
  
  setRuntimeError: (runtimeError) => set({ runtimeError }),
}));