
import { getRandomMacaronColor } from './Constant';
import React, { useState, useEffect, useRef } from 'react';
import { EventsOff, EventsOn } from '../../wailsjs/runtime/runtime';
import { GetMessages, SendMessage,StopChat } from '../../wailsjs/go/main/App';

export default function Dialog() {
    // 获取对话列表
    const [chatId, setChatId] = useState('');
    const [messages, setMessages] = useState([]);
    const messagesRef = useRef(null);

    useEffect(() => {
        const handleConversationChange = (event) => {
            const { conversationId } = event.detail;
            setChatId(conversationId);
            const getMessages = async () => {
                const messages = await GetMessages(conversationId);
                setMessages(messages);
                console.log("1收到消息更新事件:", messages);
            }
            getMessages();
        };

        window.addEventListener('conversationChanged', handleConversationChange);

        return () => {
            window.removeEventListener('conversationChanged', handleConversationChange);
        };
    }, []);

    // 新增：监听 messages 变化，自动滚动到底部
    useEffect(() => {
        const container = messagesRef.current;
        if (!container) return;

        // 2. requestAnimationFrame 确保 DOM 渲染完成再滚动
        const scrollId = requestAnimationFrame(() => {
            container.scrollTop = container.scrollHeight;
        });

        // 3. 清理：防止组件卸载后报错
        return () => cancelAnimationFrame(scrollId);
    }, [messages]); // 消息变化就触发

    useEffect(() => {
        const handleMessage = (message) => {
            console.log("收到消息更新事件:", message);
            setMessages(prevMessages => {
                // 如果消息列表为空或不存在，直接返回包含新消息的数组
                if (!prevMessages?.length) {
                    return [message];
                }

                // 检查是否已存在该消息ID
                const messageExists = prevMessages.some(msg => msg.messageID === message.messageID);

                // 如果消息已存在，返回原数组；否则添加新消息
                return messageExists ? prevMessages : [...prevMessages, message];
            });

            setChatId(message.chatID); // 更新当前对话ID
        }

        const appendMessage = (message) => {
            console.log("收到消息更新事件:", message);
            setMessages((prevMessages) => {
                if (!prevMessages || prevMessages.length === 0) {
                    return [message];
                }

                // 检查是否已存在该消息ID
                const messageExists = prevMessages.some(msg => msg.messageID === message.messageID);

                if (messageExists) {
                    // 使用 map 创建新数组，保持不可变性
                    return prevMessages.map((msg) => {
                        if (msg.messageID === message.messageID) {
                            // 创建新对象，保持不可变性
                            return {
                                ...msg,
                                content: msg.content + message.content
                            };
                        }
                        return msg;
                    });
                } else {
                    // 如果是新消息，添加到列表中
                    return [...prevMessages, message];
                }
            });

            setChatId(message.chatID); // 更新当前对话ID
        }

        const handleSenderror = (error) => {
            alert("发送消息失败: " + error);
            console.log("发送消息失败: ", error);
        }

        EventsOn("dialogAppend", appendMessage); // 监听对话追加事件
        EventsOn("GetMessagesByMessageID", handleMessage); // 监听消息更新事件
        EventsOn("sendMessageError", handleSenderror); // 监听发送错误事件

        return () => {
            EventsOff("dialogAppend", handleMessage); // 组件卸载时取消事件监听
            EventsOff("GetMessagesByMessageID", handleMessage); // 组件卸载时取消事件监听
            EventsOff("sendMessageError", handleSenderror); // 组件卸载时取消事件监听
        }
    }, []);



    const sendMessage = () => {
        console.log("发送消息,当前chatId:", chatId);
        const input = document.querySelector('.dialog__input textarea');
        const content = input.value.trim();
        console.log("输入的消息内容:", content);

        if (content) {
            SendMessage(chatId, content, "user");
            input.value = ''; // 发送后清空输入框
        }
        // 让stoopbutton可见
        const stopButton = document.getElementById('stop-button');
        if (stopButton) {
            stopButton.style.display = 'inline-block';
        }

    }

    const stopDialog = () => {
        console.log("停止对话");
        StopChat(chatId); // 发送空消息作为停止信号
    }

    return (
        <div id={"dialog_" + chatId} className="dialog">
            <div className="dialog__header">
                对话

            </div>
            <div className="dialog__messages" ref={messagesRef}>
                {
                    messages && messages.filter(msg => msg.role != 'reasoning').map((msg) => {
                        const isUser = msg.role === 'user';
                        const colors = getRandomMacaronColor(msg.messageID);
                        return (
                            <div key={"dialogmessage_" + msg.messageID}
                                id={"dialogmessage_" + msg.messageID}
                                data-role={isUser ? 'user' : 'assistant'}
                                className={`dialogmessage dialogmessage_${isUser ? 'user' : 'assistant'}`}>
                                <div className="message-avatar clay-card">
                                    {isUser ? '🧑🏻' : '🤖'}
                                    <span className="message-timestamp">
                                        {msg.timestamp}
                                    </span>
                                </div>
                                <div style={{
                                    backgroundColor: colors.bg,
                                    color: colors.text,
                                    whiteSpace: 'pre-wrap',
                                }} className="messagecontent">
                                    {msg.content}
                                </div>
                            </div>
                        )
                    })
                }
            </div>

            <div className="dialog__input">
                <button id='stop-button' onClick={stopDialog}> 
                    <span className="send-icon">🛑
                </span></button>
                <textarea type="text" placeholder="请输入消息"
                    onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                            sendMessage();
                        }
                    }}
                />
                <button id="send-button" onClick={sendMessage}>
                    <span className="send-icon">🚀</span>
                </button>
            </div>
        </div>
    )
}
