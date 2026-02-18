# OpenPaw - Farmwork
# Run `just --list` to see all commands

# Variables
project_root := justfile_directory()
frontend_dir := project_root / "web" / "frontend"
binary_name := "openpaw"

# ============================================
# DEVELOPMENT
# ============================================

# Start frontend dev server (Vite + HMR on :5173, proxies /api to :8080)
dev:
    cd {{frontend_dir}} && npm run dev

# Run the Go backend server
serve:
    go run ./cmd/openpaw

# Run frontend dev + Go backend in parallel
dev-full:
    @echo "Starting Go backend and Vite frontend..."
    @(go run ./cmd/openpaw &) && cd {{frontend_dir}} && npm run dev

# Run linter (frontend ESLint)
lint:
    cd {{frontend_dir}} && npm run lint

# Run Go vet
vet:
    go vet ./...

# Run Go tests
test:
    go test ./...

# ============================================
# BUILD
# ============================================

# Build frontend (TypeScript check + Vite bundle → web/frontend/dist/)
frontend-build:
    cd {{frontend_dir}} && npm run build

# Build Go binary (embeds frontend dist via go:embed)
go-build:
    CGO_ENABLED=1 go build -o {{binary_name}} ./cmd/openpaw

# Full build: frontend → Go binary (production single-binary)
build: frontend-build go-build
    @echo "Built ./{{binary_name}} with embedded frontend"

# Rebuild and run the binary
run: build
    ./{{binary_name}}

# Reset database and run fresh (kills existing server, wipes DB, rebuilds, starts)
fresh: build
    -pkill -f "./{{binary_name}}" 2>/dev/null
    @sleep 1
    rm -f data/openpaw.db data/openpaw.db-wal data/openpaw.db-shm
    @echo "Database wiped. Starting fresh..."
    ./{{binary_name}}

# Reset database and run E2E tests from scratch
e2e-fresh: build
    -pkill -f "./{{binary_name}}" 2>/dev/null
    @sleep 1
    rm -f data/openpaw.db data/openpaw.db-wal data/openpaw.db-shm
    @echo "Database wiped. Starting server for E2E..."
    @OPENPAW_NO_OPEN=1 ./{{binary_name}} & sleep 3 && npx playwright test; pkill -f "./{{binary_name}}" 2>/dev/null || true
    @echo "E2E tests complete. Run 'just e2e-report' to view results."

# Install frontend dependencies
frontend-install:
    cd {{frontend_dir}} && npm install

# Clean build artifacts
clean:
    rm -f {{binary_name}}
    rm -rf {{frontend_dir}}/dist

# ============================================
# DATABASE
# ============================================

# Show database file info
db-info:
    @ls -lh data/openpaw.db 2>/dev/null || echo "No database found (created on first run)"

# Reset database (deletes all data - requires confirmation)
db-reset:
    @echo "This will DELETE the database. Press Ctrl+C to cancel, Enter to continue."
    @read _confirm
    rm -f data/openpaw.db data/openpaw.db-wal data/openpaw.db-shm
    @echo "Database deleted. Will be recreated on next run."

# Show migration files
db-migrations:
    @ls -1 internal/database/migrations/

# ============================================
# E2E TESTING (Playwright)
# ============================================

# Run all Playwright E2E tests (server must be running)
e2e:
    npx playwright test

# Run E2E tests with visible browser
e2e-headed:
    npx playwright test --headed

# Run only the setup + auth tests
e2e-auth:
    npx playwright test --project=setup && npx playwright test auth.spec.ts

# Run specific test file (e.g., just e2e-file chat)
e2e-file name:
    npx playwright test {{name}}.spec.ts

# Show last E2E test report
e2e-report:
    npx playwright show-report

# Run E2E with full build + server (builds, starts server, runs tests, stops server)
e2e-full: build
    @echo "Starting server for E2E tests..."
    @OPENPAW_NO_OPEN=1 ./{{binary_name}} & sleep 3 && npx playwright test; pkill -f "./{{binary_name}}" 2>/dev/null || true
    @echo "E2E tests complete. Run 'just e2e-report' to view results."

# ============================================
# CODE QUALITY
# ============================================

# Run full quality gate (lint + vet + test + build)
quality: lint vet test build
    @echo "All quality checks passed"

