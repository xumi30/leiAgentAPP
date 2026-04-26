import React, { useCallback, useMemo, useRef, useState } from 'react';
import assistantAvatar from '../../../assets/images/aitx.png';

export interface MentionOption {
  agent_id: string;
  agent_name: string;
  avatar_image?: string;
}

export interface ChatInputProps {
  value: string;
  onChange: (next: string) => void;
  onSend: (trimmed: string) => void;
  disabled?: boolean;
  placeholder?: string;

  mentionOptions?: MentionOption[];
  onMentionPicked?: (picked: { agent_id: string; agent_name: string }) => void;
}

function caretXYInTextarea(textarea: HTMLTextAreaElement, position: number) {
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

  const lineHeight = Number.parseFloat(style.lineHeight) || 20;
  return { left, top, lineHeight };
}

function isMentionBoundaryChar(ch: string) {
  return ch === '' || ch === ' ' || ch === '\n' || ch === '\t' || ch === ',' || ch === '，';
}

export default function ChatInput(props: ChatInputProps) {
  const {
    value,
    onChange,
    onSend,
    disabled = false,
    placeholder = '输入消息，Enter 发送 · Shift+Enter 换行。输入 @ 选择群成员',
    mentionOptions = [],
    onMentionPicked,
  } = props;

  const inputRef = useRef<HTMLTextAreaElement | null>(null);
  const shellRef = useRef<HTMLDivElement | null>(null);
  const imeComposingRef = useRef(false);

  const [mentionOpen, setMentionOpen] = useState(false);
  const [mentionQuery, setMentionQuery] = useState('');
  const [mentionActiveIndex, setMentionActiveIndex] = useState(0);
  const mentionAtRef = useRef<{ start: number; end: number }>({ start: -1, end: -1 });
  const [mentionAnchor, setMentionAnchor] = useState({ left: 10, top: 0 });

  const mentionCandidates = useMemo(() => {
    if (!mentionOpen) return [];
    const q = mentionQuery.trim().toLowerCase();
    if (!q) return mentionOptions;
    return mentionOptions.filter((a) => a.agent_name.toLowerCase().includes(q));
  }, [mentionOpen, mentionQuery, mentionOptions]);

  const updateMentionAnchorFromInput = useCallback((textarea?: HTMLTextAreaElement | null) => {
    const ta = textarea ?? inputRef.current;
    const shell = shellRef.current;
    if (!ta || !shell) return;

    const shellRect = shell.getBoundingClientRect();
    const taRect = ta.getBoundingClientRect();
    const pos = typeof ta.selectionStart === 'number' ? ta.selectionStart : ta.value.length;
    const caret = caretXYInTextarea(ta, pos);

    const rawLeft = taRect.left - shellRect.left + caret.left;
    const rawTop = taRect.top - shellRect.top + caret.top;
    const shellW = shellRect.width || 0;
    const dropdownW = 260;

    const left = Math.max(10, Math.min(rawLeft, Math.max(10, shellW - dropdownW - 10)));
    const top = Math.max(0, rawTop - 8);
    setMentionAnchor({ left, top });
  }, []);

  const syncMentionState = useCallback(
    (nextValue: string, cursorPos?: number | null) => {
      const cursor = typeof cursorPos === 'number' ? cursorPos : nextValue.length;
      const upto = nextValue.slice(0, cursor);
      const at = upto.lastIndexOf('@');
      if (at < 0) {
        setMentionOpen(false);
        setMentionQuery('');
        mentionAtRef.current = { start: -1, end: -1 };
        return;
      }

      const before = at === 0 ? '' : upto[at - 1];
      if (!isMentionBoundaryChar(before)) {
        setMentionOpen(false);
        setMentionQuery('');
        mentionAtRef.current = { start: -1, end: -1 };
        return;
      }

      const frag = upto.slice(at + 1);
      if ([...frag].some((c) => isMentionBoundaryChar(c))) {
        setMentionOpen(false);
        setMentionQuery('');
        mentionAtRef.current = { start: -1, end: -1 };
        return;
      }

      setMentionOpen(true);
      setMentionQuery(frag);
      setMentionActiveIndex(0);
      mentionAtRef.current = { start: at, end: cursor };
    },
    [],
  );

  const applyMentionPick = useCallback(
    (picked: MentionOption) => {
      const name = String(picked?.agent_name ?? '').trim();
      if (!name) return;

      const pickedID = String(picked?.agent_id ?? '').trim();
      const { start, end } = mentionAtRef.current;
      if (start < 0 || end < start) return;

      const insert = `@${name} `;
      const next = value.slice(0, start) + insert + value.slice(end);
      onChange(next);
      if (pickedID) onMentionPicked?.({ agent_id: pickedID, agent_name: name });

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
        updateMentionAnchorFromInput(el);
      });
    },
    [value, onChange, onMentionPicked, updateMentionAnchorFromInput],
  );

  const runSend = useCallback(() => {
    const trimmed = String(value ?? '').trim();
    if (!trimmed || disabled) return;
    onSend(trimmed);
  }, [value, disabled, onSend]);

  const onKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
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
          if (mentionCandidates.length > 0) {
            e.preventDefault();
            applyMentionPick(mentionCandidates[mentionActiveIndex] ?? mentionCandidates[0]);
            return;
          }
        }
      }

      if (e.key !== 'Enter' || e.shiftKey) return;
      if (imeComposingRef.current) return;
      // 229: IME 正在处理该键（拼音阶段）
      if ((e as any).keyCode === 229 || (e as any).which === 229) return;

      e.preventDefault();
      runSend();
    },
    [mentionOpen, mentionCandidates, mentionActiveIndex, applyMentionPick, runSend],
  );

  return (
    <div className="dialog__input-row">
      <div className="dialog__textarea-shell" ref={shellRef}>
        <div
          className="dialog__textarea-overlay"
          aria-hidden="true"
          onMouseDown={() => {
            inputRef.current?.focus();
          }}
        >
          {value ? (
            <span style={{ whiteSpace: 'pre-wrap' }}>{value}</span>
          ) : (
            <span className="dialog__textarea-placeholder">{placeholder}</span>
          )}
        </div>

        <textarea
          ref={inputRef}
          className="dialog__textarea"
          placeholder=""
          rows={1}
          value={value}
          disabled={disabled}
          onCompositionStart={() => {
            imeComposingRef.current = true;
          }}
          onCompositionEnd={() => {
            imeComposingRef.current = false;
          }}
          onBlur={() => {
            imeComposingRef.current = false;
            setMentionOpen(false);
            setMentionQuery('');
            mentionAtRef.current = { start: -1, end: -1 };
          }}
          onKeyDown={onKeyDown}
          onChange={(e) => {
            const ta = e.target;
            const v = ta.value;
            onChange(v);
            syncMentionState(v, ta.selectionStart);
            if (mentionOpen) updateMentionAnchorFromInput(ta);
          }}
          onInput={(e) => {
            const ta = e.currentTarget;
            ta.style.height = 'auto';
            ta.style.height = `${Math.min(ta.scrollHeight, 200)}px`;
          }}
          onClick={(e) => {
            if (mentionOpen) updateMentionAnchorFromInput(e.currentTarget);
          }}
          onKeyUp={(e) => {
            if (mentionOpen) updateMentionAnchorFromInput(e.currentTarget);
          }}
          onScroll={(e) => {
            const ta = e.currentTarget;
            const overlay = ta.parentElement?.querySelector?.('.dialog__textarea-overlay') as
              | HTMLDivElement
              | undefined;
            if (overlay) overlay.scrollTop = ta.scrollTop;
            if (mentionOpen) updateMentionAnchorFromInput(ta);
          }}
        />

        {mentionOpen ? (
          <div
            className="dialog__mention-dropdown"
            role="listbox"
            aria-label="选择群成员"
            style={{ left: `${mentionAnchor.left}px`, top: `${mentionAnchor.top}px` }}
          >
            {mentionCandidates.length > 0 ? (
              mentionCandidates.slice(0, 12).map((a, idx) => {
                const active = idx === mentionActiveIndex;
                return (
                  <button
                    key={a.agent_id}
                    type="button"
                    className={`dialog__mention-item${active ? ' dialog__mention-item--active' : ''}`}
                    role="option"
                    aria-selected={active}
                    onMouseDown={(ev) => ev.preventDefault()}
                    onClick={() => applyMentionPick(a)}
                  >
                    <img
                      className="dialog__mention-item-avatar"
                      src={a.avatar_image || assistantAvatar}
                      onError={(e) => {
                        if (e.currentTarget?.src !== assistantAvatar) e.currentTarget.src = assistantAvatar;
                      }}
                      alt=""
                      aria-hidden="true"
                    />
                    <span className="dialog__mention-item-name">{a.agent_name}</span>
                  </button>
                );
              })
            ) : (
              <div className="dialog__mention-empty">没有匹配的成员</div>
            )}
          </div>
        ) : null}
      </div>

      <button
        type="button"
        className="dialog__btn-send"
        onClick={runSend}
        aria-label="发送"
        title="发送"
        disabled={disabled || !String(value ?? '').trim()}
      >
        <span className="dialog__btn-send-icon" aria-hidden>
          🚀
        </span>
      </button>
    </div>
  );
}

