import { MESSAGE_AVATAR_GROUP_GAP_MS } from '../constants';

/** @param {unknown} ts */
export function messageTimestampMs(ts) {
  if (ts == null || ts === '') return Number.NaN;
  if (typeof ts === 'number') return Number.isFinite(ts) ? ts : Number.NaN;
  const n = new Date(ts).getTime();
  return Number.isFinite(n) ? n : Number.NaN;
}

/**
 * 与列表中上一条已展示消息相比：同 role 且发送间隔小于阈值则不重复头像。
 * 但包含 agentID 的消息不受阈值规则限制，总是显示头像。
 *
 * @param {{ role: string, timestamp?: unknown, agentID?: string, content?: string }[]} list
 * @param {number} index
 */
export function shouldShowMessageAvatar(list, index) {
  if (index <= 0) return true;
  const cur = list[index];
  const prev = list[index - 1];
  if (!cur || !prev) return true;

  const hasAgentIDInContent = cur.content && String(cur.content).trim().startsWith('{agentID:');
  const hasAgentIDField = cur.agentID && String(cur.agentID).trim() !== '';
  if (hasAgentIDInContent || hasAgentIDField) return true;

  const curUser = cur.role === 'user';
  const prevUser = prev.role === 'user';
  if (curUser !== prevUser) return true;

  const curMs = messageTimestampMs(cur.timestamp);
  const prevMs = messageTimestampMs(prev.timestamp);
  if (Number.isNaN(curMs) || Number.isNaN(prevMs)) return true;
  return curMs - prevMs >= MESSAGE_AVATAR_GROUP_GAP_MS;
}

/**
 * 从助手正文里解析 `{agentID:xxx}` 的前缀（若存在）。
 * @param {unknown} rawContent
 * @returns {{ agentID: string, content: string }}
 */
export function parseAssistantAgentMeta(rawContent) {
  const raw = String(rawContent ?? '');
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

