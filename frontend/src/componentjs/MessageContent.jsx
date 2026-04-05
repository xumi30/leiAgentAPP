import { useMemo, useState, useCallback, useRef, useLayoutEffect } from 'react';
import { createPortal } from 'react-dom';
import ReactMarkdown from 'react-markdown';

function tryParseJson(s) {
  try {
    JSON.parse(s);
    return true;
  } catch {
    return false;
  }
}

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
    if (c === '"' || c === "'") {
      inStr = true;
      q = c;
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

/** @param {string} text @param {number} start */
function nextJsonSlice(text, start) {
  const obj = extractBalanced(text, start, '{', '}');
  const arr = extractBalanced(text, start, '[', ']');
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
    if (tryParseJson(n.raw) && n.raw.trim().length > 1) {
      parts.push({ type: 'json', value: n.raw });
    } else {
      parts.push({ type: 'md', value: n.raw });
    }
    pos = n.end;
  }
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
    let isJson = lang === 'json';
    if (!isJson && (trimmed.startsWith('{') || trimmed.startsWith('['))) {
      try {
        JSON.parse(trimmed);
        isJson = true;
      } catch {
        /* not json */
      }
    }
    parts.push({ kind: isJson ? 'json' : 'code', lang, raw: inner });
    last = m.index + m[0].length;
  }
  if (last < text.length) {
    parts.push({ kind: 'md', raw: text.slice(last) });
  }
  return parts.length ? parts : [{ kind: 'md', raw: text }];
}

/** @param {string} fullText */
function buildRenderableParts(fullText) {
  /** @type {{ type: 'md' | 'json' | 'code', value: string, lang?: string }[]} */
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
        out.push({ type: 'md', value: p.value });
      } else {
        out.push({ type: 'json', value: p.value });
      }
    }
  }
  return out;
}

function prettyJson(raw) {
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}

/** @param {{ raw: string, onOpen: (s: string) => void }} props */
function JsonSnippetCard({ raw, onOpen }) {
  const pretty = useMemo(() => prettyJson(raw), [raw]);
  const short = pretty.length > 400 ? `${pretty.slice(0, 400)}…` : pretty;
  return (
    <div
      className="msg-json-snippet"
      role="button"
      tabIndex={0}
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
        <span className="msg-json-snippet__action">点击查看全文 ✨</span>
      </div>
      <pre className="msg-json-snippet__pre">{short}</pre>
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

const mdComponents = {
  /** @param {any} props */
  a: ({ href, children, ...rest }) => (
    <a href={href} {...rest} target="_blank" rel="noreferrer">
      {children}
    </a>
  ),
};

/**
 * @param {{ content: string, variant?: 'user' | 'assistant', isStreaming?: boolean }} props
 */
export default function MessageContent({ content, variant = 'assistant', isStreaming = false }) {
  const [jsonModalText, setJsonModalText] = useState(null);
  const parts = useMemo(() => buildRenderableParts(content ?? ''), [content]);
  const openJson = useCallback((s) => setJsonModalText(s), []);
  const closeJson = useCallback(() => setJsonModalText(null), []);
  const bodyRef = useRef(null);

  useLayoutEffect(() => {
    if (!isStreaming || !bodyRef.current) return;
    const el = bodyRef.current;
    el.scrollTop = el.scrollHeight;
  }, [content, isStreaming]);

  return (
    <div ref={bodyRef} className={`message-body message-body--${variant}`}>
      {parts.map((p, idx) => {
        if (p.type === 'json') {
          return <JsonSnippetCard key={`j-${idx}`} raw={p.value} onOpen={openJson} />;
        }
        if (p.type === 'code') {
          return (
            <pre key={`c-${idx}`} className="msg-fence-block">
              {p.lang ? <span className="msg-fence-block__lang">{p.lang}</span> : null}
              <code>{p.value}</code>
            </pre>
          );
        }
        const md = p.value.trim() ? (
          <ReactMarkdown components={mdComponents}>{p.value}</ReactMarkdown>
        ) : null;
        return md ? (
          <div key={`m-${idx}`} className="message-markdown">
            {md}
          </div>
        ) : null;
      })}
      {jsonModalText
        ? createPortal(
            <JsonFullModal text={jsonModalText} onClose={closeJson} />,
            document.body
          )
        : null}
    </div>
  );
}
