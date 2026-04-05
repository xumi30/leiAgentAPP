package noveltool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"leiAgent/internal/doclib"
	"leiAgent/internal/memory"
	"leiAgent/internal/proxy"
	"leiAgent/internal/tools"
	"leiAgent/logging"
	"leiAgent/utils"
)

const (
	maxChapters       = 30
	defaultChapters   = 6
	tailPrevRunes     = 3500
	maxBibleRunes     = 14000
	maxMetaInPrompt   = 4000
	milestoneEvery    = 10
	fileNovelBible    = "novel_bible.md"
	fileReaderNotes   = "reader_milestones.md"
	fileAuthorLog     = "author_log.md"
	fileOutline       = "outline.md"
	dirChapterMeta    = "meta" // 章浓缩 JSON：meta/chapter_XX.meta.json
	maxTitleSlugRunes = 72    // 文件名中标题 slug 长度上限
)

// novelChapterMDRe 匹配 chapter_03.md 或 chapter_03_标题slug.md（避免 chapter_01 误匹配 chapter_012）。
var novelChapterMDRe = regexp.MustCompile(`(?i)^chapter_(\d+)(?:_[^.]+)?\.md$`)

// LongFormNovelTool 在规划模式下由一步调用：内部多轮 LLM，分文件写入 workspace，不把长篇塞进规划 JSON。
type LongFormNovelTool struct{}

func New() tools.Tool {
	return &LongFormNovelTool{}
}

func (t *LongFormNovelTool) Name() string {
	return "novel_longform"
}

func (t *LongFormNovelTool) Description() string {
	return `Write or continue long-form fiction under workspace/: chapter_XX[_标题slug].md (slug from model chapter_title), rolling novel_bible.md, per-chapter JSON under meta/, author_log.md, and reader_milestones.md every 10 chapters.
Use the same output_dir + resume=true to continue tomorrow with new ideas (author_notes). premise each time can restate direction plus new beats.
Parameters: premise (required), outline (optional), chapter_count (chapters to write this run, 1–30, default 6), output_dir (default novels/story_<unix>), resume (continue existing folder), author_notes (new ideas this session), refresh_outline (regenerate outline when resuming), style, language.`
}

