export default function Dialog({ chatId, messages }) {
    return (
        <div id={chatId} className="dialog">
            <div className="dialog__header">
                {/* 对话标题栏 */}
            </div>
            <div className="dialog__messages">
                {
                    messages.map((message) => {
                        const isUser = message.role === 'user';
                        return (
                            <div 
                                key={message.id} 
                                data-role={isUser ? 'user' : 'assistant'}
                                className={`dialog__message dialog__message--${isUser ? 'user' : 'assistant'}`}
                            >
                                <div className="message__content">
                                    {message.content}
                                </div>
                            </div>
                        )
                    })
                }
            </div>
            <div className="dialog__input">
                <input type="text" placeholder="请输入消息" />
                <button id="send-button"> <span className="send-icon">🚀</span></button>
            </div>
        </div>
    )
}
