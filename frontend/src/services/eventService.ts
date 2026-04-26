import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';
import type { Message } from '../types/chat';

export class EventService {
  
  private static listeners: Map<string, Function[]> = new Map();

  /**
   * 监听新消息事件
   */
  static onNewMessage(callback: (message: Message) => void): () => void {
    return this.addListener('newMessage', callback);
  }

  /**
   * 监听聊天错误事件
   */
  static onChatError(callback: (error: any) => void): () => void {
    return this.addListener('chatError', callback);
  }

  /**
   * 监听流式响应事件
   */
  static onStreamingStart(callback: () => void): () => void {
    return this.addListener('streamingStart', callback);
  }

  /**
   * 监听流式响应结束事件
   */
  static onStreamingEnd(callback: () => void): () => void {
    return this.addListener('streamingEnd', callback);
  }

  /**
   * 监听连接状态变化
   */
  static onConnectionChange(callback: (connected: boolean) => void): () => void {
    return this.addListener('connectionChange', callback);
  }

  /**
   * 添加事件监听器
   */
  private static addListener(event: string, callback: Function): () => void {
    if (!this.listeners.has(event)) {
      this.listeners.set(event, []);
    }
    
    this.listeners.get(event)!.push(callback);
    
    // 如果是首次监听Wails原生事件，设置相应的处理
    if (event === 'newMessage' || event === 'chatError') {
      EventsOn(event, (data: any) => {
        this.notifyListeners(event, data);
      });
    }
    
    // 返回取消监听函数
    return () => {
      this.removeListener(event, callback);
    };
  }

  /**
   * 移除事件监听器
   */
  private static removeListener(event: string, callback: Function): void {
    const eventListeners = this.listeners.get(event);
    if (eventListeners) {
      const index = eventListeners.indexOf(callback);
      if (index > -1) {
        eventListeners.splice(index, 1);
      }
      
      // 如果没有监听器了，移除Wails事件监听
      if (eventListeners.length === 0) {
        EventsOff(event);
        this.listeners.delete(event);
      }
    }
  }

  /**
   * 通知所有监听器
   */
  private static notifyListeners(event: string, data: any): void {
    const eventListeners = this.listeners.get(event);
    if (eventListeners) {
      eventListeners.forEach(callback => {
        try {
          callback(data);
        } catch (error) {
          console.error(`Event listener error for ${event}:`, error);
        }
      });
    }
  }

  /**
   * 触发自定义事件
   */
  static emit(event: string, data?: any): void {
    this.notifyListeners(event, data);
  }

  /**
   * 清理所有事件监听
   */
  static cleanup(): void {
    for (const [event] of this.listeners) {
      EventsOff(event);
    }
    this.listeners.clear();
  }
}