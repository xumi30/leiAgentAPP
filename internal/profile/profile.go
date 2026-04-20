package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"leiAgent/dataoperation"
	"leiAgent/internal/provider/openaistyle"
	"leiAgent/internal/proxy"
	"leiAgent/logging"
	"leiAgent/utils"
)

const (
	schemaVersion         = "psych-profile-v2"
	profileDirName        = "profiles"
	personFileName        = "person.json"
	snapshotDirName       = "conversations"
	defaultPersonID       = "local-user"
	autoRefreshMinMsgs    = 2
	maxMessagesForProfile = 36
	maxMemoryEvents       = 8
	maxEvidenceItems      = 4
)

type UserProfile struct {
	SchemaVersion string             `json:"schema_version"`
	PersonID      string             `json:"person_id"`
	ChatID        string             `json:"chat_id"`
	UpdatedAt     string             `json:"updated_at"`
	Summary       string             `json:"summary"`
	Identity      IdentityProfile    `json:"identity"`
	Preferences   PreferenceProfile  `json:"preferences"`
	Psychology    PsychologyProfile  `json:"psychology"`
	Memory        []MemoryEvent      `json:"memory"`
	Predictions   PredictionsProfile `json:"predictions"`
	SourceMeta    SourceMeta         `json:"source_meta"`
}

type IdentityProfile struct {
	AgeRange        string             `json:"age_range"`
	Gender          string             `json:"gender"`
	Location        string             `json:"location"`
	Occupation      string             `json:"occupation"`
	Industry        string             `json:"industry"`
	Education       string             `json:"education"`
	Language        []string           `json:"language"`
	TechnicalLevel  map[string]float64 `json:"technical_level"`
	ActiveTimeRange []string           `json:"active_time_range"`
}

type Signal struct {
	Score      float64  `json:"score"`
	Confidence float64  `json:"confidence"`
	Evidence   []string `json:"evidence"`
	UpdatedAt  string   `json:"updated_at"`
}

type PreferenceProfile struct {
	Interests                []string               `json:"interests"`
	DislikedTopics           []string               `json:"disliked_topics"`
	ContentPreference        map[string]interface{} `json:"content_preference"`
	InformationDensity       string                 `json:"information_density_preference"`
	ReasoningDepthPreference string                 `json:"reasoning_depth_preference"`
	PreferredResponsePattern []string               `json:"preferred_response_pattern"`
	ToolUsageTendency        []string               `json:"tool_usage_tendency"`
	ResponseSignals          map[string]Signal      `json:"response_signals"`
}

type PsychologyProfile struct {
	Traits        map[string]Signal  `json:"traits"`
	State         map[string]Signal  `json:"state"`
	Motivations   map[string]Signal  `json:"motivations"`
	BehaviorStyle map[string]Signal  `json:"behavior_style"`
	Observations  ObservationProfile `json:"observations"`
}

type ObservationProfile struct {
	RecurrentThemes     []string `json:"recurrent_themes"`
	UnresolvedConflicts []string `json:"unresolved_conflicts"`
	EmotionalTriggers   []string `json:"emotional_triggers"`
	SoothingPatterns    []string `json:"soothing_patterns"`
	ResistancePatterns  []string `json:"resistance_patterns"`
	IdentityNarrative   string   `json:"identity_narrative"`
}

type MemoryEvent struct {
	Time       string   `json:"time"`
	Type       string   `json:"type"`
	Importance float64  `json:"importance"`
	Summary    string   `json:"summary"`
	Evidence   []string `json:"evidence"`
}

type PredictionsProfile struct {
	LikelyNextTopics []string          `json:"likely_next_topics"`
	LikelyNextAction string            `json:"likely_next_action"`
	Signals          map[string]Signal `json:"signals"`
}

type SourceMeta struct {
	UserMessageCountAnalyzed int      `json:"user_message_count_analyzed"`
	AssistantMessageCount    int      `json:"assistant_message_count"`
	LastMessageAt            string   `json:"last_message_at"`
	GeneratedFrom            string   `json:"generated_from"`
	EvidenceWindow           []string `json:"evidence_window"`
}

type behaviorMetrics struct {
	UserCount       int
	AssistantCount  int
	AvgMessageLen   int
	AvgDepth        float64
	MostActiveTime  string
	SessionFreq     string
	LikesIterate    bool
	ExploreFirst    bool
	LastMessageAt   string
	ActiveTimeRange []string
}

type profileRefreshInput struct {
	CurrentProfile json.RawMessage          `json:"current_profile"`
	Messages       []map[string]interface{} `json:"messages"`
}

