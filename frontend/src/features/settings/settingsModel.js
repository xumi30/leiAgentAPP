// Pure transformations shared by the settings UI.
// Keep Wails calls and React state in SettingsModal; this module stays easy to test.

export function shellSeverityLevel(s) {
  const x = String(s ?? '').trim().toLowerCase();
  if (x === 'high' || x === '高' || x === 'h') return 'high';
  if (x === 'low' || x === '低' || x === 'l') return 'low';
  if (x === 'pending' || x === '待定' || x === 'tbd') return 'pending';
  return 'medium';
}

export function shellSeverityHintLabel(s) {
  const x = String(s ?? '').trim().toLowerCase();
  if (x === 'high' || x === '高' || x === 'h') return '高';
  if (x === 'low' || x === '低' || x === 'l') return '低';
  if (x === 'pending' || x === '待定' || x === 'tbd') return '待定';
  return '中';
}

export const FALLBACK_HUB_CATEGORIES = [
  'business',
  'cloud',
  'communication',
  'developer',
  'education',
  'education-science',
  'finance',
  'gaming-entertainment',
  'government',
  'hardware',
  'health-wellness',
  'home-assistant',
  'home-automation',
  'lifestyle',
  'media-generate',
  'news',
  'productivity',
  'science-education',
  'security',
  'services',
  'shopping',
  'social',
  'sports',
  'tools',
  'travel-transport',
  'utility',
  'weather',
  'web-search',
];

export function emptyLLMConfig() {
  return {
    apiKey: '',
    baseUrl: '',
    model: '',
    maxOutputTokens: 0,
  };
}

export function emptyMcpRow() {
  return {
    label: '',
    transportType: 'stdio',
    url: '',
    command: '',
    argsText: '',
    allowedTools: '',
    headersText: '',
    envText: '',
    cachedTools: [],
    cachedToolDetails: [],
    lastCheckState: '',
    lastCheckMessage: '',
    lastCheckedAt: '',
  };
}

export function emptyMcpStatus() {
  return {
    state: 'idle',
    message: '',
    tools: [],
    toolCount: 0,
    checkedAt: '',
  };
}

export function emptyHubStatus() {
  return {
    registered: false,
    credentialsPath: '',
    message: '',
  };
}

export function parseMcpImportText(rawText) {
  const text = String(rawText ?? '').trim();
  if (!text) {
    throw new Error('请先粘贴 MCP JSON 配置');
  }
  let parsed;
  try {
    parsed = JSON.parse(text);
  } catch (e) {
    throw new Error(`JSON 解析失败：${String(e?.message || e)}`);
  }

  const source =
    parsed && typeof parsed === 'object' && parsed.mcpServers && typeof parsed.mcpServers === 'object'
      ? parsed.mcpServers
      : parsed;

  if (!source || typeof source !== 'object' || Array.isArray(source)) {
    throw new Error('未找到可解析的 mcpServers 对象');
  }

  const rows = Object.entries(source).flatMap(([label, cfg]) => {
    if (!cfg || typeof cfg !== 'object' || Array.isArray(cfg)) {
      return [];
    }
    const command = String(cfg.command ?? '').trim();
    const url = String(cfg.url ?? cfg.server_url ?? '').trim();
    const args = Array.isArray(cfg.args) ? cfg.args.map((v) => String(v)) : [];
    const headers = cfg.headers && typeof cfg.headers === 'object' && !Array.isArray(cfg.headers) ? cfg.headers : {};
    const env = cfg.env && typeof cfg.env === 'object' && !Array.isArray(cfg.env) ? cfg.env : {};
    const allowedTools = Array.isArray(cfg.allowed_tools)
      ? cfg.allowed_tools
      : Array.isArray(cfg.allowedTools)
        ? cfg.allowedTools
        : [];

    return [
      {
        ...emptyMcpRow(),
        label: String(label ?? '').trim(),
        transportType: String(cfg.transport_type ?? cfg.transportType ?? (command ? 'stdio' : 'streamable_http')).trim(),
        url,
        command,
        argsText: args.join('\n'),
        allowedTools: allowedTools.map((v) => String(v)).join('\n'),
        headersText: Object.entries(headers)
          .map(([k, v]) => `${k}: ${String(v)}`)
          .join('\n'),
        envText: Object.entries(env)
          .map(([k, v]) => `${k}: ${String(v)}`)
          .join('\n'),
      },
    ];
  });

  if (rows.length === 0) {
    throw new Error('没有解析出任何 MCP 服务');
  }
  return rows;
}

export function isMcpRowReady(row) {
  const label = String(row?.label ?? '').trim();
  const url = String(row?.url ?? '').trim();
  const command = String(row?.command ?? '').trim();
  return label !== '' && (url !== '' || command !== '');
}

export function sameToolList(a, b) {
  const aa = Array.isArray(a) ? a : [];
  const bb = Array.isArray(b) ? b : [];
  if (aa.length !== bb.length) return false;
  for (let i = 0; i < aa.length; i += 1) {
    if (aa[i] !== bb[i]) return false;
  }
  return true;
}

export function mcpToolsForDisplay(status, row) {
  const source =
    status?.state === 'ok' || status?.state === 'warning'
      ? status?.tools
      : status?.state === 'idle'
        ? row?.cachedTools
        : [];
  return Array.isArray(source) ? source : [];
}

export function mapLLMConfig(r) {
  return {
    apiKey: r.apiKey ?? r.api_key ?? '',
    baseUrl: r.baseUrl ?? r.base_url ?? '',
    model: r.model ?? '',
    maxOutputTokens:
      typeof r.maxOutputTokens === 'number'
        ? r.maxOutputTokens
        : typeof r.max_output_tokens === 'number'
          ? r.max_output_tokens
          : 0,
  };
}

export function formatCount(value) {
  if (!value) return '0';
  if (value >= 1000) return `${(value / 1000).toFixed(value >= 10000 ? 0 : 1)}k`;
  return String(value);
}

export function hubRatingText(item) {
  const rating = typeof item?.ratingAverage === 'number' ? item.ratingAverage : 0;
  if (!rating) return '暂无评分';
  return `${rating.toFixed(1)} / 5`;
}
