export default function Reasoning({ chatId, reasongingMessages }) {
    return(
        <div id={`reasoning_${chatId}`} className="reasongings">
            <div className="reasoning">
                {reasongingMessages.map((reasongingMessage) => (
                    <span key={reasongingMessage.id} id={reasongingMessage.id}>
                        {reasongingMessage.text}
                    </span>  
                ))}
            </div>
        </div>
    )
}
