import { useState, useEffect } from 'react';

import './App.css';

import ConversationList from './componentjs/ConversationList.jsx';
import Dialog from './componentjs/Dialog.jsx';
import Header from './componentjs/Header.jsx';
import Reasoning from './componentjs/Reasonging.jsx';

function App() {
    // 状态管理
    const [conversations, setConversations] = useState([
        { id: 1, text: '工作讨论工作讨论工作讨论工作讨论工作讨论工作讨论工作讨论' },
        { id: 2, text: '项目计划' },
        { id: 3, text: '日常交流' },
        { id: 4, text: '工作讨论' },
        { id: 5, text: '项目计划' },
        { id: 6, text: '日常交流' },
        { id: 7, text: '工作讨论' },
        { id: 8, text: '项目计划' },
        { id: 9, text: '日常交流' },
        { id: 10, text: '工作讨论' },
        { id: 11, text: '项目计划' },
        { id: 12, text: '日常交流' },
        { id: 13, text: '工作讨论' },
        { id: 14, text: '项目计划' },
        { id: 15, text: '日常交流' },
    ]);

    const [reasonings, setReasonings] = useState([
        { id: 1, text: '工作讨论' },
        { id: 2, text: '项目计划' },
        { id: 3, text: '日常交流' },
        { id: 4, text: '工作讨论' },
        { id: 5, text: '项目计划' },
        { id: 6, text: '日常交流' },
        { id: 7, text: '工作讨论' },
        { id: 8, text: '项目计划' },
        { id: 9, text: '日常交流' },
        { id: 10, text: '工作讨论' },
        { id: 11, text: '项目计划' },
        { id: 12, text: '日常交流' },
        { id: 13, text: '工作讨论' },
        { id: 14, text: '项目计划' },
        { id: 15, text: '日常交流' },
    ]);


    const [currentChatId, setCurrentChatId] = useState(null);
    const [messages, setMessages] = useState([
        { id: 1, role: "user", content: '你好，今天天气不错！' },
        { id: 2, role: "assistant", content: '是的，阳光明媚。' },
        { id: 3, role: "user", content: '那我们去公园走走吧！' },
        { id: 4, role: "user", content: '你好，今天天气不错！' },
        { id: 5, role: "assistant", content: '是的，阳光明媚。' },
        { id: 6, role: "user", content: '那我们去公园走走吧！' },
        { id: 7, role: "user", content: '你好，今天天气不错！' },
        { id: 8, role: "assistant", content: '是的，阳光明媚。' },
        { id: 9, role: "user", content: '那我们去公园走走吧！' },
        { id: 10, role: "user", content: '你好，今天天气不错！' },
        { id: 11, role: "assistant", content: '是的，阳光明媚。' },
        { id: 12, role: "user", content: '那我们去公园走走吧！' },

    ]);

    return (
        <div id="App" className='board-column'>
            <Header
                isConnected={true}
                showReasoningPanel={true}
            />
            <div className="main-content">
                <ConversationList
                    conversations={conversations}
                    onSelectChat={setCurrentChatId}
                />
                <Dialog
                    chatId={currentChatId}
                    messages={messages}
                />
                <Reasoning
                    chatId={currentChatId}
                    reasongingMessages={reasonings}
                />


            </div>
        </div>
    )
}

export default App
