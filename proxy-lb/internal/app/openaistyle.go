package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	ToolChoiceAuto     = "auto"
	ToolChoiceRequired = "required"
	ToolChoiceNone     = "none"
)

type ChatCompletionRequest struct {
	Model            string          `json:"model"`
	Messages         []ChatMessage   `json:"messages"`
	Stream           bool            `json:"stream,omitempty"`
	StreamOptions    *StreamOptions  `json:"stream_options,omitempty"`
	Temperature      *float64        `json:"temperature,omitempty"`
	TopP             *float64        `json:"top_p,omitempty"`
	MaxTokens        *int            `json:"max_tokens,omitempty"`
	Tools            []Tool          `json:"tools,omitempty"`
	ToolChoice       *ToolChoice     `json:"tool_choice,omitempty"`
	Stop             []string        `json:"stop,omitempty"`
	RequestID        string          `json:"request_id,omitempty"`
	UserID           string          `json:"user_id,omitempty"`
	Thinking         *ChatThinking   `json:"thinking,omitempty"`
	FrequencyPenalty *float64        `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64        `json:"presence_penalty,omitempty"`
	ResponseFormat   *ResponseFormat `json:"response_format,omitempty"`
	Seed             *int            `json:"seed,omitempty"`
	Enablesearch     bool            `json:"enable_search,omitempty"`
	EnableThinking   *bool           `json:"enable_thinking,omitempty"`
}

type StreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

type ChatMessage struct {
	Role       string        `json:"role"`
	Content    interface{}   `json:"content"`
	ToolCalls  []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	Audio      *AudioMessage `json:"audio,omitempty"`
}

type Tool struct {
	Type      string     `json:"type"`
	Function  *Function  `json:"function,omitempty"`
	Retrieval *Retrieval `json:"retrieval,omitempty"`
	WebSearch *WebSearch `json:"web_search,omitempty"`
	MCP       *MCP       `json:"mcp,omitempty"`
}

type Function struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
	Arguments   map[string]interface{} `json:"arguments,omitempty"`
}

func (f *Function) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) || len(data) == 0 {
		*f = Function{}
		return nil
	}

	var raw struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Parameters  map[string]interface{} `json:"parameters,omitempty"`
		Arguments   json.RawMessage        `json:"arguments,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	f.Name = raw.Name
	f.Description = raw.Description
	f.Parameters = raw.Parameters
	f.Arguments = nil

	if len(bytes.TrimSpace(raw.Arguments)) == 0 {
		return nil
	}

	// Support both object and OpenAI-style stringified JSON arguments.
	trimmed := bytes.TrimSpace(raw.Arguments)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var argStr string
		if err := json.Unmarshal(trimmed, &argStr); err != nil {
			return err
		}
		argStr = strings.TrimSpace(argStr)
		if argStr == "" {
			return nil
		}
		var argObj map[string]interface{}
		if err := json.Unmarshal([]byte(argStr), &argObj); err != nil {
			// Be permissive: some clients send non-JSON strings here.
			f.Arguments = map[string]interface{}{"_raw": argStr}
			if f.Parameters == nil {
				f.Parameters = f.Arguments
			}
			return nil
		}
		f.Arguments = argObj
		// Compatibility: some internal code historically uses Parameters to carry args.
		if f.Parameters == nil {
			f.Parameters = argObj
		}
		return nil
	}

	var argObj map[string]interface{}
	if err := json.Unmarshal(trimmed, &argObj); err != nil {
		// Also be permissive for non-object values.
		var any interface{}
		if err2 := json.Unmarshal(trimmed, &any); err2 != nil {
			return err
		}
		f.Arguments = map[string]interface{}{"_value": any}
		if f.Parameters == nil {
			f.Parameters = f.Arguments
		}
		return nil
	}
	f.Arguments = argObj
	return nil
}

type Retrieval struct {
	KnowledgeID    string `json:"knowledge_id"`
	PromptTemplate string `json:"prompt_template,omitempty"`
}

type WebSearch struct {
	Enable              bool   `json:"enable"`
	SearchEngine        string `json:"search_engine"`
	SearchQuery         string `json:"search_query,omitempty"`
	SearchIntent        string `json:"search_intent,omitempty"`
	Count               *int   `json:"count,omitempty"`
	SearchDomainFilter  string `json:"search_domain_filter,omitempty"`
	SearchRecencyFilter string `json:"search_recency_filter,omitempty"`
	ContentSize         string `json:"content_size,omitempty"`
	ResultSequence      string `json:"result_sequence,omitempty"`
	SearchResult        bool   `json:"search_result,omitempty"`
	RequireSearch       bool   `json:"require_search,omitempty"`
	SearchPrompt        string `json:"search_prompt,omitempty"`
}

