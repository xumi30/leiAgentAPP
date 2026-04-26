// 基础API服务封装
import {
    AddAgentToConversation,
    AddConversation,
    GetMessages,
    GetConversationAgents,
    ListAgents,
    AppendMemoMarkdown,
    ComposeMemoWithLLM,
    GetMemoReferencedMessageIDs,
    SendMessage,
    SendUserDisplayOnly,
    SwitchChat,
    StopChat,
} from '../../wailsjs/go/main/App';

// API响应类型定义
export interface APIResponse<T = any> {
    success: boolean;
    data?: T;
    error?: string;
}

// 通用API调用封装
export const apiCall = async <T>(
    apiFunc: () => Promise<T>,
    errorMessage?: string
): Promise<APIResponse<T>> => {
    try {
        const data = await apiFunc();
        return { success: true, data };
    } catch (error) {
        console.error(errorMessage || 'API调用失败:', error);
        return { 
            success: false, 
            error: error instanceof Error ? error.message : '未知错误'
        };
    }
};

// 导出现有API函数
export {
    AddAgentToConversation,
    AddConversation,
    GetMessages,
    GetConversationAgents,
    ListAgents,
    AppendMemoMarkdown,
    ComposeMemoWithLLM,
    GetMemoReferencedMessageIDs,
    SendMessage,
    SendUserDisplayOnly,
    SwitchChat,
    StopChat,
};