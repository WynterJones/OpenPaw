package browser

import (
	"encoding/base64"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

func (m *Manager) screenshotLoop(s *Session) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.screenshotStop:
			return
		case <-ticker.C:
			m.captureAndBroadcast(s)
		}
	}
}

func (m *Manager) captureAndBroadcast(s *Session) {
	// Capture the page reference under the lock, then release before taking the screenshot.
	s.mu.Lock()
	if s.page == nil || s.browser == nil {
		s.mu.Unlock()
		return
	}
	status := s.Status
	if status != StatusActive && status != StatusBusy && status != StatusHuman {
		s.mu.Unlock()
		return
	}
	page := s.page
	sessionID := s.ID
	s.mu.Unlock()

	// Take the screenshot outside the lock to avoid blocking other operations.
	quality := 60
	screenshot, err := page.Screenshot(true, &proto.PageCaptureScreenshot{
		Format:  proto.PageCaptureScreenshotFormatJpeg,
		Quality: &quality,
	})
	if err != nil {
		return
	}

	info, _ := page.Info()
	url := ""
	title := ""
	if info != nil {
		url = info.URL
		title = info.Title
	}

	// Re-acquire lock to store results.
	s.mu.Lock()
	s.lastScreenshot = screenshot
	if info != nil {
		s.CurrentURL = url
		s.CurrentTitle = title
	}
	s.mu.Unlock()

	encoded := base64.StdEncoding.EncodeToString(screenshot)

	payload := map[string]interface{}{
		"session_id":      sessionID,
		"image":           encoded,
		"url":             url,
		"title":           title,
		"viewport_width":  1280,
		"viewport_height": 900,
		"timestamp":       time.Now().UTC(),
	}

	// Send screenshots only to clients subscribed to this browser session's topic.
	// Falls back to global broadcast if topic broadcasting is not configured.
	if m.topicBroadcast != nil {
		m.topicBroadcast("browser:"+sessionID, "browser_screenshot", payload)
	} else {
		m.broadcast("browser_screenshot", payload)
	}
}
