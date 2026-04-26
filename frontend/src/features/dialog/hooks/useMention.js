import { useCallback, useMemo, useRef, useState } from 'react';

function caretXYInTextarea(textarea, position) {
  if (!textarea) return { left: 0, top: 0, lineHeight: 20 };
  const style = window.getComputedStyle(textarea);
  const div = document.createElement('div');
  div.style.position = 'absolute';
  div.style.visibility = 'hidden';
  div.style.whiteSpace = 'pre-wrap';
  div.style.wordWrap = 'break-word';
  div.style.fontFamily = style.fontFamily;
  div.style.fontSize = style.fontSize;
  div.style.fontWeight = style.fontWeight;
  div.style.letterSpacing = style.letterSpacing;
  div.style.lineHeight = style.lineHeight;
  div.style.padding = style.padding;
  div.style.border = style.border;
  div.style.boxSizing = style.boxSizing;
  div.style.width = style.width;
  div.style.overflow = 'hidden';
  div.style.height = 'auto';
  const text = textarea.value.substring(0, position);
  div.textContent = text;
  const span = document.createElement('span');
  span.textContent = textarea.value.substring(position) || '.';
  div.appendChild(span);
  document.body.appendChild(div);
  const left = span.offsetLeft - textarea.scrollLeft;
  const top = span.offsetTop - textarea.scrollTop;
  document.body.removeChild(div);
  const lh = Number.parseFloat(style.lineHeight) || 20;
  return { left, top, lineHeight: lh };
}

/**
 * useMention
 * - 负责：@ 触发、query 维护、候选过滤、anchor 计算、选中回填范围维护
 *
 * @param {{ candidates: { agent_id: string, agent_name: string, avatar_image?: string }[] }} opts
 */
export function useMention(opts) {
  const { candidates } = opts || {};
  const [mentionOpen, setMentionOpen] = useState(false);
  const [mentionQuery, setMentionQuery] = useState('');
  const [mentionActiveIndex, setMentionActiveIndex] = useState(0);
  const [mentionAnchor, setMentionAnchor] = useState({ left: 10, top: 0 });
  const mentionAtRef = useRef({ start: -1, end: -1 }); // [start,end)

  const mentionCandidates = useMemo(() => {
    if (!mentionOpen) return [];
    const list = Array.isArray(candidates) ? candidates : [];
    const q = mentionQuery.trim().toLowerCase();
    if (!q) return list;
    return list.filter((a) => String(a?.agent_name ?? '').toLowerCase().includes(q));
  }, [mentionOpen, mentionQuery, candidates]);

  const updateMentionAnchorFromInput = useCallback((textarea, shell) => {
    const ta = textarea;
    const wrap = shell;
    if (!ta || !wrap) return;
    const shellRect = wrap.getBoundingClientRect();
    const taRect = ta.getBoundingClientRect();
    const pos = typeof ta.selectionStart === 'number' ? ta.selectionStart : (ta.value || '').length;
    const caret = caretXYInTextarea(ta, pos);
    const rawLeft = (taRect.left - shellRect.left) + caret.left;
    const rawTop = (taRect.top - shellRect.top) + caret.top;
    const shellW = shellRect.width || 0;
    const dropdownW = 260;
    const left = Math.max(10, Math.min(rawLeft, Math.max(10, shellW - dropdownW - 10)));
    const top = Math.max(0, rawTop - 8);
    setMentionAnchor({ left, top });
  }, []);

  const syncMentionState = useCallback((nextValue, cursorPos) => {
    const value = String(nextValue ?? '');
    const cursor = typeof cursorPos === 'number' ? cursorPos : value.length;
    const upto = value.slice(0, cursor);
    const at = upto.lastIndexOf('@');
    if (at < 0) {
      setMentionOpen(false);
      setMentionQuery('');
      mentionAtRef.current = { start: -1, end: -1 };
      return;
    }
    const before = at === 0 ? '' : upto[at - 1];
    const okBoundary = at === 0 || before === ' ' || before === '\n' || before === '\t' || before === ',' || before === '，';
    if (!okBoundary) {
      setMentionOpen(false);
      setMentionQuery('');
      mentionAtRef.current = { start: -1, end: -1 };
      return;
    }
    const frag = upto.slice(at + 1);
    if (frag.includes(' ') || frag.includes('\n') || frag.includes('\t') || frag.includes(',') || frag.includes('，')) {
      setMentionOpen(false);
      setMentionQuery('');
      mentionAtRef.current = { start: -1, end: -1 };
      return;
    }
    setMentionOpen(true);
    setMentionQuery(frag);
    setMentionActiveIndex(0);
    mentionAtRef.current = { start: at, end: cursor };
  }, []);

  const closeMention = useCallback(() => {
    setMentionOpen(false);
    setMentionQuery('');
    mentionAtRef.current = { start: -1, end: -1 };
  }, []);

  return {
    mentionOpen,
    mentionQuery,
    mentionActiveIndex,
    setMentionActiveIndex,
    mentionAnchor,
    mentionCandidates,
    mentionAtRef,
    updateMentionAnchorFromInput,
    syncMentionState,
    closeMention,
  };
}

