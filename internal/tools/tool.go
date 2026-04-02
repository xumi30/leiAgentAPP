package tools

import (
	"context"
	"errors"
)

var (
	ErrFunctionNotFound = errors.New("function not found")
	ErrInvalidParams    = errors.New("invalid parameters")
	ErrExecutionFailed  = errors.New("function execution failed")
)

// Tool represents a tool that can be used by an agent
type Tool interface {
	// Name returns the name of the tool
	Name() string

	// Description returns a description of what the tool does
	Description() string

	// Run executes the tool with the given input
	// Run(ctx context.Context, input string) (string, error)

	// Parameters returns the parameters that the tool accepts
	Parameters() map[string]interface{}

	// Execute executes the tool with the given arguments
	Execute(ctx context.Context, args string) (string, error)
}

// ToolWithDisplayName is an optional interface that tools can implement
// to provide a human-friendly display name
type ToolWithDisplayName interface {
	// DisplayName returns a human-friendly name for the tool
	DisplayName() string
}

// InternalTool is an optional interface that tools can implement
// to indicate they should be hidden from users
type InternalTool interface {
	// Internal returns true if this tool's usage should be hidden from users
	// This can be used to filter tool calls in OnToolCall or other display contexts
	Internal() bool
}

// ParameterSpec defines the specification for a tool parameter

type ParameterSpec struct {
	Type        string                  `json:"type"`
	Description string                  `json:"description"`
	Properties  map[string]PropertySpec `json:"properties,omitempty"`
	Required    []string                `json:"required,omitempty"`
}

type PropertySpec struct {
	Type        string        `json:"type"`
	Description string        `json:"description"`
	Enum        []string      `json:"enum,omitempty"`
	Default     interface{}   `json:"default,omitempty"`
	Items       *PropertySpec `json:"items,omitempty"`
}

// ToolRegistry is a registry of available tools
type ToolRegistry interface {
	// Register registers a tool with the registry
	Register(tool Tool)

	// Get returns a tool by name
	Get(name string) (Tool, bool)

	// List returns all registered tools
	List() []Tool
}
