package openclawtool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"leiAgent/internal/proxy"
	"leiAgent/utils"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

const defaultClawdRegistryBaseURL = "https://backend.clawd.org.cn/api"

// InstallOpenClawSkillFromMarket searches the ClawHub registry and installs a skill into ./skills.
// It wraps proxy.InstallOpenClawSkill so the model can do "install skill X" by query.
type InstallOpenClawSkillFromMarket struct {
	httpClient *http.Client
}

func NewInstallOpenClawSkillFromMarket(httpClient *http.Client) *InstallOpenClawSkillFromMarket {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &InstallOpenClawSkillFromMarket{httpClient: httpClient}
}

func (t *InstallOpenClawSkillFromMarket) Name() string {
	return "install_openclaw_skill_from_market"
}

func (t *InstallOpenClawSkillFromMarket) Description() string {
	return `Search the OpenClaw/ClawHub skill marketplace and install a skill into the current workspace ./skills directory.

Workflow:
1) Search registry for query (or use explicit slug).
2) Pick the best match.
3) Call the existing controlled installer (ClawHub npx or clawd fallback).
4) Return install result + refreshed installed skills list.`
}

func (t *InstallOpenClawSkillFromMarket) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search keyword, e.g. 'baidu search'. Ignored if slug is provided.",
			},
			"slug": map[string]interface{}{
				"type":        "string",
				"description": "Exact skill slug. If provided, search step is skipped.",
			},
			"force": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, append --force to the install input (use when safety policy blocks install).",
				"default":     false,
			},
			"registry_base_url": map[string]interface{}{
				"type":        "string",
				"description": "Optional ClawHub registry base URL override (defaults to env CLAWHUB_REGISTRY or https://backend.clawd.org.cn/api).",
			},
		},
	}
}

func (t *InstallOpenClawSkillFromMarket) Results() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"ok":            map[string]interface{}{"type": "boolean"},
			"slug":          map[string]interface{}{"type": "string"},
			"picked_reason": map[string]interface{}{"type": "string"},
			"install":       map[string]interface{}{"type": "object"},
			"skill_state":   map[string]interface{}{"type": "object"},
			"warning":       map[string]interface{}{"type": "string"},
		},
	}
}

func (t *InstallOpenClawSkillFromMarket) SimpleInfo() map[string]string {
	return utils.SimpleInfoMap("OpenClaw Skills", "从 ClawHub 市场搜索并自动安装 skill 到当前工作区。")
}

func (t *InstallOpenClawSkillFromMarket) Execute(ctx context.Context, args string) (string, error) {
	params := map[string]interface{}{}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", err
	}

	query := strings.TrimSpace(asString(params["query"]))
	rawSlug := strings.TrimSpace(asString(params["slug"]))
	force := asBool(params["force"])
	registryBase := strings.TrimSpace(asString(params["registry_base_url"]))
	if registryBase == "" {
		registryBase = strings.TrimSpace(os.Getenv("CLAWHUB_REGISTRY"))
	}
	if registryBase == "" {
		registryBase = defaultClawdRegistryBaseURL
	}

	pickedReason := ""
	chosenSlug := ""
	searchTries := []string{}

	// Build candidate slugs/queries for better hit rate.
	slugCandidates := normalizeSkillSlugCandidates(rawSlug)
	queryCandidates := normalizeSkillQueryCandidates(query, rawSlug)

	// If user provided a slug-like input, try installing by slug directly (with normalization variants).
	if len(slugCandidates) > 0 {
		pickedReason = "explicit_slug"
		for i, cand := range slugCandidates {
			input := cand
			if force {
				input = fmt.Sprintf("install %s --force", cand)
			}
			installRes, err := proxy.InstallOpenClawSkill(ctx, input)
			if err == nil {
				chosenSlug = cand
				pickedReason = fmt.Sprintf("slug_candidate[%d]", i)
				out, _ := json.MarshalIndent(map[string]interface{}{
					"ok":            true,
					"slug":          chosenSlug,
					"picked_reason": pickedReason,
					"install":       installRes,
					"skill_state":   proxy.GetOpenClawSkillState(),
				}, "", "  ")
				return string(out), nil
			}
		}
		// fallthrough to search-based install
	}

	// Search-based install (either slug was absent, or direct install failed).
	if len(queryCandidates) == 0 {
		return "", fmt.Errorf("query 或 slug 至少提供一个")
	}
	var lastInstall interface{}
	var lastErr error
	for qi, q := range queryCandidates {
		searchTries = append(searchTries, q)
		results, err := searchSkills(ctx, t.httpClient, registryBase, q)
		if err != nil {
			lastErr = err
			continue
		}
		best, reason := pickBestSkillSearchResult(results, q, slugCandidates)
		if strings.TrimSpace(best.Slug) == "" {
			lastErr = fmt.Errorf("未搜索到可安装的 skill：%q", q)
			continue
		}
		chosenSlug = best.Slug
		pickedReason = fmt.Sprintf("search[%d]:%s", qi, reason)

		input := chosenSlug
		if force {
			input = fmt.Sprintf("install %s --force", chosenSlug)
		}
		installRes, err := proxy.InstallOpenClawSkill(ctx, input)
		lastInstall = installRes
		if err == nil {
			out, _ := json.MarshalIndent(map[string]interface{}{
				"ok":            true,
				"slug":          chosenSlug,
				"picked_reason": pickedReason,
				"install":       installRes,
				"skill_state":   proxy.GetOpenClawSkillState(),
				"search_tries":  searchTries,
			}, "", "  ")
			return string(out), nil
		}
		lastErr = err
	}

	// Failed: surface attempts for debugging/prompting the model next time.
	out, _ := json.MarshalIndent(map[string]interface{}{
		"ok":              false,
		"slug":            chosenSlug,
		"picked_reason":   pickedReason,
		"install":         lastInstall,
		"skill_state":     proxy.GetOpenClawSkillState(),
		"warning":         "install_failed",
		"slug_candidates": slugCandidates,
		"query_candidates": func() []string {
			// cap to keep response compact
			if len(queryCandidates) > 8 {
				return queryCandidates[:8]
			}
			return queryCandidates
		}(),
		"search_tries": searchTries,
	}, "", "  ")
	if lastErr == nil {
		lastErr = fmt.Errorf("install failed")
	}
	return string(out), lastErr
}

