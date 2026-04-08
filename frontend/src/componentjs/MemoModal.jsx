import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
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

/** 仅单行 `# 标题` 算一条备忘录；`##`～`######` 与后续内容一律视为正文（同 Apple Notes）。 */
function extractNoteHeadings(md) {
  return extractMarkdownHeadings(md).filter((h) => h.level === 1);
}

/** @param {string} text @param {{ level: number, charIndex: number }[]} headings @param {number} index */
function getSectionBounds(text, headings, index) {
  if (index < 0 || index >= headings.length) return { start: 0, end: 0 };
  const { charIndex: start, level } = headings[index];
  let end = text.length;
  for (let j = index + 1; j < headings.length; j++) {
    if (headings[j].level <= level) {
      end = headings[j].charIndex;
      break;
    }
  }
  return { start, end };
}

/** @param {string} text @param {{ level: number, charIndex: number }[]} headings @param {number} index */
function getSectionSlice(text, headings, index) {
  const { start, end } = getSectionBounds(text, headings, index);
  return text.slice(start, end);
}

/** @param {string} text @param {{ level: number, charIndex: number }[]} headings @param {number} index @param {string} newSlice */
function replaceSectionSlice(text, headings, index, newSlice) {
  const { start, end } = getSectionBounds(text, headings, index);
  return text.slice(0, start) + newSlice + text.slice(end);
}

/** @param {string} text @param {{ level: number, charIndex: number }[]} headings @param {number} index */
function removeHeadingSection(text, headings, index) {
  if (index < 0 || index >= headings.length) return text;
  const { start, end } = getSectionBounds(text, headings, index);
  const left = text.slice(0, start);
  const right = text.slice(end);
  const lt = left.replace(/\s+$/, '');
  const rt = right.replace(/^\s+/, '');
  if (!lt) return rt;
  if (!rt) return lt;
  return `${lt}\n\n${rt}`;
}

/** @param {string} slice */
function sectionBodyAfterTitleLine(slice) {
  const nl = slice.indexOf('\n');
  if (nl === -1) return '';
  return slice.slice(nl + 1);
}

/** 去掉末尾源消息追踪注释，预览与列表摘要不展示 */
function stripLeiAgentMemoSource(md) {
  return String(md ?? '').replace(/\r?\n<!--leiAgent-memo-src:[^\n>]*-->\s*$/m, '').trimEnd();
}

