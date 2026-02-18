package browser

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/logger"
)

type ActionRequest struct {
	SessionID string  `json:"session_id"`
	Action    string  `json:"action"`
	Selector  string  `json:"selector,omitempty"`
	Value     string  `json:"value,omitempty"`
	X         float64 `json:"x,omitempty"`
	Y         float64 `json:"y,omitempty"`
	Timeout   int     `json:"timeout,omitempty"`
}

type ActionResult struct {
	Success    bool   `json:"success"`
	Data       string `json:"data,omitempty"`
	URL        string `json:"url,omitempty"`
	Title      string `json:"title,omitempty"`
	Screenshot string `json:"screenshot,omitempty"`
	Error      string `json:"error,omitempty"`
}

func (m *Manager) ExecuteAction(ctx context.Context, req ActionRequest) ActionResult {
	m.mu.RLock()
	s, exists := m.sessions[req.SessionID]
	m.mu.RUnlock()

	if !exists {
		return ActionResult{Error: "session not found"}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.browser == nil || s.page == nil {
		return ActionResult{Error: "session not running"}
	}

	if s.Status == StatusHuman {
		return ActionResult{Error: "session under human control — wait for the human to release control"}
	}

	prevURL := s.CurrentURL
	timeout := time.Duration(req.Timeout) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	s.Status = StatusBusy
	m.updateSessionStatus(s)

	result := m.executeOnPage(s.page, req, timeout)

	info, err := s.page.Info()
	if err == nil {
		s.CurrentURL = info.URL
		s.CurrentTitle = info.Title
	}

	s.Status = StatusActive
	m.updateSessionStatus(s)

	result.URL = s.CurrentURL
	result.Title = s.CurrentTitle

	m.logAction(req, result, s.ID, prevURL)

	return result
}

func (m *Manager) executeOnPage(page *rod.Page, req ActionRequest, timeout time.Duration) ActionResult {
	switch req.Action {
	case "navigate":
		if req.Value == "" {
			return ActionResult{Error: "value (URL) required for navigate"}
		}
		err := page.Timeout(timeout).Navigate(req.Value)
		if err != nil {
			return ActionResult{Error: fmt.Sprintf("navigate failed: %v", err)}
		}
		page.Timeout(timeout).WaitLoad()
		return ActionResult{Success: true, Data: "Navigated to " + req.Value}

	case "click":
		if req.Selector != "" {
			el, err := page.Timeout(timeout).Element(req.Selector)
			if err != nil {
				return ActionResult{Error: fmt.Sprintf("element not found: %v", err)}
			}
			err = el.Click(proto.InputMouseButtonLeft, 1)
			if err != nil {
				return ActionResult{Error: fmt.Sprintf("click failed: %v", err)}
			}
			return ActionResult{Success: true, Data: "Clicked " + req.Selector}
		}
		if req.X > 0 || req.Y > 0 {
			err := page.Mouse.MoveTo(proto.Point{X: req.X, Y: req.Y})
			if err != nil {
				return ActionResult{Error: fmt.Sprintf("mouse move failed: %v", err)}
			}
			err = page.Mouse.Click(proto.InputMouseButtonLeft, 1)
			if err != nil {
				return ActionResult{Error: fmt.Sprintf("click failed: %v", err)}
			}
			return ActionResult{Success: true, Data: fmt.Sprintf("Clicked at (%.0f, %.0f)", req.X, req.Y)}
		}
		return ActionResult{Error: "selector or coordinates required for click"}

	case "type":
		if req.Selector != "" {
			el, err := page.Timeout(timeout).Element(req.Selector)
			if err != nil {
				return ActionResult{Error: fmt.Sprintf("element not found: %v", err)}
			}
			el.SelectAllText()
			err = el.Input(req.Value)
			if err != nil {
				return ActionResult{Error: fmt.Sprintf("type failed: %v", err)}
			}
			return ActionResult{Success: true, Data: "Typed into " + req.Selector}
		}
		err := page.InsertText(req.Value)
		if err != nil {
			return ActionResult{Error: fmt.Sprintf("keyboard type failed: %v", err)}
		}
		return ActionResult{Success: true, Data: "Typed text"}

	case "screenshot":
		quality := 60
		screenshot, err := page.Screenshot(true, &proto.PageCaptureScreenshot{
			Format:  proto.PageCaptureScreenshotFormatJpeg,
			Quality: &quality,
		})
		if err != nil {
			return ActionResult{Error: fmt.Sprintf("screenshot failed: %v", err)}
		}
		encoded := base64.StdEncoding.EncodeToString(screenshot)
		return ActionResult{Success: true, Screenshot: encoded, Data: "Screenshot captured"}

	case "extract_text":
		if req.Selector != "" {
			el, err := page.Timeout(timeout).Element(req.Selector)
			if err != nil {
				return ActionResult{Error: fmt.Sprintf("element not found: %v", err)}
			}
			text, err := el.Text()
			if err != nil {
				return ActionResult{Error: fmt.Sprintf("extract failed: %v", err)}
			}
			return ActionResult{Success: true, Data: text}
		}
		text, err := page.Eval(`() => document.body.innerText`)
		if err != nil {
			return ActionResult{Error: fmt.Sprintf("extract failed: %v", err)}
		}
		return ActionResult{Success: true, Data: text.Value.String()}

	case "wait_element":
		if req.Selector == "" {
			return ActionResult{Error: "selector required for wait_element"}
		}
		_, err := page.Timeout(timeout).Element(req.Selector)
		if err != nil {
			return ActionResult{Error: fmt.Sprintf("wait failed: %v", err)}
		}
		return ActionResult{Success: true, Data: "Element found: " + req.Selector}

	case "scroll":
		x := req.X
		y := req.Y
		if y == 0 {
			y = 300
		}
		_, err := page.Eval(fmt.Sprintf(`() => window.scrollBy(%f, %f)`, x, y))
		if err != nil {
			return ActionResult{Error: fmt.Sprintf("scroll failed: %v", err)}
		}
		return ActionResult{Success: true, Data: fmt.Sprintf("Scrolled by (%.0f, %.0f)", x, y)}

	case "back":
		err := page.NavigateBack()
		if err != nil {
			return ActionResult{Error: fmt.Sprintf("back failed: %v", err)}
		}
		return ActionResult{Success: true, Data: "Navigated back"}

	case "forward":
		err := page.NavigateForward()
		if err != nil {
			return ActionResult{Error: fmt.Sprintf("forward failed: %v", err)}
		}
		return ActionResult{Success: true, Data: "Navigated forward"}

	case "eval":
		if req.Value == "" {
			return ActionResult{Error: "value (JavaScript) required for eval"}
		}
		result, err := page.Eval(req.Value)
		if err != nil {
			return ActionResult{Error: fmt.Sprintf("eval failed: %v", err)}
		}
		return ActionResult{Success: true, Data: result.Value.String()}

	case "key_press":
		if req.Value == "" {
			return ActionResult{Error: "value (key) required for key_press"}
		}
		err := page.Keyboard.Press(keyFromString(req.Value))
		if err != nil {
			return ActionResult{Error: fmt.Sprintf("key press failed: %v", err)}
		}
		return ActionResult{Success: true, Data: "Pressed " + req.Value}

	default:
		return ActionResult{Error: "unknown action: " + req.Action}
	}
}

func keyFromString(s string) input.Key {
	switch s {
	case "Enter":
		return input.Enter
	case "Tab":
		return input.Tab
	case "Escape":
		return input.Escape
	case "Backspace":
		return input.Backspace
	case "ArrowUp":
		return input.ArrowUp
	case "ArrowDown":
		return input.ArrowDown
	case "ArrowLeft":
		return input.ArrowLeft
	case "ArrowRight":
		return input.ArrowRight
	case "Space":
		return input.Space
	default:
		if len(s) == 1 {
			return input.Key(rune(s[0]))
		}
		return input.Enter
	}
}

func (m *Manager) logAction(req ActionRequest, result ActionResult, sessionID, urlBefore string) {
	id := uuid.New().String()
	success := 0
	if result.Success {
		success = 1
	}
	errStr := result.Error

	_, err := m.db.Exec(
		"INSERT INTO browser_action_log (id, session_id, action, selector, value, success, error, url_before, url_after, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		id, sessionID, req.Action, req.Selector, req.Value, success, errStr, urlBefore, result.URL, time.Now().UTC(),
	)
	if err != nil {
		logger.Error("Failed to log browser action: %v", err)
	}

	m.broadcast("browser_action_log", map[string]interface{}{
		"session_id": sessionID,
		"action":     req.Action,
		"selector":   req.Selector,
		"value":      req.Value,
		"success":    result.Success,
		"error":      errStr,
		"timestamp":  time.Now().UTC(),
	})
}
