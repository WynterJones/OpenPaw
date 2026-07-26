package media

import "testing"

func TestShortenNames_DropsParentheticalWhenUnambiguous(t *testing.T) {
	models := []Model{
		{ID: "a", Name: "Nano Banana Pro (Gemini 3 Pro Image Preview)"},
		{ID: "b", Name: "GPT-5 Image"},
		{ID: "c", Name: "Nano Banana Lite (Gemini 3.1 Flash Lite Image)"},
	}
	shortenNames(models)

	want := []string{"Nano Banana Pro", "GPT-5 Image", "Nano Banana Lite"}
	for i, w := range want {
		if models[i].Name != w {
			t.Errorf("models[%d].Name = %q, want %q", i, models[i].Name, w)
		}
	}
}

// Two models whose marketing names collide must keep their suffixes — an
// ambiguous picker is worse than a wordy one.
func TestShortenNames_KeepsParentheticalWhenItDisambiguates(t *testing.T) {
	models := []Model{
		{ID: "a", Name: "Nano Banana 2 (Gemini 3.1 Flash Image Preview)"},
		{ID: "b", Name: "Nano Banana 2 (Gemini 3.1 Flash Image)"},
		{ID: "c", Name: "GPT-5 Image Mini"},
	}
	shortenNames(models)

	if models[0].Name == models[1].Name {
		t.Errorf("colliding models both became %q — the picker cannot tell them apart", models[0].Name)
	}
	if models[0].Name != "Nano Banana 2 (Gemini 3.1 Flash Image Preview)" {
		t.Errorf("models[0].Name = %q, want the full name kept", models[0].Name)
	}
	if models[2].Name != "GPT-5 Image Mini" {
		t.Errorf("models[2].Name = %q, want it untouched", models[2].Name)
	}
}

func TestStripParenthetical_Edges(t *testing.T) {
	cases := map[string]string{
		"Nano Banana Pro (Gemini 3 Pro)": "Nano Banana Pro",
		"GPT-5 Image":                    "GPT-5 Image",
		"(only a group)":                 "(only a group)", // nothing left over
		"Trailing space (x) ":            "Trailing space",
		"no closing paren (":             "no closing paren (",
	}
	for in, want := range cases {
		if got := stripParenthetical(in); got != want {
			t.Errorf("stripParenthetical(%q) = %q, want %q", in, got, want)
		}
	}
}
