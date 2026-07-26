package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/openpaw/openpaw/internal/media"
)

// stubProvider is a Provider that reports fixed capabilities. Nothing here
// reaches the network — these tests are about what the agent is told it can do.
type stubProvider struct {
	name       string
	configured bool
	kinds      []media.Kind
}

func (s stubProvider) Name() string       { return s.name }
func (s stubProvider) Configured() bool   { return s.configured }
func (s stubProvider) Kinds() []media.Kind { return s.kinds }
func (s stubProvider) Models(_ context.Context, _ media.Kind) ([]media.Model, error) {
	return nil, nil
}
func (s stubProvider) Generate(_ context.Context, _ media.Request) (*media.Asset, error) {
	return nil, nil
}

func registryWith(providers ...media.Provider) *media.Registry {
	r := media.NewRegistry()
	for _, p := range providers {
		r.Register(p)
	}
	return r
}

func TestBuildStudioToolDefs_NilRegistryYieldsNoTools(t *testing.T) {
	if defs := BuildStudioToolDefs(nil); len(defs) != 0 {
		t.Fatalf("got %d defs for a nil registry, want 0", len(defs))
	}
}

// With no configured provider the agent can still browse, but must not be
// offered a generate tool that could only ever fail.
func TestBuildStudioToolDefs_OmitsGenerateWhenNothingConfigured(t *testing.T) {
	reg := registryWith(stubProvider{name: "replicate", configured: false, kinds: []media.Kind{media.KindImage}})

	names := map[string]bool{}
	for _, d := range BuildStudioToolDefs(reg) {
		names[d.Function.Name] = true
	}

	if !names["studio_list_folders"] || !names["studio_list_media"] {
		t.Errorf("browse tools should always be present, got %v", names)
	}
	if names["studio_generate"] {
		t.Error("studio_generate was offered with no configured provider")
	}
}

func TestBuildStudioToolDefs_SchemasAreValid(t *testing.T) {
	reg := registryWith(stubProvider{
		name: "replicate", configured: true,
		kinds: []media.Kind{media.KindImage, media.KindVideo, media.KindAudio},
	})

	defs := BuildStudioToolDefs(reg)
	if len(defs) != 3 {
		t.Fatalf("got %d defs, want 3", len(defs))
	}

	for _, d := range defs {
		if d.Type != "function" {
			t.Errorf("%s: type = %q, want function", d.Function.Name, d.Type)
		}
		if d.Function.Description == "" {
			t.Errorf("%s: empty description", d.Function.Name)
		}
		// A malformed schema is rejected by the provider and takes every other
		// tool on the run down with it.
		var schema map[string]interface{}
		if err := json.Unmarshal(d.Function.Parameters, &schema); err != nil {
			t.Errorf("%s: parameters are not valid JSON: %v", d.Function.Name, err)
		}
		if schema["type"] != "object" {
			t.Errorf("%s: schema type = %v, want object", d.Function.Name, schema["type"])
		}
	}
}

// The enum must reflect what is actually configured, or the model will happily
// request a kind that cannot be produced.
func TestBuildStudioGenerateDef_EnumTracksConfiguredKinds(t *testing.T) {
	reg := registryWith(stubProvider{
		name: "openrouter", configured: true, kinds: []media.Kind{media.KindImage},
	})

	def := buildStudioGenerateDef(reg)
	var schema struct {
		Properties struct {
			Type struct {
				Enum []string `json:"enum"`
			} `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(def.Function.Parameters, &schema); err != nil {
		t.Fatalf("bad schema: %v", err)
	}

	got := schema.Properties.Type.Enum
	if len(got) != 1 || got[0] != "image" {
		t.Errorf("enum = %v, want [image] only", got)
	}
}

// Generation costs real money, so the description has to tell the model to
// offer options rather than spend on its own initiative.
func TestBuildStudioGenerateDef_WarnsAboutCost(t *testing.T) {
	reg := registryWith(stubProvider{name: "fal", configured: true, kinds: []media.Kind{media.KindVideo}})
	desc := strings.ToLower(buildStudioGenerateDef(reg).Function.Description)

	for _, want := range []string{"money", "options"} {
		if !strings.Contains(desc, want) {
			t.Errorf("description missing %q: %s", want, desc)
		}
	}
}

func TestBuildStudioPromptSection_ExplainsMissingProviders(t *testing.T) {
	reg := registryWith(stubProvider{name: "replicate", configured: false, kinds: []media.Kind{media.KindVideo}})
	section := buildStudioPromptSection(reg)

	if !strings.Contains(section, "No media provider is configured") {
		t.Errorf("unconfigured section should say so, got: %s", section)
	}
	if !strings.Contains(section, "Settings") {
		t.Error("section should point at where to fix it")
	}
}

func TestBuildStudioPromptSection_ListsConfiguredKinds(t *testing.T) {
	reg := registryWith(stubProvider{
		name: "replicate", configured: true,
		kinds: []media.Kind{media.KindImage, media.KindAudio},
	})
	section := buildStudioPromptSection(reg)

	for _, want := range []string{"image", "audio", "studio_generate"} {
		if !strings.Contains(section, want) {
			t.Errorf("section missing %q", want)
		}
	}
	if strings.Contains(section, "No media provider") {
		t.Error("configured registry should not render the empty-state text")
	}
}

func TestBuildStudioPromptSection_NilRegistryIsEmpty(t *testing.T) {
	if s := buildStudioPromptSection(nil); s != "" {
		t.Errorf("nil registry should render nothing, got %q", s)
	}
}

func TestHandleStudioGenerate_RequiresRegistry(t *testing.T) {
	m := &Manager{}
	res := m.handleStudioGenerate("thread-1")(context.Background(), "", json.RawMessage(`{"type":"image","prompt":"x"}`))
	if !res.IsError {
		t.Fatalf("expected an error result with no registry, got %+v", res)
	}
}

func TestHandleStudioGenerate_RejectsEmptyPrompt(t *testing.T) {
	m := &Manager{MediaRegistry: registryWith(stubProvider{
		name: "openrouter", configured: true, kinds: []media.Kind{media.KindImage},
	})}
	res := m.handleStudioGenerate("t")(context.Background(), "", json.RawMessage(`{"type":"image","prompt":"   "}`))
	if !res.IsError {
		t.Errorf("expected an error for a blank prompt, got %+v", res)
	}
}

func TestHandleStudioGenerate_RejectsUnsupportedKind(t *testing.T) {
	m := &Manager{MediaRegistry: registryWith(stubProvider{
		name: "openrouter", configured: true, kinds: []media.Kind{media.KindImage},
	})}
	res := m.handleStudioGenerate("t")(context.Background(), "", json.RawMessage(`{"type":"video","prompt":"a cat"}`))
	if !res.IsError {
		t.Errorf("expected an error asking for video with an image-only provider, got %+v", res)
	}
}
