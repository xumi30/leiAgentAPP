import { useMemo, useState, useCallback, useRef, useLayoutEffect } from 'react';
import { createPortal } from 'react-dom';
import ReactMarkdown from 'react-markdown';
import { tryParseJsonRepaired, formatJsonPretty } from '../utils/llmJson.js';
import { highlightKeywordChildren } from '../utils/keywordHighlight.jsx';

/** @param {string} source @param {number} fromIdx @param {string} open @param {string} close */
function extractBalanced(source, fromIdx, open, close) {
  const i = source.indexOf(open, fromIdx);
  if (i < 0) return null;
  let depth = 0;
  let inStr = false;
  let q = '';
  let esc = false;
  for (let j = i; j < source.length; j++) {
    const c = source[j];
    if (inStr) {
      if (esc) {
        esc = false;
        continue;
      }
      if (c === '\\') {
        esc = true;
        continue;
      }
      if (c === q) {
        inStr = false;
        continue;
      }
      continue;
    }
    // JSON 仅使用双引号字符串；把单引号当作定界符会破坏 "don't" 等合法内容
    if (c === '"') {
      inStr = true;
      q = '"';
      continue;
    }
    if (c === open) depth++;
    if (c === close) {
      depth--;
      if (depth === 0) {
        const raw = source.slice(i, j + 1);
        return { start: i, end: j + 1, raw };
      }
    }
  }
  return null;
}

/**
 * 从 start 起找第一个「像 JSON 数组」的 [...] 块，且排除 Markdown 链接的 [标签](url)（标签在第一个 ] 就闭合，后面紧跟 (）。
 * 否则会把 [D:\a.md](doc:...) 拆成 [D:\a.md] 与 (doc:...) 两段，链接失效。
 */
function extractFirstJsonArraySlice(text, start) {
  let search = start;
  while (search < text.length) {
    const i = text.indexOf('[', search);
    if (i < 0) return null;
    const bal = extractBalanced(text, i, '[', ']');
    if (!bal) return null;
    if (text[bal.end] === '(') {
      search = i + 1;
      continue;
    }
    return bal;
  }
  return null;
}

/** @param {string} text @param {number} start */
function nextJsonSlice(text, start) {
  const obj = extractBalanced(text, start, '{', '}');
  const arr = extractFirstJsonArraySlice(text, start);
  if (!obj && !arr) return null;
  if (!obj) return arr;
  if (!arr) return obj;
  return obj.start <= arr.start ? obj : arr;
}

/** @param {string} text */
function splitMarkdownSegmentForJson(text) {
  /** @type {{ type: 'md' | 'json', value: string }[]} */
  const parts = [];
  let pos = 0;
  while (pos < text.length) {
    const n = nextJsonSlice(text, pos);
    if (!n) {
      if (pos < text.length) parts.push({ type: 'md', value: text.slice(pos) });
      break;
    }
    if (n.start > pos) {
      parts.push({ type: 'md', value: text.slice(pos, n.start) });
    }
    const pr = tryParseJsonRepaired(n.raw);
    if (pr.ok && n.raw.trim().length > 1) {
      parts.push({ type: 'json', value: pr.text });
    } else {
      parts.push({ type: 'md', value: n.raw });
    }
    pos = n.end;
  }
  return parts;
}

function isMarkdownTableSeparatorLine(line) {
  const trimmed = String(line || '').trim();
  if (!trimmed) return false;
  const normalized = trimmed.replace(/\|/g, '').trim();
  return normalized !== '' && /^:?-{3,}:?(?:\s+:?-{3,}:?)*$/.test(normalized.replace(/\s+/g, ' '));
}

function splitPipeColumns(line) {
  const trimmed = String(line || '').trim();
  if (!trimmed.includes('|')) return [];
  const body = trimmed.replace(/^\|/, '').replace(/\|$/, '');
  return body.split('|').map((cell) => cell.trim()).filter((cell, idx, arr) => !(cell === '' && arr.length === 1));
}

function splitTabColumns(line) {
  if (!String(line || '').includes('\t')) return [];
  return String(line).split('\t').map((cell) => cell.trim());
}

function parseSimpleTableLine(line) {
  const pipeCols = splitPipeColumns(line);
  if (pipeCols.length >= 2) {
    return { kind: 'pipe', cells: pipeCols };
  }
  const tabCols = splitTabColumns(line);
  if (tabCols.length >= 2) {
    return { kind: 'tab', cells: tabCols };
  }
  return null;
}

