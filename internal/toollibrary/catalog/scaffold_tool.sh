#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 3 ]]; then
  echo "Usage: $0 <slug> <name> <description> [category] [icon] [env_csv]"
  echo "Example: $0 brave-search \"Brave Search\" \"Web and news search via Brave\" Data search BRAVE_SEARCH_API_KEY"
  exit 1
fi

slug="$1"
name="$2"
description="$3"
category="${4:-Data}"
icon="${5:-wrench}"
env_csv="${6:-}"

repo_root="$(cd "$(dirname "$0")/../../.." && pwd)"
catalog_dir="$repo_root/internal/toollibrary/catalog"
tool_dir="$catalog_dir/$slug"
registry="$catalog_dir/registry.json"

if [[ -d "$tool_dir" ]]; then
  echo "Tool directory already exists: $tool_dir"
  exit 1
fi

if jq -e --arg slug "$slug" '.[] | select(.slug == $slug)' "$registry" > /dev/null; then
  echo "Registry already contains slug: $slug"
  exit 1
fi

mkdir -p "$tool_dir"
cp "$catalog_dir/weather/go.mod.tmpl" "$tool_dir/go.mod.tmpl"
cp "$catalog_dir/weather/handlers.go" "$tool_dir/handlers.go"
cp "$catalog_dir/weather/main.go.tmpl" "$tool_dir/main.go.tmpl"
cp "$catalog_dir/weather/widget.js" "$tool_dir/widget.js"

cat > "$tool_dir/$slug.go" <<GOEOF
package main

import (
    "net/http"

    "github.com/go-chi/chi/v5"
)

func registerRoutes(r chi.Router) {
    r.Get("/status", handleStatus)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
    writeJSON(w, http.StatusOK, map[string]interface{}{
        "tool": "$slug",
        "status": "ready",
        "note": "Scaffolded tool. Implement API handlers.",
    })
}
GOEOF

cat > "$tool_dir/manifest.json.tmpl" <<'JSONEOF'
{
  "id": "{{.ToolID}}",
  "name": "{{.Name | jsonEscape}}",
  "description": "{{.Description | jsonEscape}}",
  "version": "1.0.0",
  "health_check": "/health",
  "endpoints": [
    {
      "method": "GET",
      "path": "/status",
      "description": "Scaffold status endpoint",
      "response": {"status": "ready"}
    }
  ],
  "env": [],
  "widget": {
    "enabled": true,
    "types": ["auto"]
  }
}
JSONEOF

if [[ -n "$env_csv" ]]; then
  cat > "$tool_dir/main.go.tmpl" <<MAINENV
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	port := envOr("PORT", "9000")
	for _, key := range []string{"$(echo "$env_csv" | sed 's/,/","/g')"} {
		envRequired(key)
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Get("/widget.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		http.ServeFile(w, r, "widget.js")
	})

	registerRoutes(r)

	srv := &http.Server{Addr: ":" + port, Handler: r, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		log.Printf("{{.Name}} listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envRequired(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required environment variable %s is not set", key)
	}
	return v
}
MAINENV

  jq --arg env_csv "$env_csv" '.env = ($env_csv | split(",") | map(select(length > 0)))' "$tool_dir/manifest.json.tmpl" > /tmp/manifest.json.tmpl
  mv /tmp/manifest.json.tmpl "$tool_dir/manifest.json.tmpl"
fi

jq --arg slug "$slug" \
   --arg name "$name" \
   --arg description "$description" \
   --arg category "$category" \
   --arg icon "$icon" \
   --arg env_csv "$env_csv" \
   '. + [{
      slug: $slug,
      name: $name,
      description: $description,
      version: "1.0.0",
      category: $category,
      icon: $icon,
      tags: [$slug],
      env: (if $env_csv == "" then [] else ($env_csv | split(",") | map(select(length > 0))) end)
    }]' "$registry" > /tmp/registry.json
mv /tmp/registry.json "$registry"

gofmt -w "$tool_dir"/*.go "$tool_dir"/*.go.tmpl

echo "Scaffolded $slug at $tool_dir"
echo "Next: implement $tool_dir/$slug.go and update tags/endpoints in manifest and registry."
