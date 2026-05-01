// Package fileFunctions contains workspace-scoped file tools.
package filefunctions

import (
	"context"
	"encoding/json"
	"fmt"
	"leiAgent/internal/doclib"
	"leiAgent/internal/tools"
	"leiAgent/logging"
	"leiAgent/utils"

	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// WriteFileChunk implements the Tool interface for writing content chunks to files
type WriteFileChunk struct{}

var (
	writeFileChunkInstance *WriteFileChunk
	writeFileChunkOnce     sync.Once
)

// GetWriteFileChunk returns a singleton instance of WriteFileChunk
func GetWriteFileChunk() tools.Tool {
	writeFileChunkOnce.Do(func() {
		writeFileChunkInstance = &WriteFileChunk{}
	})
	return writeFileChunkInstance
}

// Name returns the name of the tool
func (t *WriteFileChunk) Name() string {
	return "write_file_chunk"
}

// Description returns the description of what the tool does
func (t *WriteFileChunk) Description() string {
	return `
	Writes a chunk of content to a file at a specific offset.
	
	This tool is designed for writing large files that exceed the token limit.
	You should break down large content into smaller chunks and call this tool
	multiple times, each with a different offset.
	
	IMPORTANT: 
	- The first call should have offset = 0
	- Subsequent calls should have offset equal to total size of previously written content
	- The last chunk should have is_last = true
	- If is_last is true, file will be truncated at current offset + content length
	- CRITICAL: Each chunk must not exceed 100 characters to ensure reliable processing
	
	To use this tool:
	1. Provide the actual filename, content, offset, and is_last values
	2. Do NOT provide the parameter schema/definition
	3. Call this tool multiple times for large files, each time with the next offset
	
	Args (example values):
	{
	  "filename": "large_file.txt",
	  "content": "First part of the content...",
	  "offset": 0,
	  "is_last": false
	}
	
	Example usage for writing a large file:
	1. First chunk:
	   write_file_chunk({
	     "filename": "large_file.txt",
	     "content": "First part of the content...",
	     "offset": 0,
	     "is_last": false
	   })
	   
	2. Second chunk:
	   write_file_chunk({
	     "filename": "large_file.txt",
	     "content": "Second part of the content...",
	     "offset": 1024,
	     "is_last": false
	   })
	   
	3. Last chunk:
	   write_file_chunk({
	     "filename": "large_file.txt",
	     "content": "Last part of the content...",
	     "offset": 2048,
	     "is_last": true
	   })
	   
	Note: When writing large files, ensure each chunk is within the 100 character limit
	for reliable processing. If a chunk exceeds this limit, it may be truncated or fail.
	`
}

// Parameters returns the parameters that the tool accepts
func (t *WriteFileChunk) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"filename": map[string]interface{}{
				"type":        "string",
				"description": "The path to the file. Can be an absolute path or a relative path.",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The content chunk to write to the file.",
			},
			"offset": map[string]interface{}{
				"type":        "integer",
				"description": "The byte offset in the file where this chunk should be written.",
			},
			"is_last": map[string]interface{}{
				"type":        "boolean",
				"description": "True if this is the last chunk, false otherwise.",
			},
		},
		"required": []string{"filename", "content", "offset", "is_last"},
	}
}

// Execute executes the tool with the given arguments
func (t *WriteFileChunk) Execute(ctx context.Context, args string) (string, error) {
	var params map[string]interface{}

	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %v", err)
	}

	// 获取必需参数
	filename, ok := params["filename"].(string)
	if !ok || filename == "" {
		return "", fmt.Errorf("filename parameter is required")
	}

	content, ok := params["content"].(string)
	if !ok || content == "" {
		return "", fmt.Errorf("content parameter is required")
	}

	offsetFloat, ok := params["offset"].(float64)
	if !ok {
		return "", fmt.Errorf("offset parameter is required")
	}
	offset := int(offsetFloat)

	isLast, ok := params["is_last"].(bool)
	if !ok {
		return "", fmt.Errorf("is_last parameter is required")
	}

	// 规范化路径
	if runtime.GOOS == "windows" {
		filename = strings.ReplaceAll(filename, "/", "\\")
	} else {
		filename = strings.ReplaceAll(filename, "\\", "/")
	}

	// 如果是相对路径，转换为绝对路径
	if !filepath.IsAbs(filename) {
		absPath, err := filepath.Abs(filename)
		if err != nil {
			return "", fmt.Errorf("failed to resolve absolute path: %v", err)
		}
		filename = absPath
	}

	// 确保目录存在
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %v", err)
	}

	// 打开文件
	var file *os.File
	var err error

	if isLast && offset == 0 {
		// 如果是最后一个块且偏移量为0，直接创建或覆盖文件
		file, err = os.Create(filename)
	} else {
		// 否则以读写模式打开文件
		file, err = os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0644)
	}

	if err != nil {
		return "", fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// 定位到指定偏移量
	if _, err := file.Seek(int64(offset), 0); err != nil {
		return "", fmt.Errorf("failed to seek to offset: %v", err)
	}

	// 写入内容
	if _, err := file.WriteString(content); err != nil {
		return "", fmt.Errorf("failed to write content: %v", err)
	}

	// 如果是最后一个块，截断文件
	if isLast {
		if err := file.Truncate(int64(offset + len(content))); err != nil {
			return "", fmt.Errorf("failed to truncate file: %v", err)
		}
		doclib.Register(filename)
	}

	logging.Info("Wrote chunk of %d bytes at offset %d to file %s (is_last: %v)", len(content), offset, filename, isLast)

	result := map[string]interface{}{
		"success":       true,
		"bytes_written": len(content),
		"offset":        offset,
		"filename":      filename,
		"is_last":       isLast,
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %v", err)
	}

	return string(jsonBytes), nil

}

// Run executes the tool with the given input
func (t *WriteFileChunk) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}
func (t *WriteFileChunk) SimpleInfo() map[string]string {
	return utils.SimpleInfoMap(utils.ToolTopicFiles, "按字节偏移分块写入大文件，适合超长内容分段落盘。")
}

func (t *WriteFileChunk) Results() map[string]interface{} {
	return map[string]interface{}{
		"type":        "object",
		"description": "Result of the file write operation",
		"properties": map[string]interface{}{
			"success": map[string]interface{}{
				"type":        "boolean",
				"description": "Indicates whether the write operation was successful",
				"example":     true,
			},
			"bytes_written": map[string]interface{}{
				"type":        "integer",
				"description": "Number of bytes written in this chunk",
				"example":     1024,
			},
			"offset": map[string]interface{}{
				"type":        "integer",
				"description": "The offset in the file where the chunk was written",
				"example":     0,
			},
			"filename": map[string]interface{}{
				"type":        "string",
				"description": "The absolute path of the file that was written to",
				"example":     "/path/to/large_file.txt",
			},
			"is_last": map[string]interface{}{
				"type":        "boolean",
				"description": "Indicates whether this was the last chunk to write",
				"example":     false,
			},
		},
	}
}
