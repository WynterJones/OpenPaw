package platform

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// OpenBrowser opens the given URL in the user's default browser.
// On macOS, it first attempts to activate an existing browser tab matching the URL.
func OpenBrowser(url string) {
	switch runtime.GOOS {
	case "darwin":
		if activateExistingTab(url) {
			return
		}
		exec.Command("open", url).Start()
	case "linux":
		exec.Command("xdg-open", url).Start()
	case "windows":
		exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}
}

// activateExistingTab uses AppleScript to find and activate an existing browser
// tab matching the URL in Chrome or Safari. Returns true if a tab was found.
func activateExistingTab(url string) bool {
	script := fmt.Sprintf(`
		tell application "System Events"
			set browserList to {"Google Chrome", "Safari"}
			repeat with browserName in browserList
				if (name of processes) contains browserName then
					if browserName is "Google Chrome" then
						tell application "Google Chrome"
							set found to false
							repeat with w in windows
								set tabIndex to 0
								repeat with t in tabs of w
									set tabIndex to tabIndex + 1
									if URL of t starts with "%s" then
										set active tab index of w to tabIndex
										set index of w to 1
										activate
										set found to true
										exit repeat
									end if
								end repeat
								if found then exit repeat
							end repeat
							if found then return "found"
						end tell
					else if browserName is "Safari" then
						tell application "Safari"
							set found to false
							repeat with w in windows
								set tabIndex to 0
								repeat with t in tabs of w
									set tabIndex to tabIndex + 1
									if URL of t starts with "%s" then
										set current tab of w to t
										set index of w to 1
										activate
										set found to true
										exit repeat
									end if
								end repeat
								if found then exit repeat
							end repeat
							if found then return "found"
						end tell
					end if
				end if
			end repeat
		end tell
		return "not_found"`, url, url)

	out, err := exec.Command("osascript", "-e", script).Output()
	return err == nil && strings.TrimSpace(string(out)) == "found"
}
