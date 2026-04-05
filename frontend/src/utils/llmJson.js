/**
 * Mirrors backend utils.ExtractJSON + EscapeRawNewlinesInJSONStrings for LLM output.
 * @param {string} raw
 */
export function extractJsonObjectSlice(raw) {
  let t = String(raw).trim();
  if (t.startsWith('```json')) {
    t = t.slice(7).trimStart();
  } else if (t.startsWith('```')) {
    t = t.slice(3).trimStart();
  }
  if (t.endsWith('```')) {
    t = t.slice(0, -3).trimEnd();
  }
  const start = t.indexOf('{');
  const end = t.lastIndexOf('}');
  if (start >= 0 && end > start) {
    return t.slice(start, end + 1);
  }
  return t;
}

/** Same algorithm as utils.EscapeRawNewlinesInJSONStrings */
export function escapeRawNewlinesInJsonStrings(s) {
  let b = '';
  let inString = false;
  let escaped = false;
  for (let i = 0; i < s.length; i++) {
    const c = s[i];
    if (escaped) {
      b += c;
      escaped = false;
      continue;
    }
    if (inString && c === '\\') {
      b += c;
      escaped = true;
      continue;
    }
    if (c === '"') {
      inString = !inString;
      b += c;
      continue;
    }
    if (inString) {
      if (c === '\n') b += '\\n';
      else if (c === '\r') b += '\\r';
      else if (c === '\t') b += '\\t';
      else b += c;
      continue;
    }
    b += c;
  }
  return b;
}

export function prepareLLMJsonForParse(raw) {
  return escapeRawNewlinesInJsonStrings(extractJsonObjectSlice(raw));
}

function canParseJson(s) {
  try {
    JSON.parse(s);
    return true;
  } catch {
    return false;
  }
}

/**
 * Try strict parse, then newline repair, then extract+fence strip + repair (aligns with utils.PrepareLLMJSON).
 * @returns {{ ok: boolean, text: string }}
 */
export function tryParseJsonRepaired(s) {
  const str = String(s);
  if (canParseJson(str)) return { ok: true, text: str };
  const repaired = escapeRawNewlinesInJsonStrings(str);
  if (canParseJson(repaired)) return { ok: true, text: repaired };
  const prepared = prepareLLMJsonForParse(str);
  if (canParseJson(prepared)) return { ok: true, text: prepared };
  return { ok: false, text: str };
}

export function formatJsonPretty(raw) {
  const pr = tryParseJsonRepaired(raw);
  if (!pr.ok) return raw;
  try {
    return JSON.stringify(JSON.parse(pr.text), null, 2);
  } catch {
    return raw;
  }
}
