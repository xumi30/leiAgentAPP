import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  DeleteOpenClawSkill,
  GetLLMConfigFormState,
  GetMCPConfigFormState,
  GetMCPHubPluginDetail,
  GetMCPHubStatus,
  GetOpenClawSkillState,
  InstallOpenClawSkill,
  InstallOpenClawSkillDeps,
  RegisterMCPHub,
  SaveLLMConfigForm,
  SaveMCPConfigForm,
  SearchMCPHub,
  ValidateMCPConfigRow,
} from '../../wailsjs/go/main/App';
import '../componentcss/SettingsModal.css';

const FALLBACK_HUB_CATEGORIES = [
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

function emptyRow() {
  return {
    name: '',
    apiKey: '',
    baseUrl: '',
    model: '',
    provider: '',
    streamMode: 'both',
    maxOutputTokens: 0,
    enabled: true,
  };
}

function emptyMcpRow() {
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

function emptyMcpStatus() {
  return {
    state: 'idle',
    message: '',
    tools: [],
    toolCount: 0,
    checkedAt: '',
  };
}

function emptyHubStatus() {
  return {
    registered: false,
    credentialsPath: '',
    message: '',
  };
}

function parseMcpImportText(rawText) {
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

function isMcpRowReady(row) {
  const label = String(row?.label ?? '').trim();
  const url = String(row?.url ?? '').trim();
  const command = String(row?.command ?? '').trim();
  return label !== '' && (url !== '' || command !== '');
}

function sameToolList(a, b) {
  const aa = Array.isArray(a) ? a : [];
  const bb = Array.isArray(b) ? b : [];
  if (aa.length !== bb.length) return false;
  for (let i = 0; i < aa.length; i += 1) {
    if (aa[i] !== bb[i]) return false;
  }
  return true;
}

function mcpToolsForDisplay(status, row) {
  const source =
    status?.state === 'ok' || status?.state === 'warning'
      ? status?.tools
      : status?.state === 'idle'
        ? row?.cachedTools
        : [];
  return Array.isArray(source) ? source : [];
}

function effectiveStreamMode(raw, provider) {
  const s = String(raw ?? '').trim();
  if (s) return s;
  return String(provider ?? '').toLowerCase().trim() === 'gemini' ? 'nonstream' : 'both';
}

function mapBackendRow(r) {
  return {
    name: r.name ?? '',
    apiKey: r.apiKey ?? r.api_key ?? '',
    baseUrl: r.baseUrl ?? r.base_url ?? '',
    model: r.model ?? '',
    provider: r.provider ?? '',
    streamMode: effectiveStreamMode(r.streamMode ?? r.stream_mode, r.provider),
    maxOutputTokens:
      typeof r.maxOutputTokens === 'number'
        ? r.maxOutputTokens
        : typeof r.max_output_tokens === 'number'
          ? r.max_output_tokens
          : 0,
    enabled: r.enabled !== false,
  };
}

function mapToText(obj) {
  if (!obj || typeof obj !== 'object' || Array.isArray(obj)) return '';
  return Object.entries(obj)
    .map(([k, v]) => `${k}: ${String(v)}`)
    .join('\n');
}

function formatCount(value) {
  if (!value) return '0';
  if (value >= 1000) return `${(value / 1000).toFixed(value >= 10000 ? 0 : 1)}k`;
  return String(value);
}

function uniqueMcpLabel(rows, desired) {
  const base = String(desired ?? '').trim() || 'mcp-service';
  const labels = new Set(rows.map((row) => String(row?.label ?? '').trim()).filter(Boolean));
  if (!labels.has(base)) return base;
  let index = 2;
  while (labels.has(`${base}-${index}`)) {
    index += 1;
  }
  return `${base}-${index}`;
}

function rowFromHubDeployment(detail, option, existingRows) {
  const connection = option?.connection ?? {};
  const command = String(connection.command ?? '').trim();
  const url = String(connection.url ?? '').trim();
  const rawType = String(connection.type ?? '').trim().toLowerCase();
  const transportType = command ? 'stdio' : rawType === 'sse' ? 'sse' : 'streamable_http';
  return {
    ...emptyMcpRow(),
    label: uniqueMcpLabel(existingRows, detail?.identifier || detail?.name || 'mcp-service'),
    transportType,
    url,
    command,
    argsText: Array.isArray(connection.args) ? connection.args.map((v) => String(v)).join('\n') : '',
    headersText: mapToText(connection.headers),
    envText: mapToText(connection.env),
  };
}

function hubRatingText(item) {
  const rating = typeof item?.ratingAverage === 'number' ? item.ratingAverage : 0;
  if (!rating) return '暂无评分';
  return `${rating.toFixed(1)} / 5`;
}

export default function SettingsModal({ open, onClose, onSaved }) {
  const [activeTab, setActiveTab] = useState('mcp');
  const [llmConfigEnabled, setLlmConfigEnabled] = useState(false);
  const [backends, setBackends] = useState(() => []);
  const [mcpServers, setMcpServers] = useState(() => []);
  const [mcpStatuses, setMcpStatuses] = useState(() => []);
  const [selectedMcpIndex, setSelectedMcpIndex] = useState(null);
  const [mcpImportText, setMcpImportText] = useState('');
  const [savePath, setSavePath] = useState('');
  const [usingExample, setUsingExample] = useState(false);
  const [loadErr, setLoadErr] = useState('');
  const [saveErr, setSaveErr] = useState('');
  const [saving, setSaving] = useState(false);
  const [hubStatus, setHubStatus] = useState(() => emptyHubStatus());
  const [hubRegisterName, setHubRegisterName] = useState('leiagentapp');
  const [hubRegisterDesc, setHubRegisterDesc] = useState('A desktop AI assistant that manages MCP services for local workflows.');
  const [registeringHub, setRegisteringHub] = useState(false);
  const [hubQuery, setHubQuery] = useState('');
  const [hubCategory, setHubCategory] = useState('');
  const [hubCategoryMenuOpen, setHubCategoryMenuOpen] = useState(false);
  const [hubLoading, setHubLoading] = useState(false);
  const [hubSearchErr, setHubSearchErr] = useState('');
  const [hubResults, setHubResults] = useState(() => []);
  const [hubCategories, setHubCategories] = useState(() => []);
  const [hubSelectedIdentifier, setHubSelectedIdentifier] = useState('');
  const [hubSelectedDetail, setHubSelectedDetail] = useState(null);
  const [hubDetailLoading, setHubDetailLoading] = useState(false);
  const [hubDetailErr, setHubDetailErr] = useState('');
  const [hubNotice, setHubNotice] = useState('');
  const [hubInstallStates, setHubInstallStates] = useState({});
  const [skillState, setSkillState] = useState({ workspaceRoot: '', skillsRoot: '', skills: [] });
  const [skillInstallText, setSkillInstallText] = useState('claw skill install official/baidu-search');
  const [skillInstalling, setSkillInstalling] = useState(false);
  const [skillNotice, setSkillNotice] = useState('');
  const [skillErr, setSkillErr] = useState('');
  const [skillBusyPath, setSkillBusyPath] = useState('');

  const lastValidatedRef = useRef([]);

  const loadLLMOnly = useCallback(async () => {
    setLoadErr('');
    try {
      const llmState = await GetLLMConfigFormState();
      const llmList = Array.isArray(llmState.backends) ? llmState.backends : [];
      setLlmConfigEnabled(!!llmState.configEnabled);
      setBackends(llmList.length > 0 ? llmList.map(mapBackendRow) : []);
      setSavePath(llmState.path ?? '');
      setUsingExample(!!llmState.usingExample);
    } catch (e) {
      setLoadErr(String(e?.message || e));
    }
  }, []);

  const loadNonLLM = useCallback(async () => {
    try {
      const [mcpState, nextHubStatus, nextSkillState] = await Promise.all([
        GetMCPConfigFormState(),
        GetMCPHubStatus(),
        GetOpenClawSkillState(),
      ]);

      const mcpList = Array.isArray(mcpState.servers) ? mcpState.servers : [];
      const nextMcp = mcpList.length > 0 ? mcpList.map((row) => ({ ...emptyMcpRow(), ...row })) : [];
      setMcpServers(nextMcp);
      setSelectedMcpIndex(null);
      setMcpStatuses(
        nextMcp.map((row) =>
          row.lastCheckState
            ? {
                state: row.lastCheckState,
                message: row.lastCheckMessage || (Array.isArray(row.cachedTools) && row.cachedTools.length > 0 ? `已缓存 ${row.cachedTools.length} 个工具` : ''),
                tools: Array.isArray(row.cachedTools) ? row.cachedTools : [],
                toolCount: Array.isArray(row.cachedTools) ? row.cachedTools.length : 0,
                checkedAt: row.lastCheckedAt || '',
              }
            : emptyMcpStatus()
        )
      );
      lastValidatedRef.current = nextMcp.map(() => '');

      // Prefer LLM path (already set). Fall back to MCP path if LLM path is empty.
      setSavePath((prev) => prev || mcpState.path || '');
      setUsingExample((prev) => prev || !!mcpState.usingExample);

      setHubStatus({ ...emptyHubStatus(), ...(nextHubStatus ?? {}) });
      setHubNotice('');
      setHubSearchErr('');
      setHubDetailErr('');

      setSkillState(nextSkillState ?? { workspaceRoot: '', skillsRoot: '', skills: [] });
      setSkillNotice('');
      setSkillErr('');
    } catch (e) {
      // Non-LLM panels failed: keep LLM visible and only show the error banner.
      setLoadErr(String(e?.message || e));
    }
  }, []);

  useEffect(() => {
    if (open) {
      void (async () => {
        await loadLLMOnly();
        // Load other panels in background so LLM renders first.
        loadNonLLM();
      })();
    }
  }, [open, loadLLMOnly, loadNonLLM]);

  useEffect(() => {
    if (llmConfigEnabled || activeTab !== 'llm') return;
    setActiveTab('mcp');
  }, [activeTab, llmConfigEnabled]);

  useEffect(() => {
    if (open) return;
    setHubQuery('');
    setHubCategory('');
    setHubResults([]);
    setHubCategories([]);
    setHubSelectedIdentifier('');
    setHubSelectedDetail(null);
    setHubCategoryMenuOpen(false);
    setHubDetailErr('');
    setHubNotice('');
    setHubSearchErr('');
    setHubInstallStates({});
    setSkillNotice('');
    setSkillErr('');
    setSkillBusyPath('');
  }, [open]);

  const refreshSkills = useCallback(async () => {
    const nextSkillState = await GetOpenClawSkillState();
    setSkillState(nextSkillState ?? { workspaceRoot: '', skillsRoot: '', skills: [] });
  }, []);

  const handleInstallSkill = useCallback(async () => {
    setSkillInstalling(true);
    setSkillErr('');
    setSkillNotice('');
    try {
      const result = await InstallOpenClawSkill(skillInstallText);
      setSkillNotice(result?.ok ? `已安装 ${result.slug || 'skill'}` : '安装命令已执行');
      await refreshSkills();
    } catch (e) {
      setSkillErr(String(e?.message || e));
      await refreshSkills().catch(() => {});
    } finally {
      setSkillInstalling(false);
    }
  }, [refreshSkills, skillInstallText]);

  const handleRecheckSkill = useCallback(async (skill) => {
    if (!skill?.path) return;
    setSkillBusyPath(skill.path);
    setSkillErr('');
    setSkillNotice('');
    try {
      await refreshSkills();
      setSkillNotice(`已重新校验 ${skill.name || 'skill'}`);
    } catch (e) {
      setSkillErr(String(e?.message || e));
    } finally {
      setSkillBusyPath('');
    }
  }, [refreshSkills]);

  const handleDeleteSkill = useCallback(async (skill) => {
    if (!skill?.path) return;
    setSkillBusyPath(skill.path);
    setSkillErr('');
    setSkillNotice('');
    try {
      const result = await DeleteOpenClawSkill(skill.path);
      setSkillNotice(result?.message || `已删除 ${skill.name || 'skill'}`);
      await refreshSkills();
    } catch (e) {
      setSkillErr(String(e?.message || e));
      await refreshSkills().catch(() => {});
    } finally {
      setSkillBusyPath('');
    }
  }, [refreshSkills]);

  const handleInstallSkillDeps = useCallback(async (skill) => {
    if (!skill?.path) return;
    setSkillBusyPath(skill.path);
    setSkillErr('');
    setSkillNotice('');
    try {
      const result = await InstallOpenClawSkillDeps(skill.path);
      setSkillNotice(result?.message || `已安装 ${skill.name || 'skill'} 依赖`);
      await refreshSkills();
    } catch (e) {
      setSkillErr(String(e?.message || e));
      await refreshSkills().catch(() => {});
    } finally {
      setSkillBusyPath('');
    }
  }, [refreshSkills]);

  const updateBackend = (index, field, value) => {
    setBackends((prev) => {
      const next = [...prev];
      next[index] = { ...next[index], [field]: value };
      return next;
    });
  };

  const addBackendRow = () => {
    setBackends((prev) => [...prev, emptyRow()]);
  };

  const removeBackendRow = (index) => {
    setBackends((prev) => prev.filter((_, i) => i !== index));
  };

  const updateMcpServer = (index, field, value) => {
    setMcpServers((prev) => {
      const next = [...prev];
      next[index] = { ...next[index], [field]: value, ...(field === 'cachedTools' ? {} : { cachedTools: [] }) };
      return next;
    });
  };

  const addMcpServerRow = () => {
    const nextIndex = mcpServers.length;
    setMcpServers((prev) => [...prev, emptyMcpRow()]);
    setMcpStatuses((prev) => [...prev, emptyMcpStatus()]);
    lastValidatedRef.current = [...lastValidatedRef.current, ''];
    setSelectedMcpIndex(nextIndex);
  };

  const removeMcpServerRow = (index) => {
    setMcpServers((prev) => prev.filter((_, i) => i !== index));
    setMcpStatuses((prev) => prev.filter((_, i) => i !== index));
    lastValidatedRef.current = lastValidatedRef.current.filter((_, i) => i !== index);
    setSelectedMcpIndex((prev) => {
      if (prev == null) return prev;
      if (prev === index) return null;
      return prev > index ? prev - 1 : prev;
    });
  };

  const handleImportMcp = () => {
    setSaveErr('');
    try {
      const rows = parseMcpImportText(mcpImportText);
      setMcpServers((prev) => [...prev, ...rows]);
      setMcpStatuses((prev) => [...prev, ...rows.map(() => emptyMcpStatus())]);
      lastValidatedRef.current = [...lastValidatedRef.current, ...rows.map(() => '')];
      setMcpImportText('');
      setHubNotice(`已导入 ${rows.length} 个 MCP 服务`);
    } catch (e) {
      setSaveErr(String(e?.message || e));
    }
  };

  const setMcpStatusAt = useCallback((index, next) => {
    setMcpStatuses((prev) => {
      const out = [...prev];
      out[index] = { ...emptyMcpStatus(), ...(out[index] ?? {}), ...next };
      return out;
    });
  }, []);

  const validateMcpRow = useCallback(async (index, row) => {
    if (!isMcpRowReady(row)) {
      setMcpStatusAt(index, emptyMcpStatus());
      return null;
    }
    setMcpStatusAt(index, { state: 'checking', message: '校验中…' });
    try {
      const result = await ValidateMCPConfigRow(row);
      const tools = Array.isArray(result?.tools) ? result.tools : [];
      setMcpStatusAt(index, {
        state: result?.lastCheckState || (result?.ok ? 'ok' : 'error'),
        message: String(result?.message ?? ''),
        tools,
        toolCount: typeof result?.toolCount === 'number' ? result.toolCount : tools.length,
        checkedAt: result?.checkedAt ?? '',
      });
      if (result?.ok || result?.lastCheckState === 'warning') {
        setMcpServers((prev) => {
          const next = [...prev];
          const current = next[index];
          if (!current) return prev;
          const nextDetails = Array.isArray(result?.toolDetails) ? result.toolDetails : [];
          if (
            sameToolList(current.cachedTools, tools) &&
            current.lastCheckState === (result?.lastCheckState || (result?.ok ? 'ok' : 'error')) &&
            current.lastCheckMessage === String(result?.message ?? '') &&
            current.lastCheckedAt === (result?.checkedAt ?? '')
          ) {
            return prev;
          }
          next[index] = {
            ...current,
            cachedTools: tools,
            cachedToolDetails: nextDetails,
            lastCheckState: result?.lastCheckState || (result?.ok ? 'ok' : 'error'),
            lastCheckMessage: String(result?.message ?? ''),
            lastCheckedAt: result?.checkedAt ?? '',
          };
          return next;
        });
      } else {
        setMcpServers((prev) => {
          const next = [...prev];
          const current = next[index];
          if (!current) return prev;
          next[index] = {
            ...current,
            cachedTools: [],
            cachedToolDetails: [],
            lastCheckState: result?.lastCheckState || 'error',
            lastCheckMessage: String(result?.message ?? ''),
            lastCheckedAt: result?.checkedAt ?? '',
          };
          return next;
        });
      }
      return result;
    } catch (e) {
      const message = String(e?.message || e);
      setMcpStatusAt(index, { state: 'error', message, tools: [], toolCount: 0, checkedAt: '' });
      return { ok: false, message, tools: [], toolCount: 0 };
    }
  }, [setMcpStatusAt]);

  const validateAllMcpServers = useCallback(async () => {
    const nextRows = [...mcpServers];
    for (let i = 0; i < nextRows.length; i += 1) {
      const row = nextRows[i];
      if (!isMcpRowReady(row)) continue;
      const result = await validateMcpRow(i, row);
      if ((result?.ok || result?.lastCheckState === 'warning') && Array.isArray(result.tools)) {
        nextRows[i] = { ...nextRows[i], cachedTools: result.tools };
      }
    }
    return nextRows;
  }, [mcpServers, validateMcpRow]);

  const mcpSignatures = useMemo(
    () =>
      mcpServers.map((row) =>
        JSON.stringify({
          label: row.label,
          transportType: row.transportType,
          url: row.url,
          command: row.command,
          argsText: row.argsText,
          allowedTools: row.allowedTools,
          headersText: row.headersText,
          envText: row.envText,
        })
      ),
    [mcpServers]
  );

  useEffect(() => {
    if (!open || activeTab !== 'mcp') return undefined;
    const timer = setTimeout(() => {
      mcpServers.forEach((row, index) => {
        const sig = mcpSignatures[index] ?? '';
        if (!isMcpRowReady(row)) {
          lastValidatedRef.current[index] = '';
          setMcpStatusAt(index, emptyMcpStatus());
          return;
        }
        if (lastValidatedRef.current[index] === sig) return;
        lastValidatedRef.current[index] = sig;
        void validateMcpRow(index, row);
      });
    }, 700);
    return () => clearTimeout(timer);
  }, [activeTab, mcpServers, mcpSignatures, open, setMcpStatusAt, validateMcpRow]);

  const handleSave = async () => {
    setSaveErr('');
    setSaving(true);
    try {
      if (activeTab === 'mcp') {
        const validatedRows = await validateAllMcpServers();
        await SaveMCPConfigForm(validatedRows);
      } else {
        await SaveLLMConfigForm({}, backends);
      }
      setUsingExample(false);
      onSaved?.();
      onClose();
    } catch (e) {
      setSaveErr(String(e?.message || e));
    } finally {
      setSaving(false);
    }
  };

  const handleRegisterHub = async () => {
    setHubSearchErr('');
    setHubNotice('');
    setRegisteringHub(true);
    try {
      const result = await RegisterMCPHub(hubRegisterName, hubRegisterDesc);
      setHubStatus({
        registered: !!result?.registered,
        credentialsPath: result?.credentialsPath ?? '',
        message: result?.message ?? 'MCP Hub 注册完成',
      });
      setHubNotice(result?.message || 'MCP Hub 注册完成');
    } catch (e) {
      setHubSearchErr(String(e?.message || e));
    } finally {
      setRegisteringHub(false);
    }
  };

  const handleSearchHub = useCallback(async () => {
    if (!hubStatus.registered) {
      setHubSearchErr('请先完成 MCP Hub 注册');
      return;
    }
    if (!String(hubQuery).trim()) {
      setHubSearchErr('请输入搜索关键词');
      return;
    }
    setHubLoading(true);
    setHubSearchErr('');
    setHubNotice('');
    try {
      const result = await SearchMCPHub(hubQuery, hubCategory, 1, 12);
      setHubResults(Array.isArray(result?.items) ? result.items : []);
      setHubCategories(Array.isArray(result?.categories) ? result.categories : []);
      if (Array.isArray(result?.items) && result.items[0]?.identifier) {
        setHubSelectedIdentifier(result.items[0].identifier);
      } else {
        setHubSelectedIdentifier('');
        setHubSelectedDetail(null);
      }
    } catch (e) {
      setHubSearchErr(String(e?.message || e));
    } finally {
      setHubLoading(false);
    }
  }, [hubCategory, hubQuery, hubStatus.registered]);

  const handleSelectHubItem = useCallback(async (identifier) => {
    if (!identifier) return;
    setHubSelectedIdentifier(identifier);
    setHubSelectedDetail(null);
    setHubDetailErr('');
    setHubDetailLoading(true);
    try {
      const detail = await GetMCPHubPluginDetail(identifier);
      setHubSelectedDetail(detail ?? null);
    } catch (e) {
      setHubDetailErr(String(e?.message || e));
    } finally {
      setHubDetailLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!open || activeTab !== 'mcp' || !hubSelectedIdentifier) return;
    void handleSelectHubItem(hubSelectedIdentifier);
  }, [activeTab, handleSelectHubItem, hubSelectedIdentifier, open]);

  useEffect(() => {
    if (String(hubQuery).trim()) return;
    setHubResults([]);
    setHubSelectedIdentifier('');
    setHubSelectedDetail(null);
    setHubCategoryMenuOpen(false);
    setHubDetailErr('');
    setHubNotice('');
    setHubSearchErr('');
    setHubInstallStates({});
  }, [hubQuery]);

  const hubInstallKey = (detailIdentifier, option, index) =>
    `${String(detailIdentifier || 'unknown')}::${String(option?.installationMethod || 'manual')}::${String(option?.connection?.type || 'unknown')}::${index}`;

  const installHubOption = async (option, installKey) => {
    const nextRow = rowFromHubDeployment(hubSelectedDetail, option, mcpServers);
    const nextIndex = mcpServers.length;
    const nextRows = [...mcpServers, nextRow];
    setHubInstallStates((prev) => ({
      ...prev,
      [installKey]: { state: 'installing', message: '安装中…' },
    }));
    setMcpServers(nextRows);
    setMcpStatuses((prev) => [...prev, emptyMcpStatus()]);
    lastValidatedRef.current = [...lastValidatedRef.current, ''];
    try {
      const result = await validateMcpRow(nextIndex, nextRow);
      if (result?.ok) {
        const persistedRows = [...nextRows];
        persistedRows[nextIndex] = {
          ...persistedRows[nextIndex],
          cachedTools: Array.isArray(result?.tools) ? result.tools : [],
          cachedToolDetails: Array.isArray(result?.toolDetails) ? result.toolDetails : [],
          lastCheckState: result?.lastCheckState || 'ok',
          lastCheckMessage: String(result?.message ?? ''),
          lastCheckedAt: result?.checkedAt ?? '',
        };
        await SaveMCPConfigForm(persistedRows);
        setMcpServers(persistedRows);
        setHubInstallStates((prev) => ({
          ...prev,
          [installKey]: { state: 'installed', message: '已安装' },
        }));
        setHubNotice(`已将 ${nextRow.label} 安装到配置文件`);
        return;
      }
      setHubInstallStates((prev) => ({
        ...prev,
        [installKey]: { state: 'failed', message: `失败：${String(result?.message || '重试？')}` },
      }));
    } catch (e) {
      setMcpServers((prev) => prev.filter((_, index) => index !== nextIndex));
      setMcpStatuses((prev) => prev.filter((_, index) => index !== nextIndex));
      lastValidatedRef.current = lastValidatedRef.current.filter((_, index) => index !== nextIndex);
      setHubInstallStates((prev) => ({
        ...prev,
        [installKey]: { state: 'failed', message: `失败：${String(e?.message || e || '重试？')}` },
      }));
    }
  };

  if (!open) return null;

  const statusClassName = (state) => `settings-status-dot settings-status-dot--${state || 'idle'}`;
  const selectedMcpRow = selectedMcpIndex != null ? mcpServers[selectedMcpIndex] ?? null : null;
  const selectedMcpStatus = selectedMcpIndex != null ? mcpStatuses[selectedMcpIndex] ?? emptyMcpStatus() : emptyMcpStatus();
  const selectedMcpTools = mcpToolsForDisplay(selectedMcpStatus, selectedMcpRow);
  const hubDeploymentOptions = Array.isArray(hubSelectedDetail?.deploymentOptions) ? hubSelectedDetail.deploymentOptions : [];
  const availableHubCategories = hubCategories.length > 0 ? hubCategories : FALLBACK_HUB_CATEGORIES;

  const colInputs = (row, index) => (
    <>
      <td className="settings-table__cell-check">
        <input
          type="checkbox"
          className="settings-table__check"
          checked={row.enabled}
          onChange={(e) => updateBackend(index, 'enabled', e.target.checked)}
          title="启用后参与故障转移"
          aria-label="启用此后端"
        />
      </td>
      <td>
        <input className="settings-table__input" value={row.name} onChange={(e) => updateBackend(index, 'name', e.target.value)} placeholder="标识" spellCheck={false} autoComplete="off" />
      </td>
      <td>
        <input className="settings-table__input" type="password" value={row.apiKey} onChange={(e) => updateBackend(index, 'apiKey', e.target.value)} placeholder="api_key" spellCheck={false} autoComplete="off" />
      </td>
      <td>
        <input className="settings-table__input" value={row.baseUrl} onChange={(e) => updateBackend(index, 'baseUrl', e.target.value)} placeholder="base_url" spellCheck={false} autoComplete="off" />
      </td>
      <td>
        <input className="settings-table__input" value={row.model} onChange={(e) => updateBackend(index, 'model', e.target.value)} placeholder="model" spellCheck={false} autoComplete="off" />
      </td>
      <td>
        <input className="settings-table__input" value={row.provider} onChange={(e) => updateBackend(index, 'provider', e.target.value)} placeholder="留空 / gemini" spellCheck={false} autoComplete="off" />
      </td>
      <td>
        <input className="settings-table__input" value={row.streamMode} onChange={(e) => updateBackend(index, 'streamMode', e.target.value)} placeholder="nonstream | stream | both" spellCheck={false} autoComplete="off" />
      </td>
      <td>
        <input
          className="settings-table__input settings-table__input--num"
          type="number"
          min={0}
          value={row.maxOutputTokens || ''}
          onChange={(e) => {
            const v = e.target.value;
            updateBackend(index, 'maxOutputTokens', v === '' ? 0 : parseInt(v, 10) || 0);
          }}
          placeholder="0=默认"
        />
      </td>
    </>
  );

  return (
    <div className="settings-overlay" role="presentation" onMouseDown={onClose}>
      <div className="settings-sheet settings-sheet--wide" role="dialog" aria-labelledby="settings-title" onMouseDown={(e) => e.stopPropagation()}>
        <div className="settings-sheet__header">
          <h2 id="settings-title" className="settings-sheet__title">设置</h2>
          <button type="button" className="settings-sheet__close" onClick={onClose} aria-label="关闭">完成</button>
        </div>

        <div className="settings-tabs" role="tablist" aria-label="设置分组">
          {llmConfigEnabled ? (
            <button type="button" className={`settings-tabs__btn ${activeTab === 'llm' ? 'settings-tabs__btn--active' : ''}`} onClick={() => setActiveTab('llm')}>LLM</button>
          ) : null}
          <button type="button" className={`settings-tabs__btn ${activeTab === 'mcp' ? 'settings-tabs__btn--active' : ''}`} onClick={() => setActiveTab('mcp')}>MCP</button>
          <button type="button" className={`settings-tabs__btn ${activeTab === 'skills' ? 'settings-tabs__btn--active' : ''}`} onClick={() => setActiveTab('skills')}>Skills</button>
        </div>

        <p className="settings-sheet__path">
          {savePath ? (
            <>
              <span className="settings-sheet__path-label">保存路径</span>
              <code className="settings-sheet__path-value">{savePath}</code>
            </>
          ) : null}
          {usingExample ? <span className="settings-sheet__hint">尚未有配置文件，保存后将创建 config.yaml</span> : null}
          {activeTab === 'mcp' && hubStatus.credentialsPath ? (
            <>
              <span className="settings-sheet__path-label">Hub 凭据</span>
              <code className="settings-sheet__path-value">{hubStatus.credentialsPath}</code>
            </>
          ) : null}
          {activeTab === 'skills' && skillState.skillsRoot ? (
            <>
              <span className="settings-sheet__path-label">Skills</span>
              <code className="settings-sheet__path-value">{skillState.skillsRoot}</code>
            </>
          ) : null}
        </p>

        <p className="settings-sheet__note">
          {activeTab === 'skills'
            ? '支持粘贴 ClawHub 安装命令，安装后 leiAgent 会扫描 ./skills 并将已支持的 skill 映射为本地工具。当前竖切片支持 baidu-search。'
            : activeTab === 'mcp'
            ? 'MCP 页支持从 LobeHub MCP Hub 搜索并安装配置，也保留手动 JSON 导入与表单编辑。安装后可直接点击服务进入详情页进行测试、修改和删除。'
            : '按顺序故障转移；仅勾选启用的行会参与。每行须填写 base_url、model。某行未填 API Key 时，若文件中 llm.api_key 已填写会回退使用该 Key，否则用环境变量。'}
        </p>

        {loadErr ? <div className="settings-sheet__error">{loadErr}</div> : null}
        {saveErr ? <div className="settings-sheet__error">{saveErr}</div> : null}
        {hubSearchErr ? <div className="settings-sheet__error">{hubSearchErr}</div> : null}
        {hubNotice ? <div className="settings-sheet__notice">{hubNotice}</div> : null}
        {skillErr ? <div className="settings-sheet__error">{skillErr}</div> : null}
        {skillNotice ? <div className="settings-sheet__notice">{skillNotice}</div> : null}

        {activeTab === 'llm' ? (
          <div className="settings-sheet__scroll">
            <div className="settings-list-block">
              <div className="settings-list-block__toolbar">
                <button type="button" className="settings-btn settings-btn--secondary settings-btn--small" onClick={addBackendRow}>添加一行</button>
              </div>
              <table className="settings-table settings-table--wide">
                <thead>
                  <tr>
                    <th className="settings-table__th-check">启用</th>
                    <th>名称</th>
                    <th>API Key</th>
                    <th>Base URL</th>
                    <th>Model</th>
                    <th>Provider</th>
                    <th>Stream</th>
                    <th>Max tokens</th>
                    <th className="settings-table__th-actions" />
                  </tr>
                </thead>
                <tbody>
                  {backends.length === 0 ? (
                    <tr><td colSpan={9} className="settings-table__empty">暂无行，点击「添加一行」配置多后端。</td></tr>
                  ) : (
                    backends.map((row, i) => (
                      <tr key={i}>
                        {colInputs(row, i)}
                        <td className="settings-table__actions">
                          <button type="button" className="settings-table__remove" onClick={() => removeBackendRow(i)} aria-label="删除此行">删除</button>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>
        ) : activeTab === 'skills' ? (
          <div className="settings-sheet__scroll settings-sheet__scroll--mcp">
            <div className="settings-list-block">
              <div className="settings-list-block__toolbar settings-list-block__toolbar--stack">
                <div className="settings-hub">
                  <div className="settings-hub__search">
                    <input
                      className="settings-table__input"
                      value={skillInstallText}
                      onChange={(e) => setSkillInstallText(e.target.value)}
                      placeholder="claw skill install official/baidu-search"
                      spellCheck={false}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                          e.preventDefault();
                          void handleInstallSkill();
                        }
                      }}
                    />
                    <button type="button" className="settings-btn settings-btn--primary settings-btn--small" onClick={handleInstallSkill} disabled={skillInstalling}>
                      {skillInstalling ? '安装中…' : '安装'}
                    </button>
                    <button type="button" className="settings-btn settings-btn--secondary settings-btn--small" onClick={() => void refreshSkills()} disabled={skillInstalling}>
                      刷新
                    </button>
                  </div>
                </div>
              </div>
              <table className="settings-table settings-table--mcp">
                <thead>
                  <tr>
                    <th>Skill</th>
                    <th>状态</th>
                    <th>依赖</th>
                    <th>适配器</th>
                    <th className="settings-table__th-actions" />
                  </tr>
                </thead>
                <tbody>
                  {!Array.isArray(skillState.skills) || skillState.skills.length === 0 ? (
                    <tr><td colSpan={5} className="settings-table__empty">暂无已安装 skill。可以先安装 baidu-search 试跑。</td></tr>
                  ) : (
                    skillState.skills.map((skill) => {
                      const busy = skillBusyPath === skill.path;
                      return (
                        <tr key={skill.path || skill.name}>
                          <td>
                            <div className="settings-table__title">{skill.name}</div>
                            <div className="settings-table__sub">{skill.description || skill.path}</div>
                          </td>
                          <td>
                            <span className={`settings-status-dot settings-status-dot--${skill.ready ? 'ok' : 'warning'}`} />
                            {skill.statusDetail || skill.status || '未知'}
                          </td>
                          <td>
                            <div className="settings-table__sub">
                              bins: {(skill.requires?.bins || []).join(', ') || '无'}
                            </div>
                            <div className="settings-table__sub">
                              pip: {(skill.pythonDeps || []).join(', ') || '无'}
                            </div>
                            <div className="settings-table__sub">
                              env: {(skill.requires?.env || []).join(', ') || '无'}
                            </div>
                          </td>
                          <td>{skill.supported ? '已支持' : '未适配'}</td>
                          <td className="settings-table__actions settings-table__actions--icons">
                            <button
                              type="button"
                              className="settings-icon-btn"
                              onClick={() => void handleRecheckSkill(skill)}
                              disabled={busy || skillInstalling}
                              title="重新校验"
                              aria-label={`重新校验 ${skill.name || 'skill'}`}
                            >
                              <span aria-hidden>↻</span>
                            </button>
                            <button
                              type="button"
                              className="settings-icon-btn"
                              onClick={() => void handleInstallSkillDeps(skill)}
                              disabled={busy || skillInstalling || !Array.isArray(skill.pythonDeps) || skill.pythonDeps.length === 0}
                              title="安装依赖"
                              aria-label={`安装 ${skill.name || 'skill'} 依赖`}
                            >
                              <span aria-hidden>↓</span>
                            </button>
                            <button
                              type="button"
                              className="settings-icon-btn settings-icon-btn--danger"
                              onClick={() => void handleDeleteSkill(skill)}
                              disabled={busy || skillInstalling}
                              title="删除"
                              aria-label={`删除 ${skill.name || 'skill'}`}
                            >
                              <span aria-hidden>×</span>
                            </button>
                          </td>
                        </tr>
                      );
                    })
                  )}
                </tbody>
              </table>
            </div>
          </div>
        ) : (
          <div className="settings-sheet__scroll settings-sheet__scroll--mcp">
            <div className="settings-list-block">
              <div className="settings-list-block__toolbar settings-list-block__toolbar--stack">
                {!hubStatus.registered ? (
                  <div className="settings-hub-register">
                    <div className="settings-hub-register__title">连接 MCP Hub</div>
                    <div className="settings-hub-register__desc">{hubStatus.message || '使用 LobeHub 官方 marketplace CLI 前需要先完成一次身份注册。'}</div>
                    <div className="settings-hub-register__grid">
                      <label className="settings-mcp-field">
                        <span className="settings-mcp-field__label">显示名称</span>
                        <input className="settings-table__input" value={hubRegisterName} onChange={(e) => setHubRegisterName(e.target.value)} placeholder="leiagentapp" spellCheck={false} />
                      </label>
                      <label className="settings-mcp-field">
                        <span className="settings-mcp-field__label">描述</span>
                        <input className="settings-table__input" value={hubRegisterDesc} onChange={(e) => setHubRegisterDesc(e.target.value)} placeholder="Describe this client" spellCheck={false} />
                      </label>
                    </div>
                    <div className="settings-import__actions">
                      <button type="button" className="settings-btn settings-btn--primary settings-btn--small" onClick={handleRegisterHub} disabled={registeringHub}>
                        {registeringHub ? '注册中…' : '注册 Hub'}
                      </button>
                    </div>
                  </div>
                ) : (
                  <div className="settings-hub">
                    <div className="settings-hub__search">
                      <input
                        className="settings-table__input"
                        value={hubQuery}
                        onChange={(e) => setHubQuery(e.target.value)}
                        placeholder="搜索 MCP Hub，例如 filesystem、postgres、github"
                        spellCheck={false}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter') {
                            e.preventDefault();
                            void handleSearchHub();
                          }
                        }}
                      />
                      <div className="settings-select">
                        <button
                          type="button"
                          className="settings-table__input settings-select__trigger"
                          onClick={() => {
                            if (availableHubCategories.length === 0) return;
                            setHubCategoryMenuOpen((prev) => !prev);
                          }}
                        >
                          <span>{hubCategory || '全部分类'}</span>
                          <span className="settings-select__arrow">{hubCategoryMenuOpen ? '▲' : '▼'}</span>
                        </button>
                        {hubCategoryMenuOpen ? (
                          <div className="settings-select__menu">
                            <button
                              type="button"
                              className={`settings-select__option${hubCategory === '' ? ' settings-select__option--active' : ''}`}
                              onClick={() => {
                                setHubCategory('');
                                setHubCategoryMenuOpen(false);
                              }}
                            >
                              全部分类
                            </button>
                            {availableHubCategories.map((category) => (
                              <button
                                key={category}
                                type="button"
                                className={`settings-select__option${hubCategory === category ? ' settings-select__option--active' : ''}`}
                                onClick={() => {
                                  setHubCategory(category);
                                  setHubCategoryMenuOpen(false);
                                }}
                              >
                                {category}
                              </button>
                            ))}
                          </div>
                        ) : null}
                      </div>
                      <button type="button" className="settings-btn settings-btn--primary settings-btn--small" onClick={() => void handleSearchHub()} disabled={hubLoading}>
                        {hubLoading ? '搜索中…' : '搜索 Hub'}
                      </button>
                    </div>

                    <div className="settings-hub__results">
                      {hubResults.length === 0 ? (
                        <div className="settings-table__empty">输入关键词后可从 MCP Hub 搜索并查看安装方案。</div>
                      ) : (
                        hubResults.map((item) => (
                          <div key={item.identifier} className={`settings-hub-card-wrap${hubSelectedIdentifier === item.identifier ? ' settings-hub-card-wrap--active' : ''}`}>
                            <button
                              type="button"
                              className={`settings-hub-card${hubSelectedIdentifier === item.identifier ? ' settings-hub-card--active' : ''}`}
                              onClick={() => void handleSelectHubItem(item.identifier)}
                            >
                              <div className="settings-hub-card__title-row">
                                <div className="settings-hub-card__title">{item.name || item.identifier}</div>
                                <div className="settings-hub-card__badges">
                                  {item.isValidated ? <span className="settings-chip settings-chip--ok">Validated</span> : null}
                                  {item.isOfficial ? <span className="settings-chip">Official</span> : null}
                                </div>
                              </div>
                              <div className="settings-hub-card__meta">{item.author || 'Unknown'} · {item.category || 'uncategorized'} · {item.connectionType || 'local'}</div>
                              <div className="settings-hub-card__desc">{item.description || '暂无描述'}</div>
                              <div className="settings-hub-card__stats">
                                <span>安装 {formatCount(item.installCount)}</span>
                                <span>评分 {hubRatingText(item)}</span>
                                <span>版本 {item.version || '-'}</span>
                              </div>
                            </button>

                            {hubSelectedIdentifier === item.identifier ? (
                              <div className="settings-hub-card__detail">
                                {hubDetailLoading ? (
                                  <div className="settings-table__empty">正在加载安装详情…</div>
                                ) : hubDetailErr ? (
                                  <div className="settings-sheet__error">{hubDetailErr}</div>
                                ) : hubSelectedDetail ? (
                                  <>
                                    <div className="settings-hub-detail__header">
                                      <div>
                                        <h3 className="settings-hub-detail__title">{hubSelectedDetail.name || hubSelectedDetail.identifier}</h3>
                                        <div className="settings-hub-detail__meta">
                                          {hubSelectedDetail.author?.name || 'Unknown'} · {hubSelectedDetail.category || 'uncategorized'} · 安装 {formatCount(hubSelectedDetail.installCount)}
                                        </div>
                                      </div>
                                      {hubSelectedDetail.homepage ? (
                                        <a className="settings-btn settings-btn--secondary settings-btn--small" href={hubSelectedDetail.homepage} target="_blank" rel="noreferrer">主页</a>
                                      ) : null}
                                    </div>
                                    <p className="settings-hub-detail__desc">{hubSelectedDetail.description || '暂无描述'}</p>

                                    <div className="settings-hub-deployments">
                                      {hubDeploymentOptions.length === 0 ? (
                                        <div className="settings-table__empty">该 MCP 未提供可解析的部署方式，暂时无法一键安装。</div>
                                      ) : (
                                        hubDeploymentOptions.map((option, index) => {
                                          const installKey = hubInstallKey(hubSelectedDetail?.identifier, option, index);
                                          const installState = hubInstallStates[installKey] ?? { state: 'idle', message: '' };
                                          let installLabel = '安装到配置';
                                          if (installState.state === 'installing') installLabel = '安装中…';
                                          if (installState.state === 'installed') installLabel = '已安装';
                                          if (installState.state === 'failed') installLabel = '失败：重试？';

                                          return (
                                          <div key={`${option.installationMethod}-${index}`} className="settings-hub-deployment">
                                            <div className="settings-hub-deployment__title-row">
                                              <div className="settings-hub-deployment__title">
                                                {option.installationMethod || 'manual'} · {option.connection?.type || 'unknown'}
                                              </div>
                                              {option.isRecommended ? <span className="settings-chip settings-chip--ok">推荐</span> : null}
                                            </div>
                                            <div className="settings-hub-deployment__desc">{option.description || '暂无说明'}</div>
                                            <div className="settings-hub-deployment__command">
                                              <code>{option.connection?.command || option.connection?.url || '未提供 command/url'}</code>
                                            </div>
                                            {Array.isArray(option.connection?.args) && option.connection.args.length > 0 ? (
                                              <div className="settings-hub-deployment__args">{option.connection.args.join(' ')}</div>
                                            ) : null}
                                            {Array.isArray(option.systemDependencies) && option.systemDependencies.length > 0 ? (
                                              <div className="settings-hub-deployment__deps">
                                                依赖: {option.systemDependencies.map((dep) => dep.name).filter(Boolean).join(', ')}
                                              </div>
                                            ) : null}
                                            {installState.state === 'failed' && installState.message ? (
                                              <div className="settings-hub-deployment__error" title={installState.message}>
                                                {installState.message}
                                              </div>
                                            ) : null}
                                            <div className="settings-import__actions">
                                              <button
                                                type="button"
                                                className={`settings-btn settings-btn--small ${installState.state === 'installed' ? 'settings-btn--secondary' : 'settings-btn--primary'}`}
                                                onClick={() => void installHubOption(option, installKey)}
                                                disabled={installState.state === 'installing' || installState.state === 'installed'}
                                              >
                                                {installLabel}
                                              </button>
                                            </div>
                                          </div>
                                        )})
                                      )}
                                    </div>
                                  </>
                                ) : null}
                              </div>
                            ) : null}
                          </div>
                        ))
                      )}
                    </div>
                  </div>
                )}

                <div className="settings-import">
                  <textarea
                    className="settings-import__textarea"
                    value={mcpImportText}
                    onChange={(e) => setMcpImportText(e.target.value)}
                    placeholder={'粘贴 mcpServers JSON，例如：{\n  "mcpServers": {\n    "novel-workflow": {\n      "command": "npx",\n      "args": ["-y", "@ttaqt/novel-workflow-mcp@latest"]\n    }\n  }\n}'}
                    spellCheck={false}
                  />
                  <div className="settings-import__actions">
                    <button type="button" className="settings-btn settings-btn--secondary settings-btn--small" onClick={addMcpServerRow}>添加空白服务</button>
                    <button type="button" className="settings-btn settings-btn--primary settings-btn--small" onClick={handleImportMcp}>粘贴解析</button>
                  </div>
                </div>
              </div>

              <div className="settings-installed">
                <div className="settings-installed__title">已安装到当前配置</div>
                <div className="settings-mcp-list">
                  {mcpServers.length === 0 ? (
                    <div className="settings-table__empty">暂无 MCP 服务，先从 Hub 安装或手动添加。</div>
                  ) : (
                    mcpServers.map((row, i) => {
                      const status = mcpStatuses[i] ?? emptyMcpStatus();
                      const tools = mcpToolsForDisplay(status, row);
                      return (
                        <button
                          key={i}
                          type="button"
                          className={`settings-mcp-pill${selectedMcpIndex === i ? ' settings-mcp-pill--active' : ''}`}
                          onClick={() => setSelectedMcpIndex(i)}
                          title={status.message || '点击查看和配置'}
                        >
                          <span className={statusClassName(status.state)} />
                          <span className="settings-mcp-pill__label">{row.label || `未命名服务 ${i + 1}`}</span>
                          <span className="settings-mcp-pill__meta">
                            {row.transportType || '未设置 transport'}
                            {tools.length ? ` · ${tools.length} tools` : ''}
                          </span>
                        </button>
                      );
                    })
                  )}
                </div>
              </div>
            </div>
          </div>
        )}

        <div className="settings-sheet__actions">
          <button type="button" className="settings-btn settings-btn--secondary" onClick={onClose}>取消</button>
          <button type="button" className="settings-btn settings-btn--primary" onClick={handleSave} disabled={saving}>
            {saving ? '保存中…' : '保存并校验'}
          </button>
        </div>
      </div>

      {activeTab === 'mcp' && selectedMcpRow ? (
        <div className="settings-modal-layer" role="presentation" onMouseDown={(e) => { e.stopPropagation(); setSelectedMcpIndex(null); }}>
          <div className="settings-mcp-editor" role="dialog" aria-label="MCP 服务配置" onMouseDown={(e) => e.stopPropagation()}>
            <div className="settings-mcp-editor__header">
              <div className="settings-mcp-editor__title-wrap">
                <span className={statusClassName(selectedMcpStatus?.state)} title={selectedMcpStatus?.message || '未校验'} />
                <div>
                  <h3 className="settings-mcp-editor__title">{selectedMcpRow.label || `未命名服务 ${selectedMcpIndex + 1}`}</h3>
                  <div className="settings-mcp-editor__subtitle">{selectedMcpStatus?.message || '等待配置完成后自动校验'}</div>
                </div>
              </div>
              <button type="button" className="settings-sheet__close" onClick={() => setSelectedMcpIndex(null)} aria-label="关闭 MCP 配置弹窗">关闭</button>
            </div>

            <div className="settings-mcp-editor__body">
              <div className="settings-mcp-editor__grid">
                <label className="settings-mcp-field">
                  <span className="settings-mcp-field__label">Label</span>
                  <input className="settings-table__input" value={selectedMcpRow.label} onChange={(e) => updateMcpServer(selectedMcpIndex, 'label', e.target.value)} placeholder="chrome-devtools" spellCheck={false} autoComplete="off" />
                </label>
                <label className="settings-mcp-field">
                  <span className="settings-mcp-field__label">Transport</span>
                  <input className="settings-table__input" value={selectedMcpRow.transportType} onChange={(e) => updateMcpServer(selectedMcpIndex, 'transportType', e.target.value)} placeholder="stdio | sse | streamable_http" spellCheck={false} autoComplete="off" />
                </label>
                <label className="settings-mcp-field settings-mcp-field--full">
                  <span className="settings-mcp-field__label">URL</span>
                  <input className="settings-table__input" value={selectedMcpRow.url} onChange={(e) => updateMcpServer(selectedMcpIndex, 'url', e.target.value)} placeholder="http://127.0.0.1:3001" spellCheck={false} autoComplete="off" />
                </label>
                <label className="settings-mcp-field settings-mcp-field--full">
                  <span className="settings-mcp-field__label">Command</span>
                  <input className="settings-table__input" value={selectedMcpRow.command} onChange={(e) => updateMcpServer(selectedMcpIndex, 'command', e.target.value)} placeholder="/abs/path/to/bin 或 npx" spellCheck={false} autoComplete="off" />
                </label>
                <label className="settings-mcp-field">
                  <span className="settings-mcp-field__label">Args</span>
                  <textarea className="settings-table__textarea" value={selectedMcpRow.argsText} onChange={(e) => updateMcpServer(selectedMcpIndex, 'argsText', e.target.value)} placeholder="每行一个参数" spellCheck={false} />
                </label>
                <label className="settings-mcp-field">
                  <span className="settings-mcp-field__label">Allowed Tools</span>
                  <textarea className="settings-table__textarea" value={selectedMcpRow.allowedTools} onChange={(e) => updateMcpServer(selectedMcpIndex, 'allowedTools', e.target.value)} placeholder="每行一个工具名" spellCheck={false} />
                </label>
                <label className="settings-mcp-field">
                  <span className="settings-mcp-field__label">Headers</span>
                  <textarea className="settings-table__textarea" value={selectedMcpRow.headersText} onChange={(e) => updateMcpServer(selectedMcpIndex, 'headersText', e.target.value)} placeholder="Authorization: Bearer xxx" spellCheck={false} />
                </label>
                <label className="settings-mcp-field">
                  <span className="settings-mcp-field__label">Env</span>
                  <textarea className="settings-table__textarea" value={selectedMcpRow.envText} onChange={(e) => updateMcpServer(selectedMcpIndex, 'envText', e.target.value)} placeholder="KEY: value" spellCheck={false} />
                </label>
              </div>

              {selectedMcpTools.length ? (
                <div className="settings-mcp-meta">
                  <div className="settings-mcp-meta__text">已发现 {selectedMcpStatus?.toolCount || selectedMcpTools.length} 个工具</div>
                  <div className="settings-mcp-tools">
                    {selectedMcpTools.map((tool) => (
                      <span key={tool} className="settings-mcp-tool-chip">{tool}</span>
                    ))}
                  </div>
                </div>
              ) : null}
            </div>

            <div className="settings-mcp-editor__actions">
              <button type="button" className="settings-table__validate" onClick={() => validateMcpRow(selectedMcpIndex, selectedMcpRow)} aria-label="校验此服务">测试</button>
              <button type="button" className="settings-table__remove" onClick={() => removeMcpServerRow(selectedMcpIndex)} aria-label="删除此服务">删除</button>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}
