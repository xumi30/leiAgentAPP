package utils

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/6tail/lunar-go/calendar"
	"go.yaml.in/yaml/v2"
)

// 将 ReadConfig 改为函数类型的变量
var ReadConfig = func(filename string) (map[string]interface{}, error) {
	// 读取配置文件
	config, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	// 解析配置文件
	var data map[string]interface{}
	err = yaml.Unmarshal(config, &data)
	if err != nil {
		return nil, err
	}
	fmt.Println(data)
	return data, nil
}

func GetProviderUrl(providerName string) (string, error) {
	// 读取配置文件
	config, err := ReadConfig("config/config.yaml")
	if err != nil {
		return "", err
	}

	// 获取 providers 数组
	providersList, ok := config["providers"].([]interface{})
	if !ok {
		return "", errors.New("config Providers not found")
	}

	// 遍历 providers 数组，查找匹配的 provider
	for _, p := range providersList {
		provider, ok := p.(map[interface{}]interface{})
		if !ok {
			continue
		}
		if name, ok := provider["name"].(string); ok && name == providerName {
			if url, ok := provider["url"].(string); ok {
				fmt.Println(url)
				return url, nil
			}
		}
	}

	return "", errors.New("config Provider not found")
}

func GetdateInfo() string {
	now := time.Now()

	// 获取星期几
	weekdays := []string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}
	weekday := weekdays[now.Weekday()]

	// 获取节日信息
	holiday := getHoliday(now)

	// 构建详细的时间信息
	result := fmt.Sprintf("当前时间: %s\n", now.Format("2006-01-02 15:04:05"))
	result += fmt.Sprintf("星期: %s\n", weekday)
	result += fmt.Sprintf("农历: %s\n", getLunarDate(now))
	if holiday != "" {
		result += fmt.Sprintf("节日: %s\n", holiday)
	}

	return result
}

func getHoliday(date time.Time) string {
	// 固定日期的节日
	holidays := map[string]string{
		"01-01": "元旦",
		"02-14": "情人节",
		"03-08": "妇女节",
		"03-12": "植树节",
		"04-01": "愚人节",
		"05-01": "劳动节",
		"05-04": "青年节",
		"06-01": "儿童节",
		"07-01": "建党节",
		"08-01": "建军节",
		"09-10": "教师节",
		"10-01": "国庆节",
		"12-25": "圣诞节",
	}

	monthDay := date.Format("01-02")
	if holiday, ok := holidays[monthDay]; ok {
		return holiday
	}

	// 可以添加更多动态节日判断逻辑，如春节、端午节、中秋节等
	// 这里只实现了固定日期的节日

	return ""
}

func getLunarDate(date time.Time) string {
	// 创建公历对象
	solar := calendar.NewSolarFromDate(date)
	// 转换为农历对象
	lunar := calendar.NewLunarFromSolar(solar)

	// 构建农历日期字符串
	return fmt.Sprintf("%s年%s%s",
		lunar.GetYearInGanZhi(),   // 干支年
		lunar.GetMonthInChinese(), // 月份
		lunar.GetDayInChinese())   // 日期
}

func IsBlank(s string) bool {
	return len(strings.TrimSpace(s)) == 0
}

func ExtractJSON(raw string) string {
	raw = strings.TrimSpace(raw)

	// 去掉 markdown 包裹
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")

	// 找 JSON 区间
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")

	if start >= 0 && end > start {
		return raw[start : end+1]
	}

	return raw
}

// EscapeRawNewlinesInJSONStrings replaces literal control characters inside JSON string
// values with escape sequences. LLMs often emit real newlines inside "content" fields,
// which makes the payload invalid for encoding/json.
func EscapeRawNewlinesInJSONStrings(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			b.WriteByte(c)
			escaped = false
			continue
		}
		if inString && c == '\\' {
			b.WriteByte(c)
			escaped = true
			continue
		}
		if c == '"' {
			inString = !inString
			b.WriteByte(c)
			continue
		}
		if inString {
			switch c {
			case '\n':
				b.WriteString(`\n`)
			case '\r':
				b.WriteString(`\r`)
			case '\t':
				b.WriteString(`\t`)
			default:
				b.WriteByte(c)
			}
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// PrepareLLMJSON extracts a JSON object region from model output (see ExtractJSON)
// and escapes literal newlines/tabs inside string values. Use before encoding/json.Unmarshal
// for any LLM-produced JSON payload.
func PrepareLLMJSON(raw string) string {
	return EscapeRawNewlinesInJSONStrings(ExtractJSON(raw))
}

func GenerateChatID() string {
	chatID := fmt.Sprintf("%d%03d", time.Now().UnixMilli(), rand.Intn(1000))
	return chatID
}