function normalizeSimpleTableRows(rows) {
  if (!Array.isArray(rows) || rows.length < 2) return null;
  const width = rows.reduce((max, row) => Math.max(max, row.cells.length), 0);
  if (width < 2) return null;
  const normalized = rows.map((row) => ({
    ...row,
    cells: [...row.cells, ...Array(Math.max(0, width - row.cells.length)).fill('')],
  }));
  const hasSeparator = normalized.length >= 2 && isMarkdownTableSeparatorLine(normalized[1].cells.join(' | '));
  return {
    header: hasSeparator ? normalized[0].cells : null,
    rows: hasSeparator ? normalized.slice(2).map((row) => row.cells) : normalized.map((row) => row.cells),
  };
}

function splitMarkdownSegmentForSimpleTables(text) {
  const lines = String(text || '').split('\n');
  /** @type {{ type: 'md' | 'table', value?: string, table?: { header: string[] | null, rows: string[][] } }[]} */
  const parts = [];
  let mdBuffer = [];
  let tableBuffer = [];
  let tableKind = '';

  const flushMarkdown = () => {
    if (!mdBuffer.length) return;
    parts.push({ type: 'md', value: mdBuffer.join('\n') });
    mdBuffer = [];
  };

  const flushTable = () => {
    if (tableBuffer.length < 2) {
      mdBuffer.push(...tableBuffer.map((row) => row.raw));
      tableBuffer = [];
      tableKind = '';
      return;
    }
    const table = normalizeSimpleTableRows(tableBuffer);
    if (table && table.rows.length > 0) {
      parts.push({ type: 'table', table });
    } else {
      mdBuffer.push(...tableBuffer.map((row) => row.raw));
    }
    tableBuffer = [];
    tableKind = '';
  };

  for (const line of lines) {
    const parsed = parseSimpleTableLine(line);
    if (parsed) {
      if (!tableBuffer.length) {
        flushMarkdown();
        tableKind = parsed.kind;
        tableBuffer.push({ ...parsed, raw: line });
        continue;
      }
      if (parsed.kind === tableKind) {
        tableBuffer.push({ ...parsed, raw: line });
        continue;
      }
      flushTable();
      flushMarkdown();
      tableKind = parsed.kind;
      tableBuffer.push({ ...parsed, raw: line });
      continue;
    }

    if (tableBuffer.length) {
      flushTable();
    }
    mdBuffer.push(line);
  }

  if (tableBuffer.length) {
    flushTable();
  }
  flushMarkdown();
  return parts;
}

/** @param {string} text */
function splitByCodeFences(text) {
  /** @type {{ kind: 'md' | 'json' | 'code', lang?: string, raw: string }[]} */
  const parts = [];
  let last = 0;
  const re = /```([\w.-]*)?\r?\n([\s\S]*?)```/g;
  let m;
  while ((m = re.exec(text)) !== null) {
    if (m.index > last) {
      parts.push({ kind: 'md', raw: text.slice(last, m.index) });
    }
    const lang = (m[1] || '').trim().toLowerCase();
    const inner = m[2];
    const trimmed = inner.trim();
    const pr = tryParseJsonRepaired(trimmed);
    let isJson = lang === 'json' && pr.ok;
    if (!isJson && (trimmed.startsWith('{') || trimmed.startsWith('[')) && pr.ok) {
      isJson = true;
    }
    parts.push({ kind: isJson ? 'json' : 'code', lang, raw: isJson && pr.ok ? pr.text : inner });
    last = m.index + m[0].length;
  }
  if (last < text.length) {
    parts.push({ kind: 'md', raw: text.slice(last) });
  }
  return parts.length ? parts : [{ kind: 'md', raw: text }];
}

/** @param {string} fullText */
function buildRenderableParts(fullText) {
  /** @type {{ type: 'md' | 'json' | 'code' | 'table', value?: string, lang?: string, table?: { header: string[] | null, rows: string[][] } }[]} */
  const out = [];
  const chunks = splitByCodeFences(fullText);
  for (const ch of chunks) {
    if (ch.kind === 'json') {
      out.push({ type: 'json', value: ch.raw.trim() });
      continue;
    }
    if (ch.kind === 'code') {
      out.push({ type: 'code', value: ch.raw, lang: ch.lang });
      continue;
    }
    const sub = splitMarkdownSegmentForJson(ch.raw);
    for (const p of sub) {
      if (p.type === 'md') {
        const mdParts = splitMarkdownSegmentForSimpleTables(p.value);
        for (const mdPart of mdParts) {
          out.push(mdPart);
        }
      } else {
        out.push({ type: 'json', value: p.value });
      }
    }
  }
  return out;
}

