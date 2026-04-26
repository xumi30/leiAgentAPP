import { MEMO_CUSTOM_PRESETS_STORAGE_KEY } from '../constants';

function clipSheetTitle(text) {
  const line = String(text ?? '').split(/\r?\n/)[0].trim();
  if (!line) return '便签';
  const max = 22;
  return line.length > max ? `${line.slice(0, max)}…` : line;
}

/** 从助手正文取备忘录 # 标题（首行，去 Markdown 标题前缀） */
export function titleForMemoFromBody(body) {
  const lines = String(body ?? '')
    .split(/\r?\n/)
    .map((l) => l.trim())
    .filter(Boolean);
  const first = lines[0] ?? '对话摘录';
  const t = first.replace(/^#{1,6}\s+/, '').slice(0, 56).trim();
  return t || '对话摘录';
}

/** @param {string} role */
export function roleLabelForMemo(role) {
  if (role === 'user') return '用户';
  if (role === 'assistant') return '助手';
  return '消息';
}

/**
 * @param {{ role: string, content?: string }[]} orderedMarked 按时间顺序、已勾选的条目
 * @returns {{ title: string, body: string } | null}
 */
export function buildMemoMarkdownFromMarked(orderedMarked) {
  if (!orderedMarked.length) return null;
  const parts = orderedMarked.map((m) => {
    const label = roleLabelForMemo(m.role);
    return `**${label}**\n\n${String(m.content ?? '').trim()}`;
  });
  const body = parts.join('\n\n---\n\n');
  const titleSource =
    orderedMarked.find((m) => m.role === 'user')?.content
    ?? orderedMarked.find((m) => m.role !== 'user')?.content
    ?? orderedMarked[0].content;
  const title = titleForMemoFromBody(titleSource).replace(/\s+/g, ' ').trim();
  return { title, body };
}

/** @returns {{ id: string, label: string, text: string }[]} */
export function loadCustomMemoPresets() {
  try {
    const raw = localStorage.getItem(MEMO_CUSTOM_PRESETS_STORAGE_KEY);
    if (!raw) return [];
    const arr = JSON.parse(raw);
    if (!Array.isArray(arr)) return [];
    return arr
      .filter((p) => p && typeof p.label === 'string' && typeof p.text === 'string')
      .map((p) => ({
        id:
          typeof p.id === 'string' && p.id
            ? p.id
            : `u:${Date.now()}_${Math.random().toString(36).slice(2, 9)}`,
        label: p.label.trim().slice(0, 24),
        text: p.text.trim().slice(0, 800),
      }))
      .filter((p) => p.label && p.text);
  } catch {
    return [];
  }
}

/** @param {{ id: string, label: string, text: string }[]} list */
export function saveCustomMemoPresets(list) {
  try {
    localStorage.setItem(MEMO_CUSTOM_PRESETS_STORAGE_KEY, JSON.stringify(list));
  } catch (e) {
    console.warn('saveCustomMemoPresets', e);
  }
}

/**
 * @param {string} title
 * @param {string} body
 * @param {Array<string|number>} messageIds
 */
export function formatMemoAppendBlock(title, body, messageIds) {
  const ids = (Array.isArray(messageIds) ? messageIds : []).map(String).join(',');
  return `# ${String(title || '').trim()}\n\n${String(body || '').trim()}\n\n<!--leiAgent-memo-src:${ids}-->`;
}

export const __private = { clipSheetTitle };

