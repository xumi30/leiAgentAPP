package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	mcpHubCommandTimeout = 45 * time.Second
	mcpHubLocale         = "zh-CN"
)

type MCPHubStatus struct {
	Registered      bool   `json:"registered"`
	CredentialsPath string `json:"credentialsPath"`
	Message         string `json:"message"`
}

type MCPHubRegisterResult struct {
	Registered      bool   `json:"registered"`
	CredentialsPath string `json:"credentialsPath"`
	Message         string `json:"message"`
}

type MCPHubSearchItem struct {
	Identifier          string                 `json:"identifier"`
	Name                string                 `json:"name"`
	Description         string                 `json:"description"`
	Author              string                 `json:"author"`
	Category            string                 `json:"category"`
	ConnectionType      string                 `json:"connectionType"`
	InstallCount        int                    `json:"installCount"`
	RatingAverage       float64                `json:"ratingAverage"`
	RatingCount         int                    `json:"ratingCount"`
	CommentCount        int                    `json:"commentCount"`
	Version             string                 `json:"version"`
	Homepage            string                 `json:"homepage"`
	Icon                string                 `json:"icon"`
	ManifestURL         string                 `json:"manifestUrl"`
	InstallationMethods string                 `json:"installationMethods"`
	IsValidated         bool                   `json:"isValidated"`
	IsFeatured          bool                   `json:"isFeatured"`
	IsOfficial          bool                   `json:"isOfficial"`
	Github              map[string]interface{} `json:"github"`
}

type MCPHubSearchResult struct {
	Items       []MCPHubSearchItem `json:"items"`
	Categories  []string           `json:"categories"`
	CurrentPage int                `json:"currentPage"`
	PageSize    int                `json:"pageSize"`
	TotalCount  int                `json:"totalCount"`
	TotalPages  int                `json:"totalPages"`
}

type MCPHubAuthor struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type MCPHubSystemDependency struct {
	Name                string            `json:"name"`
	Type                string            `json:"type"`
	CheckCommand        string            `json:"checkCommand"`
	InstallInstructions map[string]string `json:"installInstructions"`
}

type MCPHubDeploymentConnection struct {
	Type         string                 `json:"type"`
	Command      string                 `json:"command"`
	URL          string                 `json:"url"`
	Args         []string               `json:"args"`
	Headers      map[string]string      `json:"headers"`
	Env          map[string]string      `json:"env"`
	ConfigSchema map[string]interface{} `json:"configSchema"`
}

type MCPHubDeploymentOption struct {
	InstallationMethod string                     `json:"installationMethod"`
	Description        string                     `json:"description"`
	IsRecommended      bool                       `json:"isRecommended"`
	Connection         MCPHubDeploymentConnection `json:"connection"`
	InstallationDetail map[string]interface{}     `json:"installationDetails"`
	SystemDependencies []MCPHubSystemDependency   `json:"systemDependencies"`
}

type MCPHubPluginDetail struct {
	Identifier        string                   `json:"identifier"`
	Name              string                   `json:"name"`
	Description       string                   `json:"description"`
	Category          string                   `json:"category"`
	Version           string                   `json:"version"`
	Homepage          string                   `json:"homepage"`
	Icon              string                   `json:"icon"`
	ConnectionType    string                   `json:"connectionType"`
	InstallCount      int                      `json:"installCount"`
	RatingAverage     float64                  `json:"ratingAverage"`
	RatingCount       int                      `json:"ratingCount"`
	IsValidated       bool                     `json:"isValidated"`
	IsFeatured        bool                     `json:"isFeatured"`
	IsOfficial        bool                     `json:"isOfficial"`
	HaveCloudEndpoint bool                     `json:"haveCloudEndpoint"`
	Author            MCPHubAuthor             `json:"author"`
	Github            map[string]interface{}   `json:"github"`
	Overview          map[string]interface{}   `json:"overview"`
	Tags              []string                 `json:"tags"`
	DeploymentOptions []MCPHubDeploymentOption `json:"deploymentOptions"`
}

