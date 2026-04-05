import React, { useState, useEffect, useRef, useMemo } from 'react';
import { SwitchChat, ListConversation, DeleteConversation, UpdateConversationTitle } from '../../wailsjs/go/main/App';
import { getRandomMacaronColor } from './Constant';
import { EventsOff, EventsOn } from '../../wailsjs/runtime/runtime';
import ConversationCalendar from './ConversationCalendar.jsx';

/** 对话的创建/更新日期（本地日历日）是否与选中日期一致 */
function conversationMatchesCalendarDay(conv, ymd) {
    if (!ymd) return true;
    for (const key of ['updated_at', 'created_at']) {
        const v = conv[key];
        if (v == null || v === '') continue;
        const d = new Date(v);
        if (Number.isNaN(d.getTime())) continue;
        const s = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
        if (s === ymd) return true;
    }
    return false;
}

export default function ConversationList({ memoDates = new Set(), refreshMemoDates }) {
    const [openMenuId, setOpenMenuId] = useState(null);
    const menuRef = useRef(null);
    const [cons, setCons] = useState([]);
    const [selectedDate, setSelectedDate] = useState(null);

    const displayedCons = useMemo(() => {
        if (!selectedDate) return cons;
        return cons.filter((c) => conversationMatchesCalendarDay(c, selectedDate));
    }, [cons, selectedDate]);

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
                if (conversations.length > 0) {
                    switchDialog(conversations[0].id, conversations[0].title);
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
            switchDialog(updatedConversation.id, updatedConversation.title);
        };

        const handleConversationListError = (error) => {
            alert("更新失败: " + error);
        };

        const handleDeleteSuccess = () => {
            window.location.reload();
        };

        EventsOn("deleteConversationSuccess", handleDeleteSuccess);
        EventsOn("getConversationError", handleConversationListError);
        EventsOn("getConversation", handleConversationListUpdated);
        EventsOn("updateConversationError", handleConversationListError);

        return () => {
            EventsOff("deleteConversationSuccess");
            EventsOff("getConversationError");
            EventsOff("getConversation");
            EventsOff("updateConversationError");
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
        switchDialog("", '');
        // const newConversationName = prompt("请输入新对话的名称:");
        // if (!newConversationName) {
        //     console.log("用户取消了输入或未输入内容");
        //     return;
        // }

        // AddConversation(newConversationName)
    };

    const switchDialog = (conversationId, titleHint) => {
        SwitchChat(conversationId);
        const title =
            titleHint !== undefined && titleHint !== null
                ? titleHint
                : cons.find((c) => c.id === conversationId)?.title ?? '';
        window.dispatchEvent(
            new CustomEvent('conversationChanged', { detail: { conversationId, title } }),
        );
    };



    return (
        <div>
            <ConversationCalendar
                memoDates={memoDates}
                selectedDate={selectedDate}
                onSelectDate={setSelectedDate}
                onVisibleMonthChange={typeof refreshMemoDates === 'function' ? refreshMemoDates : undefined}
            />
            <button className="new-conversation-btn" onClick={handleNewConversation}>
                <span className="btn-icon"> + </span>
                <span className="btn-text">新建对话</span>
            </button>

            <div className="conversation-list">
                <div className="conversation">
                    {displayedCons && displayedCons.length > 0 &&
                        displayedCons.map((conversation) => {
                        const colors = getRandomMacaronColor(conversation.id);
                        const isMenuOpen = openMenuId === conversation.id;

                        return (
                            <div key={conversation.id}
                                className="conversation_item"
                                id={conversation.id}
                                onClick={() => switchDialog(conversation.id, conversation.title)}
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
                    {displayedCons.length === 0 && cons.length > 0 && selectedDate ? (
                        <p className="conversation-filter-empty">
                            这一天还没有对话，试试其它日期或点下方「显示全部对话」。
                        </p>
                    ) : null}
                </div>
            </div>
        </div>
    );
}

