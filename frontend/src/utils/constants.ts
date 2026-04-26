// 应用常量定义

// 消息头像显示间隔（毫秒）
export const MESSAGE_AVATAR_GROUP_GAP_MS = 3 * 60 * 1000;

// 队列提示延迟
// 小于 1000ms，立即反馈
// 1000-2200ms，给予快速状态提示
// >2200ms，显示专门等待界面
export const QUEUE_HINT_DELAY_MS = 2200;

// 备忘录预设存储键名
export const MEMO_CUSTOM_PRESETS_STORAGE_KEY = 'leiAgent.memoComposeCustomPresets.v1';

// 默认备忘录预设
export const MEMO_COMPOSE_PRESETS_DEFAULT = [
  { id: 'builtin:0', label: '傲娇女王', text: '以傲娇女王的口气总结一下' },
  { id: 'builtin:1', label: '萝莉语气', text: '以娇滴滴的萝莉语气复述一下' },
  { id: 'builtin:2', label: '项羽霸王', text: '以刚猛项羽霸王的姿态' },
  { id: 'builtin:3', label: '毛式智慧', text: '以毛主席的智慧讲讲' },
];

// 角色类型定义
export type RoleType = 'user' | 'assistant';

// 主标签页ID
export const MAIN_SHEET_ID = 'main';