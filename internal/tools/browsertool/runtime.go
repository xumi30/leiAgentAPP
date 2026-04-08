package browsertool

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

// chromeSession is one Chrome process + one tab bound to a chat. CDP runs must use a context
// derived from tabCtx (e.g. WithTimeout(tabCtx, d)), never from the HTTP/agent request context.
type chromeSession struct {
	allocCtx    context.Context
	allocCancel context.CancelFunc
	tabCtx      context.Context
	tabCancel   context.CancelFunc
	runMu       sync.Mutex
}

var (
	sessionMu sync.Mutex
	sessions  = make(map[string]*chromeSession)
)

func envHeadlessDefaultTrue() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("LEIAGENT_BROWSER_HEADLESS")))
	if v == "0" || v == "false" || v == "no" {
		return false
	}
	return true
}

func getOrCreateSession(chatID string, headless bool) (*chromeSession, error) {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	if s, ok := sessions[chatID]; ok {
		return s, nil
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", headless),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)
	if p := strings.TrimSpace(os.Getenv("LEIAGENT_CHROME_PATH")); p != "" {
		opts = append(opts, chromedp.ExecPath(p))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	tabCtx, tabCancel := chromedp.NewContext(allocCtx)

	s := &chromeSession{
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		tabCtx:      tabCtx,
		tabCancel:   tabCancel,
	}
	sessions[chatID] = s
	return s, nil
}

func sessionMustExist(chatID string) (*chromeSession, error) {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	s, ok := sessions[chatID]
	if !ok || s == nil {
		return nil, fmt.Errorf("no browser session for this chat; call navigate first")
	}
	return s, nil
}

func closeSession(chatID string) map[string]interface{} {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	s, ok := sessions[chatID]
	if !ok {
		return map[string]interface{}{"action": "close_session", "success": true, "note": "no active session"}
	}
	delete(sessions, chatID)
	if s.tabCancel != nil {
		s.tabCancel()
	}
	if s.allocCancel != nil {
		s.allocCancel()
	}
	return map[string]interface{}{"action": "close_session", "success": true, "closed": true}
}

// runChrome runs chromedp actions on the tab for this chat. Timeout is applied as a child of
// tabCtx only — never use the agent/HTTP context here (it is often already cancelled).
func runChrome(chatID string, timeout time.Duration, actions ...chromedp.Action) error {
	s, err := sessionMustExist(chatID)
	if err != nil {
		return err
	}
	s.runMu.Lock()
	defer s.runMu.Unlock()

	opCtx, cancel := context.WithTimeout(s.tabCtx, timeout)
	defer cancel()
	return chromedp.Run(opCtx, actions...)
}

func validateHTTPURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("invalid url: %q", raw)
	}
	if sch := strings.ToLower(u.Scheme); sch != "http" && sch != "https" {
		return fmt.Errorf("only http/https allowed")
	}
	return nil
}

func errNoAutomationSession() error {
	return fmt.Errorf("no automation session: pass \"url\" in this call (opens automated Chrome) or navigate first; browser_operate does not create this session")
}
