# Design: migrate gemini provider to google.golang.org/genai

Date: 2026-07-13
Status: approved (user requested the migration; approach agreed in session)

## Goal

Replace the deprecated `github.com/google/generative-ai-go` SDK in
`internal/llm/gemini` with Google's current unified SDK,
`google.golang.org/genai` (v1.63.0, requires go 1.24 — matches our floor).
No user-visible behavior change.

## Why now

Google deprecated `generative-ai-go`; it still works against v1beta but will
eventually be sunset. Migrating also drops its heavyweight dependency tree
(`cloud.google.com/go/*`, `google.golang.org/api`, grpc/otel indirects) —
gemini.go is the only consumer.

## Changes

### 1. `internal/llm/gemini/gemini.go` — rewrite against the new SDK

- Client: `genai.NewClient(ctx, &genai.ClientConfig{APIKey: p.apiKey,
  Backend: genai.BackendGeminiAPI, HTTPOptions: genai.HTTPOptions{BaseURL:
  p.baseURL}})` — BaseURL passed only when configured (empty string means
  SDK default endpoint).
- **New: honor `cfg.BaseURL`** — the config schema has had a per-provider
  `baseURL` since v2 and ollama honors it; gemini silently ignored it. The
  new SDK makes this a one-field pass-through, and it's what makes the
  provider unit-testable against `httptest`.
- Generate: `client.Models.GenerateContent(ctx, model, genai.Text(prompt),
  cfg)` where `cfg.SystemInstruction = genai.NewContentFromText(req.System,
  "")` when `req.System != ""` — native system instruction replaces the old
  System+"\n\n"+Prompt string concatenation.
- Response: `resp.Text()` (SDK helper concatenating all text parts —
  replaces the old `fmt.Sprintf("%v", Parts[0])` which read only the first
  part). Empty result → the existing `gemini: empty response` error.
- Unchanged: provider `Name` "gemini", `defaultModel` "gemini-flash-latest",
  self-registration via `init`, API-key-required check, per-request model
  override (`req.Model` beats configured model).

### 2. Tests — `internal/llm/gemini/gemini_test.go` (new)

The old provider had zero tests (couldn't point it at a fake server). With
BaseURL support, add `httptest`-backed tests speaking the Gemini API wire
format (`POST .../models/<model>:generateContent`, JSON candidates
response):

- happy path: prompt in request body → text extracted from response
- system instruction: `req.System` appears as `systemInstruction` in the
  request JSON, not concatenated into the user prompt
- model selection: configured model used; `req.Model` override wins
- API error (500) surfaced as an error, not a panic/empty response
- empty candidates → `gemini: empty response` error
- missing API key → constructor error (existing behavior, now pinned)

### 3. `go.mod` / `go.sum`

`go get google.golang.org/genai@v1.63.0` + `go mod tidy`. Expect
`generative-ai-go`, `google.golang.org/api`, and the `cloud.google.com/go/*`
tree to drop out. The `go` directive must stay at `1.24.x` (genai v1.63.0
requires exactly go 1.24 — verified via its .mod file).

## Non-goals

- No Vertex AI backend support (BackendGeminiAPI only).
- No streaming, no retry logic (Phase 6 items).
- No changes to other providers or the Provider interface.

## Verification

- Unit tests above; full `go test -race ./...`; lint.
- Live: `promptshell --provider gemini --dry-run "print hello"` with the
  real key (exercises default model through the new SDK end-to-end).

## Delivery

Branch `refactor/gemini-genai-sdk` off main, single PR. Commit type
`refactor:` — release-please will NOT cut a release for it; it ships in
v0.7.0 together with the upcoming update-hint feature (`feat:`), which is
fine because current v0.6.2 gemini still works.