type skillSearchResponse struct {
	Results []skillSearchItem `json:"results"`
}

type clawdSkillSearchItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	UpdatedAt   string `json:"updated_at"`
}

type skillSearchItem struct {
	Score       float64 `json:"score"`
	Slug        string  `json:"slug"`
	DisplayName string  `json:"displayName"`
	Summary     string  `json:"summary"`
	UpdatedAt   int64   `json:"updatedAt"`
}

func searchSkills(ctx context.Context, client *http.Client, registryBase, query string) ([]skillSearchItem, error) {
	base := strings.TrimRight(strings.TrimSpace(registryBase), "/")
	if base == "" {
		return nil, fmt.Errorf("registry_base_url 不能为空")
	}
	if isClawdAPIRegistry(base) {
		return searchClawdSkills(ctx, client, base, query)
	}
	return searchClawHubSkills(ctx, client, base, query)
}

func searchClawHubSkills(ctx context.Context, client *http.Client, base, query string) ([]skillSearchItem, error) {
	u, err := url.Parse(base + "/api/v1/search")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("q", query)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("skill market search failed: http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed skillSearchResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("skill market search json parse failed: %w", err)
	}
	return parsed.Results, nil
}

func searchClawdSkills(ctx context.Context, client *http.Client, base, query string) ([]skillSearchItem, error) {
	searchURL := strings.TrimRight(base, "/")
	if !strings.HasSuffix(searchURL, "/api") {
		searchURL += "/api"
	}
	u, err := url.Parse(searchURL + "/skills")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("q", query)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("skill market search failed: http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed []clawdSkillSearchItem
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("skill market search json parse failed: %w", err)
	}
	items := make([]skillSearchItem, 0, len(parsed))
	normalizedQuery := normalizeLoose(query)
	for _, item := range parsed {
		slug := strings.TrimSpace(item.ID)
		if slug == "" {
			continue
		}
		score := 1.0
		if normalizedQuery != "" {
			switch {
			case normalizeLoose(slug) == normalizedQuery || normalizeLoose(item.Name) == normalizedQuery:
				score = 100
			case strings.Contains(normalizeLoose(slug), normalizedQuery):
				score = 90
			case strings.Contains(normalizeLoose(item.Name), normalizedQuery):
				score = 80
			case strings.Contains(normalizeLoose(item.Description), normalizedQuery):
				score = 50
			}
		}
		items = append(items, skillSearchItem{
			Score:       score,
			Slug:        slug,
			DisplayName: item.Name,
			Summary:     item.Description,
		})
	}
	return items, nil
}

func isClawdAPIRegistry(base string) bool {
	u, err := url.Parse(base)
	if err != nil {
		return strings.Contains(base, "backend.clawd.org.cn")
	}
	return strings.EqualFold(u.Host, "backend.clawd.org.cn") || strings.HasSuffix(strings.TrimRight(u.Path, "/"), "/api")
}

