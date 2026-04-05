import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import { GetMemoContent, GetMemoFilePath, SaveMemoContent } from '../../wailsjs/go/main/App';
import '../componentcss/MemoModal.css';

/** @param {string} md */
function extractH2Sections(md) {
  const re = /^## (.+)$/gm;
  const items = [];
  let m;
  while ((m = re.exec(md)) !== null) {
    items.push({ title: m[1].trim(), charIndex: m.index });
  }
  return items;
}

/** @param {HTMLTextAreaElement | null} textarea @param {number} charIndex */
function scrollTextareaToChar(textarea, charIndex) {
  if (!textarea) return;
  const before = textarea.value.slice(0, charIndex);
  const line = before.split('\n').length;
  const lh = parseFloat(getComputedStyle(textarea).lineHeight);
  const lineHeight = Number.isFinite(lh) && lh > 0 ? lh : 22;
  textarea.scrollTop = Math.max(0, (line - 1) * lineHeight - 12);
  textarea.setSelectionRange(charIndex, charIndex);
  textarea.focus();
}

export default function MemoModal({ open, onClose, activeChatId, activeChatTitle, onMemoSaved }) {
  const [text, setText] = useState('');
  const [filePath, setFilePath] = useState('');
  const [loadErr, setLoadErr] = useState('');
  const [saveErr, setSaveErr] = useState('');
  const [saving, setSaving] = useState(false);
  /** @type {'edit' | 'preview'} */
  const [panelMode, setPanelMode] = useState('edit');
  const [activeTocIndex, setActiveTocIndex] = useState(null);
  const textareaRef = useRef(null);

  const headings = useMemo(() => extractH2Sections(text), [text]);

  const markdownComponents = useMemo(() => {
    let i = 0;
    return {
      /** @param {{ children?: import('react').ReactNode }} props */
      h2: ({ children }) => {
        const id = `memo-h2-${i++}`;
        return <h2 id={id}>{children}</h2>;
      },
    };
  }, [text]);

  const todayLabel = useMemo(() => {
    try {
      return new Intl.DateTimeFormat('zh-CN', {
        weekday: 'long',
        year: 'numeric',
        month: 'long',
        day: 'numeric',
      }).format(new Date());
    } catch {
      return '';
    }
  }, [open]);

  const load = useCallback(async () => {
    setLoadErr('');
    try {
      const [body, path] = await Promise.all([GetMemoContent(), GetMemoFilePath()]);
      setText(body ?? '');
      setFilePath(path ?? '');
    } catch (e) {
      setLoadErr(String(e?.message || e));
    }
  }, []);

  useEffect(() => {
    if (open) {
      load();
      setPanelMode('edit');
      setActiveTocIndex(null);
    }
  }, [open, load]);

  const handleSave = async () => {
    setSaveErr('');
    setSaving(true);
    try {
      await SaveMemoContent(text);
      if (typeof onMemoSaved === 'function') {
        onMemoSaved();
      }
      onClose();
    } catch (e) {
      setSaveErr(String(e?.message || e));
    } finally {
      setSaving(false);
    }
  };

  const jumpToSection = useCallback(
    (index, charIndex) => {
      setActiveTocIndex(index);
      if (panelMode === 'edit') {
        scrollTextareaToChar(textareaRef.current, charIndex);
      } else {
        const el = document.getElementById(`memo-h2-${index}`);
        el?.scrollIntoView({ behavior: 'smooth', block: 'start' });
      }
    },
    [panelMode],
  );

  if (!open) return null;

  return (
    <div className="memo-overlay" role="presentation" onMouseDown={onClose}>
      <div
        className="memo-sheet"
        role="dialog"
        aria-labelledby="memo-title"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="memo-sheet__header">
          <h2 id="memo-title" className="memo-sheet__title">
            备忘录
          </h2>
          <button type="button" className="memo-sheet__close" onClick={onClose} aria-label="关闭">
            完成
          </button>
        </div>

        <div className="memo-context">
          <div className="memo-context__block memo-context__block--chat">
            <span className="memo-context__label">当前对话</span>
            <span className="memo-context__value" title={activeChatId || undefined}>
              {activeChatTitle || '—'}
            </span>
            <span className="memo-context__meta">{activeChatId ? `#${activeChatId.slice(-8)}` : ''}</span>
          </div>
          <div className="memo-context__block memo-context__block--date">
            <span className="memo-context__label">今天</span>
            <span className="memo-context__value">{todayLabel}</span>
          </div>
        </div>

        <p className="memo-sheet__hint">
          备忘录是<strong>全局一份</strong>（任意对话里调用 <code>memo_write</code> 都会写入同一文件）。下面左侧按{' '}
          <code>## 小标题</code> 列出条目，通常对应一次追加的<strong>日期或主题</strong>；与「当前对话」无自动绑定，仅作对照参考。
        </p>

        {filePath ? (
          <details className="memo-sheet__path-details">
            <summary>存储路径</summary>
            <code className="memo-sheet__path-value">{filePath}</code>
          </details>
        ) : null}

        {loadErr ? <div className="memo-sheet__error">{loadErr}</div> : null}
        {saveErr ? <div className="memo-sheet__error">{saveErr}</div> : null}

        <div className="memo-body">
          <aside className="memo-toc" aria-label="按小标题跳转">
            <div className="memo-toc__title">条目</div>
            {headings.length === 0 ? (
              <p className="memo-toc__empty">暂无 <code>##</code> 分段。可用工具追加或手动输入标题行。</p>
            ) : (
              <ul className="memo-toc__list">
                {headings.map((h, i) => (
                  <li key={`${h.charIndex}-${i}`}>
                    <button
                      type="button"
                      className={`memo-toc__btn${activeTocIndex === i ? ' is-active' : ''}`}
                      onClick={() => jumpToSection(i, h.charIndex)}
                    >
                      <span className="memo-toc__idx">{i + 1}</span>
                      <span className="memo-toc__text">{h.title}</span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </aside>

          <div className="memo-main">
            <div className="memo-main__toolbar">
              <div className="memo-mode-tabs" role="tablist">
                <button
                  type="button"
                  role="tab"
                  aria-selected={panelMode === 'edit'}
                  className={`memo-mode-tabs__btn${panelMode === 'edit' ? ' is-active' : ''}`}
                  onClick={() => setPanelMode('edit')}
                >
                  编辑
                </button>
                <button
                  type="button"
                  role="tab"
                  aria-selected={panelMode === 'preview'}
                  className={`memo-mode-tabs__btn${panelMode === 'preview' ? ' is-active' : ''}`}
                  onClick={() => setPanelMode('preview')}
                >
                  预览
                </button>
              </div>
              <span className="memo-main__toolbar-hint">
                {headings.length > 0 ? `${headings.length} 个分段` : '未分段'}
              </span>
            </div>

            {panelMode === 'edit' ? (
              <textarea
                ref={textareaRef}
                className="memo-sheet__editor"
                value={text}
                onChange={(e) => setText(e.target.value)}
                spellCheck
                autoComplete="off"
                autoCorrect="off"
                placeholder="在这里记录想法… 使用 ## 日期或主题 作为分段标题。"
              />
            ) : (
              <div className="memo-sheet__preview">
                {text.trim() ? (
                  <ReactMarkdown components={markdownComponents}>
                    {text}
                  </ReactMarkdown>
                ) : (
                  <p className="memo-sheet__preview-empty">暂无内容</p>
                )}
              </div>
            )}
          </div>
        </div>

        <div className="memo-sheet__actions">
          <button type="button" className="memo-btn memo-btn--secondary" onClick={load} disabled={saving}>
            重新加载
          </button>
          <button type="button" className="memo-btn memo-btn--secondary" onClick={onClose}>
            取消
          </button>
          <button type="button" className="memo-btn memo-btn--primary" onClick={handleSave} disabled={saving}>
            {saving ? '保存中…' : '保存'}
          </button>
        </div>
      </div>
    </div>
  );
}
