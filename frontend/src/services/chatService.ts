import { apiCall, SendMessage, StopChat, SwitchChat, SendUserDisplayOnly } from './api';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';
import type { Message } from '../types/chat';

export class ChatService {
  
  /**
   * 发送消息到后端
   */
  static async sendMessage(
    message: Omit<Message, 'messageId'>,
    opts: { chatId: string; aite?: string[] } = { chatId: '' }
  ): Promise<any> {
    const chatId = String(opts?.chatId ?? '').trim();
    return apiCall(
      () => SendMessage(chatId, JSON.stringify({ content: message.content, aite: Array.isArray(opts?.aite) ? opts.aite : [] }), 'user'),
      '发送消息失败'
    );
  }

  /**
   * 停止当前聊天生成
   */
  static async stopChat(chatId: string): Promise<any> {
    const cid = String(chatId ?? '').trim();
    return apiCall(
      () => StopChat(cid),
      '停止聊天失败'
    );
  }

  /**
   * 切换到指定聊天
   */
  static async switchChat(chatId: string): Promise<any> {
    return apiCall(
      () => SwitchChat(chatId),
      '切换聊天失败'
    );
  }

  /**
   * 发送用户显示信息（不触发回复）
   */
  static async sendUserDisplayOnly(content: string): Promise<any> {
    return apiCall(
      () => SendUserDisplayOnly('', content),
      '发送用户显示信息失败'
    );
  }

  /**
   * 处理流式响应事件监听
   */
  static setupEventListeners(onMessageReceived: (message: Message) => void) {
    const onAppend = (data: any) => {
      const message: Message = {
        role: String(data?.role ?? 'assistant'),
        content: String(data?.content ?? ''),
        timestamp: data?.timestamp ?? Date.now(),
        agentID: data?.agentID ?? data?.agent_id,
        messageId: data?.messageID ?? data?.messageId ?? data?.id
      };
      onMessageReceived(message);
    };

    const onStreamEnd = () => {
      // 交给上层决定如何收尾（这里保持兼容，至少不报错）
    };

    const onError = (error: any) => {
      console.error('聊天错误:', error);
    };

    EventsOn('dialogAppend', onAppend);
    EventsOn('dialogStreamEnd', onStreamEnd);
    EventsOn('sendMessageError', onError);
    EventsOn('dispatcherError', onError);

    // 监听错误事件
    return () => {
      EventsOff('dialogAppend');
      EventsOff('dialogStreamEnd');
      EventsOff('sendMessageError');
      EventsOff('dispatcherError');
    };
  }

  /**
   * 消息预处理和验证
   */
  static preprocessMessage(content: string): string {
    return content.trim();
  }

  /**
   * 消息内容安全检查
   */
  static validateMessage(content: string): boolean {
    if (!content || content.trim().length === 0) {
      return false;
    }
    
    // 检查长度限制（可根据需求调整）
    if (content.length > 10000) {
      console.warn('消息过长，已截断');
      return false;
    }
    
    return true;
  }
}