/** @param {string} bodyMd @param {number} maxLen */
function extractSnippet(bodyMd, maxLen = 100) {
  const plain = bodyMd
    .replace(/^#{1,6}\s+.*/gm, '')
    .replace(/```[\s\S]*?```/g, ' ')
    .replace(/`([^`]+)`/g, '$1')
    .replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')
    .replace(/[*_#>|]/g, '')
    .split(/\s+/)
    .join(' ')
    .trim();
  if (plain.length <= maxLen) return plain || '无附加正文';
  return `${plain.slice(0, maxLen - 1)}…`;
}

/** @param {string} sectionText */
function extractDateFromSection(sectionText) {
  const m = sectionText.slice(0, 600).match(/\b(\d{4}-\d{2}-\d{2})\b/);
  return m ? m[1] : null;
}

/** @param {HTMLTextAreaElement | null} el */
function focusTextareaEnd(el) {
  if (!el) return;
  const n = el.value.length;
  el.focus();
  el.setSelectionRange(n, n);
}

export default function MemoModal({ open, onClose, onMemoSaved }) {
  const [text, setText] = useState('');
  const [filePath, setFilePath] = useState('');
  const [loadErr, setLoadErr] = useState('');
  const [saveErr, setSaveErr] = useState('');
  const [saving, setSaving] = useState(false);
  /** @type {'view' | 'edit'} */
  const [detailMode, setDetailMode] = useState('view');
  const [listQuery, setListQuery] = useState('');
  const [activeIndex, setActiveIndex] = useState(null);
  const sectionEditorRef = useRef(null);
  const memoWindowRef = useRef(null);
  const resizeDragRef = useRef(null);
  const focusNewSectionRef = useRef(false);
  /** 对话「生成备忘」写入后：打开备忘录并选中最新一条 */
  const pendingFocusLatestRef = useRef(false);
  const pendingSelectLastRef = useRef(false);
  /** @type {React.MutableRefObject<null | { type: 'empty' } | { type: 'nav'; index: number }>} */
  const postDeleteNavRef = useRef(null);
  const [maximized, setMaximized] = useState(false);
  /** @type {[{ w: number, h: number } | null, React.Dispatch<React.SetStateAction<{ w: number, h: number } | null>>]} */
  const [customSize, setCustomSize] = useState(null);

  const headings = useMemo(() => extractNoteHeadings(text), [text]);

  const listRows = useMemo(() => {
    const q = listQuery.trim().toLowerCase();
    const rows = headings.map((h, i) => {
      const slice = getSectionSlice(text, headings, i);
      const body = sectionBodyAfterTitleLine(stripLeiAgentMemoSource(slice));
      return {
        index: i,
        title: h.title,
        snippet: extractSnippet(body),
        date: extractDateFromSection(slice),
      };
    }).filter((row) => !q || row.title.toLowerCase().includes(q) || row.snippet.toLowerCase().includes(q));
    // 文档中越靠后的分节越新（新建 / memo_write 追加在末尾）；列表倒序，新的在上
    return rows.slice().reverse();
  }, [headings, text, listQuery]);

  const activeSlice = useMemo(() => {
    if (activeIndex === null || activeIndex >= headings.length) return '';
    return getSectionSlice(text, headings, activeIndex);
  }, [text, headings, activeIndex]);

  const activeBodyMd = useMemo(
    () => sectionBodyAfterTitleLine(stripLeiAgentMemoSource(activeSlice)),
    [activeSlice],
  );

  const activeTitle = activeIndex !== null && headings[activeIndex] ? headings[activeIndex].title : '';

  const activeDateLabel = useMemo(() => extractDateFromSection(activeSlice), [activeSlice]);

  const load = useCallback(async () => {
    setLoadErr('');
    try {
      const [body, path] = await Promise.all([GetMemoContent(), GetMemoFilePath()]);
      const b = body ?? '';
      setText(b);
      setFilePath(path ?? '');
      return b;
    } catch (e) {
      setLoadErr(String(e?.message || e));
      return '';
    }
  }, []);

  /** 在 load 拿到正文后：若需要则标记「选中最后一条」 */
  const applyFocusLatestAfterLoad = useCallback((body) => {
    if (!pendingFocusLatestRef.current) return;
    pendingFocusLatestRef.current = false;
    const hs = extractNoteHeadings(body);
    if (hs.length > 0) {
      pendingSelectLastRef.current = true;
    } else {
      setActiveIndex(null);
    }
  }, []);

  useEffect(() => {
    const onSavedCapture = (e) => {
      const d = /** @type {CustomEvent<{ focusLatest?: boolean }>} */ (e).detail;
      if (d && d.focusLatest) pendingFocusLatestRef.current = true;
    };
    window.addEventListener('memoSaved', onSavedCapture, true);
    return () => window.removeEventListener('memoSaved', onSavedCapture, true);
  }, []);

  useEffect(() => {
    if (!open) {
      focusNewSectionRef.current = false;
      postDeleteNavRef.current = null;
      return;
    }
    setDetailMode('view');
    setListQuery('');
    setMaximized(false);
    setCustomSize(null);
    let cancelled = false;
    (async () => {
      const body = await load();
      if (cancelled) return;
      if (pendingFocusLatestRef.current) {
        applyFocusLatestAfterLoad(body);
      } else {
        setActiveIndex(null);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [open, load, applyFocusLatestAfterLoad]);

  /** 备忘录已打开时再次写入：重新加载并选中最新条 */
  useEffect(() => {
    const onSaved = (e) => {
      const d = /** @type {CustomEvent<{ focusLatest?: boolean }>} */ (e).detail;
      if (!open || !d?.focusLatest) return;
      pendingFocusLatestRef.current = true;
      void (async () => {
        const body = await load();
        applyFocusLatestAfterLoad(body);
      })();
    };
    window.addEventListener('memoSaved', onSaved);
    return () => window.removeEventListener('memoSaved', onSaved);
  }, [open, load, applyFocusLatestAfterLoad]);

  const onResizeMouseDown = useCallback(
    (e) => {
      e.preventDefault();
      e.stopPropagation();
      if (maximized) return;
      const el = memoWindowRef.current;
      if (!el) return;
      const rect = el.getBoundingClientRect();
      resizeDragRef.current = {
        x: e.clientX,
        y: e.clientY,
        w: rect.width,
        h: rect.height,
      };
      const move = (ev) => {
        const d = resizeDragRef.current;
        if (!d) return;
        const nw = Math.max(480, d.w + (ev.clientX - d.x));
        const nh = Math.max(280, d.h + (ev.clientY - d.y));
        setCustomSize({ w: nw, h: nh });
      };
      const up = () => {
        resizeDragRef.current = null;
        window.removeEventListener('mousemove', move);
        window.removeEventListener('mouseup', up);
      };
      window.addEventListener('mousemove', move);
      window.addEventListener('mouseup', up);
    },
    [maximized],
  );

  useEffect(() => {
    if (!open) return;
    if (headings.length === 0) {
      setActiveIndex(null);
      return;
    }
    if (focusNewSectionRef.current) return;
    if (pendingSelectLastRef.current) {
      pendingSelectLastRef.current = false;
      setActiveIndex(headings.length - 1);
      return;
    }
    setActiveIndex((prev) => {
      if (prev !== null && prev < headings.length) return prev;
      return headings.length - 1;
    });
  }, [open, text, headings.length]);

  useLayoutEffect(() => {
    if (!open || !focusNewSectionRef.current) return;
    focusNewSectionRef.current = false;
    const hs = extractNoteHeadings(text);
    if (hs.length === 0) return;
    const lastIdx = hs.length - 1;
    setActiveIndex(lastIdx);
    setDetailMode('edit');
    requestAnimationFrame(() => focusTextareaEnd(sectionEditorRef.current));
  }, [open, text]);

  useLayoutEffect(() => {
    const nav = postDeleteNavRef.current;
    if (!nav) return;
    postDeleteNavRef.current = null;
    if (nav.type === 'empty') {
      setActiveIndex(null);
      setDetailMode('view');
      return;
    }
    setActiveIndex(nav.index);
    setDetailMode('view');
  }, [text]);

  useEffect(() => {
    if (!open || detailMode !== 'edit' || activeIndex === null) return;
    const id = requestAnimationFrame(() => sectionEditorRef.current?.focus());
    return () => cancelAnimationFrame(id);
  }, [open, detailMode, activeIndex]);

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

  const appendNewSection = useCallback(() => {
    setText((prev) => {
      const base = prev.replace(/\s+$/, '');
      return base ? `${base}\n\n# 新备忘录\n\n` : '# 新备忘录\n\n';
    });
    focusNewSectionRef.current = true;
  }, []);

  const deleteActiveSection = useCallback(() => {
    if (activeIndex === null || headings.length === 0) return;
    const next = removeHeadingSection(text, headings, activeIndex);
    const remaining = extractNoteHeadings(next);
    if (remaining.length === 0) {
      postDeleteNavRef.current = { type: 'empty' };
    } else {
      const ni = Math.min(activeIndex, remaining.length - 1);
      postDeleteNavRef.current = { type: 'nav', index: ni };
    }
    setText(next);
  }, [activeIndex, headings, text]);

  const onPickRow = useCallback((index) => {
    setActiveIndex(index);
    setDetailMode('view');
  }, []);

  const onSectionEditorChange = useCallback(
    (e) => {
      if (activeIndex === null || activeIndex >= headings.length) return;
      setText(replaceSectionSlice(text, headings, activeIndex, e.target.value));
    },
    [activeIndex, headings, text],
  );

  if (!open) return null;

  const windowStyle =
    maximized || !customSize
      ? undefined
      : { width: customSize.w, height: customSize.h, maxHeight: 'none' };

  return (
    <div
      className={`memo-overlay${maximized ? ' memo-overlay--maximized' : ''}`}
      role="presentation"
      onMouseDown={onClose}
    >
      <div
        ref={memoWindowRef}
        className={`memo-notes-window${maximized ? ' memo-notes-window--maximized' : ''}`}
        style={windowStyle}
        role="dialog"
        aria-labelledby="memo-notes-title"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <header className="memo-notes-titlebar">
          <div className="memo-notes-traffic" aria-hidden="true">
            <span className="memo-notes-traffic__dot memo-notes-traffic__dot--red" />
            <span className="memo-notes-traffic__dot memo-notes-traffic__dot--yellow" />
            <span className="memo-notes-traffic__dot memo-notes-traffic__dot--green" />
          </div>
          <h2 id="memo-notes-title" className="memo-notes-titlebar__title">
            备忘录
          </h2>
          <div className="memo-notes-titlebar__actions">
            <button
              type="button"
              className="memo-notes-titlebar__max"
              onClick={() => setMaximized((m) => !m)}
              title={maximized ? '还原窗口' : '最大化'}
            >
              {maximized ? '还原' : '⛶'}
            </button>
            <button type="button" className="memo-notes-titlebar__done" onClick={onClose}>
              完成
            </button>
          </div>
        </header>

        {loadErr ? <div className="memo-notes-banner memo-notes-banner--error">{loadErr}</div> : null}
        {saveErr ? <div className="memo-notes-banner memo-notes-banner--error">{saveErr}</div> : null}

        {filePath ? (
          <details className="memo-notes-path">
            <summary>存储路径</summary>
            <code>{filePath}</code>
          </details>
        ) : null}

        <div className="memo-notes-split">
          <aside className="memo-notes-sidebar" aria-label="备忘录列表">
            <div className="memo-notes-sidebar__caption">全部备忘录</div>
            <label className="memo-notes-search">
              <span className="memo-notes-search__icon" aria-hidden />
              <input
                type="search"
                className="memo-notes-search__input"
                placeholder="搜索"
                value={listQuery}
                onChange={(e) => setListQuery(e.target.value)}
                autoComplete="off"
              />
            </label>
            <button type="button" className="memo-notes-compose" onClick={appendNewSection}>
              ＋ 新建备忘录
            </button>
            <ul className="memo-notes-list">
              {headings.length === 0 ? (
                <li className="memo-notes-list__empty">
                  暂无条目。每条以 <code># 标题</code> 开头（仅一级标题算一条）；可用新建或 <code>memo_write</code> 添加。
                </li>
              ) : listRows.length === 0 ? (
                <li className="memo-notes-list__empty">没有匹配的备忘录</li>
              ) : (
                listRows.map((row) => (
                  <li key={row.index}>
                    <button
                      type="button"
                      className={`memo-notes-row${activeIndex === row.index ? ' is-selected' : ''}`}
                      onClick={() => onPickRow(row.index)}
                    >
                      <div className="memo-notes-row__top">
                        <span className="memo-notes-row__title">{row.title}</span>
                        {row.date ? <span className="memo-notes-row__date">{row.date}</span> : null}
                      </div>
                      <div className="memo-notes-row__snippet">{row.snippet}</div>
                    </button>
                  </li>
                ))
              )}
            </ul>
          </aside>

          <main className="memo-notes-detail">
            {activeIndex === null || headings.length === 0 ? (
              <div className="memo-notes-detail-placeholder">
                <p className="memo-notes-detail-placeholder__line">未选择备忘录</p>
                <p className="memo-notes-detail-placeholder__hint">在左侧列表中选择一条，或新建备忘录。</p>
              </div>
            ) : (
              <>
                <div className="memo-notes-detail-toolbar">
                  {detailMode === 'view' ? (
                    <button type="button" className="memo-notes-toolbtn" onClick={() => setDetailMode('edit')}>
                      编辑
                    </button>
                  ) : (
                    <button type="button" className="memo-notes-toolbtn memo-notes-toolbtn--strong" onClick={() => setDetailMode('view')}>
                      完成
                    </button>
                  )}
                  <button type="button" className="memo-notes-toolbtn memo-notes-toolbtn--danger" onClick={deleteActiveSection}>
                    删除
                  </button>
                </div>

                {detailMode === 'view' ? (
                  <div className="memo-notes-detail-scroll">
                    <h1 className="memo-notes-detail-h1">{activeTitle}</h1>
                    {activeDateLabel ? <div className="memo-notes-detail-meta">{activeDateLabel}</div> : null}
                    <div className="memo-notes-detail-prose">
                      {activeBodyMd.trim() ? (
                        <ReactMarkdown>{activeBodyMd}</ReactMarkdown>
                      ) : (
                        <p className="memo-notes-detail-emptybody">暂无正文</p>
                      )}
                    </div>
                  </div>
                ) : (
                  <textarea
                    ref={sectionEditorRef}
                    className="memo-notes-detail-editor"
                    value={activeSlice}
                    onChange={onSectionEditorChange}
                    spellCheck
                    autoComplete="off"
                    autoCorrect="off"
                    placeholder="此条 Markdown：首行须为 # 标题，以下为正文（可用 ##、列表等）…"
                  />
                )}
              </>
            )}
          </main>
        </div>

        <button
          type="button"
          className="memo-notes-resize"
          aria-label="拖拽调整窗口大小"
          title="拖拽调整大小"
          onMouseDown={onResizeMouseDown}
        />

        <footer className="memo-notes-footer">
          <span className="memo-notes-footer__hint">
            主存储为 SQLite（<code>data/memo_notes.db</code>）；与 <code>memo_write</code> 共用。旧版 <code>data/memo.md</code> 会在首次打开时导入。
          </span>
          <div className="memo-notes-footer__actions">
            <button type="button" className="memo-notes-footer-btn" onClick={load} disabled={saving}>
              重新加载
            </button>
            <button type="button" className="memo-notes-footer-btn" onClick={onClose}>
              取消
            </button>
            <button type="button" className="memo-notes-footer-btn memo-notes-footer-btn--primary" onClick={handleSave} disabled={saving}>
              {saving ? '保存中…' : '保存'}
            </button>
          </div>
        </footer>
      </div>
    </div>
  );
}
