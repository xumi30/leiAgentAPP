import React, { useState, useEffect, useMemo, useRef } from 'react';
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

export default function ConversationList({
    memoDates = new Set(),
    refreshMemoDates,
    streamingChatIds = new Set(),
}) {
    const [openMenuId, setOpenMenuId] = useState(null);
    /** Wails/WebView 常不显示 window.confirm，用应用内弹层代替 */
    const [deletePending, setDeletePending] = useState(null);
    /** 替代 window.prompt 的改名弹层 */
    const [renameTarget, setRenameTarget] = useState(null);
    const [renameTitle, setRenameTitle] = useState('');
    const renameInputRef = useRef(null);
    const [cons, setCons] = useState([]);
    const [selectedDate, setSelectedDate] = useState(null);

    const displayedCons = useMemo(() => {
        if (!selectedDate) return cons;
        return cons.filter((c) => conversationMatchesCalendarDay(c, selectedDate));
    }, [cons, selectedDate]);

    // 不在 document 上监听 click：Wails/WebView 里可能与 React 委托顺序冲突，先关闭菜单导致菜单项 onClick 永远不触发。
    useEffect(() => {
        if (renameTarget) {
            const onKey = (e) => {
                if (e.key === 'Escape') {
                    setRenameTarget(null);
                    setRenameTitle('');
                }
            };
            window.addEventListener('keydown', onKey);
            return () => window.removeEventListener('keydown', onKey);
        }
        if (deletePending) {
            const onKey = (e) => {
                if (e.key === 'Escape') setDeletePending(null);
            };
            window.addEventListener('keydown', onKey);
            return () => window.removeEventListener('keydown', onKey);
        }
        if (openMenuId == null) return;
        const onKey = (e) => {
            if (e.key === 'Escape') setOpenMenuId(null);
        };
        window.addEventListener('keydown', onKey);
        return () => window.removeEventListener('keydown', onKey);
    }, [openMenuId, deletePending, renameTarget]);

    useEffect(() => {
        if (!renameTarget) return;
        const id = window.setTimeout(() => {
            const el = renameInputRef.current;
            if (el) {
                el.focus();
                el.select();
            }
        }, 0);
        return () => window.clearTimeout(id);
    }, [renameTarget]);

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

        const handleDeleteSuccess = async () => {
            try {
                const conversations = await ListConversation();
                setCons(conversations);
                if (conversations.length > 0) {
                    const first = conversations[0];
                    SwitchChat(String(first.id ?? ''));
                    window.dispatchEvent(
                        new CustomEvent('conversationChanged', {
                            detail: { conversationId: String(first.id ?? ''), title: first.title ?? '' },
                        }),
                    );
                } else {
                    SwitchChat('');
                    window.dispatchEvent(
                        new CustomEvent('conversationChanged', {
                            detail: { conversationId: '', title: '' },
                        }),
                    );
                }
            } catch (err) {
                console.error('刷新对话列表失败:', err);
            }
        };

        const handleDeleteError = (error) => {
            alert("删除失败: " + error);
        };

        EventsOn("deleteConversationSuccess", handleDeleteSuccess);
        EventsOn("deleteConversationError", handleDeleteError);
        EventsOn("getConversationError", handleConversationListError);
        EventsOn("getConversation", handleConversationListUpdated);
        EventsOn("updateConversationError", handleConversationListError);

        return () => {
            EventsOff("deleteConversationSuccess");
            EventsOff("deleteConversationError");
            EventsOff("getConversationError");
            EventsOff("getConversation");
            EventsOff("updateConversationError");
        };
    }, []);


    const handleButtonClick = (e, conversationId) => {
        e.stopPropagation();
        const idKey = conversationId != null ? String(conversationId) : '';
        setOpenMenuId(openMenuId === idKey ? null : idKey);
    };

    const handleMenuAction = async (action, conversation) => {
        if (action === 'edit') {
            setRenameTarget(conversation);
            setRenameTitle(String(conversation.title ?? ''));
        } else if (action === 'delete') {
            setDeletePending(conversation);
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

    const closeRenameModal = () => {
        setRenameTarget(null);
        setRenameTitle('');
    };

    const runRenameSave = () => {
        if (!renameTarget) return;
        const trimmed = renameTitle.trim();
        if (!trimmed) return;
        const chatID = renameTarget.id != null ? String(renameTarget.id) : '';
        UpdateConversationTitle(chatID, trimmed);
        setCons((prev) =>
            prev.map((c) => (String(c.id ?? '') === chatID ? { ...c, title: trimmed } : c)),
        );
        console.log(`修改对话 ${chatID} 的名称为:`, trimmed);
        closeRenameModal();
    };

    const runConfirmedDelete = async (conversation) => {
        setDeletePending(null);
        const chatID = conversation.id != null ? String(conversation.id) : '';
        try {
            await DeleteConversation(chatID);
            setCons((prevCons) => prevCons.filter((con) => String(con.id ?? '') !== chatID));
            console.log('删除对话:', conversation);
        } catch (err) {
            console.error('删除对话失败:', err);
            alert('删除失败: ' + (err?.message || String(err)));
        }
    };

    const switchDialog = (conversationId, titleHint) => {
        const idStr = conversationId != null ? String(conversationId) : '';
        SwitchChat(idStr);
        const title =
            titleHint !== undefined && titleHint !== null
                ? titleHint
                : cons.find((c) => String(c.id ?? '') === idStr)?.title ?? '';
        window.dispatchEvent(
            new CustomEvent('conversationChanged', { detail: { conversationId: idStr, title } }),
        );
    };



    return (
        <div className="conversation-list-panel">
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

            {renameTarget ? (
                <div
                    className="conversation-rename-overlay"
                    role="presentation"
                    onMouseDown={(e) => {
                        if (e.target === e.currentTarget) closeRenameModal();
                    }}
                >
                    <div
                        className="conversation-rename-dialog"
                        role="dialog"
                        aria-labelledby="conversation-rename-title"
                        aria-modal="true"
                        onMouseDown={(e) => e.stopPropagation()}
                    >
                        <p id="conversation-rename-title" className="conversation-delete-dialog__title">
                            修改名称
                        </p>
                        <label className="conversation-rename-dialog__label" htmlFor="conversation-rename-input">
                            对话名称
                        </label>
                        <input
                            id="conversation-rename-input"
                            ref={renameInputRef}
                            className="conversation-rename-dialog__input"
                            type="text"
                            value={renameTitle}
                            onChange={(e) => setRenameTitle(e.target.value)}
                            onKeyDown={(e) => {
                                if (e.key === 'Enter') {
                                    e.preventDefault();
                                    runRenameSave();
                                }
                            }}
                            autoComplete="off"
                        />
                        <div className="conversation-delete-dialog__actions">
                            <button type="button" className="conversation-delete-dialog__btn" onClick={closeRenameModal}>
                                取消
                            </button>
                            <button
                                type="button"
                                className="conversation-delete-dialog__btn conversation-rename-dialog__btn-primary"
                                disabled={!renameTitle.trim()}
                                onClick={runRenameSave}
                            >
                                保存
                            </button>
                        </div>
                    </div>
                </div>
            ) : null}

            {deletePending ? (
                <div
                    className="conversation-delete-overlay"
                    role="presentation"
                    onMouseDown={(e) => {
                        if (e.target === e.currentTarget) setDeletePending(null);
                    }}
                >
                    <div
                        className="conversation-delete-dialog"
                        role="alertdialog"
                        aria-labelledby="conversation-delete-title"
                        aria-describedby="conversation-delete-desc"
                        onMouseDown={(e) => e.stopPropagation()}
                    >
                        <p id="conversation-delete-title" className="conversation-delete-dialog__title">
                            删除对话
                        </p>
                        <p id="conversation-delete-desc" className="conversation-delete-dialog__desc">
                            确定要删除「{deletePending.title ?? '未命名对话'}」吗？此操作无法撤销。
                        </p>
                        <div className="conversation-delete-dialog__actions">
                            <button type="button" className="conversation-delete-dialog__btn" onClick={() => setDeletePending(null)}>
                                取消
                            </button>
                            <button
                                type="button"
                                className="conversation-delete-dialog__btn conversation-delete-dialog__btn--danger"
                                onClick={() => void runConfirmedDelete(deletePending)}
                            >
                                删除
                            </button>
                        </div>
                    </div>
                </div>
            ) : null}

            <div className="conversation-list">
                <div className="conversation">
                    {displayedCons && displayedCons.length > 0 &&
                        displayedCons.map((conversation) => {
                        const rowId = conversation.id != null ? String(conversation.id) : '';
                        const colors = getRandomMacaronColor(rowId);
                        const isMenuOpen = openMenuId === rowId;
                        const isStreaming = rowId && streamingChatIds.has(rowId);

                        return (
                            <div key={rowId}
                                className={`conversation_item${isMenuOpen ? ' conversation_item--menu-open' : ''}`}
                                id={rowId}
                                onClick={(e) => {
                                    if (e.target.closest?.('.conversation-menu')) return;
                                    setOpenMenuId(null);
                                    switchDialog(rowId, conversation.title);
                                }}
                                style={{
                                    backgroundColor: colors.bg,
                                    color: colors.text,
                                    position: 'relative'
                                }}>
                                <div className="conversation_item__main">
                                    {isStreaming ? (
                                        <span
                                            className="conversation-item__busy"
                                            role="status"
                                            aria-label="该对话仍有回复生成中"
                                        />
                                    ) : null}
                                    <span className="conversation_name">
                                        {conversation.title}
                                    </span>
                                </div>
                                <div style={{ position: 'relative' }}>
                                    <button
                                        type="button"
                                        className="conversation_button"
                                        onClick={(e) => {
                                            e.stopPropagation();
                                            handleButtonClick(e, rowId)
                                        }}
                                    >
                                        ...
                                    </button>

                                </div>
                                {isMenuOpen && (
                                    <div
                                        className="conversation-menu"
                                        onMouseDown={(e) => e.stopPropagation()}
                                        onClick={(e) => e.stopPropagation()}
                                    >
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

