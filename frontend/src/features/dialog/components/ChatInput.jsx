import React, { useCallback, useEffect, useMemo, useRef } from 'react';
import assistantAvatar from '../../../assets/images/aitx.png';
import { INPUT_TEXTAREA_MAX_HEIGHT_PX, MENTION_DROPDOWN_MAX } from '../constants';
import { useMention } from '../hooks/useMention';

/**
 * ChatInput（含 Mention UI + 选中回填）
 *
 * 约束：不直接调用后端 API，只通过 props 回调输出 intent。
 *
 * @param {{
 *  value: string,
 *  onChange: (v: string) => void,
 *  onSend: () => void,
 *  mentionOptions: { agent_id: string, agent_name: string, avatar_image?: string }[],
 *  onMentionPicked?: (picked: any) => void,
 *  disabled?: boolean,
 * }} props
 */
export default function ChatInput({
  value,
  onChange,
  onSend,
  mentionOptions,
  onMentionPicked,
  disabled = false,
}) {
  const inputRef = useRef(null);
  const textareaShellRef = useRef(null);
  const imeComposingRef = useRef(false);

  const {
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
  } = useMention({ candidates: mentionOptions });

  const knownNames = useMemo(() => new Set((mentionOptions || []).map((a) => String(a?.agent_name ?? ''))), [mentionOptions]);

  useEffect(() => {
    const el = inputRef.current;
    if (!el || String(value ?? '') !== '') return;
    el.style.height = '';
    el.scrollTop = 0;
    const overlay = textareaShellRef.current?.querySelector?.('.dialog__textarea-overlay');
    if (overlay) overlay.scrollTop = 0;
  }, [value]);

  const removeMentionAt = useCallback((start, end) => {
    const s = Number(start);
    const e = Number(end);
    if (!Number.isFinite(s) || !Number.isFinite(e) || s < 0 || e <= s) return;
    let left = s;
    let right = e;
    const tail = String(value ?? '').slice(right);
    if (tail.startsWith(' ')) right += 1;
    else if (tail.startsWith(', ') || tail.startsWith('，')) right += tail.startsWith(', ') ? 2 : 1;
    const next = String(value ?? '').slice(0, left) + String(value ?? '').slice(right);
    onChange(next);
    requestAnimationFrame(() => {
      const el = inputRef.current;
      if (!el) return;
      el.focus();
      el.setSelectionRange(left, left);
      el.style.height = 'auto';
      el.style.height = `${Math.min(el.scrollHeight, INPUT_TEXTAREA_MAX_HEIGHT_PX)}px`;
    });
  }, [value, onChange]);

  const renderOverlay = useMemo(() => {
    const text = String(value ?? '');
    if (!text) return <span className="dialog__textarea-placeholder">输入消息，Enter 发送 · Shift+Enter 换行.输入@选择群成员</span>;
    const nodes = [];
    const re = /@([^\s,@，]+)/g;
    let last = 0;
    for (;;) {
      const m = re.exec(text);
      if (!m) break;
      const full = m[0];
      const name = m[1];
      const start = m.index;
      const end = start + full.length;
      if (start > last) nodes.push(<span key={`t_${last}`}>{text.slice(last, start)}</span>);
      if (knownNames.has(name)) {
        nodes.push(
          <span key={`m_${start}_${end}`} className="dialog__mention-chip" data-start={start} data-end={end}>
            <span className="dialog__mention-chip-text">{full}</span>
            <button
              type="button"
              className="dialog__mention-chip-x"
              aria-label={`移除提及 ${name}`}
              onMouseDown={(e) => e.preventDefault()}
              onClick={(e) => {
                e.preventDefault();
                e.stopPropagation();
                removeMentionAt(start, end);
              }}
            >
              ×
            </button>
          </span>
        );
      } else {
        nodes.push(<span key={`u_${start}_${end}`}>{full}</span>);
      }
      last = end;
    }
    if (last < text.length) nodes.push(<span key={`tail_${last}`}>{text.slice(last)}</span>);
    return nodes;
  }, [value, knownNames, removeMentionAt]);

  const applyMentionPick = useCallback((picked) => {
    const name = String(picked?.agent_name ?? '').trim();
    if (!name) return;
    const { start, end } = mentionAtRef.current || {};
    if (start == null || end == null || start < 0 || end < start) return;
    const insert = `@${name} `;
    const next = String(value ?? '').slice(0, start) + insert + String(value ?? '').slice(end);
    onChange(next);
    onMentionPicked?.(picked);
    closeMention();
    requestAnimationFrame(() => {
      const el = inputRef.current;
      if (!el) return;
      const pos = start + insert.length;
      el.focus();
      el.setSelectionRange(pos, pos);
      el.style.height = 'auto';
      el.style.height = `${Math.min(el.scrollHeight, INPUT_TEXTAREA_MAX_HEIGHT_PX)}px`;
      updateMentionAnchorFromInput(el, textareaShellRef.current);
    });
  }, [value, onChange, closeMention, onMentionPicked, mentionAtRef, updateMentionAnchorFromInput]);

  const onKeyDown = useCallback((e) => {
    if (mentionOpen) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setMentionActiveIndex((v) => Math.min(v + 1, Math.max(mentionCandidates.length - 1, 0)));
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        setMentionActiveIndex((v) => Math.max(v - 1, 0));
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        closeMention();
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
    if (e.keyCode === 229 || e.which === 229) return;
    e.preventDefault();
    onSend?.();
  }, [mentionOpen, mentionCandidates, mentionActiveIndex, setMentionActiveIndex, closeMention, applyMentionPick, onSend]);

  return (
    <div className="dialog__textarea-shell" ref={textareaShellRef}>
      <div
        className="dialog__textarea-overlay"
        aria-hidden="true"
        onMouseDown={() => {
          const el = inputRef.current;
          if (el) el.focus();
        }}
      >
        {renderOverlay}
      </div>

      <textarea
        ref={inputRef}
        className="dialog__textarea"
        placeholder=""
        rows={1}
        disabled={disabled}
        onCompositionStart={() => { imeComposingRef.current = true; }}
        onCompositionEnd={() => { imeComposingRef.current = false; }}
        onBlur={() => { imeComposingRef.current = false; closeMention(); }}
        onKeyDown={onKeyDown}
        value={String(value ?? '')}
        onChange={(e) => {
          const ta = e.target;
          const v = ta.value;
          onChange?.(v);
          syncMentionState(v, ta.selectionStart);
          if (mentionOpen) updateMentionAnchorFromInput(ta, textareaShellRef.current);
        }}
        onInput={(e) => {
          const ta = e.target;
          ta.style.height = 'auto';
          ta.style.height = `${Math.min(ta.scrollHeight, INPUT_TEXTAREA_MAX_HEIGHT_PX)}px`;
        }}
        onClick={(e) => {
          if (mentionOpen) updateMentionAnchorFromInput(e.target, textareaShellRef.current);
        }}
        onKeyUp={(e) => {
          if (mentionOpen) updateMentionAnchorFromInput(e.target, textareaShellRef.current);
        }}
        onScroll={(e) => {
          const ta = e.target;
          const overlay = ta?.parentElement?.querySelector?.('.dialog__textarea-overlay');
          if (overlay) overlay.scrollTop = ta.scrollTop;
          if (mentionOpen) updateMentionAnchorFromInput(ta, textareaShellRef.current);
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
            mentionCandidates.slice(0, MENTION_DROPDOWN_MAX).map((a, idx) => {
              const active = idx === mentionActiveIndex;
              return (
                <button
                  key={a.agent_id}
                  type="button"
                  className={`dialog__mention-item${active ? ' dialog__mention-item--active' : ''}`}
                  role="option"
                  aria-selected={active}
                  onMouseDown={(e) => e.preventDefault()}
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
  );
}