type MCP struct {
	ServerLabel   string            `json:"server_label"`
	ServerURL     string            `json:"server_url,omitempty"`
	TransportType string            `json:"transport_type,omitempty"`
	AllowedTools  []string          `json:"allowed_tools,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
}

type ToolChoice struct {
	Type     string    `json:"-"`
	Function *Function `json:"function,omitempty"`
}

func (t *ToolChoice) MarshalJSON() ([]byte, error) {
	if t == nil {
		return []byte("null"), nil
	}
	switch t.Type {
	case ToolChoiceAuto, ToolChoiceRequired, ToolChoiceNone:
		if t.Function == nil {
			return json.Marshal(t.Type)
		}
	}
	type alias struct {
		Type     string    `json:"type"`
		Function *Function `json:"function,omitempty"`
	}
	return json.Marshal(alias{Type: t.Type, Function: t.Function})
}

func (t *ToolChoice) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) || len(data) == 0 {
		*t = ToolChoice{}
		return nil
	}

	// Accept OpenAI-style shorthand: "auto" | "none" | "required"
	if len(data) > 0 && data[0] == '"' {
		var mode string
		if err := json.Unmarshal(data, &mode); err != nil {
			return err
		}
		switch mode {
		case ToolChoiceAuto, ToolChoiceNone, ToolChoiceRequired:
			t.Type = mode
			t.Function = nil
			return nil
		default:
			return fmt.Errorf("invalid tool_choice %q", mode)
		}
	}

	// Accept object form: {"type":"auto"} or {"type":"function","function":{...}}
	var raw struct {
		Type     string    `json:"type"`
		Function *Function `json:"function,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	t.Type = raw.Type
	t.Function = raw.Function
	return nil
}

type ToolCall struct {
	ID       string    `json:"id"`
	Type     string    `json:"type"`
	Function *Function `json:"function"`
	Index    int       `json:"index"`
}

type ChatThinking struct {
	Type          string `json:"type"`
	ClearThinking bool   `json:"clear_thinking,omitempty"`
}

type AudioMessage struct {
	ID        string `json:"id"`
	Data      string `json:"data"`
	ExpiresAt string `json:"expires_at"`
}

type ResponseFormat struct {
	Type string `json:"type"`
}

type ChatCompletionResponse struct {
	ID            string                 `json:"id"`
	RequestID     string                 `json:"request_id"`
	Created       int64                  `json:"created"`
	Model         string                 `json:"model"`
	Choices       []ChatCompletionChoice `json:"choices"`
	Usage         *TokenUsage            `json:"usage,omitempty"`
	VideoResult   []VideoResult          `json:"video_result,omitempty"`
	WebSearch     []WebSearchResult      `json:"web_search,omitempty"`
	ContentFilter []ContentFilter        `json:"content_filter,omitempty"`
}

type ChatCompletionChoice struct {
	Index        int                    `json:"index"`
	Message      *ChatCompletionMessage `json:"message"`
	Delta        *ChatCompletionDelta   `json:"delta"`
	FinishReason string                 `json:"finish_reason"`
}

type ChatCompletionMessage struct {
	Role             string                   `json:"role"`
	Content          interface{}              `json:"content"`
	ReasoningContent string                   `json:"reasoning_content,omitempty"`
	Audio            *AudioMessage            `json:"audio,omitempty"`
	ToolCalls        []ChatCompletionToolCall `json:"tool_calls,omitempty"`
}

type ChatCompletionDelta struct {
	Role             string                   `json:"role,omitempty"`
	Content          interface{}              `json:"content,omitempty"`
	ReasoningContent string                   `json:"reasoning_content,omitempty"`
	Audio            *AudioMessage            `json:"audio,omitempty"`
	ToolCalls        []ChatCompletionToolCall `json:"tool_calls,omitempty"`
}

type ChatCompletionToolCall struct {
	ID       string        `json:"id"`
	Type     string        `json:"type"`
	Function *FunctionCall `json:"function"`
	Index    int           `json:"index"`
	MCP      *MCPToolCall  `json:"mcp,omitempty"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type MCPToolCall struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	ServerLabel string                 `json:"server_label"`
	Name        string                 `json:"name"`
	Arguments   string                 `json:"arguments"`
	Error       string                 `json:"error,omitempty"`
	Output      map[string]interface{} `json:"output,omitempty"`
}

type TokenUsage struct {
	PromptTokens        int                  `json:"prompt_tokens"`
	CompletionTokens    int                  `json:"completion_tokens"`
	TotalTokens         int                  `json:"total_tokens"`
	PromptTokensDetails *PromptTokensDetails `json:"prompt_tokens_details,omitempty"`
}

type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type VideoResult struct {
	URL           string `json:"url"`
	CoverImageURL string `json:"cover_image_url"`
}

type WebSearchResult struct {
	Icon        string `json:"icon"`
	Title       string `json:"title"`
	Link        string `json:"link"`
	Media       string `json:"media"`
	PublishDate string `json:"publish_date"`
	Content     string `json:"content"`
	Refer       string `json:"refer"`
}

type ContentFilter struct {
	Role  string `json:"role"`
	Level int    `json:"level"`
}
