package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	llm "github.com/openpaw/openpaw/internal/llm"
)

// Session is a per-agent-run MCP session. The CLI provider creates one before
// spawning the CLI process and releases it when the run ends. The token is the
// only credential: it is unguessable, single-run, and the endpoint binds to
// the local server. Handlers are the exact closures assembled for the run
// (agent slug / thread ID already baked in), so tool calls are attributed
// correctly without extra plumbing.
type Session struct {
	Token     string
	AgentSlug string
	ThreadID  string
	WorkDir   string
	Tools     []llm.ToolDef
	Handlers  map[string]llm.ToolHandler
	Expires   time.Time

	mu           sync.Mutex
	lastImageURL string
}

func (s *Session) setImageURL(url string) {
	s.mu.Lock()
	s.lastImageURL = url
	s.mu.Unlock()
}

// ImageURL returns the last image URL produced by a generate_image tool call
// during this session (for AgentResult.ImageURL parity with the native loop).
func (s *Session) ImageURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastImageURL
}

type Registry struct {
	sessions sync.Map // token -> *Session
}

func NewRegistry() *Registry {
	return &Registry{}
}

// Create registers a session, assigns it a token, and returns the session.
func (r *Registry) Create(s *Session) *Session {
	buf := make([]byte, 32)
	rand.Read(buf)
	s.Token = hex.EncodeToString(buf)
	if s.Expires.IsZero() {
		s.Expires = time.Now().Add(2 * time.Hour)
	}
	r.sessions.Store(s.Token, s)
	return s
}

func (r *Registry) Get(token string) *Session {
	v, ok := r.sessions.Load(token)
	if !ok {
		return nil
	}
	s := v.(*Session)
	if time.Now().After(s.Expires) {
		r.sessions.Delete(token)
		return nil
	}
	return s
}

func (r *Registry) Release(token string) {
	r.sessions.Delete(token)
}
