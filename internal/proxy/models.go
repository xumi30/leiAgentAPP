package proxy

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"sync"
)

type ModelAPIInfo struct {
	backendName string // YAML name，用于日志
	provider    string
	url         string
	token       string
	modelName   string
	isStream    int // 0 : false, 1: true 3:both
	// maxOutputTokens > 0 时覆盖默认 max_tokens；0 表示使用代理内建默认与规划加成逻辑
	maxOutputTokens int

	//状态
	status string
	//token总消耗量
	tokenTotal int
	//平均每次调用时长 性能
	performance int
	//调用次数
	callCount int
}

// GetProvider 获取provider
func (m *ModelAPIInfo) GetProvider() string {
	return m.provider
}

func (m *ModelAPIInfo) logLabel() string {
	if m.backendName != "" {
		return m.backendName
	}
	return m.modelName
}

// ModelAPIPool 模型池，使用单例模式
type ModelAPIPool struct {
	models map[string]*ModelAPIInfo // 存储模型信息的map
	mu     sync.RWMutex             // 读写锁，保证并发安全
}

var (
	instance *ModelAPIPool // 单例实例
	once     sync.Once     // 确保只初始化一次
)

// GetModelPool 获取模型池单例实例
func GetModelPool() *ModelAPIPool {
	once.Do(func() {
		instance = &ModelAPIPool{
			models: make(map[string]*ModelAPIInfo),
		}
	})
	return instance
}

// AddModel 添加模型
func (p *ModelAPIPool) AddModel(provider, modelName string, url, token string, isStream int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 计算token的MD5值作为key
	tokenMD5 := md5.Sum([]byte(token))
	tokenKey := hex.EncodeToString(tokenMD5[:])

	if _, exists := p.models[tokenKey]; exists {
		return fmt.Errorf("model with token %s already exists", tokenKey)
	}

	p.models[tokenKey] = &ModelAPIInfo{
		provider:  provider,
		url:       url,
		token:     token,
		modelName: modelName,
		isStream:  isStream,
	}
	return nil
}

// DeleteModel 删除模型
func (p *ModelAPIPool) DeleteModel(token string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 计算token的MD5值作为key
	tokenMD5 := md5.Sum([]byte(token))
	tokenKey := hex.EncodeToString(tokenMD5[:])

	if _, exists := p.models[tokenKey]; !exists {
		return fmt.Errorf("model with token %s not found", tokenKey)
	}

	delete(p.models, tokenKey)
	return nil
}

// GetModel 获取模型配置
func (p *ModelAPIPool) GetModel(token string) (*ModelAPIInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// 计算token的MD5值作为key
	tokenMD5 := md5.Sum([]byte(token))
	tokenKey := hex.EncodeToString(tokenMD5[:])

	model, exists := p.models[tokenKey]
	if !exists {
		return nil, fmt.Errorf("model %s not found", tokenKey)
	}
	return model, nil
}

// GetAllModels 获取所有模型配置
func (p *ModelAPIPool) GetAllModels() map[string]*ModelAPIInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// 返回模型的副本，防止外部修改
	result := make(map[string]*ModelAPIInfo)
	for k, v := range p.models {
		result[k] = v
	}
	return result
}
