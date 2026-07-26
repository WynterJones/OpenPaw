package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/openpaw/openpaw/internal/database"
)

const secretValue = "sk_live_SUPERSECRET_VALUE_9271"

func newSecretTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.New(t.TempDir())
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	db.Exec(
		"INSERT INTO secrets (id, name, encrypted_value, description) VALUES (?,?,?,?)",
		"s1", "STRIPE_SECRET_KEY", secretValue, "Stripe API key",
	)
	db.Exec(
		"INSERT INTO secrets (id, name, encrypted_value, description) VALUES (?,?,?,?)",
		"s2", "RAILWAY_TOKEN", secretValue, "",
	)
	return db
}

func callTool(t *testing.T, db *database.DB, name, args string) string {
	t.Helper()
	h, ok := MakeSecretToolHandlers(db)[name]
	if !ok {
		t.Fatalf("handler %q not registered", name)
	}
	return h(context.Background(), "", json.RawMessage(args)).Output
}

// The entire point of these tools: names yes, values never.
func TestSecretTools_NeverLeakValues(t *testing.T) {
	db := newSecretTestDB(t)

	outputs := []string{
		callTool(t, db, "list_secrets", `{}`),
		callTool(t, db, "list_secrets", `{"filter":"stripe"}`),
		callTool(t, db, "check_secrets", `{"names":["STRIPE_SECRET_KEY","NOPE"]}`),
		buildSecretsPromptSection(db),
	}
	for i, out := range outputs {
		if strings.Contains(out, secretValue) {
			t.Errorf("output %d leaked the secret value:\n%s", i, out)
		}
	}
}

func TestListSecrets_NamesAndFilter(t *testing.T) {
	db := newSecretTestDB(t)

	all := callTool(t, db, "list_secrets", `{}`)
	for _, want := range []string{"STRIPE_SECRET_KEY", "RAILWAY_TOKEN", "Stripe API key"} {
		if !strings.Contains(all, want) {
			t.Errorf("list_secrets missing %q:\n%s", want, all)
		}
	}

	filtered := callTool(t, db, "list_secrets", `{"filter":"railway"}`)
	if !strings.Contains(filtered, "RAILWAY_TOKEN") {
		t.Errorf("filter dropped the matching secret:\n%s", filtered)
	}
	if strings.Contains(filtered, "STRIPE_SECRET_KEY") {
		t.Errorf("filter kept a non-matching secret:\n%s", filtered)
	}
}

// A model asking for "stripe_secret_key" means the configured
// STRIPE_SECRET_KEY; reporting it missing would send the user to re-add a
// credential they already have.
func TestCheckSecrets_CaseInsensitive(t *testing.T) {
	db := newSecretTestDB(t)

	out := callTool(t, db, "check_secrets", `{"names":["stripe_secret_key","POSTMARK_SERVER_TOKEN"]}`)
	if !strings.Contains(out, "SET (1): STRIPE_SECRET_KEY") {
		t.Errorf("expected the lowercase name to resolve to the stored one:\n%s", out)
	}
	if !strings.Contains(out, "MISSING (1): POSTMARK_SERVER_TOKEN") {
		t.Errorf("expected the absent name to be reported missing:\n%s", out)
	}
}

func TestCheckSecrets_AllPresent(t *testing.T) {
	db := newSecretTestDB(t)
	out := callTool(t, db, "check_secrets", `{"names":["RAILWAY_TOKEN"]}`)
	if !strings.Contains(out, "MISSING (0)") {
		t.Errorf("expected no missing secrets:\n%s", out)
	}
}

func TestSecretToolDefs_DeclareNoValueAccess(t *testing.T) {
	defs := BuildSecretToolDefs()
	if len(defs) != 2 {
		t.Fatalf("got %d secret tools, want 2", len(defs))
	}
	for _, d := range defs {
		if !strings.Contains(strings.ToLower(d.Function.Description), "never") {
			t.Errorf("tool %q does not tell the model values are off-limits: %q",
				d.Function.Name, d.Function.Description)
		}
	}
}
