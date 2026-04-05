import { useCallback, useEffect, useState } from 'react';
import { GetLLMConfigEditorState, SaveLLMConfigText } from '../../wailsjs/go/main/App';
import '../componentcss/SettingsModal.css';

export default function SettingsModal({ open, onClose, onSaved }) {
  const [text, setText] = useState('');
  const [savePath, setSavePath] = useState('');
  const [usingExample, setUsingExample] = useState(false);
  const [loadErr, setLoadErr] = useState('');
  const [saveErr, setSaveErr] = useState('');
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    setLoadErr('');
    try {
      const state = await GetLLMConfigEditorState();
      setText(state.content ?? '');
      setSavePath(state.path ?? '');
      setUsingExample(!!state.usingExample);
    } catch (e) {
      setLoadErr(String(e?.message || e));
    }
  }, []);

  useEffect(() => {
    if (open) {
      load();
    }
  }, [open, load]);

  const handleSave = async () => {
    setSaveErr('');
    setSaving(true);
    try {
      await SaveLLMConfigText(text);
      setUsingExample(false);
      onSaved?.();
      onClose();
    } catch (e) {
      setSaveErr(String(e?.message || e));
    } finally {
      setSaving(false);
    }
  };

  if (!open) return null;

  return (
    <div className="settings-overlay" role="presentation" onMouseDown={onClose}>
      <div
        className="settings-sheet"
        role="dialog"
        aria-labelledby="settings-title"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="settings-sheet__header">
          <h2 id="settings-title" className="settings-sheet__title">
            模型与连接
          </h2>
          <button type="button" className="settings-sheet__close" onClick={onClose} aria-label="关闭">
            完成
          </button>
        </div>

        <p className="settings-sheet__path">
          {savePath ? (
            <>
              <span className="settings-sheet__path-label">保存路径</span>
              <code className="settings-sheet__path-value">{savePath}</code>
            </>
          ) : null}
          {usingExample ? (
            <span className="settings-sheet__hint">尚未有配置文件，保存后将创建 config.yaml</span>
          ) : null}
        </p>

        {loadErr ? <div className="settings-sheet__error">{loadErr}</div> : null}
        {saveErr ? <div className="settings-sheet__error">{saveErr}</div> : null}

        <textarea
          className="settings-sheet__editor"
          value={text}
          onChange={(e) => setText(e.target.value)}
          spellCheck={false}
          autoComplete="off"
          autoCorrect="off"
        />

        <div className="settings-sheet__actions">
          <button type="button" className="settings-btn settings-btn--secondary" onClick={onClose}>
            取消
          </button>
          <button
            type="button"
            className="settings-btn settings-btn--primary"
            onClick={handleSave}
            disabled={saving}
          >
            {saving ? '保存中…' : '保存并校验'}
          </button>
        </div>
      </div>
    </div>
  );
}