/**
 * Windows 绝对路径，且必须以常见文档扩展名结尾，避免把「report.md 现已…」整句链进超链接。
 * 扩展名集合与后端 doclib 一致。
 */
const WIN_DOC_FILE_EXT =
  '\\.(?:markdown|md|txt|json|csv|html?|log|yml|yaml|xml|css|scss|less|tsx?|jsx?|ts|js|go|py|rs|sql|sh|bat|ps1|env|toml|ini|cfg)\\b';
const WIN_FILE_PATH_RE = new RegExp(
  `([A-Za-z]:[\\\\/](?:[^\\\\/:*?"<>|\\r\\n]+[\\\\/])*[^\\\\/:*?"<>|\\r\\n]+${WIN_DOC_FILE_EXT})`,
  'gi'
);

/**
 * 将消息里的绝对路径转为 Markdown 链接，点击后在文库中打开。
 * @param {string} text
 */
function linkifyLocalDocumentPaths(text) {
  if (!text) return text;
  return text.replace(WIN_FILE_PATH_RE, (full) => {
    if (full.includes('](')) return full;
    try {
      const enc = encodeURIComponent(full);
      return `[${full}](doc:${enc})`;
    } catch {
      return full;
    }
  });
}

/**
 * 从原文中抽出 `[标签](doc:目标)`，单独渲染为按钮。
 * 原因：CommonMark 会把链接文字里的 Windows 反斜杠当转义，整段无法被识别为链接，只能当纯文本显示。
 * @param {string} text
 * @returns {{ kind: 'md' | 'doc', text?: string, label?: string, target?: string }[]}
 */
function splitDocMarkdownLinks(text) {
  /** @type {{ kind: 'md' | 'doc', text?: string, label?: string, target?: string }[]} */
  const out = [];
  const re = /\[([^\]]*)]\(\s*doc:([^)]+)\)/gi;
  let last = 0;
  let m;
  while ((m = re.exec(text)) !== null) {
    if (m.index > last) {
      out.push({ kind: 'md', text: text.slice(last, m.index) });
    }
    out.push({ kind: 'doc', label: m[1], target: (m[2] || '').trim() });
    last = m.index + m[0].length;
  }
  if (last < text.length) {
    out.push({ kind: 'md', text: text.slice(last) });
  }
  return out.length ? out : [{ kind: 'md', text }];
}

/** @param {string} text */
function hasDocMarkdownLinks(text) {
  const linked = linkifyLocalDocumentPaths(text);
  return /\[[^\]]*]\(\s*doc:[^)]+\)/i.test(linked);
}

/** @param {string} raw */
function decodeDocLinkTarget(raw) {
  const s = String(raw || '').trim();
  try {
    return decodeURIComponent(s);
  } catch {
    return s;
  }
}

/** @param {{ label: string, target: string }} props */
function DocLinkChip({ label, target }) {
  const path = decodeDocLinkTarget(target);
  const show = (label && label.trim()) || path;
  return (
    <button
      type="button"
      className="msg-doc-path-link msg-doc-path-link--chip"
      title={path}
      onClick={() => {
        window.dispatchEvent(new CustomEvent('leiagent-open-document', { detail: { path } }));
      }}
    >
      {show}
    </button>
  );
}

/** @param {{ text: string }} props */
function MarkdownSlicesWithDocLinks({ text }) {
  const linked = linkifyLocalDocumentPaths(text);
  const segs = splitDocMarkdownLinks(linked);
  if (!segs.some((seg) => seg.kind === 'doc')) {
    return <ReactMarkdown components={mdComponents}>{linked}</ReactMarkdown>;
  }
  return (
    <>
      {segs.map((seg, i) => {
        if (seg.kind === 'doc') {
          return <DocLinkChip key={`doc-${i}-${seg.target}`} label={seg.label || ''} target={seg.target || ''} />;
        }
        if (!seg.text || !String(seg.text).trim()) return null;
        return (
          <span className="msg-md-slice" key={`md-${i}`}>
            <ReactMarkdown components={mdComponents}>{seg.text}</ReactMarkdown>
          </span>
        );
      })}
    </>
  );
}

