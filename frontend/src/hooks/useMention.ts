import { useState, useCallback } from 'react';

// 提及功能 Hook
export const useMention = () => {
  const [mentionTarget, setMentionTarget] = useState<string | null>(null);
  const [mentionPosition, setMentionPosition] = useState<{ x: number; y: number } | null>(null);
  const [isMentioning, setIsMentioning] = useState(false);

  // 开始提及模式
  const startMentioning = useCallback((target: string, position: { x: number; y: number }) => {
    setMentionTarget(target);
    setMentionPosition(position);
    setIsMentioning(true);
  }, []);

  // 结束提及模式
  const stopMentioning = useCallback(() => {
    setMentionTarget(null);
    setMentionPosition(null);
    setIsMentioning(false);
  }, []);

  // 插入提及到文本
  const insertMention = useCallback((inputValue: string, start: number, end: number, agentName: string) => {
    const mentionText = `@${agentName} `;
    
    // 在光标位置插入提及
    const before = inputValue.substring(0, start);
    const after = inputValue.substring(end);
    
    return before + mentionText + after;
  }, []);

  // 检测提及输入
  const detectMention = useCallback((text: string, cursorPosition: number) => {
    // 从光标位置向左查找，检测是否以@开头
    let start = cursorPosition - 1;
    while (start >= 0 && text[start] !== ' ' && text[start] !== '\n') {
      if (text[start] === '@') {
        return {
          isMention: true,
          start,
          end: cursorPosition,
          query: text.substring(start + 1, cursorPosition)
        };
      }
      start--;
    }
    
    return { isMention: false };
  }, []);

  return {
    // 状态
    mentionTarget,
    mentionPosition,
    isMentioning,
    
    // 操作方法
    startMentioning,
    stopMentioning,
    insertMention,
    detectMention
  };
};