func pickBestSkillSearchResult(items []skillSearchItem, query string, preferredSlugs []string) (skillSearchItem, string) {
	if len(items) == 0 {
		return skillSearchItem{}, "empty"
	}

	// Prefer exact slug matches if we have preferred candidates.
	if len(preferredSlugs) > 0 {
		for _, ps := range preferredSlugs {
			ps = strings.TrimSpace(ps)
			if ps == "" {
				continue
			}
			for _, it := range items {
				if strings.EqualFold(strings.TrimSpace(it.Slug), ps) {
					return it, "exact_slug_match"
				}
			}
		}
	}

	normalizedQuery := normalizeLoose(query)
	// Prefer "contains" slug/displayName match when query looks like a slug-ish token.
	if normalizedQuery != "" {
		for _, it := range items {
			if normalizeLoose(it.Slug) == normalizedQuery || strings.Contains(normalizeLoose(it.Slug), normalizedQuery) {
				return it, "slug_contains_query"
			}
		}
		for _, it := range items {
			if strings.Contains(normalizeLoose(it.DisplayName), normalizedQuery) {
				return it, "name_contains_query"
			}
		}
	}

	// Deterministic: choose highest score, tie-break by updatedAt desc, then slug asc.
	cp := append([]skillSearchItem(nil), items...)
	sort.Slice(cp, func(i, j int) bool {
		if cp[i].Score != cp[j].Score {
			return cp[i].Score > cp[j].Score
		}
		if cp[i].UpdatedAt != cp[j].UpdatedAt {
			return cp[i].UpdatedAt > cp[j].UpdatedAt
		}
		return strings.ToLower(cp[i].Slug) < strings.ToLower(cp[j].Slug)
	})
	return cp[0], "best_score"
}

func asString(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	default:
		return fmt.Sprintf("%v", v)
	}
}

func asBool(v interface{}) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "1", "true", "yes", "y", "on":
			return true
		default:
			return false
		}
	case float64:
		return x != 0
	default:
		return false
	}
}

func normalizeSkillSlugCandidates(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	// strip quotes
	raw = strings.Trim(raw, `"'`)
	raw = strings.TrimSpace(raw)

	// Turn "irre-nnn/comic skills" -> "irre-nnn/comic skills" (kept) then split.
	// If it's "owner/repo" style, prefer repo segment.
	candidates := []string{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		s = strings.Trim(s, `"'`)
		if s == "" {
			return
		}
		s = strings.ToLower(s)
		s = strings.TrimSpace(s)
		for _, bad := range []string{" skill", " skills", "-skill", "-skills", "_skill", "_skills", ".skill", ".skills"} {
			s = strings.ReplaceAll(s, bad, "")
		}
		s = strings.Trim(s, "-_./ ")
		if s == "" {
			return
		}
		// "irre-nnn/comic-skills" -> try also "comic-skills"
		candidates = append(candidates, s)
	}

	add(raw)
	// If contains '/', try last segment.
	if strings.Contains(raw, "/") {
		parts := strings.Split(raw, "/")
		last := strings.TrimSpace(parts[len(parts)-1])
		add(last)
	}
	// Replace separators with dash variants.
	loose := strings.NewReplacer(" ", "-", "_", "-", ".", "-", "/", "-").Replace(raw)
	add(loose)
	add(strings.TrimSuffix(loose, "-skills"))
	add(strings.TrimSuffix(loose, "-skill"))

	// Dedupe while preserving order.
	seen := map[string]struct{}{}
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}

func normalizeSkillQueryCandidates(query string, rawSlug string) []string {
	query = strings.TrimSpace(query)
	rawSlug = strings.TrimSpace(rawSlug)

	cands := []string{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		s = strings.Trim(s, `"'`)
		if s == "" {
			return
		}
		// remove common filler words in CN/EN prompts
		for _, bad := range []string{"帮我安装", "帮我装", "安装", "install", "skills", "skill"} {
			s = strings.ReplaceAll(s, bad, " ")
		}
		s = strings.Join(strings.Fields(s), " ")
		if s == "" {
			return
		}
		cands = append(cands, s)
	}

	if query != "" {
		add(query)
	}
	if rawSlug != "" {
		add(rawSlug)
		if strings.Contains(rawSlug, "/") {
			parts := strings.Split(rawSlug, "/")
			add(parts[len(parts)-1])
		}
		// turn separators into spaces for search query
		add(strings.NewReplacer("/", " ", "-", " ", "_", " ", ".", " ").Replace(rawSlug))
	}

	// Dedupe and cap.
	seen := map[string]struct{}{}
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		key := strings.ToLower(strings.TrimSpace(c))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, c)
	}
	if len(out) > 12 {
		out = out[:12]
	}
	return out
}

func normalizeLoose(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	replacer := strings.NewReplacer(" ", "", "-", "", "_", "", ".", "", "/", "", ":", "")
	return replacer.Replace(s)
}
