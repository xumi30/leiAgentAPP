import { useState, useEffect, useRef } from "react";
import { GetReasoningMessage } from '../../wailsjs/go/main/App';
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { getRandomMacaronColor } from './Constant';

export default function Reasoning() {
    const [reasoningMessages, setReasoningMessages] = useState([]);
    const [chatId, setChatId] = useState('');
    const messagesRef = useRef(null);

    useEffect(() => {
        const handleConversationChange = (event) => {
            const { conversationId } = event.detail;
            setChatId(conversationId);
            const getReasoningMessage = async () => {
                const rsm = await GetReasoningMessage(conversationId);
                setReasoningMessages(rsm);
            }
            getReasoningMessage();
        }

        const handleReasoningMessagAppend = (message) => {
            console.log("收到推理消息更新事件:", message);
            setReasoningMessages((prevMessages) => {
                if (!prevMessages || prevMessages.length === 0) {
                    return [message];
                }
                const messageExists = prevMessages.some((msg) => msg.messageID === message.messageID);
                if (messageExists) {
                    return prevMessages.map((msg) => {
                        if (msg.messageID === message.messageID) {
                            return {
                                ...msg,
                                content: msg.content + message.content,
                            }
                        }
                        return msg;
                    });
                }
                return [...prevMessages,message];
            });
        }

        EventsOn('reasoningAppend', handleReasoningMessagAppend)
        window.addEventListener('conversationChanged', handleConversationChange);

        return () => {
            window.removeEventListener('conversationChanged', handleConversationChange);
        };
    }, []);

    // 新增：监听消息变化，自动滚动到底部
    useEffect(() => {
        const container = messagesRef.current;
        if (!container) return;

        const scrollId = requestAnimationFrame(() => {
            container.scrollTop = container.scrollHeight;
        });

        return () => cancelAnimationFrame(scrollId);
    }, [reasoningMessages]);

    return (
        <div id={"reasoning_" + chatId} className="reasonings" ref={messagesRef}>
            <div className="reason__header">推理过程</div>
            <div>
                <div className="reasoning" >
                    {reasoningMessages && reasoningMessages.map((rsm) => {
                        const colors = getRandomMacaronColor(rsm.messageID);
                        return (
                            <div key={"reasoning_" + rsm.messageID}
                                style={{ backgroundColor: colors.bg, color: colors.text }}
                                className="reasoning__message">
                                <span id={rsm.messageID} style={{
                                    whiteSpace: 'pre-wrap',
                                }}>
                                    {rsm.content}
                                </span>
                            </div>)
                    }
                    )}
                </div>
            </div>
        </div>
    );
}
