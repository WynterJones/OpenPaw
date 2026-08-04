package agents

import (
	"strings"
	"testing"
)

// The point of the tool: the command gets the credential, the model does not.
func TestResolveSecretEnv_ReturnsValuesWithoutRevealingThem(t *testing.T) {
	db := newSecretTestDB(t)
	m := &Manager{db: db, SecretsMgr: fakeDecryptor{}}

	env, missing, err := m.resolveSecretEnv([]string{"RAILWAY_TOKEN"}, "deploying", "gateway")
	if err != nil {
		t.Fatalf("resolveSecretEnv: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("reported %v as missing", missing)
	}
	if env["RAILWAY_TOKEN"] != secretValue {
		t.Errorf("the command would not get the credential: %q", env["RAILWAY_TOKEN"])
	}
}

// Secret names are conventionally SCREAMING_SNAKE_CASE and a model asking for
// the lowercase spelling means the same secret; treating that as missing would
// block a deploy over capitalisation.
func TestResolveSecretEnv_MatchesNamesCaseInsensitively(t *testing.T) {
	db := newSecretTestDB(t)
	m := &Manager{db: db, SecretsMgr: fakeDecryptor{}}

	env, missing, err := m.resolveSecretEnv([]string{"railway_token"}, "deploying", "gateway")
	if err != nil {
		t.Fatalf("resolveSecretEnv: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("reported %v as missing", missing)
	}
	// Set under both spellings, so a command written against either one works.
	if env["RAILWAY_TOKEN"] != secretValue || env["railway_token"] != secretValue {
		t.Errorf("the requested spelling is not in the environment: %v", keysOf(env))
	}
}

// Half a deploy is worse than none, so a missing credential stops it before it
// starts rather than failing partway through.
func TestRunWithSecrets_RefusesToStartWithoutEveryCredential(t *testing.T) {
	db := newSecretTestDB(t)
	m := &Manager{db: db, SecretsMgr: fakeDecryptor{}}

	res := call(t, m.handleRunWithSecrets("thread-1", "gateway"),
		map[string]interface{}{
			"command": "railway up",
			"secrets": []string{"RAILWAY_TOKEN", "NOT_CONFIGURED"},
			"reason":  "deploying",
		})

	if !res.IsError {
		t.Fatal("started a command with a credential missing")
	}
	if !strings.Contains(res.Output, "NOT_CONFIGURED") {
		t.Errorf("does not name what is missing:\n%s", res.Output)
	}
	if strings.Contains(res.Output, secretValue) {
		t.Errorf("leaked a secret value into the reply:\n%s", res.Output)
	}
}

// The tool is only offered where it can work; declaring it with no decryptor
// would advertise a capability that always fails.
func TestBuildSecretRunToolDefs_NeedsADecryptor(t *testing.T) {
	if defs := BuildSecretRunToolDefs(nil); len(defs) != 0 {
		t.Errorf("offered run_with_secrets with no decryptor: %d defs", len(defs))
	}
	defs := BuildSecretRunToolDefs(fakeDecryptor{})
	if len(defs) != 1 || defs[0].Function.Name != "run_with_secrets" {
		t.Fatalf("unexpected defs: %+v", defs)
	}
	// It only displaces get_secret if the model is told which to reach for.
	if !strings.Contains(defs[0].Function.Description, "get_secret") {
		t.Errorf("description does not say when to prefer it:\n%s", defs[0].Function.Description)
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
