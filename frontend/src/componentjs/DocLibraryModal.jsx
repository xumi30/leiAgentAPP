import { useCallback, useEffect, useMemo, useState } from 'react';
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
 * @param {{ open: boolean, onClose: () => void, focusPath?: string | null }} props
 */
export default function DocLibraryModal({ open, onClose, focusPath = null }) {
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
  const [draftContent, setDraftContent] = useState('');
  const [editMode, setEditMode] = useState(false);
  const [viewErr, setViewErr] = useState('');
  const [viewLoading, setViewLoading] = useState(false);
  const [saveBusy, setSaveBusy] = useState(false);

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
    try {
      const res = await ReadDocumentForViewer(p);
      const text = String(res?.content ?? '');
      setViewContent(text);
      setDraftContent(text);
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

  const handleMkdir = useCallback(async () => {
    const name = window.prompt('新文件夹名称（在当前目录下）', '');
    if (!name || !name.trim()) return;
    const base = currentRel ? `${currentRel}/${name.trim()}` : name.trim();
    try {
      await LibraryWorkspaceMkdir(base.replace(/\\/g, '/'));
      await loadWorkspace(currentRel);
    } catch (e) {
      setListErr(String(e?.message || e));
    }
  }, [currentRel, loadWorkspace]);

  const handleNewFile = useCallback(async () => {
    const name = window.prompt('新文件名（含扩展名，如 第一回.md）', '');
    if (!name || !name.trim()) return;
    const rel = currentRel ? `${currentRel}/${name.trim()}` : name.trim();
    const relPosix = rel.replace(/\\/g, '/');
    try {
      await LibraryWorkspaceWriteFile(relPosix, '');
      const data = await loadWorkspace(currentRel);
      const rootAbs = data?.rootAbs ?? '';
      const absFixed = joinLibraryAbs(rootAbs, relPosix);
      const ent = data?.entries?.find((e) => String(e.relPath ?? '') === relPosix);
      if (ent) openWorkspaceFile(ent);
      else await loadDocument(absFixed, relPosix);
    } catch (e) {
      setListErr(String(e?.message || e));
    }
  }, [currentRel, loadWorkspace, openWorkspaceFile, loadDocument]);

  const handleDelete = useCallback(async () => {
    if (!selectedEntry) {
      setListErr('请先在列表中点选一个文件或文件夹');
      return;
    }
    const rel = selectedEntry.relPath;
    let recursive = false;
    if (selectedEntry.isDir) {
      recursive = window.confirm(
        `删除文件夹「${selectedEntry.name}」\n\n「确定」= 删除其中所有文件与子文件夹\n「取消」= 仅删除空文件夹（非空会失败）`
      );
    } else if (!window.confirm(`删除文件「${selectedEntry.name}」？`)) return;
    try {
      await LibraryWorkspaceDelete(rel, recursive);
      setSelectedEntry(null);
      setSelectedPath('');
      setViewContent('');
      setDraftContent('');
      await loadWorkspace(currentRel);
    } catch (e) {
      setListErr(String(e?.message || e));
    }
  }, [selectedEntry, currentRel, loadWorkspace]);

  const handleRename = useCallback(async () => {
    if (!selectedEntry) {
      setListErr('请先在列表中点选一个文件或文件夹');
      return;
    }
    const next = window.prompt('新名称（仅名称，不含路径）', selectedEntry.name);
    if (!next || !next.trim() || next.trim() === selectedEntry.name) return;
    const parent = currentRel;
    const oldRel = selectedEntry.relPath;
    const newRel = parent ? `${parent}/${next.trim()}` : next.trim();
    try {
      await LibraryWorkspaceRename(oldRel, newRel);
      const data = await loadWorkspace(currentRel);
      if (!selectedEntry.isDir) {
        const absNew = joinLibraryAbs(data?.rootAbs ?? '', newRel);
        await loadDocument(absNew, newRel);
        setSelectedEntry({ ...selectedEntry, relPath: newRel, name: next.trim(), absPath: absNew });
      } else {
        setSelectedEntry(null);
      }
    } catch (e) {
      setListErr(String(e?.message || e));
    }
  }, [selectedEntry, currentRel, loadWorkspace, loadDocument]);

  const handleSave = useCallback(async () => {
    if (!selectedEntry || selectedEntry.isDir) return;
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
  }, [selectedEntry, draftContent]);

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
    setEditMode(false);
    setListErr('');
    setViewErr('');
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
          「文件夹」为应用目录下 <code>workspace/</code>，可在此整理连载与项目；工具模式下的 <code>library_fs</code> 也在此根目录操作。全局登记仍收录助手写入路径与对话中出现的本地文件。
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
                              if (isDir) enterDir(relPath);
                              else openWorkspaceFile(it);
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
                {tab === 'workspace' && selectedEntry && !selectedEntry.isDir ? (
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
                {tab === 'workspace' && selectedEntry && !selectedEntry.isDir && editMode ? (
                  <button type="button" className="doclib-btn doclib-btn--primary" disabled={saveBusy} onClick={handleSave}>
                    {saveBusy ? '保存中…' : '保存'}
                  </button>
                ) : null}
                <button type="button" className="doclib-btn doclib-btn--secondary" disabled={!selectedPath} onClick={reveal} title="在资源管理器中显示">
                  在文件夹中显示
                </button>
              </div>
            </div>
            <div className="doclib-preview__body">
              {viewLoading ? (
                <p className="doclib-preview__placeholder">加载中…</p>
              ) : !selectedPath ? (
                <p className="doclib-preview__placeholder">请选择文件预览；文件夹点选即可进入。</p>
              ) : editMode && tab === 'workspace' && selectedEntry && !selectedEntry.isDir ? (
                <textarea className="doclib-editor" value={draftContent} onChange={(e) => setDraftContent(e.target.value)} spellCheck={false} />
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
    </div>
  );
}