/** @param {{ raw: string, onOpen: (s: string) => void }} props */
function JsonSnippetCard({ raw, onOpen }) {
  const pretty = useMemo(() => formatJsonPretty(raw), [raw]);
  return (
    <div
      className="msg-json-snippet msg-json-snippet--collapsed"
      role="button"
      tabIndex={0}
      aria-label="JSON 已折叠，点击展开"
      onClick={() => onOpen(pretty)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onOpen(pretty);
        }
      }}
    >
      <div className="msg-json-snippet__bar">
        <span className="msg-json-snippet__badge">JSON</span>
       
      </div>
    </div>
  );
}

/** @param {{ text: string, onClose: () => void }} props */
function JsonFullModal({ text, onClose }) {
  return (
    <div className="msg-json-modal-overlay" role="presentation" onMouseDown={onClose}>
      <div
        className="msg-json-modal"
        role="dialog"
        aria-label="JSON 全文"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="msg-json-modal__head">
          <span>JSON 全文</span>
          <button type="button" className="msg-json-modal__close" onClick={onClose}>
            关闭
          </button>
        </div>
        <pre className="msg-json-modal__body">{text}</pre>
      </div>
    </div>
  );
}

function SimpleTableBlock({ table }) {
  const header = Array.isArray(table?.header) ? table.header : null;
  const rows = Array.isArray(table?.rows) ? table.rows : [];
  if (!rows.length) return null;
  return (
    <div className="msg-simple-table-wrap">
      <table className="msg-simple-table">
        {header ? (
          <thead>
            <tr>
              {header.map((cell, idx) => (
                <th key={`h-${idx}`}>{highlightKeywordChildren(cell)}</th>
              ))}
            </tr>
          </thead>
        ) : null}
        <tbody>
          {rows.map((row, ridx) => (
            <tr key={`r-${ridx}`}>
              {row.map((cell, cidx) => (
                <td key={`c-${ridx}-${cidx}`}>{highlightKeywordChildren(cell)}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** @param {any} props */
function MarkdownAnchor({ href, children, node: _n, inline: _i, ...rest }) {
  const inner = highlightKeywordChildren(children);
  if (typeof href === 'string' && href.startsWith('doc:')) {
    const raw = href.slice(4);
    let path = raw;
    try {
      path = decodeURIComponent(raw);
    } catch {
      path = raw;
    }
    return (
      <button
        type="button"
        className="msg-doc-path-link"
        title="在文库中打开"
        onClick={() => {
          window.dispatchEvent(new CustomEvent('leiagent-open-document', { detail: { path } }));
        }}
      >
        {inner}
      </button>
    );
  }
  return (
    <a href={href} {...rest} target="_blank" rel="noreferrer">
      {inner}
    </a>
  );
}

/** @param {keyof JSX.IntrinsicElements} tag */
function mdBlock(tag) {
  const Tag = tag;
  return function MdBlock({
    node: _node,
    inline: _inline,
    ordered: _ordered,
    checked: _checked,
    children,
    ...rest
  }) {
    return <Tag {...rest}>{highlightKeywordChildren(children)}</Tag>;
  };
}

const mdComponents = {
  a: MarkdownAnchor,
  p: mdBlock('p'),
  li: mdBlock('li'),
  td: mdBlock('td'),
  th: mdBlock('th'),
  blockquote: mdBlock('blockquote'),
  h1: mdBlock('h1'),
  h2: mdBlock('h2'),
  h3: mdBlock('h3'),
  h4: mdBlock('h4'),
  h5: mdBlock('h5'),
  h6: mdBlock('h6'),
};

/**
 * @param {{
 *   content: string,
 *   variant?: 'user' | 'assistant',
 *   isStreaming?: boolean,
 *   bodiesExpanded?: boolean,
 *   onExpandAllBodies?: () => void,
 *   onCollapseAllBodies?: () => void,
 * }} props
 */
export default function MessageContent({
  content,
  variant = 'assistant',
  isStreaming = false,
  bodiesExpanded = false,
  onExpandAllBodies,
  onCollapseAllBodies,
}) {
  const [jsonModalText, setJsonModalText] = useState(null);
  const [hasInnerOverflow, setHasInnerOverflow] = useState(false);
  const parts = useMemo(() => buildRenderableParts(content ?? ''), [content]);
  const openJson = useCallback((s) => setJsonModalText(s), []);
  const closeJson = useCallback(() => setJsonModalText(null), []);
  const bodyRef = useRef(null);

  useLayoutEffect(() => {
    if (!isStreaming || !bodyRef.current) return;
    const el = bodyRef.current;
    el.scrollTop = el.scrollHeight;
  }, [content, isStreaming]);

  useLayoutEffect(() => {
    // 只对“可能产生内层滚轮的消息”显示角标（展开状态下也仅对这些消息显示“收起”角标）。
    if (!onExpandAllBodies && !onCollapseAllBodies) {
      setHasInnerOverflow(false);
      return;
    }
    const el = bodyRef.current;
    if (!el) return;
    const measure = () => {
      const collapsedMaxPx = Math.min(window.innerHeight * 0.52, 440);
      setHasInnerOverflow(el.scrollHeight > collapsedMaxPx + 1);
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    window.addEventListener('resize', measure);
    return () => {
      window.removeEventListener('resize', measure);
      ro.disconnect();
    };
  }, [content, onExpandAllBodies, onCollapseAllBodies, isStreaming]);

  const showExpandAllHint = Boolean(onExpandAllBodies && !bodiesExpanded && hasInnerOverflow);
  const showCollapseAllHint = Boolean(onCollapseAllBodies && bodiesExpanded && hasInnerOverflow);
  const showBodyOverflowChip = showExpandAllHint || showCollapseAllHint;

  return (
    <>
      <div className={`message-body-outer message-body-outer--${variant}`}>
        {showBodyOverflowChip ? (
          <button
            type="button"
            className="message-body__expand-all-chip"
            onClick={(e) => {
              e.preventDefault();
              e.stopPropagation();
              if (showCollapseAllHint) onCollapseAllBodies?.();
              else onExpandAllBodies?.();
            }}
            title={
              showCollapseAllHint
                ? '收起全部长消息（恢复每条消息高度限制与内层滚动）'
                : '展开全部长消息（所有气泡不再内层截断，可一次读完）'
            }
            aria-label={showCollapseAllHint ? '收起全部长消息' : '展开全部长消息'}
          >
            <svg
              className="message-body__expand-all-chip__svg"
              width="7"
              height="7"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              aria-hidden
            >
              {showCollapseAllHint ? (
                <path d="M9 3H3v6M15 21h6v-6M3 3l6.5 6.5M21 21l-6.5-6.5" />
              ) : (
                <path d="M15 3h6v6M9 21H3v-6M21 3l-6.5 6.5M3 21l6.5-6.5" />
              )}
            </svg>
          </button>
        ) : null}
        <div
          ref={bodyRef}
          className={`message-body message-body--${variant}${bodiesExpanded ? ' message-body--expanded' : ''}`}
        >
          <div className="message-body__flow">
            {parts.map((p, idx) => {
              if (p.type === 'json') {
                return <JsonSnippetCard key={`j-${idx}`} raw={p.value} onOpen={openJson} />;
              }
              if (p.type === 'table') {
                return (
                  <div key={`t-${idx}`} className="message-markdown message-markdown--in-flow">
                    <SimpleTableBlock table={p.table} />
                  </div>
                );
              }
              if (p.type === 'code') {
                return (
                  <pre key={`c-${idx}`} className="msg-fence-block">
                    {p.lang ? <span className="msg-fence-block__lang">{p.lang}</span> : null}
                    <code>{p.value}</code>
                  </pre>
                );
              }
              const hasDocChips = hasDocMarkdownLinks(p.value);
              const md = p.value.trim() ? (
                <MarkdownSlicesWithDocLinks text={p.value} />
              ) : null;
              return md ? (
                <div
                  key={`m-${idx}`}
                  className={
                    `message-markdown${hasDocChips ? ' message-markdown--with-doc-chips' : ''} message-markdown--in-flow`
                  }
                >
                  {md}
                </div>
              ) : null;
            })}
          </div>
        </div>
      </div>
      {jsonModalText
        ? createPortal(
            <JsonFullModal text={jsonModalText} onClose={closeJson} />,
            document.body
          )
        : null}
    </>
  );
}