func (t *LongFormNovelTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"premise": map[string]interface{}{
				"type":        "string",
				"description": "Story direction: first run = core idea; resume = today's continuation goals plus any new beats (keep concise).",
			},
			"outline": map[string]interface{}{
				"type":        "string",
				"description": "Optional outline. If empty on a new project, the tool generates one. On resume, existing outline.md is kept unless refresh_outline is true.",
			},
			"chapter_count": map[string]interface{}{
				"type":        "integer",
				"description": fmt.Sprintf("Chapters to write in this invocation (1–%d). Default %d. On resume, adds this many chapters after the last existing file.", maxChapters, defaultChapters),
			},
			"output_dir": map[string]interface{}{
				"type":        "string",
				"description": "Directory under workspace/ (relative, no ..). Default: novels/story_<timestamp>. Reuse for continuation.",
			},
			"resume": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, continue from the last chapter_*.md in output_dir; load bible and meta/*.json. Same folder as a writer's project.",
			},
			"author_notes": map[string]interface{}{
				"type":        "string",
				"description": "Optional: new ideas, tone shifts, or reminders for this session; appended to author_log.md with timestamp.",
			},
			"refresh_outline": map[string]interface{}{
				"type":        "boolean",
				"description": "If true with resume, regenerates outline from premise (overwrites outline.md). Use sparingly.",
			},
			"style": map[string]interface{}{
				"type":        "string",
				"description": "Optional narrative style (e.g. literary, light novel, suspense).",
			},
			"language": map[string]interface{}{
				"type":        "string",
				"description": "Optional language name (e.g. 简体中文). If omitted, follow the premise language.",
			},
		},
		"required": []string{"premise"},
	}
}

func (t *LongFormNovelTool) Results() map[string]interface{} {
	return map[string]interface{}{
		"type":        "object",
		"description": "JSON string: output_dir, files written, chapters_written, chapter_range, resume_used.",
	}
}

func (t *LongFormNovelTool) Execute(ctx context.Context, args string) (string, error) {
	var params struct {
		Premise         string `json:"premise"`
		Outline         string `json:"outline"`
		ChapterCount    int    `json:"chapter_count"`
		OutputDir       string `json:"output_dir"`
		Resume          bool   `json:"resume"`
		AuthorNotes     string `json:"author_notes"`
		RefreshOutline  bool   `json:"refresh_outline"`
		Style           string `json:"style"`
		Language        string `json:"language"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("invalid JSON args: %w", err)
	}
	params.Premise = strings.TrimSpace(params.Premise)
	if params.Premise == "" {
		return "", fmt.Errorf("premise is required")
	}
	nThisRun := params.ChapterCount
	if nThisRun <= 0 {
		nThisRun = defaultChapters
	}
	if nThisRun > maxChapters {
		nThisRun = maxChapters
	}

	root, err := doclib.LibraryRootAbs()
	if err != nil {
		return "", err
	}
	outRel := strings.TrimSpace(params.OutputDir)
	if outRel == "" {
		outRel = fmt.Sprintf("novels/story_%d", time.Now().Unix())
	}
	outAbs, err := doclib.SafeLibraryAbs(root, outRel)
	if err != nil {
		return "", fmt.Errorf("output_dir: %w", err)
	}
	if err := os.MkdirAll(outAbs, 0755); err != nil {
		return "", fmt.Errorf("mkdir output: %w", err)
	}
	metaAbs := filepath.Join(outAbs, dirChapterMeta)
	if err := os.MkdirAll(metaAbs, 0755); err != nil {
		return "", fmt.Errorf("mkdir meta: %w", err)
	}

	baseChatID, ok := ctx.Value(utils.ChatIDString).(string)
	if !ok || baseChatID == "" {
		return "", fmt.Errorf("chatID missing in context")
	}
	ephemeral := baseChatID + "__novel_" + strconv.FormatInt(time.Now().UnixNano(), 10)

	lastWritten := findLastChapterIndex(outAbs)
	if params.Resume && lastWritten < 1 {
		return "", fmt.Errorf("resume=true but no chapter_*.md found under %s", outRel)
	}

	startCh := 1
	if params.Resume {
		startCh = lastWritten + 1
	}
	if startCh > maxChapters {
		return "", fmt.Errorf("already at or beyond max chapter index %d", maxChapters)
	}
	allowed := maxChapters - startCh + 1
	if nThisRun > allowed {
		nThisRun = allowed
	}
	endCh := startCh + nThisRun - 1

	if notes := strings.TrimSpace(params.AuthorNotes); notes != "" {
		if err := appendAuthorLog(outAbs, notes); err != nil {
			return "", err
		}
	}

	outline := strings.TrimSpace(params.Outline)
	outlinePath := filepath.Join(outAbs, fileOutline)
	if params.Resume && !params.RefreshOutline {
		if outline == "" {
			if b, err := os.ReadFile(outlinePath); err == nil {
				outline = strings.TrimSpace(string(b))
			}
		}
	}
	if outline == "" {
		logging.Info("novel_longform: generating outline (%d chapters span ending %d)", nThisRun, endCh)
		outline, err = t.oneShotLLM(ctx, baseChatID, ephemeral+"_o", outlineSystemPrompt(params.Style, params.Language, nThisRun, endCh),
			outlineUserPrompt(params.Premise, nThisRun, startCh, endCh, params.Resume))
		if err != nil {
			return "", fmt.Errorf("outline generation: %w", err)
		}
		outline = normalizeModelText(outline)
	}
	if err := os.WriteFile(outlinePath, []byte(outline), 0644); err != nil {
		return "", fmt.Errorf("write outline: %w", err)
	}
	doclib.Register(outlinePath)

	bible := strings.TrimSpace(loadTextFile(filepath.Join(outAbs, fileNovelBible)))
	if !params.Resume && bible == "" {
		bible = "# 小说圣经（滚动）\n\n- 人物：\n- 人物关系：\n- 主线：\n- 设定 / 伏笔：\n"
	}

	outlineTotal := max(countOutlineChapters(outline), endCh)

	files := []string{filepath.ToSlash(filepath.Join(outRel, fileOutline))}
	if params.AuthorNotes != "" {
		files = append(files, filepath.ToSlash(filepath.Join(outRel, fileAuthorLog)))
	}

	var prevTail string
	var prevMetaJSON string
	if startCh > 1 {
		prevPath := resolveChapterMarkdownPath(outAbs, startCh-1)
		if prevBody, err := os.ReadFile(prevPath); err == nil {
			prevTail = tailRunes(string(prevBody), tailPrevRunes)
		}
		prevMetaJSON = readChapterMetaJSON(outAbs, startCh-1)
	}

	authorBlock := strings.TrimSpace(loadTextFile(filepath.Join(outAbs, fileAuthorLog)))
	if len([]rune(authorBlock)) > maxMetaInPrompt {
		authorBlock = string([]rune(authorBlock)[len([]rune(authorBlock))-maxMetaInPrompt:])
	}

	for ch := startCh; ch <= endCh; ch++ {
		logging.Info("novel_longform: writing chapter %d (batch ends %d)", ch, endCh)
		user := chapterUserPrompt(chapterUserParams{
			Premise:       params.Premise,
			Outline:       outline,
			Chapter:       ch,
			Total:         outlineTotal,
			PrevTail:      prevTail,
			PrevMetaJSON:  prevMetaJSON,
			NovelBible:    truncateRunes(bible, maxBibleRunes),
			AuthorLogTail: authorBlock,
			Style:         params.Style,
			Lang:          params.Language,
			Resume:        params.Resume,
		})
		raw, err := t.oneShotLLM(ctx, baseChatID, fmt.Sprintf("%s_c%d", ephemeral, ch),
			chapterSystemPrompt(params.Style, params.Language, ch, outlineTotal), user)
		if err != nil {
			return "", fmt.Errorf("chapter %d: %w", ch, err)
		}
		body, metaJSON := splitChapterResponse(raw)
		body = normalizeModelText(body)
		if metaJSON == "" {
			metaJSON = `{"chapter_title":"","chapter_summary":"","key_events":"","characters_in_scene":[],"hooks_open_threads":"","notes":""}`
		}

		var metaObj map[string]interface{}
		if err := json.Unmarshal([]byte(metaJSON), &metaObj); err != nil {
			metaObj = map[string]interface{}{"raw_meta": metaJSON, "parse_error": err.Error()}
		}
		titleStr := ""
		if v, ok := metaObj["chapter_title"].(string); ok {
			titleStr = strings.TrimSpace(v)
		}
		titleSlug := filenameSlugFromTitle(titleStr, maxTitleSlugRunes)
		if err := removeExistingChapterMarkdown(outAbs, ch); err != nil {
			return "", fmt.Errorf("chapter %d: clear old files: %w", ch, err)
		}
		chName := chapterMarkdownFileName(ch, titleSlug)
		chPath := filepath.Join(outAbs, chName)
		if err := os.WriteFile(chPath, []byte(body), 0644); err != nil {
			return "", fmt.Errorf("write %s: %w", chName, err)
		}
		doclib.Register(chPath)
		files = append(files, filepath.ToSlash(filepath.Join(outRel, chName)))

		metaPath := chapterMetaPath(outAbs, ch)
		metaObj["chapter"] = ch
		if titleSlug != "" {
			metaObj["filename_slug"] = titleSlug
		}
		metaObj["written_at"] = time.Now().UTC().Format(time.RFC3339)
		metaBytes, _ := json.MarshalIndent(metaObj, "", "  ")
		if err := os.WriteFile(metaPath, metaBytes, 0644); err != nil {
			return "", fmt.Errorf("write meta chapter %d: %w", ch, err)
		}
		doclib.Register(metaPath)
		if tt, ok := metaObj["chapter_title"].(string); ok && strings.TrimSpace(tt) != "" {
			logging.Info("novel_longform: chapter %d title: %s", ch, strings.TrimSpace(tt))
		}
		files = append(files, filepath.ToSlash(filepath.Join(outRel, dirChapterMeta, filepath.Base(metaPath))))

		bible, err = t.oneShotLLM(ctx, baseChatID, fmt.Sprintf("%s_b%d", ephemeral, ch),
			bibleUpdateSystemPrompt(params.Language),
			bibleUpdateUserPrompt(truncateRunes(bible, maxBibleRunes), ch, metaJSON, truncateRunes(body, 6000), params.Premise))
		if err != nil {
			return "", fmt.Errorf("bible update after chapter %d: %w", ch, err)
		}
		bible = normalizeModelText(bible)
		if err := os.WriteFile(filepath.Join(outAbs, fileNovelBible), []byte(bible), 0644); err != nil {
			return "", fmt.Errorf("write bible: %w", err)
		}
		doclib.Register(filepath.Join(outAbs, fileNovelBible))

		prevTail = tailRunes(body, tailPrevRunes)
		prevMetaJSON = strings.TrimSpace(string(metaBytes))

		if ch%milestoneEvery == 0 && !readerMilestoneExists(outAbs, ch) {
			if err := t.appendReaderMilestone(ctx, baseChatID, ephemeral, outAbs, ch, bible); err != nil {
				return "", err
			}
			files = append(files, filepath.ToSlash(filepath.Join(outRel, fileReaderNotes)))
		}
	}

	summary, _ := json.Marshal(map[string]interface{}{
		"tool":             t.Name(),
		"output_dir":       filepath.ToSlash(outRel),
		"files":            files,
		"chapters_written": nThisRun,
		"chapter_range":    []int{startCh, endCh},
		"resume":           params.Resume,
		"premise_excerpt":  truncateRunes(params.Premise, 200),
	})
	return string(summary), nil
}

func (t *LongFormNovelTool) appendReaderMilestone(ctx context.Context, uiChatID, ephemeral, outAbs string, upToCh int, bible string) error {
	from := upToCh - milestoneEvery + 1
	var metas []string
	for c := from; c <= upToCh; c++ {
		if s := readChapterMetaJSON(outAbs, c); s != "" {
			metas = append(metas, fmt.Sprintf("### Chapter %d meta\n%s", c, s))
		}
	}
	text, err := t.oneShotLLM(ctx, uiChatID, ephemeral+fmt.Sprintf("_r%d", upToCh),
		readerMilestoneSystemPrompt(),
		readerMilestoneUserPrompt(from, upToCh, truncateRunes(bible, maxBibleRunes), strings.Join(metas, "\n\n")))
	if err != nil {
		return fmt.Errorf("reader milestone %d–%d: %w", from, upToCh, err)
	}
	text = normalizeModelText(text)
	block := fmt.Sprintf("\n\n---\n\n## 读者笔记 · 第 %d-%d 章 · %s\n\n%s\n",
		from, upToCh, time.Now().UTC().Format("2006-01-02 15:04 UTC"), text)
	path := filepath.Join(outAbs, fileReaderNotes)
	var old []byte
	if old, err = os.ReadFile(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.WriteFile(path, append(old, []byte(block)...), 0644); err != nil {
		return err
	}
	doclib.Register(path)
	return nil
}

func readerMilestoneExists(outAbs string, upToCh int) bool {
	from := upToCh - milestoneEvery + 1
	b, err := os.ReadFile(filepath.Join(outAbs, fileReaderNotes))
	if err != nil {
		return false
	}
	marker := fmt.Sprintf("## 读者笔记 · 第 %d-%d 章", from, upToCh)
	return strings.Contains(string(b), marker)
}

func (t *LongFormNovelTool) oneShotLLM(ctx context.Context, uiChatID, memSuffix, system, user string) (string, error) {
	memID := memSuffix
	memory.GetLocalMemory().Clear(memID)
	memory.SetSystemPrompt(memID, system)
	memory.AddUserMessage(memID, user)

	sub := context.WithValue(ctx, utils.ChatIDString, memID)
	sub = context.WithValue(sub, utils.DialogOutChatIDString, uiChatID)
	sub = context.WithValue(sub, utils.IsStreamString, true)
	sub = context.WithValue(sub, utils.SkipPersistAssistantRoundString, true)

	p, err := proxy.NewProxy(nil)
	if err != nil {
		return "", err
	}
	tc, err := p.Communicate(sub)
	if err != nil {
		return "", err
	}
	if tc == nil || strings.TrimSpace(tc.Content) == "" {
		return "", fmt.Errorf("empty model response")
	}
	return tc.Content, nil
}

func outlineSystemPrompt(style, lang string, span, endCh int) string {
	var b strings.Builder
	b.WriteString("You are a fiction editor. Produce a detailed chapter-by-chapter outline.\n")
	b.WriteString("Output Markdown only: use ## Chapter N — Title and bullet beats; no JSON.\n")
	b.WriteString(fmt.Sprintf("The outline must cover chapters 1 through %d (global indices). If the user is continuing a serial, align new beats with their premise and do not contradict prior files.\n", endCh))
	b.WriteString(fmt.Sprintf("This batch needs a coherent arc across the next %d chapter(s) ending at chapter %d.\n", span, endCh))
	if strings.TrimSpace(style) != "" {
		b.WriteString("Narrative style hint: " + style + "\n")
	}
	if strings.TrimSpace(lang) != "" {
		b.WriteString("Language: " + lang + "\n")
	} else {
		b.WriteString("Use the same language as the user's premise.\n")
	}
	b.WriteString("Keep each chapter section concise but actionable for a writer.")
	return b.String()
}

func outlineUserPrompt(premise string, span, startCh, endCh int, resume bool) string {
	var b strings.Builder
	b.WriteString("Premise / continuation brief:\n")
	b.WriteString(premise)
	b.WriteString("\n\n")
	if resume {
		b.WriteString(fmt.Sprintf("This is a RESUME: next chapter index will be %d; outline should still list chapters 1–%d where earlier chapters already exist on disk.\n", startCh, endCh))
	} else {
		b.WriteString(fmt.Sprintf("New project: chapters 1–%d.\n", endCh))
	}
	b.WriteString(fmt.Sprintf("Produce the full %d-chapter outline (indices 1–%d) now.", endCh, endCh))
	return b.String()
}

func chapterSystemPrompt(style, lang string, chapter, total int) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("You are a novelist writing chapter %d of %d.\n", chapter, total))
	b.WriteString("You MUST output exactly two blocks in this order:\n")
	b.WriteString("1) First line: ---CHAPTER---\n")
	b.WriteString("2) Chapter body in Markdown (dialogue and paragraphs). The first line of the body MUST be exactly one H1 that includes the chapter index AND a distinctive chapter title taken from the outline for this chapter: e.g. `# 第3章 · 雨夜追踪` or `# Chapter 3 — Tracking in the Rain`. The title part must match `chapter_title` in the JSON below.\n")
	b.WriteString("3) A line: ---META---\n")
	b.WriteString("4) One JSON object (no markdown fence) with keys: chapter_title (short string, same as in the H1), chapter_summary (string), key_events (string), characters_in_scene (array of strings), hooks_open_threads (string), notes (string, optional).\n")
	b.WriteString("Stay consistent with outline, novel bible, previous chapter meta, and the tail of the previous chapter.\n")
	if strings.TrimSpace(style) != "" {
		b.WriteString("Style: " + style + "\n")
	}
	if strings.TrimSpace(lang) != "" {
		b.WriteString("Language: " + lang + "\n")
	} else {
		b.WriteString("Match the language of the premise and outline.\n")
	}
	return b.String()
}

type chapterUserParams struct {
	Premise, Outline       string
	Chapter, Total         int
	PrevTail, PrevMetaJSON string
	NovelBible             string
	AuthorLogTail          string
	Style, Lang            string
	Resume                 bool
}

func chapterUserPrompt(p chapterUserParams) string {
	var b strings.Builder
	b.WriteString("=== Premise / session brief ===\n")
	b.WriteString(p.Premise)
	if p.Resume {
		b.WriteString("\n(Continuing an on-disk project — align with bible and prior metas.)\n")
	}
	b.WriteString("\n\n=== Full outline ===\n")
	b.WriteString(p.Outline)
	b.WriteString("\n\n=== Novel bible (rolling) ===\n")
	b.WriteString(p.NovelBible)
	b.WriteString("\n\n")
	if strings.TrimSpace(p.AuthorLogTail) != "" {
		b.WriteString("=== Author log (recent notes & ideas) ===\n")
		b.WriteString(p.AuthorLogTail)
		b.WriteString("\n\n")
	}
	if strings.TrimSpace(p.PrevMetaJSON) != "" {
		b.WriteString("=== Previous chapter meta (JSON) ===\n")
		b.WriteString(p.PrevMetaJSON)
		b.WriteString("\n\n")
	}
	if strings.TrimSpace(p.PrevTail) != "" {
		b.WriteString("=== End of previous chapter (excerpt) ===\n")
		b.WriteString(p.PrevTail)
		b.WriteString("\n\n")
	}
	b.WriteString(fmt.Sprintf("Write **chapter %d of %d** now. Choose a concrete chapter_title consistent with the outline section for this chapter. Follow the ---CHAPTER--- / ---META--- format exactly.\n", p.Chapter, p.Total))
	if strings.TrimSpace(p.Style) != "" {
		b.WriteString("Style hint: " + p.Style + "\n")
	}
	if strings.TrimSpace(p.Lang) != "" {
		b.WriteString("Language: " + p.Lang + "\n")
	}
	return b.String()
}

func bibleUpdateSystemPrompt(lang string) string {
	s := "You are a story-bible editor. Merge new chapter information into a single Markdown document.\n" +
		"Keep sections: 人物, 人物关系, 主线进度 (include chapter index and chapter_title when provided), 设定与伏笔, 已揭示信息. Update or append; do not drop unresolved hooks unless closed this chapter.\n" +
		"Output Markdown only, no fences.\n"
	if strings.TrimSpace(lang) != "" {
		s += "Use language: " + lang + " for headings if the project is in that language; otherwise follow the manuscript language.\n"
	}
	return s
}

func bibleUpdateUserPrompt(oldBible string, chapter int, metaJSON, chapterExcerpt, premise string) string {
	return fmt.Sprintf("Current bible:\n%s\n\nChapter %d meta JSON:\n%s\n\nExcerpt from chapter body (may truncate):\n%s\n\nAuthor premise this run:\n%s\n\nRewrite the full updated bible as one coherent Markdown file.",
		oldBible, chapter, metaJSON, chapterExcerpt, premise)
}

func readerMilestoneSystemPrompt() string {
	return "You are an avid fiction reader who has just finished the stated chapter range. Write a concise reader-style note in the same language as the metas.\n" +
		"Include: (1) plot recap, (2) character impressions, (3) pacing/tone reading feel, (4) expectations and hopes for what happens next, (5) worries or confusion if any.\n" +
		"Plain Markdown, no JSON, no ---CHAPTER--- markers."
}

func readerMilestoneUserPrompt(from, to int, bible string, packedMetas string) string {
	return fmt.Sprintf("Chapters finished: %d–%d.\n\nNovel bible snapshot:\n%s\n\nChapter metas:\n%s\n\nWrite the reader note.",
		from, to, bible, packedMetas)
}

func chapterMetaPath(outAbs string, ch int) string {
	return filepath.Join(outAbs, dirChapterMeta, fmt.Sprintf("chapter_%02d.meta.json", ch))
}

func legacyChapterMetaPath(outAbs string, ch int) string {
	return filepath.Join(outAbs, fmt.Sprintf("chapter_%02d.meta.json", ch))
}

// readChapterMetaJSON 优先读 meta/，兼容旧版项目根目录下的 .meta.json。
func readChapterMetaJSON(outAbs string, ch int) string {
	p := chapterMetaPath(outAbs, ch)
	if b, err := os.ReadFile(p); err == nil {
		return strings.TrimSpace(string(b))
	}
	if b, err := os.ReadFile(legacyChapterMetaPath(outAbs, ch)); err == nil {
		return strings.TrimSpace(string(b))
	}
	return ""
}

func splitChapterResponse(raw string) (body string, metaJSON string) {
	raw = strings.TrimSpace(raw)
	const ch = "---CHAPTER---"
	const me = "---META---"
	i := strings.Index(raw, ch)
	j := strings.Index(raw, me)
	if i >= 0 && j > i {
		body = strings.TrimSpace(raw[i+len(ch):j])
		metaJSON = strings.TrimSpace(raw[j+len(me):])
	} else {
		body = strings.TrimSpace(raw)
		metaJSON = ""
	}
	metaJSON = strings.TrimPrefix(metaJSON, "```json")
	metaJSON = strings.TrimPrefix(metaJSON, "```JSON")
	metaJSON = strings.TrimPrefix(metaJSON, "```")
	metaJSON = strings.TrimSpace(metaJSON)
	if cut := strings.LastIndex(metaJSON, "```"); cut >= 0 {
		metaJSON = strings.TrimSpace(metaJSON[:cut])
	}
	return body, metaJSON
}

func novelChapterIndexFromName(name string) (int, bool) {
	m := novelChapterMDRe.FindStringSubmatch(name)
	if len(m) != 2 {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

func removeExistingChapterMarkdown(outAbs string, ch int) error {
	entries, err := os.ReadDir(outAbs)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n, ok := novelChapterIndexFromName(e.Name())
		if !ok || n != ch {
			continue
		}
		path := filepath.Join(outAbs, e.Name())
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func resolveChapterMarkdownPath(outAbs string, ch int) string {
	entries, err := os.ReadDir(outAbs)
	if err != nil {
		return filepath.Join(outAbs, fmt.Sprintf("chapter_%02d.md", ch))
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n, ok := novelChapterIndexFromName(e.Name())
		if ok && n == ch {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return filepath.Join(outAbs, fmt.Sprintf("chapter_%02d.md", ch))
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	return filepath.Join(outAbs, names[0])
}

func chapterMarkdownFileName(ch int, titleSlug string) string {
	if titleSlug == "" {
		return fmt.Sprintf("chapter_%02d.md", ch)
	}
	return fmt.Sprintf("chapter_%02d_%s.md", ch, titleSlug)
}

func filenameSlugFromTitle(title string, maxRunes int) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range title {
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '　':
			b.WriteByte('_')
		case r < 32 || strings.ContainsRune(`<>:"/\|?*`, r):
			continue
		default:
			b.WriteRune(r)
		}
	}
	s := strings.Trim(b.String(), "._")
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}
	rr := []rune(s)
	if len(rr) > maxRunes {
		s = string(rr[:maxRunes])
		s = strings.TrimRight(s, "._")
	}
	s = strings.TrimSpace(s)
	if s == "" || s == "_" {
		return ""
	}
	return s
}

func findLastChapterIndex(outAbs string) int {
	entries, err := os.ReadDir(outAbs)
	if err != nil {
		return 0
	}
	maxN := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n, ok := novelChapterIndexFromName(e.Name())
		if !ok {
			continue
		}
		if n > maxN {
			maxN = n
		}
	}
	return maxN
}

func countOutlineChapters(outline string) int {
	re := regexp.MustCompile(`(?m)^##\s+`)
	n := len(re.FindAllString(outline, -1))
	if n < 1 {
		return 1
	}
	return n
}

func loadTextFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func appendAuthorLog(outAbs, notes string) error {
	path := filepath.Join(outAbs, fileAuthorLog)
	line := fmt.Sprintf("\n## %s\n\n%s\n", time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(notes))
	var old []byte
	if b, err := os.ReadFile(path); err == nil {
		old = b
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.WriteFile(path, append(old, []byte(line)...), 0644); err != nil {
		return err
	}
	doclib.Register(path)
	return nil
}

func normalizeModelText(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	rest := strings.TrimPrefix(s, "```")
	if idx := strings.IndexByte(rest, '\n'); idx >= 0 {
		rest = rest[idx+1:]
	}
	rest = strings.TrimSpace(rest)
	if cut := strings.LastIndex(rest, "```"); cut >= 0 {
		rest = strings.TrimSpace(rest[:cut])
	}
	return rest
}

func tailRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[len(r)-maxRunes:])
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
