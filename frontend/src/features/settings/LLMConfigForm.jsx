export default function LLMConfigForm({ config, onChange }) {
  const update = (field, value) => onChange({ ...config, [field]: value });

  return (
    <div className="settings-sheet__scroll">
      <div className="settings-llm-form">
        <label className="settings-llm-field settings-llm-field--full">
          <span className="settings-llm-field__label">Chat Completions URL</span>
          <input
            className="settings-table__input"
            value={config.baseUrl}
            onChange={(event) => update('baseUrl', event.target.value)}
            placeholder="https://api.example.com/v1/chat/completions"
            spellCheck={false}
            autoComplete="off"
          />
        </label>

        <label className="settings-llm-field">
          <span className="settings-llm-field__label">Model</span>
          <input
            className="settings-table__input"
            value={config.model}
            onChange={(event) => update('model', event.target.value)}
            placeholder="model-name"
            spellCheck={false}
            autoComplete="off"
          />
        </label>

        <label className="settings-llm-field">
          <span className="settings-llm-field__label">API Key</span>
          <input
            className="settings-table__input"
            type="password"
            value={config.apiKey}
            onChange={(event) => update('apiKey', event.target.value)}
            placeholder="可留空"
            spellCheck={false}
            autoComplete="off"
          />
        </label>

        <label className="settings-llm-field">
          <span className="settings-llm-field__label">Max output tokens</span>
          <input
            className="settings-table__input settings-table__input--num"
            type="number"
            min={0}
            value={config.maxOutputTokens || ''}
            onChange={(event) => {
              const value = event.target.value;
              update('maxOutputTokens', value === '' ? 0 : Number.parseInt(value, 10) || 0);
            }}
            placeholder="8192"
          />
        </label>
      </div>
    </div>
  );
}
