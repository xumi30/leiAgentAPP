/**
 * Format a timestamp into Beijing time (Asia/Shanghai) using 24-hour clock.
 * Accepts: number(ms) | string(ISO/RFC3339) | Date | null/undefined
 *
 * Output: YYYY-MM-DD HH:mm:ss
 */
export function formatBeijingTimestamp(input) {
  if (input == null || input === '') return '';

  let d;
  if (input instanceof Date) {
    d = input;
  } else if (typeof input === 'number') {
    d = new Date(input);
  } else if (typeof input === 'string') {
    const s = input.trim();
    if (!s) return '';
    // If it's numeric-like, treat as ms since epoch.
    if (/^\d{10,17}$/.test(s)) {
      const n = Number(s);
      d = new Date(n);
    } else {
      d = new Date(s);
    }
  } else {
    d = new Date(String(input));
  }

  if (!d || Number.isNaN(d.getTime())) return '';

  // Intl handles timezone conversion reliably across platforms.
  const parts = new Intl.DateTimeFormat('zh-CN', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).formatToParts(d);

  const pick = (type) => parts.find((p) => p.type === type)?.value || '';
  const yyyy = pick('year');
  const mm = pick('month');
  const dd = pick('day');
  const hh = pick('hour');
  const mi = pick('minute');
  const ss = pick('second');

  if (!yyyy || !mm || !dd || !hh || !mi || !ss) return '';
  return `${yyyy}-${mm}-${dd} ${hh}:${mi}:${ss}`;
}

