// 聊天功能类型定义

export interface Message {
  role: 'user' | 'assistant';
  content: string;
  timestamp?: number | string;
  agentID?: string;
  messageId?: string;
}

export interface Conversation {
  id: string;
  title: string;
  lastMessage?: string;
  timestamp: number;
}

export interface Sheet {
  id: string;
  title: string;
  startIdx: number;
}

export interface ChatState {
  chatId: string;
  messages: Message[];
  stopVisible: boolean;
  taskBusy: boolean;
  streamPulse: string | null;
  queuedInputs: string[];
  sheets: Sheet[];
  activeSheetId: string;
  conversationTitle: string;
  conversationAgents: any[];
  allAgents: any[];
  agentsById?: Map<string, any>;
  classifyHint: string;
  runtimeError: string;
}

export const MAIN_SHEET_ID = 'main';

// 消息分类类型
export type MessageClassType = 'control' | 'supplement' | 'newTopic';

export interface MessageClassifyResult {
  type: MessageClassType;
  label?: string;
  confidence: number;
}