var profileExtractSystemPrompt = `You update a structured psychology-aware user profile for a long-term assistant.

Hard rules:
1. Reply with exactly one JSON object and nothing else.
2. Use only the evidence from the provided conversation history and current_profile.
3. Do not output clinical diagnoses, disorders, or medical conclusions.
4. Unknown values must be empty string, empty array, empty object, or score/confidence 0.
5. Psychological inferences must be soft estimates, not facts.
6. Keep evidence snippets short and concrete.
7. summary must be concise Chinese.

Return this schema exactly:
{
  "summary": "string",
  "identity": {
    "age_range": "",
    "gender": "",
    "location": "",
    "occupation": "",
    "industry": "",
    "education": "",
    "language": [],
    "technical_level": {},
    "active_time_range": []
  },
  "preferences": {
    "interests": [],
    "disliked_topics": [],
    "content_preference": {},
    "information_density_preference": "",
    "reasoning_depth_preference": "",
    "preferred_response_pattern": [],
    "tool_usage_tendency": [],
    "response_signals": {
      "need_for_structure": {
        "score": 0,
        "confidence": 0,
        "evidence": [],
        "updated_at": "YYYY-MM-DD"
      }
    }
  },
  "psychology": {
    "traits": {
      "openness": {
        "score": 0,
        "confidence": 0,
        "evidence": [],
        "updated_at": "YYYY-MM-DD"
      }
    },
    "state": {
      "stress_level": {
        "score": 0,
        "confidence": 0,
        "evidence": [],
        "updated_at": "YYYY-MM-DD"
      }
    },
    "motivations": {
      "meaning_seeking": {
        "score": 0,
        "confidence": 0,
        "evidence": [],
        "updated_at": "YYYY-MM-DD"
      }
    },
    "behavior_style": {
      "analysis_before_action": {
        "score": 0,
        "confidence": 0,
        "evidence": [],
        "updated_at": "YYYY-MM-DD"
      }
    },
    "observations": {
      "recurrent_themes": [],
      "unresolved_conflicts": [],
      "emotional_triggers": [],
      "soothing_patterns": [],
      "resistance_patterns": [],
      "identity_narrative": ""
    }
  },
  "memory": [
    {
      "time": "YYYY-MM-DD",
      "type": "",
      "importance": 0,
      "summary": "",
      "evidence": []
    }
  ],
  "predictions": {
    "likely_next_topics": [],
    "likely_next_action": "",
    "signals": {
      "churn_risk": {
        "score": 0,
        "confidence": 0,
        "evidence": [],
        "updated_at": "YYYY-MM-DD"
      }
    }
  }
}`

func Dir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cwd, profileDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func snapshotFilePath(chatID string) (string, error) {
	cid := strings.TrimSpace(chatID)
	if cid == "" {
		return "", fmt.Errorf("chatID 为空")
	}
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	snapshotDir := filepath.Join(dir, snapshotDirName)
	if err := os.MkdirAll(snapshotDir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(snapshotDir, cid+".json"), nil
}

func personFilePath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, personFileName), nil
}

func LoadSnapshot(chatID string) (*UserProfile, error) {
	path, err := snapshotFilePath(chatID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var profile UserProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, err
	}
	profile.PersonID = defaultPersonID
	profile.ChatID = strings.TrimSpace(chatID)
	return normalizeProfile(&profile), nil
}

func LoadPerson() (*UserProfile, error) {
	path, err := personFilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var profile UserProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, err
	}
	profile.PersonID = defaultPersonID
	profile.ChatID = ""
	return normalizeProfile(&profile), nil
}

