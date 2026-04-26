import { useState, useCallback, useRef } from 'react';

// 流式响应控制Hook
export const useStreaming = () => {
  const [taskBusy, setTaskBusy] = useState(false);
  const [stopVisible, setStopVisible] = useState(false);
  const [streamingContent, setStreamingContent] = useState('');
  
  const streamingRef = useRef({
    isActive: false,
    abortController: null
  });

  // 开始流式响应
  const startStreaming = useCallback(() => {
    setTaskBusy(true);
    setStopVisible(true);
    setStreamingContent('');
    streamingRef.current.isActive = true;
    streamingRef.current.abortController = new AbortController();
  }, []);

  // 停止流式响应
  const stopStreaming = useCallback(() => {
    if (streamingRef.current.abortController) {
      streamingRef.current.abortController.abort();
    }
    setTaskBusy(false);
    setStopVisible(false);
    setStreamingContent('');
    streamingRef.current.isActive = false;
  }, []);

  // 添加流式内容
  const appendStreamContent = useCallback((content) => {
    if (streamingRef.current.isActive) {
      setStreamingContent(prev => prev + content);
    }
  }, []);

  // 添加任务到队列（主要用于批量处理）
  const addToQueue = useCallback((task) => {
    // 简单实现：如果有任务在执行，将新任务加入队列等待
    // 实际项目中可能需更复杂的队列管理
    if (!taskBusy) {
      task();
    } else {
      // 延迟执行
      setTimeout(() => addToQueue(task), 100);
    }
  }, [taskBusy]);

  // 处理队列任务
  const processQueue = useCallback(() => {
    // 队列处理逻辑
    setTaskBusy((current) => {
      if (!current) {
        // 处理队列中的任务
        return false; // 队列处理完成
      }
      return current;
    });
  }, []);

  // 检查是否需要流式响应
  const needsStreamingResponse = useCallback((messageType) => {
    return messageType === 'message' && !taskBusy;
  }, [taskBusy]);

  return {
    // 状态
    taskBusy,
    stopVisible,
    streamingContent,
    
    // 控制方法
    startStreaming,
    stopStreaming,
    appendStreamContent,
    addToQueue,
    processQueue,
    needsStreamingResponse
  };
};