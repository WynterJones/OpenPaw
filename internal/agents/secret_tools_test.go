package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/openpaw/openpaw/internal/database"
)

const secretValue = "sk_live_SUPERSECRET_VALUE_9271"

// The stored column is ciphertext; these tests use a decryptor that treats the
// stored string as already-plaintext so they can assert on a known value
// without depending on the real AES-GCM format.
type fakeDecryptor struct{}

func (fakeDecryptor) Decrypt(encrypted string) (string, error) { return encrypted, nil }

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
	h, ok := MakeSecretToolHandlers(db, fakeDecryptor{}, "gateway")[name]
	if !ok {
		t.Fatalf("handler %q not registered", name)
	}
	return h(context.Background(), "", json.RawMessage(args)).Output
}

// Values come from get_secret and nowhere else — the name tools and the system
// prompt must stay clean, or every conversation ships the whole vault to the
// model provider.
func TestSecretTools_OnlyGetSecretExposesValues(t *testing.T) {
	db := newSecretTestDB(t)

	outputs := []string{
		callTool(t, db, "list_secrets", `{}`),
		callTool(t, db, "list_secrets", `{"filter":"stripe"}`),
		callTool(t, db, "check_secrets", `{"names":["STRIPE_SECRET_KEY","NOPE"]}`),
		buildSecretsPromptSection(db, fakeDecryptor{}),
	}
	for i, out := range outputs {
		if strings.Contains(out, secretValue) {
			t.Errorf("output %d leaked the secret value:\n%s", i, out)
		}
	}
}

func TestGetSecret_ReturnsValueAndAudits(t *testing.T) {
	db := newSecretTestDB(t)

	out := callTool(t, db, "get_secret", `{"name":"stripe_secret_key","reason":"calling the Stripe API"}`)
	if !strings.Contains(out, secretValue) {
		t.Errorf("get_secret did not return the value:\n%s", out)
	}
	if !strings.Contains(out, "STRIPE_SECRET_KEY") {
		t.Errorf("get_secret did not name the secret it revealed:\n%s", out)
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM audit_logs WHERE action = 'secret_revealed'").Scan(&count)
	if count != 1 {
		t.Errorf("got %d secret_revealed audit entries, want 1", count)
	}
}

func TestGetSecret_UnknownName(t *testing.T) {
	db := newSecretTestDB(t)
	out := callTool(t, db, "get_secret", `{"name":"NOPE","reason":"testing"}`)
	if !strings.Contains(out, "No secret named") {
		t.Errorf("expected a not-found message:\n%s", out)
	}
}

// A placeholder decrypts cleanly but is not a credential; handing "REPLACE_ME"
// over unlabelled sends the agent off to authenticate with it.
func TestGetSecret_FlagsPlaceholder(t *testing.T) {
	db := newSecretTestDB(t)
	db.Exec("INSERT INTO secrets (id, name, encrypted_value, description) VALUES (?,?,?,?)",
		"s3", "POSTMARK_TOKEN", "REPLACE_ME", "")

	out := callTool(t, db, "get_secret", `{"name":"POSTMARK_TOKEN","reason":"testing"}`)
	if !strings.Contains(out, "placeholder") {
		t.Errorf("expected the placeholder to be called out:\n%s", out)
	}
}

// Without a decryptor there is no way to serve get_secret, so it must not be
// advertised — a model that sees the tool will try it and get only errors.
func TestGetSecret_OmittedWithoutDecryptor(t *testing.T) {
	db := newSecretTestDB(t)

	if _, ok := MakeSecretToolHandlers(db, nil, "gateway")["get_secret"]; ok {
		t.Error("get_secret handler registered without a decryptor")
	}
	for _, d := range BuildSecretToolDefs(nil) {
		if d.Function.Name == "get_secret" {
			t.Error("get_secret advertised without a decryptor")
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

func TestSecretToolDefs_Shape(t *testing.T) {
	defs := BuildSecretToolDefs(fakeDecryptor{})
	if len(defs) != 3 {
		t.Fatalf("got %d secret tools, want 3", len(defs))
	}

	names := map[string]string{}
	for _, d := range defs {
		names[d.Function.Name] = d.Function.Description
	}
	for _, want := range []string{"list_secrets", "check_secrets", "get_secret"} {
		if _, ok := names[want]; !ok {
			t.Errorf("tool %q not registered", want)
		}
	}
	// The name tools must keep steering value requests at get_secret rather
	// than at the user's clipboard.
	for _, n := range []string{"list_secrets", "check_secrets"} {
		if !strings.Contains(names[n], "get_secret") {
			t.Errorf("tool %q does not point at get_secret for values: %q", n, names[n])
		}
	}
	if !strings.Contains(strings.ToLower(names["get_secret"]), "never echo") {
		t.Errorf("get_secret does not warn against echoing the value: %q", names["get_secret"])
	}
}
