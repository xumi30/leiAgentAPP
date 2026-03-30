export default function ConversationList({ chatID, conversations }) {
    return (
        <div className="conversation-list">
            <div id={`conv_${chatID}`} className="conversation">

                <div className="conversation-info">
                    {conversations.map((conversation) => (
                        <div key={conversation.id} className="conversation_item">
                            <span className="conversation_name">{conversation.text}</span>
                            <button className="conversation_button">...</button>
                        </div>
                    ))}
                </div>
            </div>
        </div>
    )
}
