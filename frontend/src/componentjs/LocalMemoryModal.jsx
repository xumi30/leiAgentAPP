import { useCallback, useEffect, useMemo, useState } from 'react';
import { GetLocalMemoryMessages } from '../../wailsjs/go/main/App';
import '../componentcss/LocalMemoryModal.css';

/**
 * @param {{ open: boolean, onClose: () => void, chatId?: string }} props
 */
export default function LocalMemoryModal({ open, onClose, chatId = '' }) {
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState('');
  /** @type {[Array<Record<string, any>>, any]} */
  const [items, setItems] = useState([]);

  const cid = String(chatId || '').trim();

  const load = useCallback(async () => {
    setErr('');
    setLoading(true);
    try {
      const list = await GetLocalMemoryMessages(cid);
      setItems(Array.isArray(list) ? list : []);
    } catch (e) {
      setItems([]);
      setErr(String(e?.message || e));
    } finally {
      setLoading(false);
    }
  }, [cid]);

  useEffect(() => {
    if (!open) return;
    void load();
  }, [open, load]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e) => {
      if (e.key === 'Escape') onClose?.();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  const summary = useMemo(() => {
    const n = Array.isArray(items) ? items.length : 0;
    return `${n} 条`;
  }, [items]);

  if (!open) return null;

  return (
    <div className="lmem-overlay" role="presentation" onMouseDown={onClose}>
      <div className="lmem-sheet" role="dialog" aria-labelledby="lmem-title" onMouseDown={(e) => e.stopPropagation()}>
        <div className="lmem-sheet__header">
          <div className="lmem-sheet__title-wrap">
            <h2 id="lmem-title" className="lmem-sheet__title">
              本地记忆
            </h2>
            <p className="lmem-sheet__sub">
              chatID: <code className="lmem-code">{cid || '(未选择对话)'}</code> · {summary}
            </p>
          </div>
          <div className="lmem-sheet__actions">
            <button type="button" className="lmem-btn lmem-btn--ghost" onClick={() => void load()} disabled={loading} title="刷新">
              ↻
            </button>
            <button type="button" className="lmem-btn lmem-btn--ghost" onClick={onClose}>
              关闭
            </button>
          </div>
        </div>

        {err ? <p className="lmem-sheet__error">{err}</p> : null}

        <div className="lmem-body">
          {loading ? (
            <p className="lmem-empty">加载中…</p>
          ) : !cid ? (
            <p className="lmem-empty">请先在左侧选择一个对话。</p>
          ) : items.length === 0 ? (
            <p className="lmem-empty">暂无 localMemory 消息。</p>
          ) : (
            <ul className="lmem-list" aria-label="localMemory 消息列表">
              {items.map((it) => {
                const idx = Number(it?.idx ?? 0);
                const role = String(it?.role ?? '');
                const content = String(it?.content ?? '');
                const toolCallCnt = Number(it?.toolCallCnt ?? 0);
                const toolCallID = String(it?.toolCallID ?? '');
                const head = content.split(/\r?\n/)[0].trim();
                const preview = head ? (head.length > 80 ? `${head.slice(0, 80)}…` : head) : '(空内容)';
                return (
                  <li key={`${idx}-${role}-${toolCallID}`}>
                    <details className="lmem-item">
                      <summary className="lmem-item__summary">
                        <span className={`lmem-badge lmem-badge--${role || 'unknown'}`}>{role || 'unknown'}</span>
                        <span className="lmem-item__idx">#{idx}</span>
                        {toolCallCnt ? <span className="lmem-item__meta">toolCalls: {toolCallCnt}</span> : null}
                        {toolCallID ? <span className="lmem-item__meta">toolCallID: {toolCallID}</span> : null}
                        <span className="lmem-item__preview" title={preview}>
                          {preview}
                        </span>
                      </summary>
                      <pre className="lmem-item__content">{content}</pre>
                      {Array.isArray(it?.toolCalls) && it.toolCalls.length ? (
                        <pre className="lmem-item__toolcalls">{JSON.stringify(it.toolCalls, null, 2)}</pre>
                      ) : null}
                    </details>
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      </div>
    </div>
  );
}

