// 备忘录功能类型定义

export interface MemoPreset {
  id: string;
  label: string;
  text: string;
}

export interface MemoState {
  memoStripOpen: boolean;
  memoCustomPresets: MemoPreset[];
  memoReferencedMessages: string[];
}

export const MEMO_CUSTOM_PRESETS_STORAGE_KEY = 'leiAgent.memoComposeCustomPresets.v1';

export const MEMO_COMPOSE_PRESETS_DEFAULT: MemoPreset[] = [
  { id: 'builtin:0', label: '傲娇女王', text: '以傲娇女王的口气总结一下' },
  { id: 'builtin:1', label: '萝莉语气', text: '以娇滴滴的萝莉语气复述一下' },
  { id: 'builtin:2', label: '项羽霸王', text: '以刚猛项羽霸王的姿态' },
  { id: 'builtin:3', label: '毛式智慧', text: '以毛主席的智慧讲讲' },
];