# The works: tidy, lint, vet, test, build, dead code — run before shipping
awesome:
    #!/usr/bin/env bash
    set -e
    echo ""
    echo "🐾 OpenPaw Awesome Check"
    echo "========================"
    echo ""

    echo "→ [1/7] Go mod tidy..."
    go mod tidy
    if ! git diff --quiet go.mod go.sum 2>/dev/null; then
        echo "  ✗ go.mod or go.sum changed — commit the tidy first"
        exit 1
    fi
    echo "  ✓ modules clean"

    echo "→ [2/7] Go vet..."
    go vet ./...
    echo "  ✓ vet passed"

    echo "→ [3/7] Frontend lint..."
    cd {{frontend_dir}} && npm run lint
    echo "  ✓ lint passed"

    echo "→ [4/7] Go tests..."
    cd {{project_root}} && go test ./...
    echo "  ✓ tests passed"

    echo "→ [5/7] Frontend build (TypeScript + Vite)..."
    cd {{frontend_dir}} && npm run build
    echo "  ✓ frontend built"

    echo "→ [6/7] Go build..."
    cd {{project_root}} && CGO_ENABLED=1 go build -o {{binary_name}} ./cmd/openpaw
    echo "  ✓ binary built"

    echo "→ [7/7] Dead code scan (knip)..."
    cd {{frontend_dir}} && npx knip --reporter compact 2>&1 || true
    echo "  ✓ dead code scan complete"

    echo ""
    echo "========================"
    echo "🐾 All checks passed — ready to ship!"
    echo ""

# Run knip dead code detection
dead-code:
    cd {{frontend_dir}} && npx knip

# Check Go module tidiness
tidy:
    go mod tidy

# ============================================
# API & DEBUG
# ============================================

# Show all API routes (grep from server.go)
routes:
    @grep -E '\.(Get|Post|Put|Delete|Patch)\(' internal/server/server.go | sed 's/.*r\.//' | sort

# Check if Claude Code CLI is installed
check-claude:
    @claude --version 2>/dev/null || echo "Claude Code CLI not found"

# Show system info (port, data dir, etc.)
info:
    @echo "Binary:     {{binary_name}}"
    @echo "Frontend:   {{frontend_dir}}"
    @echo "Port:       41295 (default)"
    @echo "Data dir:   ./data/"
    @echo "Database:   ./data/openpaw.db"
    @echo "Go module:  github.com/openpaw/openpaw"

# ============================================
# DESKTOP (Tauri)
# ============================================

# Build Go sidecar with target-triple name for Tauri
desktop-sidecar: frontend-build
    @mkdir -p desktop/src-tauri/binaries
    CGO_ENABLED=1 go build -o desktop/src-tauri/binaries/openpaw-$(rustc --print host-tuple) ./cmd/openpaw

# Run Tauri dev mode (rebuilds sidecar first)
desktop-dev: desktop-sidecar
    cd desktop && npm run tauri dev

# Build production Tauri app (rebuilds sidecar first)
desktop-build: desktop-sidecar
    cd desktop && npm run tauri build

# Clean desktop build artifacts
desktop-clean:
    rm -rf desktop/src-tauri/binaries/openpaw-*
    rm -rf desktop/src-tauri/target

# Generate desktop app icons from square icon (requires npx)
desktop-icons:
    cd desktop && npx @tauri-apps/cli icon ../assets/icon.png

# ============================================
# NAVIGATION
# ============================================

# Go to audit folder
audit:
    @echo "{{project_root}}/_AUDIT" && cd {{project_root}}/_AUDIT

# Go to plans folder
plans:
    @echo "{{project_root}}/_PLANS" && cd {{project_root}}/_PLANS

# Go to research folder
research:
    @echo "{{project_root}}/_RESEARCH" && cd {{project_root}}/_RESEARCH

# Go to commands folder
commands:
    @echo "{{project_root}}/.claude/commands" && cd {{project_root}}/.claude/commands

# Go to agents folder
agents:
    @echo "{{project_root}}/.claude/agents" && cd {{project_root}}/.claude/agents

# ============================================
# UTILITIES
# ============================================

# Show project structure
overview:
    @tree -L 2 -I 'node_modules|.git|dist|coverage|__pycache__|.venv|data' 2>/dev/null || find . -maxdepth 2 -type d | head -30

# Search for files by name
search pattern:
    @find . -name "*{{pattern}}*" -not -path "./node_modules/*" -not -path "./.git/*" -not -path "./data/*" 2>/dev/null

# Show git status
status:
    @git status --short

# ============================================
# Farmwork WORKFLOW
# ============================================

# Show beads issues
issues:
    @bd list --status open 2>/dev/null || echo "Beads not installed. Run: cargo install beads"

# Show completed issues count
completed:
    @bd list --status closed 2>/dev/null | wc -l || echo "0"
