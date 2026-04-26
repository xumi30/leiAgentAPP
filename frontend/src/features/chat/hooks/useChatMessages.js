import { useState, useCallback } from 'react';
import { useChatStore } from '../../../stores';

// Chat消息状态管理Hook
export const useChatMessages = () => {
  const { 
    messages, 
    setMessages, 
    addMessage,
    updateMessage,
    deleteMessage,
    clearMessages 
  } = useChatStore();

  // 添加用户消息
  const addUserMessage = useCallback((content) => {
    const userMessage = {
      role: 'user',
      content: content.trim(),
      timestamp: Date.now()
    };
    addMessage(userMessage);
    return userMessage;
  }, [addMessage]);

  // 添加助手消息
  const addAssistantMessage = useCallback((content, options = {}) => {
    const assistantMessage = {
      role: 'assistant',
      content: content.trim(),
      timestamp: Date.now(),
      ...options
    };
    addMessage(assistantMessage);
    return assistantMessage;
  }, [addMessage]);

  // 更新最后一条助手消息（用于流式输出）
  const updateLastAssistantMessage = useCallback((content) => {
    const lastMessage = messages[messages.length - 1];
    if (lastMessage && lastMessage.role === 'assistant') {
      updateMessage(messages.length - 1, {
        ...lastMessage,
        content: content,
        isStreaming: true
      });
    }
  }, [messages, updateMessage]);

  return {
    messages,
    addUserMessage,
    addAssistantMessage,
    updateLastAssistantMessage,
    updateMessage,
    deleteMessage,
    clearMessages
  };
};