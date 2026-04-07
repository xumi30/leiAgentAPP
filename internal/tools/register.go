package tools

import (
	"encoding/json"
	"leiAgent/internal/provider/openaistyle"

	"sync"
)

// registry implements the Toolregistry interface
type registry struct {
	tools map[string]Tool
	mu    sync.RWMutex
}

// Newregistry creates a new tool registry
func newregistry() *registry {
	return &registry{
		tools: make(map[string]Tool),
	}
}

// Register registers a tool with the registry
func (r *registry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
}

// Get returns a tool by name
func (r *registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

// List returns all registered tools
func (r *registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var tools []Tool
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

func (r *registry) ConvertTools() []openaistyle.Tool {
	toolsList := make([]openaistyle.Tool, 0)
	for _, tool := range r.List() {

		chatTool := openaistyle.Tool{
			Type: "function",
			Function: &openaistyle.Function{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  tool.Parameters(),
			},
		}
		toolsList = append(toolsList, chatTool)
	}

	return toolsList
}

var (
	registryInstance *registry
	registryOnce     sync.Once
)

// Getregistry 获取全局单例registry对象
func Getregistry() *registry {
	registryOnce.Do(func() {
		registryInstance = newregistry()
	})
	return registryInstance
}

// 遍历注册器，逐个json化
func (r *registry) ConvertToolsToJSON() ([]byte, error) {
	toolsList := make([]map[string]interface{}, 0)
	for _, tool := range r.List() {
		toolInfo := map[string]interface{}{
			"name":        tool.Name(),
			"description": tool.Description(),
			"parameters":  tool.Parameters(),
			"results":     tool.Results(),
		}
		toolsList = append(toolsList, toolInfo)
	}
	//logging.Info("Converted tools to JSON format %s", toolsList)
	return json.MarshalIndent(toolsList, "", "  ")
}
