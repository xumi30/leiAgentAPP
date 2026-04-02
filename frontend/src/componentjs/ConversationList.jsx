import React, { useState, useEffect, useRef } from 'react';
import { AddConversation, ListConversation, DeleteConversation, UpdateConversationTitle, GetMessages } from '../../wailsjs/go/main/App';
import { getRandomMacaronColor, makeChatId } from './Constant';
import { EventsOff, EventsOn } from '../../wailsjs/runtime/runtime';

export default function ConversationList() {
    const [openMenuId, setOpenMenuId] = useState(null);
    const menuRef = useRef(null);
    const [cons, setCons] = useState([]);

    // 点击外部关闭菜单
    useEffect(() => {
        const handleClickOutside = (event) => {
            if (menuRef.current && !menuRef.current.contains(event.target)) {
                setOpenMenuId(null);
            }
        };

        document.addEventListener('mousedown', handleClickOutside);
        return () => {
            document.removeEventListener('mousedown', handleClickOutside);
        };
    }, []);

    //加载对话列表
    useEffect(() => {
        const loadConversations = async () => {
            try {
                const conversations = await ListConversation();
                console.log("loadConversations:", conversations);
                setCons(conversations);
                if (conversations.length>0){
                    switchDialog(conversations[0].id);
                }
               
            } catch (error) {
                console.error("加载对话列表失败:", error);
                setCons([]); // 出错时设置为空数组
            }
        };
        loadConversations();
    }, []);

    // 监听对话列表更新事件
    useEffect(() => {
        const handleConversationListUpdated = (updatedConversation) => {
            console.log("收到对话列表更新事件，数据:", updatedConversation);
            //删除本地的对话列表中对应的对话
            setCons((prevCons) => {
                // 先过滤掉旧的对话
                const filteredCons = prevCons.filter((con) => con.id !== updatedConversation.id);
                // 然后添加新的对话
                return [updatedConversation, ...filteredCons];
            });
            switchDialog(updatedConversation.id)
        };

        const handleConversationListError = (error) => {
            alert("更新失败: " + error);
        }

        
          // 页面刷新
        EventsOn("deleteConversationSuccess", () => { window.location.reload()} );
        EventsOn("getConversationError", handleConversationListError);
        EventsOn("getConversation", handleConversationListUpdated);
        EventsOn("updateConversationError", handleConversationListError);
      

        return () => {
            EventsOff("updateConversationError", handleConversationListUpdated);
            EventsOff("updateConversationError", handleConversationListError);
            EventsOff("getConversation", handleConversationListUpdated);
        };
    }, []);


    const handleButtonClick = (e, conversationId) => {
        e.stopPropagation();
        setOpenMenuId(openMenuId === conversationId ? null : conversationId);
    };

    const handleMenuAction = (action, conversation) => {
        if (action === 'edit') {
            const newTitle = prompt("请输入新的对话名称:", conversation.title);
            if (newTitle) {
                UpdateConversationTitle(conversation.id, newTitle)

                console.log(`修改对话 ${conversation.id} 的名称为:`, newTitle);
            }
        } else if (action === 'delete') {
            if (window.confirm("确定要删除这个对话吗?")) {
                DeleteConversation(conversation.id)
                console.log(`删除对话:`, conversation);
                setCons((prevCons) => prevCons.filter((con) => con.id !== conversation.id));
            }
        } else if (action === 'share') {
            // 这里可以添加分享对话的逻辑
            console.log(`分享对话:`, conversation);
        }

        // 这里可以添加处理不同操作的逻辑
        console.log(`${action} conversation:`, conversation);
        setOpenMenuId(null);
    };

    const handleNewConversation = () => {
        switchDialog("");
        // const newConversationName = prompt("请输入新对话的名称:");
        // if (!newConversationName) {
        //     console.log("用户取消了输入或未输入内容");
        //     return;
        // }

        // AddConversation(newConversationName)
    };

    const switchDialog = (conversationId) => {

        console.log("切换到对话ID:", conversationId);
        // 这里可以添加切换对话的逻辑
        const event = new CustomEvent('conversationChanged', { detail: { conversationId } });
        window.dispatchEvent(event);
    }



    return (
        <div  >
            <button className="new-conversation-btn" onClick={handleNewConversation}>
                <span className="btn-icon"> + </span>
                <span className="btn-text">新建对话</span>
            </button>

            <div className="conversation-list">
                <div className="conversation">
                    {cons && cons.length > 0 && cons.map((conversation) => {
                        const colors = getRandomMacaronColor(conversation.id);
                        const isMenuOpen = openMenuId === conversation.id;

                        return (
                            <div key={conversation.id}
                                className="conversation_item"
                                id={conversation.id}
                                onClick={() => switchDialog(conversation.id)}
                                style={{
                                    backgroundColor: colors.bg,
                                    color: colors.text,
                                    position: 'relative'
                                }}>
                                <span className="conversation_name">
                                    {conversation.title}
                                </span>
                                <div style={{ position: 'relative' }}>
                                    <button
                                        className="conversation_button"
                                        onClick={(e) => {
                                            e.stopPropagation();
                                            handleButtonClick(e, conversation.id)
                                        }}
                                    >
                                        ...
                                    </button>

                                </div>
                                {isMenuOpen && (
                                    <div className="conversation-menu" ref={menuRef}>
                                        <div onClick={() => handleMenuAction('edit', conversation)}>✏️ 修改名称</div>
                                        <div onClick={() => handleMenuAction('delete', conversation)}>🗑️ 删除对话</div>
                                    </div>
                                )}
                            </div>
                        );
                    })}
                </div>
            </div>
        </div>
    );
}

