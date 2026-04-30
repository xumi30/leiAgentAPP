import React, { useMemo, useRef, useState, useCallback, useEffect } from 'react';
import assistantAvatar from '../../../assets/images/aitx.png';

// interface removed

const ChatInput = ({
  value,
  onChange,
  onSend,
  mentionOptions = [],
  mcpOptions = [],
  skillOptions = [],
  onMentionPicked,
  onFocus,
  disabled = false,
  placeholder = "@提及用户, 或者直接发送消息只有默认工具人"
}) => {
  const imeComposingRef = useRef(false);
  const inputRef = useRef(null);
  const shellRef = useRef(null);

  const [mentionOpen, setMentionOpen] = useState(false);
  const [mentionQuery, setMentionQuery] = useState('');
  const [mentionActiveIndex, setMentionActiveIndex] = useState(0);
  const mentionAtRef = useRef({ start: -1, end: -1 }); // [start, end)
  const [mentionAnchor, setMentionAnchor] = useState({ left: 10, top: 0 });
  const [commandOpen, setCommandOpen] = useState(false);
  const [commandKind, setCommandKind] = useState('mcp');
  const [commandQuery, setCommandQuery] = useState('');
  const [commandActiveIndex, setCommandActiveIndex] = useState(0);
  const commandRef = useRef({ start: -1, end: -1 });
  const [commandAnchor, setCommandAnchor] = useState({ left: 10, top: 0 });

  useEffect(() => {
    const el = inputRef.current;
    if (!el || String(value ?? '') !== '') return;
    el.style.height = '';
    el.scrollTop = 0;
    const overlay = shellRef.current?.querySelector?.('.dialog__textarea-overlay');
    if (overlay) overlay.scrollTop = 0;
  }, [value]);

  const mentionCandidates = useMemo(() => {
    const list = Array.isArray(mentionOptions) ? mentionOptions : [];
    const q = mentionQuery.trim().toLowerCase();
    if (!mentionOpen) return [];
    if (!q) return list;
    return list.filter((a) => String(a?.agent_name ?? '').toLowerCase().includes(q));
  }, [mentionOpen, mentionQuery, mentionOptions]);

  const getCommandLabel = useCallback((item, kind = commandKind) => {
    if (kind === 'skill') return String(item?.name ?? item?.skillName ?? '').trim();
    return String(item?.label ?? item?.name ?? '').trim();
  }, [commandKind]);

  const getCommandMeta = useCallback((item, kind = commandKind) => {
    if (kind === 'skill') {
      const detail = String(item?.description ?? item?.statusDetail ?? '').trim();
      if (detail) return detail;
      if (item?.ready === false) return '未就绪';
      return 'Skill';
    }
    const tools = Array.isArray(item?.cachedTools) ? item.cachedTools.length : 0;
    const state = String(item?.lastCheckState ?? '').trim();
    if (tools > 0) return `${tools} 个工具`;
    if (state) return state === 'ok' ? '已连接' : state;
    return 'MCP';
  }, [commandKind]);

  const commandCandidates = useMemo(() => {
    if (!commandOpen) return [];
    const source = commandKind === 'skill' ? skillOptions : mcpOptions;
    const list = Array.isArray(source) ? source : [];
    const q = commandQuery.trim().toLowerCase();
    const normalized = list
      .map((item) => ({
        item,
        label: getCommandLabel(item, commandKind),
        meta: getCommandMeta(item, commandKind),
      }))
      .filter((entry) => entry.label);
    if (!q) return normalized;
    return normalized.filter((entry) => {
      const haystack = `${entry.label} ${entry.meta}`.toLowerCase();
      return haystack.includes(q);
    });
  }, [commandKind, commandOpen, commandQuery, getCommandLabel, getCommandMeta, mcpOptions, skillOptions]);

  const caretXYInTextarea = useCallback((textarea, position) => {
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
    div.textContent = textarea.value.substring(0, position);
    const span = document.createElement('span');
    span.textContent = textarea.value.substring(position) || '.';
    div.appendChild(span);
    document.body.appendChild(div);
    const left = span.offsetLeft - textarea.scrollLeft;
    const top = span.offsetTop - textarea.scrollTop;
    document.body.removeChild(div);
    const lh = parseFloat(style.lineHeight) || 20;
    return { left, top, lineHeight: lh };
  }, []);

  const updateMentionAnchor = useCallback((textarea) => {
    const ta = textarea || inputRef.current;
    const shell = shellRef.current;
    if (!ta || !shell) return;
    const shellRect = shell.getBoundingClientRect();
    const taRect = ta.getBoundingClientRect();
    const pos = typeof ta.selectionStart === 'number' ? ta.selectionStart : (ta.value || '').length;
    const caret = caretXYInTextarea(ta, pos);
    const rawLeft = (taRect.left - shellRect.left) + caret.left;
    const rawTop = (taRect.top - shellRect.top) + caret.top;
    const shellW = shellRect.width || 0;
    const dropdownW = 260;
    const left = Math.max(10, Math.min(rawLeft, Math.max(10, shellW - dropdownW - 10)));
    // Place dropdown ABOVE the caret, so it won't cover the input area.
    // We'll anchor at caret Y, then translate the dropdown upwards by its own height.
    const top = Math.max(8, rawTop);
    setMentionAnchor({ left, top });
  }, [caretXYInTextarea]);

  const updateCommandAnchor = useCallback((textarea) => {
    const ta = textarea || inputRef.current;
    const shell = shellRef.current;
    if (!ta || !shell) return;
    const shellRect = shell.getBoundingClientRect();
    const taRect = ta.getBoundingClientRect();
    const pos = typeof ta.selectionStart === 'number' ? ta.selectionStart : (ta.value || '').length;
    const caret = caretXYInTextarea(ta, pos);
    const rawLeft = (taRect.left - shellRect.left) + caret.left;
    const rawTop = (taRect.top - shellRect.top) + caret.top;
    const shellW = shellRect.width || 0;
    const dropdownW = 280;
    const left = Math.max(10, Math.min(rawLeft, Math.max(10, shellW - dropdownW - 10)));
    const top = Math.max(8, rawTop);
    setCommandAnchor({ left, top });
  }, [caretXYInTextarea]);

  const resetCommandState = useCallback(() => {
    setCommandOpen(false);
    setCommandQuery('');
    commandRef.current = { start: -1, end: -1 };
  }, []);

  const syncMentionState = useCallback((nextValue, cursorPos) => {
    const text = String(nextValue ?? '');
    const cursor = typeof cursorPos === 'number' ? cursorPos : text.length;
    const upto = text.slice(0, cursor);
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

  const syncCommandState = useCallback((nextValue, cursorPos) => {
    const text = String(nextValue ?? '');
    const cursor = typeof cursorPos === 'number' ? cursorPos : text.length;
    const upto = text.slice(0, cursor);
    const slash = upto.lastIndexOf('/');
    const angle = upto.lastIndexOf('>');
    const triggerAt = Math.max(slash, angle);
    if (triggerAt < 0) {
      resetCommandState();
      return;
    }

    const before = triggerAt === 0 ? '' : upto[triggerAt - 1];
    const okBoundary = triggerAt === 0 || before === ' ' || before === '\n' || before === '\t' || before === ',' || before === '，';
    const frag = upto.slice(triggerAt + 1);
    if (!okBoundary || /[\s,，]/.test(frag)) {
      resetCommandState();
      return;
    }

    const nextKind = upto[triggerAt] === '>' ? 'skill' : 'mcp';
    setCommandKind(nextKind);
    setCommandOpen(true);
    setCommandQuery(frag);
    setCommandActiveIndex(0);
    commandRef.current = { start: triggerAt, end: cursor };
    setMentionOpen(false);
    setMentionQuery('');
    mentionAtRef.current = { start: -1, end: -1 };
  }, [resetCommandState]);

  const applyMentionPick = useCallback((picked) => {
    const name = String(picked?.agent_name ?? '').trim();
    if (!name) return;
    const pickedID = String(picked?.agent_id ?? '').trim();
    const { start, end } = mentionAtRef.current || {};
    if (start == null || end == null || start < 0 || end < start) return;

    const insert = `@${name} `;
    const next = String(value ?? '').slice(0, start) + insert + String(value ?? '').slice(end);
    onChange(next);
    if (pickedID && typeof onMentionPicked === 'function') onMentionPicked({ agent_id: pickedID, agent_name: name });

    setMentionOpen(false);
    setMentionQuery('');
    mentionAtRef.current = { start: -1, end: -1 };

    requestAnimationFrame(() => {
      const el = inputRef.current;
      if (!el) return;
      const pos = start + insert.length;
      el.focus();
      el.setSelectionRange(pos, pos);
      el.style.height = 'auto';
      el.style.height = `${Math.min(el.scrollHeight, 200)}px`;
      updateMentionAnchor(el);
    });
  }, [value, onChange, onMentionPicked, updateMentionAnchor]);

  const applyCommandPick = useCallback((entry) => {
    const picked = entry?.item ?? entry;
    const label = getCommandLabel(picked, commandKind);
    if (!label) return;
    const { start, end } = commandRef.current || {};
    if (start == null || end == null || start < 0 || end < start) return;

    const insert = commandKind === 'skill' ? `使用 ${label} skill ` : `使用 ${label} MCP `;
    const next = String(value ?? '').slice(0, start) + insert + String(value ?? '').slice(end);
    onChange(next);
    resetCommandState();

    requestAnimationFrame(() => {
      const el = inputRef.current;
      if (!el) return;
      const pos = start + insert.length;
      el.focus();
      el.setSelectionRange(pos, pos);
      el.style.height = 'auto';
      el.style.height = `${Math.min(el.scrollHeight, 200)}px`;
      updateCommandAnchor(el);
    });
  }, [commandKind, getCommandLabel, onChange, resetCommandState, updateCommandAnchor, value]);

  const handleKeyDown = (e) => {
    if (commandOpen) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setCommandActiveIndex((v) => {
          const max = Math.max(Math.min(commandCandidates.length, 12) - 1, 0);
          return Math.min(v + 1, max);
        });
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        setCommandActiveIndex((v) => Math.max(v - 1, 0));
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        resetCommandState();
        return;
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        if (imeComposingRef.current || e.nativeEvent?.isComposing) return;
        if (e.keyCode === 229 || e.which === 229) return;
        e.preventDefault();
        if (commandCandidates.length > 0) {
          applyCommandPick(commandCandidates[commandActiveIndex] ?? commandCandidates[0]);
          return;
        }
        return;
      }
    }

    if (mentionOpen) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setMentionActiveIndex((v) => {
          const max = Math.max(mentionCandidates.length - 1, 0);
          return Math.min(v + 1, max);
        });
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        setMentionActiveIndex((v) => Math.max(v - 1, 0));
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        setMentionOpen(false);
        setMentionQuery('');
        mentionAtRef.current = { start: -1, end: -1 };
        return;
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        if (imeComposingRef.current || e.nativeEvent?.isComposing) return;
        if (e.keyCode === 229 || e.which === 229) return;
        if (mentionCandidates.length > 0) {
          e.preventDefault();
          applyMentionPick(mentionCandidates[mentionActiveIndex] ?? mentionCandidates[0]);
          return;
        }
      }
    }
    if (e.key !== 'Enter' || e.shiftKey) return;
    if (imeComposingRef.current || e.nativeEvent?.isComposing) return;
    if (e.keyCode === 229 || e.which === 229) return;
    e.preventDefault();
    handleSend();
  };

  const handleSend = () => {
    if (value.trim() && !disabled) {
      onSend(value.trim());
      onChange('');
    }
  };

  return (
    <>
      <div className="dialog__textarea-shell" ref={shellRef}>
        <div className="dialog__textarea-overlay" aria-hidden="true">
          {value ? (
            <span style={{ whiteSpace: 'pre-wrap' }}>{value}</span>
          ) : (
            <span className="dialog__textarea-placeholder">{placeholder}</span>
          )}
        </div>
        <textarea
          ref={inputRef}
          value={value}
          onChange={(e) => {
            const ta = e.target;
            const v = ta.value;
            onChange(v);
            syncMentionState(v, ta.selectionStart);
            syncCommandState(v, ta.selectionStart);
            if (mentionOpen) updateMentionAnchor(ta);
            updateCommandAnchor(ta);
          }}
          onKeyDown={handleKeyDown}
          onCompositionStart={() => {
            imeComposingRef.current = true;
          }}
          onCompositionEnd={() => {
            imeComposingRef.current = false;
          }}
          placeholder=""
          disabled={disabled}
          className="dialog__textarea"
          rows={1}
          onInput={(e) => {
            const target = e.target;
            target.style.height = 'auto';
            target.style.height = `${Math.min(target.scrollHeight, 200)}px`;
          }}
          onClick={(e) => {
            if (mentionOpen) updateMentionAnchor(e.target);
            if (commandOpen) updateCommandAnchor(e.target);
          }}
          onFocus={onFocus}
          onKeyUp={(e) => {
            if (mentionOpen) updateMentionAnchor(e.target);
            if (commandOpen) updateCommandAnchor(e.target);
          }}
          onBlur={() => {
            imeComposingRef.current = false;
            setMentionOpen(false);
            setMentionQuery('');
            mentionAtRef.current = { start: -1, end: -1 };
            resetCommandState();
          }}
        />

        {mentionOpen ? (
          <div
            className="dialog__mention-dropdown"
            role="listbox"
            aria-label="选择群成员"
            style={{
              left: `${mentionAnchor.left}px`,
              top: `${mentionAnchor.top}px`,
              transform: 'translateY(calc(-100% - 8px))',
            }}
          >
            {mentionCandidates.length > 0 ? (
              mentionCandidates.slice(0, 12).map((a, idx) => {
                const active = idx === mentionActiveIndex;
                return (
                  <button
                    key={String(a?.agent_id ?? idx)}
                    type="button"
                    className={`dialog__mention-item${active ? ' dialog__mention-item--active' : ''}`}
                    role="option"
                    aria-selected={active}
                    onMouseDown={(ev) => ev.preventDefault()}
                    onClick={() => applyMentionPick(a)}
                  >
                    <img
                      className="dialog__mention-item-avatar"
                      src={String(a?.avatar_image ?? '') || assistantAvatar}
                      onError={(e) => {
                        if (e.currentTarget?.src !== assistantAvatar) e.currentTarget.src = assistantAvatar;
                      }}
                      alt=""
                      aria-hidden="true"
                    />
                    <span className="dialog__mention-item-name">{String(a?.agent_name ?? '')}</span>
                  </button>
                );
              })
            ) : (
              <div className="dialog__mention-empty">没有匹配的成员</div>
            )}
          </div>
        ) : null}

        {commandOpen ? (
          <div
            className="dialog__mention-dropdown dialog__command-dropdown"
            role="listbox"
            aria-label={commandKind === 'skill' ? '选择 skill' : '选择 MCP'}
            style={{
              left: `${commandAnchor.left}px`,
              top: `${commandAnchor.top}px`,
              transform: 'translateY(calc(-100% - 8px))',
            }}
          >
            {commandCandidates.length > 0 ? (
              commandCandidates.slice(0, 12).map((entry, idx) => {
                const active = idx === commandActiveIndex;
                return (
                  <button
                    key={`${commandKind}_${entry.label}_${idx}`}
                    type="button"
                    className={`dialog__mention-item${active ? ' dialog__mention-item--active' : ''}`}
                    role="option"
                    aria-selected={active}
                    onMouseDown={(ev) => ev.preventDefault()}
                    onClick={() => applyCommandPick(entry)}
                  >
                    <span className="dialog__command-item-trigger" aria-hidden>
                      {commandKind === 'skill' ? '>' : '/'}
                    </span>
                    <span className="dialog__command-item-text">
                      <span className="dialog__mention-item-name">{entry.label}</span>
                      <span className="dialog__command-item-meta">{entry.meta}</span>
                    </span>
                  </button>
                );
              })
            ) : (
              <div className="dialog__mention-empty">
                {commandKind === 'skill' ? '没有匹配的 skill' : '没有匹配的 MCP'}
              </div>
            )}
          </div>
        ) : null}
      </div>

      <button
        type="button"
        onClick={handleSend}
        disabled={disabled || !value.trim()}
        className="dialog__btn-send"
        aria-label="发送"
        title="发送"
      >
        <span className="dialog__btn-send-icon" aria-hidden>
          {disabled ? '…' : '🚀'}
        </span>
      </button>
    </>
  );
};

export default ChatInput;