func GetMCPHubStatus() (MCPHubStatus, error) {
	path, err := mcpHubCredentialsPath()
	if err != nil {
		return MCPHubStatus{}, err
	}
	if _, err := os.Stat(path); err == nil {
		return MCPHubStatus{
			Registered:      true,
			CredentialsPath: path,
			Message:         "已检测到 LobeHub Marketplace 凭据",
		}, nil
	} else if !os.IsNotExist(err) {
		return MCPHubStatus{}, err
	}
	return MCPHubStatus{
		Registered:      false,
		CredentialsPath: path,
		Message:         "首次使用 MCP Hub 前需要先注册 LobeHub Marketplace 身份",
	}, nil
}

func RegisterMCPHub(name, description string) (MCPHubRegisterResult, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" {
		return MCPHubRegisterResult{}, fmt.Errorf("名称不能为空")
	}
	if description == "" {
		return MCPHubRegisterResult{}, fmt.Errorf("描述不能为空")
	}

	var resp struct {
		ClientID        string `json:"clientId"`
		CredentialsPath string `json:"credentialsPath"`
		Message         string `json:"message"`
	}
	if err := runMCPHubCLIJSON(&resp, "register", "--name", name, "--description", description, "--source", "codex", "--output", "json"); err != nil {
		return MCPHubRegisterResult{}, err
	}

	status, err := GetMCPHubStatus()
	if err != nil {
		return MCPHubRegisterResult{}, err
	}
	return MCPHubRegisterResult{
		Registered:      status.Registered,
		CredentialsPath: firstNonBlankString(resp.CredentialsPath, status.CredentialsPath),
		Message:         firstNonBlankString(resp.Message, status.Message),
	}, nil
}

func SearchMCPHub(query, category string, page, pageSize int) (MCPHubSearchResult, error) {
	query = strings.TrimSpace(query)
	category = strings.TrimSpace(category)
	if query == "" {
		return MCPHubSearchResult{}, fmt.Errorf("搜索关键词不能为空")
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 12
	}
	if pageSize > 30 {
		pageSize = 30
	}

	args := []string{
		"mcp", "search",
		"--q", query,
		"--page", fmt.Sprintf("%d", page),
		"--page-size", fmt.Sprintf("%d", pageSize),
		"--locale", mcpHubLocale,
		"--output", "json",
	}
	if category != "" {
		args = append(args, "--category", category)
	}

	var resp MCPHubSearchResult
	if err := runMCPHubCLIJSON(&resp, args...); err != nil {
		return MCPHubSearchResult{}, err
	}
	return resp, nil
}

func GetMCPHubPluginDetail(identifier string) (MCPHubPluginDetail, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return MCPHubPluginDetail{}, fmt.Errorf("插件标识不能为空")
	}

	var resp MCPHubPluginDetail
	if err := runMCPHubCLIJSON(&resp, "mcp", "view", identifier, "--locale", mcpHubLocale, "--output", "json"); err != nil {
		return MCPHubPluginDetail{}, err
	}
	return resp, nil
}

func runMCPHubCLIJSON(out interface{}, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), mcpHubCommandTimeout)
	defer cancel()

	cmdArgs := append([]string{"-y", "@lobehub/market-cli"}, args...)
	cmd := exec.CommandContext(ctx, "npx", cmdArgs...)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("MCP Hub 请求超时，请稍后重试")
	}

	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		if strings.Contains(trimmed, "No credentials found") {
			return fmt.Errorf("尚未注册 LobeHub Marketplace，请先完成 Hub 注册")
		}
		if trimmed == "" {
			return fmt.Errorf("MCP Hub 调用失败：%w", err)
		}
		return fmt.Errorf("MCP Hub 调用失败：%s", trimmed)
	}
	if trimmed == "" {
		return fmt.Errorf("MCP Hub 未返回数据")
	}
	if err := json.Unmarshal([]byte(trimmed), out); err != nil {
		return fmt.Errorf("MCP Hub 返回解析失败：%w", err)
	}
	return nil
}

func mcpHubCredentialsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".lobehub-market", "credentials.json"), nil
}

func firstNonBlankString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
