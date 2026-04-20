import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import {
  ListDocumentLibrary,
  ListLibraryWorkspaceDir,
  GetLibraryWorkspaceRoot,
  ReadDocumentForViewer,
  RevealDocumentInExplorer,
  LibraryWorkspaceMkdir,
  LibraryWorkspaceWriteFile,
  LibraryWorkspaceDelete,
  LibraryWorkspaceRename,
  GetNovelResumeOutputDir,
  ResumeNovelLongform,
} from '../../wailsjs/go/main/App';
import '../componentcss/DocLibraryModal.css';

/** @param {string | undefined | null} modIso */
function formatModTime(modIso) {
  if (!modIso) return '';
  try {
    const d = new Date(modIso);
    return new Intl.DateTimeFormat('zh-CN', {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    }).format(d);
  } catch {
    return modIso;
  }
}

/** @param {string} p */
function isMarkdownPath(p) {
  const lower = p.toLowerCase();
  return lower.endsWith('.md') || lower.endsWith('.markdown');
}

/** @param {string | undefined | null} p */
function isPdfPath(p) {
  return String(p || '').toLowerCase().endsWith('.pdf');
}

/** @param {string} a @param {string} b */
function pathsEqualNorm(a, b) {
  return String(a || '').replace(/\\/g, '/').toLowerCase() === String(b || '').replace(/\\/g, '/').toLowerCase();
}

/** @param {string} rootAbs @param {string} relPosix */
function joinLibraryAbs(rootAbs, relPosix) {
  const root = String(rootAbs || '').replace(/[/\\]+$/, '');
  const parts = String(relPosix || '')
    .split('/')
    .filter(Boolean);
  const sep = root.includes('\\') ? '\\' : '/';
  if (parts.length === 0) return root;
  return root + sep + parts.join(sep);
}

/**
 * @param {{ open: boolean, onClose: () => void, focusPath?: string | null, activeChatId?: string }} props
 */
