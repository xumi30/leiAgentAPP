import { useState, useCallback, useRef } from 'react';

// 流式响应控制 Hook
export const useStreaming = () => {
  const [stopVisible, setStopVisible] = useState(false);
  const [taskBusy, setTaskBusy] = useState(false);
  const [streamPulse, setStreamPulse] = useState<string | null>(null);
  const [queuedInputs, setQueuedInputs] = useState<string[]>([]);
  
  const streamRef = useRef<{ interval: NodeJS.Timeout | null }>({ interval: null });

  // 开始流式响应
  const startStreaming = useCallback(() => {
    setStopVisible(true);
    setTaskBusy(true);
    
    // 创建流式动画效果
    if (streamRef.current.interval) {
      clearInterval(streamRef.current.interval);
    }
    
    streamRef.current.interval = setInterval(() => {
      setStreamPulse(Date.now().toString());
    }, 500);
  }, []);

  // 停止流式响应
  const stopStreaming = useCallback(() => {
    setStopVisible(false);
    setTaskBusy(false);
    
    if (streamRef.current.interval) {
      clearInterval(streamRef.current.interval);
      streamRef.current.interval = null;
    }
    
    setStreamPulse(null);
  }, []);

  // 添加输入到队列
  const addToQueue = useCallback((input: string) => {
    if (!input.trim()) return;
    
    setQueuedInputs(prev => [...prev, input]);
  }, []);

  // 处理队列中的输入
  const processQueue = useCallback((processor: (input: string) => void) => {
    if (queuedInputs.length === 0) return;
    
    const nextInput = queuedInputs[0];
    setQueuedInputs(prev => prev.slice(1));
    processor(nextInput);
  }, [queuedInputs]);

  // 清空队列
  const clearQueue = useCallback(() => {
    setQueuedInputs([]);
  }, []);

  return {
    // 状态
    stopVisible,
    taskBusy,
    streamPulse,
    queuedInputs,
    
    // 操作方法
    startStreaming,
    stopStreaming,
    addToQueue,
    processQueue,
    clearQueue
  };
};