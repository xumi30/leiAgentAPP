import { useMemo, useState, useEffect } from 'react';

const WEEK_LABELS = ['日', '一', '二', '三', '四', '五', '六'];

function pad2(n) {
  return String(n).padStart(2, '0');
}

/** @param {Date} d */
function toYMD(d) {
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
}

/** @returns {{ year: number, month: number }} month 1–12 */
function startOfMonth(year, month) {
  return new Date(year, month - 1, 1);
}

function daysInMonth(year, month) {
  return new Date(year, month, 0).getDate();
}

/**
 * @param {{ memoDates?: Set<string>, selectedDate: string | null, onSelectDate: (ymd: string | null) => void, onVisibleMonthChange?: () => void }} props
 */
export default function ConversationCalendar({ memoDates = new Set(), selectedDate, onSelectDate, onVisibleMonthChange }) {
  const now = new Date();
  const [cursor, setCursor] = useState(() => ({ y: now.getFullYear(), m: now.getMonth() + 1 }));

  useEffect(() => {
    if (typeof onVisibleMonthChange === 'function') {
      onVisibleMonthChange(cursor.y, cursor.m);
    }
  }, [cursor.y, cursor.m, onVisibleMonthChange]);

  const todayYMD = useMemo(() => toYMD(now), []);

  const grid = useMemo(() => {
    const { y, m } = cursor;
    const first = startOfMonth(y, m);
    const total = daysInMonth(y, m);
    const lead = first.getDay();
    const cells = [];
    for (let i = 0; i < lead; i++) {
      cells.push({ key: `e-${i}`, day: null });
    }
    for (let d = 1; d <= total; d++) {
      const ymd = `${y}-${pad2(m)}-${pad2(d)}`;
      cells.push({ key: ymd, day: d, ymd });
    }
    while (cells.length % 7 !== 0) {
      cells.push({ key: `t-${cells.length}`, day: null });
    }
    return cells;
  }, [cursor]);

  const shiftMonth = (delta) => {
    setCursor((prev) => {
      let { y, m } = prev;
      m += delta;
      if (m > 12) {
        m = 1;
        y += 1;
      } else if (m < 1) {
        m = 12;
        y -= 1;
      }
      return { y, m };
    });
  };

  const title = `${cursor.y} 年 ${cursor.m} 月`;

  return (
    <div className="conv-cal" aria-label="按日期筛选对话">
      <div className="conv-cal__head">
        <button type="button" className="conv-cal__nav" onClick={() => shiftMonth(-1)} aria-label="上月">
          ‹
        </button>
        <span className="conv-cal__title">{title}</span>
        <button type="button" className="conv-cal__nav" onClick={() => shiftMonth(1)} aria-label="下月">
          ›
        </button>
      </div>
      <div className="conv-cal__weekrow">
        {WEEK_LABELS.map((w) => (
          <span key={w} className="conv-cal__weekday">
            {w}
          </span>
        ))}
      </div>
      <div className="conv-cal__grid">
        {grid.map((cell) => {
          if (cell.day == null) {
            return <div key={cell.key} className="conv-cal__cell conv-cal__cell--empty" />;
          }
          const hasMemo = memoDates.has(cell.ymd);
          const isToday = cell.ymd === todayYMD;
          const isSel = selectedDate === cell.ymd;
          return (
            <button
              key={cell.key}
              type="button"
              className={`conv-cal__cell${isToday ? ' is-today' : ''}${isSel ? ' is-selected' : ''}`}
              onClick={() => {
                if (selectedDate === cell.ymd) {
                  onSelectDate(null);
                } else {
                  onSelectDate(cell.ymd);
                }
              }}
            >
              <span className="conv-cal__daynum">{cell.day}</span>
              {hasMemo ? (
                <span className="conv-cal__memo-dot" title="当天备忘录中有记录">
                  📝
                </span>
              ) : null}
            </button>
          );
        })}
      </div>
      <div className="conv-cal__footer">
        <button type="button" className="conv-cal__clear" onClick={() => onSelectDate(null)}>
          显示全部对话
        </button>
        {selectedDate ? (
          <span className="conv-cal__filter-hint">
            已选 <strong>{selectedDate}</strong>
          </span>
        ) : null}
      </div>
    </div>
  );
}

export { toYMD };
