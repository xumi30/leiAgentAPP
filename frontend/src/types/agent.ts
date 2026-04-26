// 代理相关类型定义

export interface Agent {
  id: string;
  name: string;
  description?: string;
  avatar?: string;
  capabilities?: string[];
}

export interface AgentConfig {
  model?: string;
  temperature?: number;
  maxTokens?: number;
  systemPrompt?: string;
}