export default function DocLibraryModal({ open, onClose, focusPath = null, activeChatId = '' }) {
  /** @type {'workspace' | 'registry'} */
  const [tab, setTab] = useState('workspace');
  const [currentRel, setCurrentRel] = useState('');
  /** @type {{ rootAbs: string, parentRel: string, entries: Array<Record<string, any>> }} */
  const [ws, setWs] = useState({ rootAbs: '', parentRel: '', entries: [] });
  /** @type {Array<Record<string, any>>} */
  const [regItems, setRegItems] = useState([]);
  const [listErr, setListErr] = useState('');
  const [listLoading, setListLoading] = useState(false);
  /** @type {{ relPath: string, absPath: string, isDir: boolean, name: string } | null} */
  const [selectedEntry, setSelectedEntry] = useState(null);

  const [selectedPath, setSelectedPath] = useState('');
  const [viewContent, setViewContent] = useState('');
  /** 非空时表示当前预览为 PDF（data URL 用） */
  const [viewPdfBase64, setViewPdfBase64] = useState('');
  const [draftContent, setDraftContent] = useState('');
  const [editMode, setEditMode] = useState(false);
  const [viewErr, setViewErr] = useState('');
  const [viewLoading, setViewLoading] = useState(false);
  const [saveBusy, setSaveBusy] = useState(false);

  /** Wails WebView 常不显示 window.prompt / window.confirm，用应用内弹层 */
  /** @type {{ type: 'mkdir' | 'newfile' | 'rename'; draft: string; entry?: { relPath: string; absPath: string; isDir: boolean; name: string } } | null} */
  const [fsPrompt, setFsPrompt] = useState(null);
  /** @type {{ relPath: string; absPath: string; isDir: boolean; name: string } | null} */
  const [deleteTarget, setDeleteTarget] = useState(null);
  const fsPromptInputRef = useRef(null);

  /** workspace 相对路径，非空表示当前选中项落在可续写的长篇目录内 */
  const [novelResumeDir, setNovelResumeDir] = useState('');
  const [novelResumeOpen, setNovelResumeOpen] = useState(false);
  const [novelPremise, setNovelPremise] = useState('');
  const [novelChapterCount, setNovelChapterCount] = useState('6');
  const [novelAuthorNotes, setNovelAuthorNotes] = useState('');

  const loadWorkspace = useCallback(async (rel) => {
    setListErr('');
    setListLoading(true);
    try {
      const data = await ListLibraryWorkspaceDir(rel ?? '');
      const next = {
        rootAbs: String(data?.rootAbs ?? ''),
        parentRel: String(data?.parentRel ?? ''),
        entries: Array.isArray(data?.entries) ? data.entries : [],
        currentRel: String(data?.currentRel ?? ''),
      };
      setWs({
        rootAbs: next.rootAbs,
        parentRel: next.parentRel,
        entries: next.entries,
      });
      setCurrentRel(next.currentRel);
      return next;
    } catch (e) {
      setWs({ rootAbs: '', parentRel: '', entries: [] });
      setListErr(String(e?.message || e));
      return null;
    } finally {
      setListLoading(false);
    }
  }, []);

  const loadRegistry = useCallback(async () => {
    setListErr('');
    setListLoading(true);
    try {
      const list = await ListDocumentLibrary();
      setRegItems(Array.isArray(list) ? list : []);
    } catch (e) {
      setRegItems([]);
      setListErr(String(e?.message || e));
    } finally {
      setListLoading(false);
    }
  }, []);

  const loadDocument = useCallback(async (absPath, relForSave) => {
    const p = String(absPath || '').trim();
    if (!p) return;
    setViewErr('');
    setViewLoading(true);
    setEditMode(false);
    setSelectedPath(p);
    setSelectedEntry(null);
    setViewPdfBase64('');
    try {
      const res = await ReadDocumentForViewer(p);
      if (String(res?.mode ?? '') === 'pdf') {
        const b64 = String(res?.contentBase64 ?? '');
        if (!b64) setViewErr('无法读取 PDF 内容');
        setViewPdfBase64(b64);
        setViewContent('');
        setDraftContent('');
      } else {
        const text = String(res?.content ?? '');
        setViewContent(text);
        setDraftContent(text);
      }
      if (res?.path) setSelectedPath(String(res.path));
      if (relForSave) {
        setSelectedEntry({
          relPath: relForSave,
          absPath: String(res?.path || p),
          isDir: false,
          name: relForSave.split('/').pop() || relForSave,
        });
      }
    } catch (e) {
      setViewContent('');
      setDraftContent('');
      setViewPdfBase64('');
      setViewErr(String(e?.message || e));
    } finally {
      setViewLoading(false);
    }
  }, []);

  const openWorkspaceFile = useCallback(
    (entry) => {
      const abs = String(entry?.absPath ?? '');
      const rel = String(entry?.relPath ?? '');
      if (!abs || entry?.isDir) return;
      setSelectedEntry({
        relPath: rel,
        absPath: abs,
        isDir: false,
        name: String(entry?.name ?? ''),
      });
      loadDocument(abs, rel);
    },
    [loadDocument]
  );

  const enterDir = useCallback(
    (relPath) => {
      setSelectedEntry(null);
      setSelectedPath('');
      setViewContent('');
      setDraftContent('');
      setViewPdfBase64('');
      setEditMode(false);
      loadWorkspace(relPath);
    },
    [loadWorkspace]
  );

  useEffect(() => {
    if (!open) return;
    if (tab === 'workspace') void loadWorkspace(currentRel);
    else void loadRegistry();
  }, [open, tab, currentRel, loadRegistry, loadWorkspace]);

  useEffect(() => {
    if (!open) return;
    const fp = focusPath && String(focusPath).trim();
    if (!fp) return;
    (async () => {
      try {
        const root = await GetLibraryWorkspaceRoot();
        const norm = fp.replace(/\\/g, '/');
        const r = root.replace(/\\/g, '/');
        const under = norm.toLowerCase() === r.toLowerCase() || norm.toLowerCase().startsWith(r.toLowerCase() + '/');
        if (under) {
          setTab('workspace');
          let relToFile = norm.length > r.length ? norm.slice(r.length).replace(/^\//, '') : '';
          if (pathsEqualNorm(norm, r)) relToFile = '';
          const slash = relToFile.lastIndexOf('/');
          const dirRel = slash >= 0 ? relToFile.slice(0, slash) : '';
          const st = await ListLibraryWorkspaceDir(dirRel);
          setCurrentRel(String(st?.currentRel ?? ''));
          setWs({
            rootAbs: String(st?.rootAbs ?? ''),
            parentRel: String(st?.parentRel ?? ''),
            entries: Array.isArray(st?.entries) ? st.entries : [],
          });
          if (relToFile) {
            await loadDocument(fp, relToFile);
          }
          return;
        }
      } catch {
        /* fall through */
      }
      setTab('registry');
      await loadDocument(fp, '');
    })();
  }, [open, focusPath, loadDocument]);

  useEffect(() => {
    if (!fsPrompt && !deleteTarget && !novelResumeOpen) return;
    const onKey = (e) => {
      if (e.key === 'Escape') {
        setFsPrompt(null);
        setDeleteTarget(null);
        setNovelResumeOpen(false);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [fsPrompt, deleteTarget, novelResumeOpen]);

  useEffect(() => {
    if (!open || tab !== 'workspace' || !selectedEntry) {
      setNovelResumeDir('');
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        const dir = await GetNovelResumeOutputDir(selectedEntry.relPath, selectedEntry.isDir);
        if (!cancelled) setNovelResumeDir(typeof dir === 'string' ? dir : '');
      } catch {
        if (!cancelled) setNovelResumeDir('');
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [open, tab, selectedEntry]);

  useEffect(() => {
    if (!fsPrompt) return;
    const id = window.setTimeout(() => {
      const el = fsPromptInputRef.current;
      if (el) {
        el.focus();
        el.select();
      }
    }, 0);
    return () => window.clearTimeout(id);
  }, [fsPrompt]);

  const crumbParts = useMemo(() => (currentRel ? currentRel.split('/').filter(Boolean) : []), [currentRel]);

  const jumpCrumb = useCallback(
    (idx) => {
      const parts = crumbParts.slice(0, idx + 1);
      enterDir(parts.join('/'));
    },
    [crumbParts, enterDir]
  );

  const mdComponents = useMemo(
    () => ({
      /** @param {any} props */
      a: ({ href, children, ...rest }) => (
        <a href={href} {...rest} target="_blank" rel="noreferrer">
          {children}
        </a>
      ),
    }),
    []
  );

  const reveal = useCallback(async () => {
    if (!selectedPath) return;
    try {
      await RevealDocumentInExplorer(selectedPath);
    } catch (e) {
      setViewErr(String(e?.message || e));
    }
  }, [selectedPath]);

  const handleMkdir = useCallback(() => {
    setListErr('');
    setFsPrompt({ type: 'mkdir', draft: '' });
  }, []);

  const handleNewFile = useCallback(() => {
    setListErr('');
    setFsPrompt({ type: 'newfile', draft: '' });
  }, []);

  const submitFsPrompt = useCallback(async () => {
    if (!fsPrompt) return;
    const trimmed = fsPrompt.draft.trim();
    if (!trimmed) return;
    if (fsPrompt.type === 'mkdir') {
      const base = currentRel ? `${currentRel}/${trimmed}` : trimmed;
      try {
        await LibraryWorkspaceMkdir(base.replace(/\\/g, '/'));
        await loadWorkspace(currentRel);
        setFsPrompt(null);
      } catch (e) {
        setListErr(String(e?.message || e));
      }
      return;
    }
    if (fsPrompt.type === 'newfile') {
      const rel = currentRel ? `${currentRel}/${trimmed}` : trimmed;
      const relPosix = rel.replace(/\\/g, '/');
      try {
        await LibraryWorkspaceWriteFile(relPosix, '');
        const data = await loadWorkspace(currentRel);
        setFsPrompt(null);
        const rootAbs = data?.rootAbs ?? '';
        const absFixed = joinLibraryAbs(rootAbs, relPosix);
        const ent = data?.entries?.find((e) => String(e.relPath ?? '') === relPosix);
        if (ent) openWorkspaceFile(ent);
        else await loadDocument(absFixed, relPosix);
      } catch (e) {
        setListErr(String(e?.message || e));
      }
      return;
    }
    const entry = fsPrompt.entry;
    if (!entry || trimmed === entry.name) return;
    const parent = currentRel;
    const oldRel = entry.relPath;
    const newRel = parent ? `${parent}/${trimmed}` : trimmed;
    try {
      await LibraryWorkspaceRename(oldRel, newRel);
      const data = await loadWorkspace(currentRel);
      setFsPrompt(null);
      if (!entry.isDir) {
        const absNew = joinLibraryAbs(data?.rootAbs ?? '', newRel);
        await loadDocument(absNew, newRel);
        setSelectedEntry({ ...entry, relPath: newRel, name: trimmed, absPath: absNew });
      } else {
        setSelectedEntry({ ...entry, relPath: newRel, name: trimmed, absPath: joinLibraryAbs(data?.rootAbs ?? '', newRel) });
      }
    } catch (e) {
      setListErr(String(e?.message || e));
    }
  }, [fsPrompt, currentRel, loadWorkspace, openWorkspaceFile, loadDocument]);

  const handleDelete = useCallback(() => {
    if (!selectedEntry) {
      setListErr('请先在列表中点选一个文件或文件夹');
      return;
    }
    setListErr('');
    setDeleteTarget(selectedEntry);
  }, [selectedEntry]);

  const runConfirmedDelete = useCallback(
    async (recursive) => {
      if (!deleteTarget) return;
      const rel = deleteTarget.relPath;
      setDeleteTarget(null);
      try {
        await LibraryWorkspaceDelete(rel, recursive);
        setSelectedEntry(null);
        setSelectedPath('');
        setViewContent('');
        setDraftContent('');
        setViewPdfBase64('');
        await loadWorkspace(currentRel);
      } catch (e) {
        setListErr(String(e?.message || e));
      }
    },
    [deleteTarget, currentRel, loadWorkspace]
  );

  const handleRename = useCallback(() => {
    if (!selectedEntry) {
      setListErr('请先在列表中点选一个文件或文件夹');
      return;
    }
    setListErr('');
    setFsPrompt({
      type: 'rename',
      draft: selectedEntry.name,
      entry: { ...selectedEntry },
    });
  }, [selectedEntry]);

  const openNovelResumeModal = useCallback(() => {
    if (!novelResumeDir) return;
    setListErr('');
    setNovelPremise('请根据已有章节、大纲与小说圣经续写后续内容，保持人物与伏笔一致。');
    setNovelChapterCount('6');
    setNovelAuthorNotes('');
    setNovelResumeOpen(true);
  }, [novelResumeDir]);

  const submitNovelResume = useCallback(async () => {
    const cid = String(activeChatId || '').trim();
    if (!cid) {
      setListErr('请先选择或新建一个对话，续写将关联到当前会话。');
      return;
    }
    if (!novelResumeDir) return;
    const rawN = parseInt(String(novelChapterCount).trim(), 10);
    const chapters = Number.isFinite(rawN) && rawN >= 1 && rawN <= 30 ? rawN : 0;
    try {
      await ResumeNovelLongform(cid, novelResumeDir, novelPremise.trim(), novelAuthorNotes.trim(), chapters);
      setNovelResumeOpen(false);
    } catch (e) {
      setListErr(String(e?.message || e));
    }
  }, [activeChatId, novelResumeDir, novelPremise, novelAuthorNotes, novelChapterCount]);

  const handleSave = useCallback(async () => {
    if (!selectedEntry || selectedEntry.isDir || isPdfPath(selectedPath)) return;
    setSaveBusy(true);
    setViewErr('');
    try {
      await LibraryWorkspaceWriteFile(selectedEntry.relPath, draftContent);
      setViewContent(draftContent);
      setEditMode(false);
    } catch (e) {
      setViewErr(String(e?.message || e));
    } finally {
      setSaveBusy(false);
    }
  }, [selectedEntry, draftContent, selectedPath]);

  useEffect(() => {
    if (open) return;
    setTab('workspace');
    setCurrentRel('');
    setWs({ rootAbs: '', parentRel: '', entries: [] });
    setRegItems([]);
    setSelectedEntry(null);
    setSelectedPath('');
    setViewContent('');
    setDraftContent('');
    setViewPdfBase64('');
    setEditMode(false);
    setListErr('');
    setViewErr('');
    setFsPrompt(null);
    setDeleteTarget(null);
    setNovelResumeDir('');
    setNovelResumeOpen(false);
    setNovelPremise('');
    setNovelChapterCount('6');
    setNovelAuthorNotes('');
  }, [open]);

  if (!open) return null;

  return (
    <div className="doclib-overlay" role="presentation" onMouseDown={onClose}>
      <div
        className="doclib-sheet"
        role="dialog"
        aria-labelledby="doclib-title"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="doclib-sheet__header">
          <h2 id="doclib-title" className="doclib-sheet__title">
            文库
          </h2>
          <div className="doclib-sheet__actions">
            <button type="button" className="doclib-btn doclib-btn--ghost" onClick={onClose}>
              关闭
            </button>
          </div>
        </div>

        <div className="doclib-tabs" role="tablist">
          <button
            type="button"
            role="tab"
            aria-selected={tab === 'workspace'}
            className={`doclib-tab${tab === 'workspace' ? ' is-active' : ''}`}
            onClick={() => setTab('workspace')}
          >
            文件夹
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={tab === 'registry'}
            className={`doclib-tab${tab === 'registry' ? ' is-active' : ''}`}
            onClick={() => setTab('registry')}
          >
            全局登记
          </button>
        </div>

        <p className="doclib-hint" style={{ marginTop: 4, marginBottom: 8 }}>
          文件操作的默认目录，比如要输出一份旅行游记，就新建一个旅行游记文件夹，然后把游记内容输出到这个文件夹里。
          <span className="doclib-hint__sub"> 文件夹请单击选中、双击进入。</span>
        </p>

        {listErr ? <p className="doclib-sheet__error">{listErr}</p> : null}
        {viewErr ? <p className="doclib-sheet__error">{viewErr}</p> : null}

        <div className="doclib-body">
          <div className="doclib-list-wrap">
            {tab === 'workspace' ? (
              <>
                <div className="doclib-list__head doclib-list__head--toolbar">
                  <span>浏览</span>
                  <div className="doclib-mini-actions">
                    <button
                      type="button"
                      className="doclib-mini-btn"
                      disabled={listLoading || currentRel === ''}
                      title="上级目录"
                      onClick={() => enterDir(ws.parentRel)}
                    >
                      ↑
                    </button>
                    <button type="button" className="doclib-mini-btn" disabled={listLoading} onClick={() => loadWorkspace(currentRel)} title="刷新">
                      ↻
                    </button>
                  </div>
                </div>
                <nav className="doclib-breadcrumb" aria-label="路径">
                  <button type="button" className="doclib-crumb" onClick={() => enterDir('')}>
                    workspace
                  </button>
                  {crumbParts.map((part, idx) => (
                    <span key={`${part}-${idx}`} className="doclib-breadcrumb__sep">
                      /
                      <button type="button" className="doclib-crumb" onClick={() => jumpCrumb(idx)}>
                        {part}
                      </button>
                    </span>
                  ))}
                </nav>
                <div className="doclib-fs-toolbar">
                  <button type="button" className="doclib-fs-btn" disabled={listLoading} onClick={handleMkdir}>
                    新建文件夹
                  </button>
                  <button type="button" className="doclib-fs-btn" disabled={listLoading} onClick={handleNewFile}>
                    新建文件
                  </button>
                  <button type="button" className="doclib-fs-btn" disabled={listLoading || !selectedEntry} onClick={handleRename}>
                    重命名
                  </button>
                  <button type="button" className="doclib-fs-btn doclib-fs-btn--danger" disabled={listLoading || !selectedEntry} onClick={handleDelete}>
                    删除
                  </button>
                  <button
                    type="button"
                    className="doclib-fs-btn doclib-fs-btn--novel"
                    disabled={listLoading || !novelResumeDir}
                    title={novelResumeDir ? `续写目录：${novelResumeDir}` : '请选中含 chapter_*.md 的长篇文件夹或某一章文件'}
                    onClick={openNovelResumeModal}
                  >
                    续写
                  </button>
                </div>
                {listLoading ? (
                  <p className="doclib-list__empty">加载中…</p>
                ) : ws.entries.length === 0 ? (
                  <p className="doclib-list__empty">当前文件夹为空。可新建文件夹或文件，或在对话里让助手使用 library_fs。</p>
                ) : (
                  <ul className="doclib-list" aria-label="文件列表">
                    {ws.entries.map((it) => {
                      const relPath = String(it.relPath ?? '');
                      const absPath = String(it.absPath ?? '');
                      const isDir = Boolean(it.isDir);
                      const sel = selectedEntry && selectedEntry.relPath === relPath;
                      return (
                        <li key={relPath}>
                          <button
                            type="button"
                            className={`doclib-list__item${sel ? ' is-active' : ''}`}
                            onClick={() => {
                              setSelectedEntry({
                                relPath,
                                absPath,
                                isDir,
                                name: String(it.name ?? ''),
                              });
                              if (!isDir) openWorkspaceFile(it);
                            }}
                            onDoubleClick={(e) => {
                              if (isDir) {
                                e.preventDefault();
                                enterDir(relPath);
                              }
                            }}
                          >
                            <span className="doclib-list__name">
                              <span className="doclib-list__icon" aria-hidden>
                                {isDir ? '📁' : '📄'}
                              </span>
                              {String(it.name ?? '')}
                            </span>
                            <span className="doclib-list__meta">
                              {isDir ? '文件夹' : formatModTime(it.modTime)}
                            </span>
                          </button>
                        </li>
                      );
                    })}
                  </ul>
                )}
              </>
            ) : (
              <>
                <div className="doclib-list__head doclib-list__head--toolbar">
                  <span>登记文档</span>
                  <button type="button" className="doclib-mini-btn" disabled={listLoading} onClick={loadRegistry} title="刷新">
                    ↻
                  </button>
                </div>
                {listLoading ? (
                  <p className="doclib-list__empty">加载中…</p>
                ) : regItems.length === 0 ? (
                  <p className="doclib-list__empty">暂无登记条目。</p>
                ) : (
                  <ul className="doclib-list" aria-label="登记列表">
                    {regItems.map((it) => {
                      const path = String(it.path ?? '');
                      const active = path && path === selectedPath;
                      return (
                        <li key={path}>
                          <button
                            type="button"
                            className={`doclib-list__item${active ? ' is-active' : ''}`}
                            onClick={() => {
                              setSelectedEntry(null);
                              loadDocument(path, '');
                            }}
                          >
                            <span className="doclib-list__name">{String(it.name ?? path)}</span>
                            <span className="doclib-list__meta">
                              {formatModTime(it.modTime)}
                              {it.relHint ? ` · ${it.relHint}` : ''}
                            </span>
                          </button>
                        </li>
                      );
                    })}
                  </ul>
                )}
              </>
            )}
          </div>

          <div className="doclib-preview-wrap">
            <div className="doclib-preview__toolbar">
              <p className="doclib-preview__path" title={selectedPath || undefined}>
                {selectedPath || '未选择文件'}
              </p>
              <div className="doclib-preview__actions">
                {tab === 'workspace' && selectedEntry && !selectedEntry.isDir && !isPdfPath(selectedPath) ? (
                  <button
                    type="button"
                    className="doclib-btn doclib-btn--secondary"
                    onClick={() => {
                      if (editMode) {
                        setDraftContent(viewContent);
                        setEditMode(false);
                      } else {
                        setEditMode(true);
                      }
                    }}
                  >
                    {editMode ? '取消编辑' : '编辑'}
                  </button>
                ) : null}
                {tab === 'workspace' && selectedEntry && !selectedEntry.isDir && !isPdfPath(selectedPath) && editMode ? (
                  <button type="button" className="doclib-btn doclib-btn--primary" disabled={saveBusy} onClick={handleSave}>
                    {saveBusy ? '保存中…' : '保存'}
                  </button>
                ) : null}
                <button type="button" className="doclib-btn doclib-btn--secondary" disabled={!selectedPath} onClick={reveal} title="在资源管理器中显示">
                  在文件夹中显示
                </button>
              </div>
            </div>
            <div className={`doclib-preview__body${viewPdfBase64 ? ' doclib-preview__body--pdf' : ''}`}>
              {viewLoading ? (
                <p className="doclib-preview__placeholder">加载中…</p>
              ) : !selectedPath ? (
                <p className="doclib-preview__placeholder">请选择文件预览；文件夹单击选中后可重命名或删除，双击进入。</p>
              ) : editMode && tab === 'workspace' && selectedEntry && !selectedEntry.isDir && !isPdfPath(selectedPath) ? (
                <textarea className="doclib-editor" value={draftContent} onChange={(e) => setDraftContent(e.target.value)} spellCheck={false} />
              ) : viewPdfBase64 ? (
                <iframe
                  className="doclib-preview__pdf-frame"
                  title="PDF 预览"
                  src={`data:application/pdf;base64,${viewPdfBase64}`}
                />
              ) : isMarkdownPath(selectedPath) ? (
                <div className="message-markdown">
                  <ReactMarkdown components={mdComponents}>{viewContent}</ReactMarkdown>
                </div>
              ) : (
                <pre className="doclib-raw">{viewContent}</pre>
              )}
            </div>
          </div>
        </div>
      </div>

      {fsPrompt ? (
        <div
          className="doclib-modal-overlay"
          role="presentation"
          onMouseDown={(e) => {
            e.stopPropagation();
            if (e.target === e.currentTarget) setFsPrompt(null);
          }}
        >
          <div
            className="doclib-modal-dialog"
            role="dialog"
            aria-modal="true"
            aria-labelledby="doclib-fs-prompt-title"
            onMouseDown={(e) => e.stopPropagation()}
          >
            <p id="doclib-fs-prompt-title" className="doclib-modal-dialog__title">
              {fsPrompt.type === 'mkdir' ? '新建文件夹' : fsPrompt.type === 'newfile' ? '新建文件' : '重命名'}
            </p>
            <label className="doclib-modal-dialog__label" htmlFor="doclib-fs-prompt-input">
              {fsPrompt.type === 'mkdir'
                ? '文件夹名称（当前目录下）'
                : fsPrompt.type === 'newfile'
                  ? '文件名（含扩展名，如 第一回.md）'
                  : '新名称（仅名称，不含路径）'}
            </label>
            <input
              id="doclib-fs-prompt-input"
              ref={fsPromptInputRef}
              className="doclib-modal-dialog__input"
              type="text"
              value={fsPrompt.draft}
              onChange={(e) => setFsPrompt((p) => (p ? { ...p, draft: e.target.value } : p))}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  void submitFsPrompt();
                }
              }}
              autoComplete="off"
            />
            <div className="doclib-modal-dialog__actions">
              <button type="button" className="doclib-modal-dialog__btn" onClick={() => setFsPrompt(null)}>
                取消
              </button>
              <button
                type="button"
                className="doclib-modal-dialog__btn doclib-modal-dialog__btn--primary"
                disabled={!fsPrompt.draft.trim() || (fsPrompt.type === 'rename' && fsPrompt.draft.trim() === (fsPrompt.entry?.name ?? ''))}
                onClick={() => void submitFsPrompt()}
              >
                确定
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {novelResumeOpen ? (
        <div
          className="doclib-modal-overlay"
          role="presentation"
          onMouseDown={(e) => {
            e.stopPropagation();
            if (e.target === e.currentTarget) setNovelResumeOpen(false);
          }}
        >
          <div
            className="doclib-modal-dialog doclib-modal-dialog--wide"
            role="dialog"
            aria-modal="true"
            aria-labelledby="doclib-novel-resume-title"
            onMouseDown={(e) => e.stopPropagation()}
          >
            <p id="doclib-novel-resume-title" className="doclib-modal-dialog__title">
              长篇小说续写
            </p>
            <p className="doclib-modal-dialog__desc doclib-modal-dialog__desc--tight">
              将在当前对话中启动模型续写，输出目录：<code className="doclib-modal-dialog__code">{novelResumeDir}</code>
            </p>
            <label className="doclib-modal-dialog__label" htmlFor="doclib-novel-premise">
              本批创作意图（premise）
            </label>
            <textarea
              id="doclib-novel-premise"
              className="doclib-modal-dialog__textarea"
              rows={3}
              value={novelPremise}
              onChange={(e) => setNovelPremise(e.target.value)}
              spellCheck={false}
            />
            <label className="doclib-modal-dialog__label" htmlFor="doclib-novel-chapters">
              本批章节数（1–30，默认由模型工具决定）
            </label>
            <input
              id="doclib-novel-chapters"
              className="doclib-modal-dialog__input doclib-modal-dialog__input--narrow"
              type="text"
              inputMode="numeric"
              value={novelChapterCount}
              onChange={(e) => setNovelChapterCount(e.target.value)}
              autoComplete="off"
            />
            <label className="doclib-modal-dialog__label" htmlFor="doclib-novel-notes">
              作者备忘（可选，写入 author_log）
            </label>
            <textarea
              id="doclib-novel-notes"
              className="doclib-modal-dialog__textarea doclib-modal-dialog__textarea--short"
              rows={2}
              value={novelAuthorNotes}
              onChange={(e) => setNovelAuthorNotes(e.target.value)}
              spellCheck={false}
            />
            <div className="doclib-modal-dialog__actions">
              <button type="button" className="doclib-modal-dialog__btn" onClick={() => setNovelResumeOpen(false)}>
                取消
              </button>
              <button
                type="button"
                className="doclib-modal-dialog__btn doclib-modal-dialog__btn--primary"
                disabled={!novelPremise.trim()}
                onClick={() => void submitNovelResume()}
              >
                开始续写
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {deleteTarget ? (
        <div
          className="doclib-modal-overlay"
          role="presentation"
          onMouseDown={(e) => {
            e.stopPropagation();
            if (e.target === e.currentTarget) setDeleteTarget(null);
          }}
        >
          <div
            className="doclib-modal-dialog"
            role="alertdialog"
            aria-labelledby="doclib-del-title"
            aria-describedby="doclib-del-desc"
            onMouseDown={(e) => e.stopPropagation()}
          >
            <p id="doclib-del-title" className="doclib-modal-dialog__title">
              {deleteTarget.isDir ? '删除文件夹' : '删除文件'}
            </p>
            <p id="doclib-del-desc" className="doclib-modal-dialog__desc">
              {deleteTarget.isDir ? (
                <>
                  确定要删除文件夹「{deleteTarget.name}」吗？
                  <br />
                  <span className="doclib-modal-dialog__desc-note">「删除全部」会移除其中所有文件与子文件夹；「仅空文件夹」在非空时会失败。</span>
                </>
              ) : (
                <>确定要删除「{deleteTarget.name}」吗？此操作无法撤销。</>
              )}
            </p>
            <div className="doclib-modal-dialog__actions">
              <button type="button" className="doclib-modal-dialog__btn" onClick={() => setDeleteTarget(null)}>
                取消
              </button>
              {deleteTarget.isDir ? (
                <>
                  <button type="button" className="doclib-modal-dialog__btn" onClick={() => void runConfirmedDelete(false)}>
                    仅空文件夹
                  </button>
                  <button type="button" className="doclib-modal-dialog__btn doclib-modal-dialog__btn--danger" onClick={() => void runConfirmedDelete(true)}>
                    删除全部
                  </button>
                </>
              ) : (
                <button type="button" className="doclib-modal-dialog__btn doclib-modal-dialog__btn--danger" onClick={() => void runConfirmedDelete(false)}>
                  删除
                </button>
              )}
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}
