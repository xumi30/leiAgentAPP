package bashfunction

import (
	"context"
	"encoding/json"
	"fmt"
	"leiAgent/internal/bashpolicy"
	"leiAgent/internal/tools"
	"leiAgent/utils"
	"os/exec"
	"runtime"
	"strings"
)

// CommandToolName 与 Name() / 模型 tool name 一致。
const CommandToolName = "execute_command"

// BashTool implements the Tool interface for executing bash or cmd commands
type BashTool struct{}

func NewBashTool() tools.Tool {
	return &BashTool{}
}

// Name returns the name of the tool
func (t *BashTool) Name() string {
	return CommandToolName
}

// Description returns a description of what the tool does
func (t *BashTool) Description() string {
	systemType := runtime.GOOS

	return "Tool for executing local bash (Linux/Mac) or cmd (Windows) commands. " +
		"Use this tool when user needs to run system commands, check system status, " +
		"or perform local operations. " +
		"Returns command output, error messages, and exit code. " +
		"当用户需要执行系统命令、根据软件运行的系统类型，选择对应的系统命令。" +
		"使用这个工具时，务必确保输入的命令是安全的，并且不会对系统造成破坏。" +
		"系统的类型是:" + systemType
}

// Parameters returns the parameters that the tool accepts
func (t *BashTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The command to execute",
			},
		},
		"required": []string{"command"},
	}
}

// Run executes the tool with the given input
func (t *BashTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

// ParseCommandFromToolArgs extracts the shell command string from execute_command JSON arguments.
func ParseCommandFromToolArgs(args string) (string, error) {
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %v", err)
	}

	var command string
	var ok bool

	if cmd, found := params["command"]; found {
		if cmdStr, isString := cmd.(string); isString {
			command = cmdStr
			ok = true
		}
	}

	if !ok {
		if props, found := params["properties"]; found {
			if propsStr, isString := props.(string); isString {
				var propParams map[string]string
				if err := json.Unmarshal([]byte(propsStr), &propParams); err == nil {
					if cmd, found := propParams["command"]; found {
						command = cmd
						ok = true
					}
				}
			}
		}
	}

	if !ok {
		return "", fmt.Errorf("command parameter is required, got params: %+v", params)
	}
	return command, nil
}

// ValidateCommand 与当前 config.yaml shell_safety.rules 同源（仅 enabled 项 + 固定结构限制）。
func ValidateCommand(command string) error {
	return bashpolicy.ValidateCommand(command)
}

func (t *BashTool) Execute(ctx context.Context, args string) (string, error) {
	command, err := ParseCommandFromToolArgs(args)
	if err != nil {
		return "", err
	}

	if err := bashpolicy.ValidateCommand(command); err != nil {
		return "", fmt.Errorf("command validation failed: %v", err)
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/c", command)
	} else {
		cmd = exec.CommandContext(ctx, "bash", "-c", command)
	}

	output, err := cmd.CombinedOutput()

	result := map[string]interface{}{
		"command": command,
		"output":  strings.TrimSpace(string(output)),
		"success": err == nil,
	}

	if err != nil {
		result["error"] = err.Error()
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode := exitErr.ExitCode()
			result["exit_code"] = exitCode
			if exitCode != 0 {
				jsonBytes, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return "", fmt.Errorf("failed to marshal result: %v", err)
				}
				return string(jsonBytes), fmt.Errorf("command failed with exit code %d: %s", exitCode, strings.TrimSpace(string(output)))
			}
		}
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %v", err)
	}

	return string(jsonBytes), nil
}

func (t *BashTool) Results() map[string]interface{} {
	return map[string]interface{}{
		"type":        "object",
		"description": "Execution results of the command including command output, error messages, and exit code",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The command that was executed",
				"example":     "ls -la",
			},
			"output": map[string]interface{}{
				"type":        "string",
				"description": "Standard output and error from the command",
				"example":     "total 24\ndrwxr-xr-x 5 user group 4096 Jan 15 10:30 .",
			},
			"success": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether the command executed successfully",
				"example":     true,
			},
			"error": map[string]interface{}{
				"type":        "string",
				"description": "Error message if the command failed (only present if there was an error)",
				"example":     "exit status 127",
			},
			"exit_code": map[string]interface{}{
				"type":        "integer",
				"description": "Exit code of the command (only present if the command failed)",
				"example":     127,
			},
		},
	}
}

func (t *BashTool) SimpleInfo() map[string]string {
	return utils.SimpleInfoMap(utils.ToolTopicSystem, "在本机执行经校验的安全 shell/cmd 命令并返回标准输出与退出码。")
}