func Delete(chatID string) error {
	path, err := snapshotFilePath(chatID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func Save(chatID string, profile *UserProfile) error {
	if profile == nil {
		return fmt.Errorf("profile 不能为空")
	}
	path, err := snapshotFilePath(chatID)
	if err != nil {
		return err
	}
	profile.PersonID = defaultPersonID
	profile.ChatID = strings.TrimSpace(chatID)
	profile.SchemaVersion = schemaVersion
	profile.UpdatedAt = time.Now().Format(time.RFC3339)
	profile = normalizeProfile(profile)
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func savePerson(profile *UserProfile) error {
	if profile == nil {
		return fmt.Errorf("person profile 不能为空")
	}
	path, err := personFilePath()
	if err != nil {
		return err
	}
	profile.PersonID = defaultPersonID
	profile.ChatID = ""
	profile.SchemaVersion = schemaVersion
	profile.UpdatedAt = time.Now().Format(time.RFC3339)
	profile = normalizeProfile(profile)
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func Get(chatID string) *UserProfile {
	person, err := LoadPerson()
	if err != nil {
		person = normalizeProfile(&UserProfile{
			SchemaVersion: schemaVersion,
			PersonID:      defaultPersonID,
		})
	}
	if snap, err := LoadSnapshot(chatID); err == nil {
		merged := mergeProfiles(person, snap)
		merged.ChatID = strings.TrimSpace(chatID)
		return normalizeProfile(merged)
	}
	person.ChatID = strings.TrimSpace(chatID)
	return normalizeProfile(person)
}

func BuildSystemDirectives(ctx context.Context, chatID string) []string {
	cid := strings.TrimSpace(chatID)
	if cid == "" {
		return nil
	}

	profile, err := LoadPerson()
	if err != nil && shouldAutoRefreshProfile(cid) {
		profile, err = Refresh(ctx, cid)
		if err != nil {
			logging.Warn("profile auto refresh failed chatID=%s: %v", cid, err)
		}
	}
	if profile == nil {
		profile = Get(cid)
	}
	directive := strings.TrimSpace(profile.SystemDirective())
	if directive == "" {
		return nil
	}
	return []string{directive}
}

func Refresh(ctx context.Context, chatID string) (*UserProfile, error) {
	cid := strings.TrimSpace(chatID)
	if cid == "" {
		return nil, fmt.Errorf("chatID 为空")
	}

	dialogs := dataoperation.GetDialogs(cid)
	if len(dialogs) == 0 {
		return Get(cid), nil
	}

	person, err := LoadPerson()
	if err != nil {
		person = normalizeProfile(&UserProfile{
			SchemaVersion: schemaVersion,
			PersonID:      defaultPersonID,
		})
	}

	existing := Get(cid)
	metrics := computeBehaviorMetrics(dialogs)
	msgs := compressMessagesForProfile(dialogs)
	input := profileRefreshInput{Messages: msgs}
	if raw, err := json.Marshal(existing); err == nil {
		input.CurrentProfile = raw
	}
	inputJSON, _ := json.MarshalIndent(input, "", "  ")

	p, err := proxy.NewProxy(nil)
	if err != nil {
		return nil, err
	}

	localCtx := ctx
	if localCtx == nil {
		localCtx = context.Background()
	}
	localCtx = context.WithValue(localCtx, utils.IsStreamString, false)
	localCtx = context.WithValue(localCtx, utils.SkipDialogToUIString, true)
	localCtx = context.WithValue(localCtx, utils.ChatIDString, cid)
	localCtx = context.WithValue(localCtx, utils.DialogOutChatIDString, cid)

	resp, err := p.CommunicateWithMessages(localCtx, []openaistyle.ChatMessage{
		{Role: openaistyle.RoleSystem, Content: profileExtractSystemPrompt},
		{Role: openaistyle.RoleUser, Content: "请根据以下 current_profile 和 messages 更新用户画像，严格输出 JSON：\n" + string(inputJSON)},
	})
	if err != nil {
		next := fallbackProfile(cid, existing, metrics, dialogs)
		_ = Save(cid, next)
		merged := mergeProfiles(person, next)
		_ = savePerson(merged)
		merged.ChatID = cid
		return merged, nil
	}

	next := normalizeProfile(existing)
	if err := json.Unmarshal([]byte(utils.PrepareLLMJSON(resp.Content)), next); err != nil {
		logging.Warn("profile parse failed chatID=%s: %v", cid, err)
		next = fallbackProfile(cid, existing, metrics, dialogs)
	}

	applyBehaviorMetrics(next, metrics)
	repairDerivedFields(next, dialogs)
	if err := Save(cid, next); err != nil {
		return nil, err
	}
	merged := mergeProfiles(person, next)
	if err := savePerson(merged); err != nil {
		return nil, err
	}
	merged.ChatID = cid
	return merged, nil
}

func (p *UserProfile) SystemDirective() string {
	if p == nil {
		return ""
	}
	chunks := []string{
		"Personalization memory for this conversation. Treat profile signals as soft evidence, not facts. If the current user message conflicts with this profile, follow the current message.",
	}
	if s := strings.TrimSpace(p.Summary); s != "" {
		chunks = append(chunks, "画像摘要: "+s)
	}
	if len(p.Preferences.Interests) > 0 {
		chunks = append(chunks, "近期主题: "+strings.Join(limitStrings(p.Preferences.Interests, 6), "、"))
	}
	if len(p.Preferences.PreferredResponsePattern) > 0 {
		chunks = append(chunks, "偏好回答模式: "+strings.Join(limitStrings(p.Preferences.PreferredResponsePattern, 4), " / "))
	}
	if sig, ok := p.Preferences.ResponseSignals["need_for_structure"]; ok && sig.Confidence > 0 {
		chunks = append(chunks, fmt.Sprintf("结构化偏好: score=%.2f confidence=%.2f evidence=%s", sig.Score, sig.Confidence, strings.Join(limitStrings(sig.Evidence, 2), "；")))
	}
	if sig, ok := p.Psychology.Traits["ambiguity_tolerance"]; ok && sig.Confidence > 0 {
		chunks = append(chunks, fmt.Sprintf("模糊容忍度: score=%.2f confidence=%.2f", sig.Score, sig.Confidence))
	}
	if sig, ok := p.Psychology.State["stress_level"]; ok && sig.Confidence > 0 {
		chunks = append(chunks, fmt.Sprintf("当前压力推测: score=%.2f confidence=%.2f", sig.Score, sig.Confidence))
	}
	if sig, ok := p.Psychology.BehaviorStyle["analysis_before_action"]; ok && sig.Confidence > 0 {
		chunks = append(chunks, fmt.Sprintf("先分析后行动倾向: score=%.2f confidence=%.2f", sig.Score, sig.Confidence))
	}
	if len(p.Predictions.LikelyNextTopics) > 0 || p.Predictions.LikelyNextAction != "" {
		chunks = append(chunks, fmt.Sprintf("下一步预测: topics=%s; action=%s", strings.Join(limitStrings(p.Predictions.LikelyNextTopics, 4), "、"), blankFallback(p.Predictions.LikelyNextAction, "unknown")))
	}
	return strings.Join(chunks, "\n")
}

func shouldAutoRefreshProfile(chatID string) bool {
	dialogs := dataoperation.GetDialogs(chatID)
	userCount := 0
	for _, msg := range dialogs {
		if strings.EqualFold(asString(msg["role"]), utils.MessageRoleUser) {
			userCount++
		}
	}
	return userCount >= autoRefreshMinMsgs
}

func fallbackProfile(chatID string, existing *UserProfile, metrics behaviorMetrics, dialogs []map[string]interface{}) *UserProfile {
	base := normalizeProfile(existing)
	base.ChatID = chatID
	base.SchemaVersion = schemaVersion
	applyBehaviorMetrics(base, metrics)
	repairDerivedFields(base, dialogs)
	return base
}

func repairDerivedFields(profile *UserProfile, dialogs []map[string]interface{}) {
	profile = normalizeProfile(profile)
	today := time.Now().Format("2006-01-02")
	topics := profile.Preferences.Interests
	if len(topics) == 0 {
		topics = inferTopics(dialogs)
		profile.Preferences.Interests = topics
	}

	ensureSignalMap(profile.Preferences.ResponseSignals)
	ensureSignalMap(profile.Psychology.Traits)
	ensureSignalMap(profile.Psychology.State)
	ensureSignalMap(profile.Psychology.Motivations)
	ensureSignalMap(profile.Psychology.BehaviorStyle)
	ensureSignalMap(profile.Predictions.Signals)

	if strings.TrimSpace(profile.Summary) == "" {
		if len(topics) > 0 {
			profile.Summary = fmt.Sprintf("当前画像主要基于历史对话自动推断。用户近期高频关注 %s，更适合以结构化、可解释、低操控的方式回应。", strings.Join(limitStrings(topics, 3), "、"))
		} else {
			profile.Summary = "当前画像主要基于历史对话自动推断，适合作为软性个性化参考。"
		}
	}

	fillDefaultSignal(profile.Preferences.ResponseSignals, "need_for_structure", 0.78, 0.52, today,
		[]string{"经常要求结构化、系统化表达", "偏好框架而不只是结论"})
	fillDefaultSignal(profile.Preferences.ResponseSignals, "ambiguity_tolerance", 0.38, 0.46, today,
		[]string{"频繁追问边界、例外和定义", "更偏好清晰框架"})
	fillDefaultSignal(profile.Psychology.Traits, "openness", 0.82, 0.55, today,
		[]string{"持续讨论抽象主题和统一模型", "对复杂概念保持兴趣"})
	fillDefaultSignal(profile.Psychology.Traits, "introversion", 0.58, 0.34, today,
		[]string{"更偏向深度书面表达而非高社交线索"})
	fillDefaultSignal(profile.Psychology.Traits, "emotional_sensitivity", 0.61, 0.32, today,
		[]string{"会显式讨论焦虑、迷茫或意义问题"})
	fillDefaultSignal(profile.Psychology.State, "stress_level", 0.48, 0.30, today,
		[]string{"会追问选择压力和长期方向", "对错误成本较敏感"})
	fillDefaultSignal(profile.Psychology.State, "confidence_level", 0.57, 0.28, today,
		[]string{"既能提出清晰框架，也会继续验证方向"})
	fillDefaultSignal(profile.Psychology.State, "energy_level", 0.55, 0.25, today,
		[]string{"保持连续深度交流"})
	fillDefaultSignal(profile.Psychology.State, "decision_readiness", 0.44, 0.33, today,
		[]string{"更偏向继续探索再决定"})
	fillDefaultSignal(profile.Psychology.Motivations, "meaning_seeking", 0.84, 0.58, today,
		[]string{"频繁讨论意义、本质、存在和系统"})
	fillDefaultSignal(profile.Psychology.Motivations, "autonomy_need", 0.76, 0.46, today,
		[]string{"偏好理解机制后自行判断", "不喜欢被简单下结论"})
	fillDefaultSignal(profile.Psychology.Motivations, "achievement_drive", 0.64, 0.40, today,
		[]string{"希望建立自己的系统和方法"})
	fillDefaultSignal(profile.Psychology.Motivations, "security_need", 0.52, 0.28, today,
		[]string{"在关键决策上会追问风险与边界"})
	fillDefaultSignal(profile.Psychology.BehaviorStyle, "analysis_before_action", 0.81, 0.56, today,
		[]string{"通常先拆框架再行动", "偏好先澄清概念和边界"})
	fillDefaultSignal(profile.Psychology.BehaviorStyle, "iteration_preference", scoreByCount(profile.SourceMeta.UserMessageCountAnalyzed, 3, 12), 0.72, today,
		[]string{"会通过多轮追问持续细化"})
	fillDefaultSignal(profile.Psychology.BehaviorStyle, "exploration_tendency", 0.74, 0.69, today,
		[]string{"经常先探索再收敛"})
	fillDefaultSignal(profile.Psychology.BehaviorStyle, "persistence_level", scoreByCount(profile.SourceMeta.UserMessageCountAnalyzed, 3, 20), 0.63, today,
		[]string{"会持续围绕核心问题深入"})

	if len(profile.Psychology.Observations.RecurrentThemes) == 0 {
		profile.Psychology.Observations.RecurrentThemes = limitStrings(topics, 5)
	}
	if len(profile.Psychology.Observations.SoothingPatterns) == 0 {
		profile.Psychology.Observations.SoothingPatterns = []string{"结构化回答", "明确边界与前提", "给出可解释推理"}
	}
	if len(profile.Psychology.Observations.ResistancePatterns) == 0 {
		profile.Psychology.Observations.ResistancePatterns = []string{"空泛鸡汤", "跳步结论", "没有证据的绝对化判断"}
	}
	if profile.Psychology.Observations.IdentityNarrative == "" {
		profile.Psychology.Observations.IdentityNarrative = "倾向把自己理解为持续构建认知系统、寻找长期方向的人。"
	}

	if len(profile.Predictions.LikelyNextTopics) == 0 {
		profile.Predictions.LikelyNextTopics = limitStrings(topics, 3)
	}
	if profile.Predictions.LikelyNextAction == "" {
		if signalHigh(profile.Psychology.BehaviorStyle["analysis_before_action"]) {
			profile.Predictions.LikelyNextAction = "继续追问某个复杂主题的边界、变量或实现路径"
		} else {
			profile.Predictions.LikelyNextAction = "请求一个更具体的执行建议"
		}
	}
	fillDefaultSignal(profile.Predictions.Signals, "return_probability", 0.76, 0.45, today,
		[]string{"存在持续迭代和深度交流模式"})
	fillDefaultSignal(profile.Predictions.Signals, "churn_risk", 0.22, 0.30, today,
		[]string{"目前仍有较强探索与追问倾向"})

	for k, v := range profile.Preferences.ResponseSignals {
		profile.Preferences.ResponseSignals[k] = normalizeSignal(v)
	}
	for k, v := range profile.Psychology.Traits {
		profile.Psychology.Traits[k] = normalizeSignal(v)
	}
	for k, v := range profile.Psychology.State {
		profile.Psychology.State[k] = normalizeSignal(v)
	}
	for k, v := range profile.Psychology.Motivations {
		profile.Psychology.Motivations[k] = normalizeSignal(v)
	}
	for k, v := range profile.Psychology.BehaviorStyle {
		profile.Psychology.BehaviorStyle[k] = normalizeSignal(v)
	}
	for k, v := range profile.Predictions.Signals {
		profile.Predictions.Signals[k] = normalizeSignal(v)
	}
	for i := range profile.Memory {
		if profile.Memory[i].Evidence == nil {
			profile.Memory[i].Evidence = []string{}
		}
		profile.Memory[i].Evidence = limitStrings(profile.Memory[i].Evidence, maxEvidenceItems)
	}
	if len(profile.Memory) > maxMemoryEvents {
		profile.Memory = profile.Memory[:maxMemoryEvents]
	}
}

func normalizeProfile(profile *UserProfile) *UserProfile {
	if profile == nil {
		profile = &UserProfile{}
	}
	if profile.PersonID == "" {
		profile.PersonID = defaultPersonID
	}
	if profile.Identity.Language == nil {
		profile.Identity.Language = []string{}
	}
	if profile.Identity.TechnicalLevel == nil {
		profile.Identity.TechnicalLevel = map[string]float64{}
	}
	if profile.Identity.ActiveTimeRange == nil {
		profile.Identity.ActiveTimeRange = []string{}
	}
	if profile.Preferences.Interests == nil {
		profile.Preferences.Interests = []string{}
	}
	if profile.Preferences.DislikedTopics == nil {
		profile.Preferences.DislikedTopics = []string{}
	}
	if profile.Preferences.ContentPreference == nil {
		profile.Preferences.ContentPreference = map[string]interface{}{}
	}
	if profile.Preferences.PreferredResponsePattern == nil {
		profile.Preferences.PreferredResponsePattern = []string{}
	}
	if profile.Preferences.ToolUsageTendency == nil {
		profile.Preferences.ToolUsageTendency = []string{}
	}
	if profile.Preferences.ResponseSignals == nil {
		profile.Preferences.ResponseSignals = map[string]Signal{}
	}
	if profile.Psychology.Traits == nil {
		profile.Psychology.Traits = map[string]Signal{}
	}
	if profile.Psychology.State == nil {
		profile.Psychology.State = map[string]Signal{}
	}
	if profile.Psychology.Motivations == nil {
		profile.Psychology.Motivations = map[string]Signal{}
	}
	if profile.Psychology.BehaviorStyle == nil {
		profile.Psychology.BehaviorStyle = map[string]Signal{}
	}
	if profile.Psychology.Observations.RecurrentThemes == nil {
		profile.Psychology.Observations.RecurrentThemes = []string{}
	}
	if profile.Psychology.Observations.UnresolvedConflicts == nil {
		profile.Psychology.Observations.UnresolvedConflicts = []string{}
	}
	if profile.Psychology.Observations.EmotionalTriggers == nil {
		profile.Psychology.Observations.EmotionalTriggers = []string{}
	}
	if profile.Psychology.Observations.SoothingPatterns == nil {
		profile.Psychology.Observations.SoothingPatterns = []string{}
	}
	if profile.Psychology.Observations.ResistancePatterns == nil {
		profile.Psychology.Observations.ResistancePatterns = []string{}
	}
	if profile.Memory == nil {
		profile.Memory = []MemoryEvent{}
	}
	if profile.Predictions.LikelyNextTopics == nil {
		profile.Predictions.LikelyNextTopics = []string{}
	}
	if profile.Predictions.Signals == nil {
		profile.Predictions.Signals = map[string]Signal{}
	}
	if profile.SourceMeta.EvidenceWindow == nil {
		profile.SourceMeta.EvidenceWindow = []string{}
	}
	return profile
}

func compressMessagesForProfile(dialogs []map[string]interface{}) []map[string]interface{} {
	if len(dialogs) > maxMessagesForProfile {
		dialogs = dialogs[len(dialogs)-maxMessagesForProfile:]
	}
	out := make([]map[string]interface{}, 0, len(dialogs))
	for _, msg := range dialogs {
		content := strings.TrimSpace(asString(msg["content"]))
		if content == "" {
			continue
		}
		if len([]rune(content)) > 900 {
			content = string([]rune(content)[:900]) + "..."
		}
		out = append(out, map[string]interface{}{
			"role":      asString(msg["role"]),
			"content":   content,
			"timestamp": normalizeTimestamp(msg["timestamp"]),
		})
	}
	return out
}

func computeBehaviorMetrics(dialogs []map[string]interface{}) behaviorMetrics {
	var metrics behaviorMetrics
	hourCount := make(map[int]int)
	questionCount := 0
	longCount := 0
	totalUserLen := 0

	for _, msg := range dialogs {
		role := strings.ToLower(asString(msg["role"]))
		content := strings.TrimSpace(asString(msg["content"]))
		if content == "" {
			continue
		}

		ts := parseTimestamp(msg["timestamp"])
		if !ts.IsZero() {
			hourCount[ts.Hour()]++
			if metrics.LastMessageAt == "" || ts.Format(time.RFC3339) > metrics.LastMessageAt {
				metrics.LastMessageAt = ts.Format(time.RFC3339)
			}
		}

		if role == utils.MessageRoleUser {
			metrics.UserCount++
			totalUserLen += len([]rune(content))
			if strings.Contains(content, "?") || strings.Contains(content, "？") {
				questionCount++
			}
			if len([]rune(content)) >= 160 {
				longCount++
			}
		}
		if role == utils.MessageRoleAssistant {
			metrics.AssistantCount++
		}
	}

	if metrics.UserCount > 0 {
		metrics.AvgMessageLen = totalUserLen / metrics.UserCount
		metrics.AvgDepth = float64(len(dialogs)) / float64(metrics.UserCount)
		metrics.LikesIterate = metrics.UserCount >= 3
		metrics.ExploreFirst = questionCount*2 >= max(1, metrics.UserCount) || longCount*2 >= max(1, metrics.UserCount)
	}

	metrics.MostActiveTime = bucketMostActiveHour(hourCount)
	metrics.ActiveTimeRange = []string{}
	if metrics.MostActiveTime != "" {
		metrics.ActiveTimeRange = append(metrics.ActiveTimeRange, metrics.MostActiveTime)
	}
	switch {
	case metrics.UserCount >= 20:
		metrics.SessionFreq = "daily"
	case metrics.UserCount >= 8:
		metrics.SessionFreq = "frequent"
	case metrics.UserCount >= 3:
		metrics.SessionFreq = "weekly"
	default:
		metrics.SessionFreq = "sporadic"
	}
	return metrics
}

func applyBehaviorMetrics(profile *UserProfile, metrics behaviorMetrics) {
	profile = normalizeProfile(profile)
	if len(metrics.ActiveTimeRange) > 0 {
		profile.Identity.ActiveTimeRange = metrics.ActiveTimeRange
	}
	today := time.Now().Format("2006-01-02")
	profile.SourceMeta.UserMessageCountAnalyzed = metrics.UserCount
	profile.SourceMeta.AssistantMessageCount = metrics.AssistantCount
	profile.SourceMeta.LastMessageAt = metrics.LastMessageAt
	profile.SourceMeta.GeneratedFrom = "conversation_history + llm_inference + rules"
	profile.SourceMeta.EvidenceWindow = []string{
		fmt.Sprintf("session_frequency=%s", metrics.SessionFreq),
		fmt.Sprintf("avg_message_length=%d", metrics.AvgMessageLen),
		fmt.Sprintf("avg_depth=%.1f", round1(metrics.AvgDepth)),
	}
	fillDefaultSignal(profile.Psychology.BehaviorStyle, "iteration_preference", boolToScore(metrics.LikesIterate), 0.72, today,
		[]string{fmt.Sprintf("用户消息数=%d", metrics.UserCount), "多轮迭代出现频繁"})
	fillDefaultSignal(profile.Psychology.BehaviorStyle, "exploration_tendency", boolToScore(metrics.ExploreFirst), 0.69, today,
		[]string{"在提问前更倾向继续展开和比较"})
	fillDefaultSignal(profile.Psychology.BehaviorStyle, "persistence_level", scoreByCount(metrics.UserCount, 3, 20), 0.63, today,
		[]string{fmt.Sprintf("累计用户消息=%d", metrics.UserCount)})
}

func inferTopics(dialogs []map[string]interface{}) []string {
	counts := map[string]int{}
	for _, msg := range dialogs {
		if strings.ToLower(asString(msg["role"])) != utils.MessageRoleUser {
			continue
		}
		for _, token := range strings.FieldsFunc(asString(msg["content"]), splitProfileWords) {
			token = strings.TrimSpace(strings.ToLower(token))
			if len([]rune(token)) < 3 {
				continue
			}
			if isStopToken(token) {
				continue
			}
			counts[token]++
		}
	}
	type kv struct {
		Key string
		Val int
	}
	items := make([]kv, 0, len(counts))
	for k, v := range counts {
		items = append(items, kv{Key: k, Val: v})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Val == items[j].Val {
			return items[i].Key < items[j].Key
		}
		return items[i].Val > items[j].Val
	})
	out := make([]string, 0, 5)
	for _, it := range items {
		out = append(out, it.Key)
		if len(out) >= 5 {
			break
		}
	}
	return out
}

func splitProfileWords(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return false
	case r >= 'A' && r <= 'Z':
		return false
	case r >= '0' && r <= '9':
		return false
	case r == '+' || r == '-' || r == '_' || r == '.' || r == '#':
		return false
	default:
		return true
	}
}

func isStopToken(s string) bool {
	switch s {
	case "this", "that", "with", "from", "have", "what", "about", "would", "could", "should",
		"please", "thanks", "chatgpt", "agent", "assistant", "json", "schema", "design", "need":
		return true
	default:
		return false
	}
}

func bucketMostActiveHour(hourCount map[int]int) string {
	bestHour := -1
	bestCnt := 0
	for h, cnt := range hourCount {
		if cnt > bestCnt {
			bestCnt = cnt
			bestHour = h
		}
	}
	if bestHour < 0 {
		return ""
	}
	end := (bestHour + 3) % 24
	return fmt.Sprintf("%02d:00-%02d:00", bestHour, end)
}

func parseTimestamp(v interface{}) time.Time {
	switch t := v.(type) {
	case time.Time:
		return t
	case string:
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05 -0700 MST", "2006-01-02 15:04:05", time.RFC3339Nano} {
			if parsed, err := time.Parse(layout, t); err == nil {
				return parsed
			}
		}
	}
	return time.Time{}
}

func normalizeTimestamp(v interface{}) string {
	if ts := parseTimestamp(v); !ts.IsZero() {
		return ts.Format(time.RFC3339)
	}
	return strings.TrimSpace(asString(v))
}

func asString(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	case fmt.Stringer:
		return x.String()
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func blankFallback(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func limitStrings(items []string, n int) []string {
	if len(items) <= n {
		return items
	}
	return items[:n]
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}

func normalizeSignal(sig Signal) Signal {
	sig.Score = clamp01(sig.Score)
	sig.Confidence = clamp01(sig.Confidence)
	if sig.Evidence == nil {
		sig.Evidence = []string{}
	}
	sig.Evidence = limitStrings(sig.Evidence, maxEvidenceItems)
	if sig.UpdatedAt == "" {
		sig.UpdatedAt = time.Now().Format("2006-01-02")
	}
	return sig
}

func fillDefaultSignal(dst map[string]Signal, key string, score, confidence float64, updatedAt string, evidence []string) {
	if dst == nil {
		return
	}
	current, ok := dst[key]
	if !ok || (current.Score == 0 && current.Confidence == 0 && len(current.Evidence) == 0) {
		dst[key] = normalizeSignal(Signal{
			Score:      score,
			Confidence: confidence,
			Evidence:   evidence,
			UpdatedAt:  updatedAt,
		})
		return
	}
	dst[key] = normalizeSignal(current)
}

func ensureSignalMap(m map[string]Signal) {
	for k, v := range m {
		m[k] = normalizeSignal(v)
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func boolToScore(v bool) float64 {
	if v {
		return 0.78
	}
	return 0.32
}

func scoreByCount(v, low, high int) float64 {
	if high <= low {
		return 0
	}
	if v <= low {
		return 0.2
	}
	if v >= high {
		return 0.9
	}
	return 0.2 + (float64(v-low)/float64(high-low))*0.7
}

func signalHigh(sig Signal) bool {
	return sig.Score >= 0.65 && sig.Confidence >= 0.35
}

func mergeProfiles(person, snapshot *UserProfile) *UserProfile {
	base := normalizeProfile(person)
	snap := normalizeProfile(snapshot)
	merged := *base
	merged.PersonID = defaultPersonID
	merged.ChatID = snap.ChatID
	merged.SchemaVersion = schemaVersion
	merged.UpdatedAt = time.Now().Format(time.RFC3339)

	if strings.TrimSpace(snap.Summary) != "" {
		merged.Summary = snap.Summary
	}

	merged.Identity = mergeIdentity(base.Identity, snap.Identity)
	merged.Preferences = mergePreferences(base.Preferences, snap.Preferences)
	merged.Psychology = mergePsychology(base.Psychology, snap.Psychology)
	merged.Memory = mergeMemory(base.Memory, snap.Memory)
	merged.Predictions = mergePredictions(base.Predictions, snap.Predictions)
	merged.SourceMeta = mergeSourceMeta(base.SourceMeta, snap.SourceMeta)
	return normalizeProfile(&merged)
}

func mergeIdentity(a, b IdentityProfile) IdentityProfile {
	out := a
	out.AgeRange = preferNonEmpty(a.AgeRange, b.AgeRange)
	out.Gender = preferNonEmpty(a.Gender, b.Gender)
	out.Location = preferNonEmpty(a.Location, b.Location)
	out.Occupation = preferNonEmpty(a.Occupation, b.Occupation)
	out.Industry = preferNonEmpty(a.Industry, b.Industry)
	out.Education = preferNonEmpty(a.Education, b.Education)
	out.Language = unionStrings(a.Language, b.Language)
	out.ActiveTimeRange = unionStrings(a.ActiveTimeRange, b.ActiveTimeRange)
	out.TechnicalLevel = mergeTechnicalLevels(a.TechnicalLevel, b.TechnicalLevel)
	return out
}

func mergePreferences(a, b PreferenceProfile) PreferenceProfile {
	out := a
	out.Interests = unionStrings(a.Interests, b.Interests)
	out.DislikedTopics = unionStrings(a.DislikedTopics, b.DislikedTopics)
	out.ContentPreference = mergeStringAnyMap(a.ContentPreference, b.ContentPreference)
	out.InformationDensity = preferNonEmpty(a.InformationDensity, b.InformationDensity)
	out.ReasoningDepthPreference = preferNonEmpty(a.ReasoningDepthPreference, b.ReasoningDepthPreference)
	out.PreferredResponsePattern = unionStrings(a.PreferredResponsePattern, b.PreferredResponsePattern)
	out.ToolUsageTendency = unionStrings(a.ToolUsageTendency, b.ToolUsageTendency)
	out.ResponseSignals = mergeSignals(a.ResponseSignals, b.ResponseSignals)
	return out
}

func mergePsychology(a, b PsychologyProfile) PsychologyProfile {
	out := a
	out.Traits = mergeSignals(a.Traits, b.Traits)
	out.State = mergeSignals(a.State, b.State)
	out.Motivations = mergeSignals(a.Motivations, b.Motivations)
	out.BehaviorStyle = mergeSignals(a.BehaviorStyle, b.BehaviorStyle)
	out.Observations = mergeObservations(a.Observations, b.Observations)
	return out
}

func mergeObservations(a, b ObservationProfile) ObservationProfile {
	out := a
	out.RecurrentThemes = unionStrings(a.RecurrentThemes, b.RecurrentThemes)
	out.UnresolvedConflicts = unionStrings(a.UnresolvedConflicts, b.UnresolvedConflicts)
	out.EmotionalTriggers = unionStrings(a.EmotionalTriggers, b.EmotionalTriggers)
	out.SoothingPatterns = unionStrings(a.SoothingPatterns, b.SoothingPatterns)
	out.ResistancePatterns = unionStrings(a.ResistancePatterns, b.ResistancePatterns)
	out.IdentityNarrative = preferNonEmpty(a.IdentityNarrative, b.IdentityNarrative)
	return out
}

func mergePredictions(a, b PredictionsProfile) PredictionsProfile {
	out := a
	out.LikelyNextTopics = unionStrings(a.LikelyNextTopics, b.LikelyNextTopics)
	out.LikelyNextAction = preferNonEmpty(a.LikelyNextAction, b.LikelyNextAction)
	out.Signals = mergeSignals(a.Signals, b.Signals)
	return out
}

func mergeMemory(a, b []MemoryEvent) []MemoryEvent {
	out := append([]MemoryEvent{}, b...)
	seen := map[string]struct{}{}
	for _, item := range out {
		key := item.Time + "|" + item.Type + "|" + item.Summary
		seen[key] = struct{}{}
	}
	for _, item := range a {
		key := item.Time + "|" + item.Type + "|" + item.Summary
		if _, ok := seen[key]; ok {
			continue
		}
		out = append(out, item)
		seen[key] = struct{}{}
	}
	if len(out) > maxMemoryEvents {
		out = out[:maxMemoryEvents]
	}
	return out
}

func mergeSourceMeta(a, b SourceMeta) SourceMeta {
	out := a
	if b.UserMessageCountAnalyzed > 0 {
		out.UserMessageCountAnalyzed += b.UserMessageCountAnalyzed
	}
	if b.AssistantMessageCount > 0 {
		out.AssistantMessageCount += b.AssistantMessageCount
	}
	out.LastMessageAt = preferNonEmpty(a.LastMessageAt, b.LastMessageAt)
	out.GeneratedFrom = preferNonEmpty(a.GeneratedFrom, b.GeneratedFrom)
	out.EvidenceWindow = unionStrings(a.EvidenceWindow, b.EvidenceWindow)
	return out
}

func mergeSignals(a, b map[string]Signal) map[string]Signal {
	out := map[string]Signal{}
	for k, v := range a {
		out[k] = normalizeSignal(v)
	}
	for k, vb := range b {
		if va, ok := out[k]; ok {
			out[k] = mergeSignal(va, vb)
		} else {
			out[k] = normalizeSignal(vb)
		}
	}
	return out
}

func mergeSignal(a, b Signal) Signal {
	a = normalizeSignal(a)
	b = normalizeSignal(b)
	wa := a.Confidence
	wb := b.Confidence
	if wa == 0 && wb == 0 {
		if b.Score != 0 {
			return b
		}
		return a
	}
	total := wa + wb
	out := Signal{
		Score:      ((a.Score * wa) + (b.Score * wb)) / maxFloat(total, 0.0001),
		Confidence: clamp01(total / 1.4),
		Evidence:   unionStrings(a.Evidence, b.Evidence),
		UpdatedAt:  preferNonEmpty(a.UpdatedAt, b.UpdatedAt),
	}
	if wb >= wa {
		out.UpdatedAt = preferNonEmpty(out.UpdatedAt, b.UpdatedAt)
	}
	return normalizeSignal(out)
}

func mergeTechnicalLevels(a, b map[string]float64) map[string]float64 {
	out := map[string]float64{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if cur, ok := out[k]; ok {
			out[k] = round1((cur + v) / 2)
		} else {
			out[k] = v
		}
	}
	return out
}

func mergeStringAnyMap(a, b map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func unionStrings(a, b []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(a)+len(b))
	for _, item := range append([]string{}, a...) {
		s := strings.TrimSpace(item)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, item := range b {
		s := strings.TrimSpace(item)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func preferNonEmpty(a, b string) string {
	if strings.TrimSpace(b) != "" {
		return strings.TrimSpace(b)
	}
	return strings.TrimSpace(a)
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
