import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import { GetMemoContent, GetMemoFilePath, SaveMemoContent } from '../../wailsjs/go/main/App';
import '../componentcss/MemoModal.css';

/** ATX headings (# … ######), line start only; strips optional closing hashes. */
/** @param {string} md @returns {{ level: number, title: string, charIndex: number }[]} */
function extractMarkdownHeadings(md) {
  const re = /^(#{1,6})\s+(.+?)\s*(?:#+\s*)?$/gm;
  const items = [];
  for (const m of md.matchAll(re)) {
    const level = m[1].length;
    const title = m[2].trim().replace(/\s+#+\s*$/, '').trim();
    items.push({ level, title, charIndex: m.index });
  }
  return items;
}

/**
 * 大纲式编号（1、1.1、2.3…）。按文中出现的最低标题层级当作「顶层」，避免全是 ## 时出现 0.1。
 * @param {{ level: number }[]} headings
 * @returns {string[]}
 */
function outlineLabelsForHeadings(headings) {
  if (headings.length === 0) return [];
  const minLevel = Math.min(...headings.map((h) => h.level));
  const count = new Array(8).fill(0);
  const out = [];
  for (const h of headings) {
    const rel = h.level - minLevel + 1;
    count[rel] += 1;
    for (let j = rel + 1; j <= 7; j++) count[j] = 0;
    out.push(count.slice(1, rel + 1).join('.'));
  }
  return out;
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

  const headings = useMemo(() => extractMarkdownHeadings(text), [text]);
  const headingOutlineLabels = useMemo(() => outlineLabelsForHeadings(headings), [headings]);

  const markdownComponents = useMemo(() => {
    let i = 0;
    /** @param {'h1'|'h2'|'h3'|'h4'|'h5'|'h6'} Tag */
    const withHeadingId = (Tag) =>
      function MemoMdHeading({ children, ...rest }) {
        const id = `memo-heading-${i++}`;
        return (
          <Tag id={id} {...rest}>
            {children}
          </Tag>
        );
      };
    return {
      h1: withHeadingId('h1'),
      h2: withHeadingId('h2'),
      h3: withHeadingId('h3'),
      h4: withHeadingId('h4'),
      h5: withHeadingId('h5'),
      h6: withHeadingId('h6'),
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
        const el = document.getElementById(`memo-heading-${index}`);
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
          备忘录是<strong>全局一份</strong>（任意对话里调用 <code>memo_write</code> 都会写入同一文件）。左侧按 Markdown 标题（<code>#</code>～<code>######</code>）列出条目并<strong>体现层级</strong>；工具追加默认一节为{' '}
          <code>##</code>，正文里可用 <code>###</code> 等细分；与「当前对话」无自动绑定，仅作对照参考。
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
          <aside className="memo-toc" aria-label="按 Markdown 标题层级跳转">
            <div className="memo-toc__title">条目</div>
            {headings.length === 0 ? (
              <p className="memo-toc__empty">
                暂无 <code>#</code>～<code>######</code> 标题行。可用工具追加或手动输入分段标题。
              </p>
            ) : (
              <ul className="memo-toc__list">
                {headings.map((h, i) => (
                  <li key={`${h.charIndex}-${i}`}>
                    <button
                      type="button"
                      className={`memo-toc__btn${activeTocIndex === i ? ' is-active' : ''}`}
                      style={{ paddingLeft: `${8 + (h.level - 1) * 12}px` }}
                      onClick={() => jumpToSection(i, h.charIndex)}
                      title={`H${h.level}`}
                    >
                      <span className="memo-toc__idx">{headingOutlineLabels[i]}</span>
                      <span className={`memo-toc__text${h.level >= 3 ? ' memo-toc__text--sub' : ''}`}>{h.title}</span>
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
                {headings.length > 0 ? `${headings.length} 个标题` : '无标题'}
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
                placeholder="在这里记录想法… 用 #～###### 写标题以在左侧目录显示层级（工具追加默认为 ##）。"
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
