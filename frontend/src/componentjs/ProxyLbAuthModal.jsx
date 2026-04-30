import { useCallback, useState } from 'react';
import { ClearProxyLbAuth, ProxyAuthRequest } from '../../wailsjs/go/main/App';

export function hasProxyLbSessionFromBackends(backends) {
  if (!Array.isArray(backends)) return false;
  return backends.some((row) => {
    if (row?.enabled === false) return false;
    if (String(row?.name ?? '').toLowerCase() !== 'proxy-lb') return false;
    return String(row?.apiKey ?? '').trim() !== '';
  });
}

function parseAPIError(payload, fallback) {
  if (payload && typeof payload === 'object') {
    const message = payload.message
      || (payload.error && typeof payload.error === 'object' ? payload.error.message : null)
      || (typeof payload.error === 'string' ? payload.error : null)
      || payload.detail;
    if (typeof message === 'string' && message.trim()) return message.trim();
  }
  return fallback;
}

/**
 * Proxy-LB 登录 / 注册 / 登出，供设置页 LLM 使用。
 */
export default function ProxyLbAuthModal({
  open,
  onClose,
  hasProxySession,
  onCompleted,
  onAuthOutcome,
}) {
  const [mode, setMode] = useState('login');
  const [busy, setBusy] = useState(false);
  const [logoutBusy, setLogoutBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [loginUsername, setLoginUsername] = useState('');
  const [loginPassword, setLoginPassword] = useState('');
  const [registerUsername, setRegisterUsername] = useState('');
  const [registerEmail, setRegisterEmail] = useState('');
  const [registerPassword, setRegisterPassword] = useState('');
  const [registerCode, setRegisterCode] = useState('');

  const resetErrors = useCallback(() => {
    setError('');
    setNotice('');
  }, []);

  const handlePerformAuth = useCallback(async (path, payload, successText) => {
    setBusy(true);
    resetErrors();
    try {
      const data = await ProxyAuthRequest(path, payload);
      const statusCode = Number(data?._statusCode ?? 0);
      if (statusCode < 200 || statusCode >= 300) {
        throw new Error(parseAPIError(data, `请求失败（HTTP ${statusCode}）`));
      }
      setNotice(successText);
      onAuthOutcome?.({ success: true });
      onCompleted?.();
      onClose?.();
    } catch (e) {
      setError(String(e?.message || e || '请求失败'));
      onAuthOutcome?.({ success: false });
    } finally {
      setBusy(false);
    }
  }, [onClose, onCompleted, onAuthOutcome, resetErrors]);

  const handleSendRegisterCode = useCallback(async () => {
    setBusy(true);
    resetErrors();
    try {
      const data = await ProxyAuthRequest('/auth/register/send-code', {
        username: String(registerUsername ?? '').trim(),
        email: String(registerEmail ?? '').trim(),
      });
      const statusCode = Number(data?._statusCode ?? 0);
      if (statusCode < 200 || statusCode >= 300) {
        throw new Error(parseAPIError(data, `发送验证码失败（HTTP ${statusCode}）`));
      }
      setNotice(parseAPIError(data, '验证码已发送，请查收邮箱。'));
    } catch (e) {
      setError(String(e?.message || e || '发送验证码失败'));
      onAuthOutcome?.({ success: false });
    } finally {
      setBusy(false);
    }
  }, [registerUsername, registerEmail, resetErrors, onAuthOutcome]);

  const handleLoginSubmit = useCallback(async () => {
    await handlePerformAuth('/auth/login', {
      username: String(loginUsername ?? '').trim(),
      password: String(loginPassword ?? ''),
    }, '登录成功，已写入配置。');
  }, [handlePerformAuth, loginUsername, loginPassword]);

  const handleRegisterSubmit = useCallback(async () => {
    await handlePerformAuth('/auth/register', {
      username: String(registerUsername ?? '').trim(),
      password: String(registerPassword ?? ''),
      email: String(registerEmail ?? '').trim(),
      code: String(registerCode ?? '').trim(),
    }, '注册成功，已写入配置。');
  }, [handlePerformAuth, registerUsername, registerPassword, registerEmail, registerCode]);

  const handleLogout = useCallback(async () => {
    setLogoutBusy(true);
    setError('');
    try {
      await ClearProxyLbAuth();
      onCompleted?.();
      onClose?.();
    } catch (e) {
      setError(String(e?.message || e || '登出失败'));
    } finally {
      setLogoutBusy(false);
    }
  }, [onClose, onCompleted]);

  if (!open) return null;

  return (
    <div
      className="auth-modal-overlay"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose?.();
      }}
    >
      <div
        className="auth-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="proxy-lb-auth-title"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <div className="auth-modal__head">
          <div>
            <p id="proxy-lb-auth-title" className="auth-modal__title">Proxy-LB</p>
            <p className="auth-modal__desc">雷哥搜集的免费token进行了负载均衡,很不稳定.</p><p className="auth-modal__desc"> 很多不支持工具执行!!慎用。最好自己配置上面的LLM</p>
          </div>
          <button type="button" className="auth-modal__secondary-btn auth-modal__close" onClick={onClose} aria-label="关闭">×</button>
        </div>

        {hasProxySession ? (
          <div className="auth-modal__form" style={{ paddingBottom: 8 }}>
            <p className="auth-modal__hint" style={{ margin: '0 0 12px', fontSize: 12, color: '#64748b' }}>
              已保存令牌；可与下方 LLM 表中共存。
            </p>
            <button
              type="button"
              className="auth-modal__secondary-btn"
              style={{ width: '100%' }}
              disabled={logoutBusy || busy}
              onClick={() => void handleLogout()}
            >
              {logoutBusy ? '处理中…' : '清除本机 Proxy-LB 令牌'}
            </button>
          </div>
        ) : null}

        <div className="auth-modal__switch">
          <button type="button" className={`auth-modal__switch-btn${mode === 'login' ? ' auth-modal__switch-btn--active' : ''}`} onClick={() => { setMode('login'); resetErrors(); }}>
            登录
          </button>
          <button type="button" className={`auth-modal__switch-btn${mode === 'register' ? ' auth-modal__switch-btn--active' : ''}`} onClick={() => { setMode('register'); resetErrors(); }}>
            注册
          </button>
        </div>

        {mode === 'login' ? (
          <div className="auth-modal__form">
            <label className="auth-modal__field">
              <span className="auth-modal__label">用户名</span>
              <input className="auth-modal__input" value={loginUsername} onChange={(e) => setLoginUsername(e.target.value)} autoComplete="username" />
            </label>
            <label className="auth-modal__field">
              <span className="auth-modal__label">密码</span>
              <input className="auth-modal__input" type="password" value={loginPassword} onChange={(e) => setLoginPassword(e.target.value)} autoComplete="current-password" />
              <span className="auth-modal__hint">至少 8 个字符</span>
            </label>
          </div>
        ) : (
          <div className="auth-modal__form">
            <label className="auth-modal__field">
              <span className="auth-modal__label">用户名</span>
              <input className="auth-modal__input" value={registerUsername} onChange={(e) => setRegisterUsername(e.target.value)} autoComplete="username" />
            </label>
            <label className="auth-modal__field">
              <span className="auth-modal__label">邮箱</span>
              <input className="auth-modal__input" type="email" value={registerEmail} onChange={(e) => setRegisterEmail(e.target.value)} autoComplete="email" />
            </label>
            <label className="auth-modal__field">
              <span className="auth-modal__label">密码</span>
              <input className="auth-modal__input" type="password" value={registerPassword} onChange={(e) => setRegisterPassword(e.target.value)} autoComplete="new-password" />
              <span className="auth-modal__hint">至少 8 个字符</span>
            </label>
            <div className="auth-modal__grid auth-modal__grid--code">
              <label className="auth-modal__field">
                <span className="auth-modal__label">验证码</span>
                <input className="auth-modal__input" value={registerCode} onChange={(e) => setRegisterCode(e.target.value)} autoComplete="one-time-code" />
              </label>
              <button type="button" className="auth-modal__ghost-btn" onClick={() => void handleSendRegisterCode()} disabled={busy}>
                发送验证码
              </button>
            </div>
          </div>
        )}

        {error ? <div className="auth-modal__error">{error}</div> : null}
        {notice ? <div className="auth-modal__notice">{notice}</div> : null}

        <div className="auth-modal__actions">
          <button type="button" className="auth-modal__secondary-btn" onClick={onClose}>取消</button>
          <button
            type="button"
            className="auth-modal__primary-btn"
            onClick={mode === 'login' ? () => void handleLoginSubmit() : () => void handleRegisterSubmit()}
            disabled={busy || logoutBusy || (mode === 'login' ? !loginUsername.trim() || !loginPassword.trim() : !registerUsername.trim() || !registerEmail.trim() || !registerPassword.trim() || !registerCode.trim())}
          >
            {busy ? '处理中…' : mode === 'login' ? '登录' : '注册'}
          </button>
        </div>
      </div>
    </div>
  );
}
