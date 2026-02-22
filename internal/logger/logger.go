package logger

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/openpaw/openpaw/internal/netutil"
	"rsc.io/qr"
)

var (
	mu      sync.Mutex
	noColor bool
)

const (
	reset   = "\033[0m"
	bold    = "\033[1m"
	dim     = "\033[2m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	magenta = "\033[35m"
	cyan    = "\033[36m"
	white   = "\033[37m"

	brightRed     = "\033[91m"
	brightMagenta = "\033[95m"

	// Pink shades (256-color)
	pink     = "\033[38;5;205m"
	hotPink  = "\033[38;5;199m"
	softPink = "\033[38;5;218m"
)

func init() {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		noColor = true
	}
}

func c(code, text string) string {
	if noColor {
		return text
	}
	return code + text + reset
}

func ts() string {
	return c(dim, time.Now().Format("15:04:05"))
}

func write(format string, args ...interface{}) {
	mu.Lock()
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	mu.Unlock()
}

func Banner() {
	lines := "\n" +
		"  " + c(magenta, `/\_/\`) + "\n" +
		" " + c(magenta, `( o.o )`) + "  " + c(bold+brightMagenta, "OpenPaw") + "\n" +
		"  " + c(magenta, `> ^ <`) + "   " + c(dim, "Ready to pounce!") + "\n" +
		c(dim, " ─────────────────────────────────") + "\n"
	mu.Lock()
	fmt.Fprint(os.Stderr, lines)
	mu.Unlock()
}

func Info(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	write("%s  %s  %s", ts(), c(cyan, "~"), msg)
}

func Success(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	write("%s  %s  %s", ts(), c(green, "✓"), msg)
}

func Warn(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	write("%s  %s  %s", ts(), c(yellow, "⚠"), c(yellow, msg))
}

func Error(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	write("%s  %s  %s", ts(), c(red, "✗"), c(red, msg))
}

func Fatal(format string, args ...interface{}) {
	Error(format, args...)
	os.Exit(1)
}

func WS(event, detail string) {
	var icon, eventColor string
	switch event {
	case "connected":
		icon = c(hotPink, "⚡")
		eventColor = hotPink
	case "disconnected":
		icon = c(softPink, "·")
		eventColor = softPink
	default:
		icon = c(pink, "↔")
		eventColor = pink
	}
	write("%s  %s %s %s",
		ts(),
		icon,
		c(eventColor, fmt.Sprintf("%-14s", "ws:"+event)),
		c(magenta, detail),
	)
}

func Listen(addr, url string, port int) {
	write("")
	write("%s  %s  Listening on %s", ts(), c(brightMagenta, "🐾"), c(bold+white, addr))
	write("              %s  %s", c(dim, "→"), c(cyan, url))

	if lanIP := netutil.GetLANIP(); lanIP != "" {
		lanURL := fmt.Sprintf("http://%s:%d", lanIP, port)
		write("              %s  %s", c(dim, "→"), c(cyan, lanURL))
		write("")
		printPinkQR(lanURL)
		write("              %s", c(dim, "Scan to connect from your phone"))
	}
	write("")
}

func printPinkQR(url string) {
	code, err := qr.Encode(url, qr.L)
	if err != nil {
		return
	}

	size := code.Size
	quiet := 1
	full := size + quiet*2

	black := func(x, y int) bool {
		qx, qy := x-quiet, y-quiet
		if qx < 0 || qy < 0 || qx >= size || qy >= size {
			return false
		}
		return code.Black(qx, qy)
	}

	pinkFg := "\033[38;5;205m"
	rst := "\033[0m"

	for y := 0; y < full; y += 2 {
		line := ""
		for x := 0; x < full; x++ {
			top := black(x, y)
			bot := y+1 < full && black(x, y+1)

			switch {
			case top && bot:
				line += "█"
			case top:
				line += "▀"
			case bot:
				line += "▄"
			default:
				line += " "
			}
		}
		mu.Lock()
		fmt.Fprintf(os.Stderr, "              %s%s%s\n", pinkFg, line, rst)
		mu.Unlock()
	}
}

func Shutdown(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	write("")
	write("%s  %s  %s", ts(), "😿", c(dim, msg))
}

func Bye() {
	write("%s  %s  %s", ts(), c(dim, "~"), c(dim, "Stopped. See you soon!"))
	write("")
}

func HTTP(method, path string, status int, dur time.Duration) {
	statusStr := fmt.Sprintf("%d", status)
	var coloredStatus string
	switch {
	case status >= 500:
		coloredStatus = "\033[41;97m " + statusStr + " \033[0m"
	case status >= 400:
		coloredStatus = "\033[41;97m " + statusStr + " \033[0m"
	default:
		coloredStatus = c(dim+pink, statusStr)
	}

	mc := pink
	switch method {
	case "POST", "PUT", "PATCH":
		mc = hotPink
	case "DELETE":
		mc = brightRed
	}

	write("%s  %s %s %s %s",
		ts(),
		c(mc, "["+method+"]"),
		coloredStatus,
		c(dim, path),
		c(dim, fmtDuration(dur)),
	)
}

func fmtDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%dµs", d.Microseconds())
	case d < time.Second:
		ms := float64(d.Microseconds()) / 1000.0
		if ms < 10 {
			return fmt.Sprintf("%.1fms", ms)
		}
		return fmt.Sprintf("%.0fms", ms)
	default:
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}
