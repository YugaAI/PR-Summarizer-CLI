# Plan: Developer Tooling — Code Quality & PR Review Automation

Status: **Implementation-Ready** — dokumen ini adalah acuan eksekusi. Setiap task punya acceptance criteria; ikuti urutan fase, jangan lompat kecuali ada alasan eksplisit yang dicatat di bagian Decision Log.

---

## 0. Cara Pakai Dokumen Ini

- Kerjakan **per fase**, urut dari atas ke bawah. Jangan mulai fase N+1 sebelum Definition of Done fase N terpenuhi.
- Tiap task punya checkbox `[ ]` — centang manual saat selesai, jangan hapus task walau sudah selesai (jadi riwayat).
- Kalau ada keputusan yang menyimpang dari plan ini saat implementasi (misal ganti library, ubah struktur), catat di **Decision Log** (bagian 9) beserta alasan — supaya plan tetap jadi sumber kebenaran yang akurat, bukan dokumen basi.
- Semua nama file/package di bawah ini final, tidak improvisasi penamaan saat coding — biar konsisten dengan struktur yang sudah direview.

---

## 1. Scope & Non-Goals

**In scope (v1):**
- Tool 2 (Diff-Aware PR Summarizer) — jadi prioritas utama karena punya breakdown paling matang.
- Tool 1 (Pre-commit Guard) — scope diperkecil ke 2 analyzer paling berdampak dulu (lihat bagian 3).

**Non-goals (v1, eksplisit ditunda):**
- Gate check (blocking merge) — baru diaktifkan di fase 4, setelah heuristic classifier terbukti reliable.
- Redis/SQLite cache backend — v1 pakai file-based cache lokal via GitHub Actions cache dulu.
- Model routing (pilih model berbeda per risk tier) — ditunda sampai ada data volume PR nyata.
- IDE integration untuk Pre-commit Guard — v1 cukup CI step.

---

## 2. Milestone & Urutan Eksekusi

| Fase | Nama | Output | Prasyarat |
|---|---|---|---|
| 1 | Skeleton & Domain Layer | Project compile, struct domain final, config loader jalan | - |
| 2 | Heuristic-only Pipeline | PR comment muncul otomatis di CI, berisi risk tag + file list, **tanpa LLM** | Fase 1 |
| 3 | LLM Layer (MiMo) | Comment berisi narasi summary, fallback teruji | Fase 2 |
| 4 | Cost Control | Cache aktif, concurrency terkontrol, low-value file terfilter | Fase 3 |
| 5 | Gate Check (opsional) | Merge terblokir untuk PR high-risk tanpa approval | Fase 4, dan tim sudah percaya false-positive rate rendah |
| 6 | Pre-commit Guard (Tool 1) | 2 analyzer jalan sebagai CI step terpisah | Independen, bisa paralel dengan fase 2+ |

**Definition of Done tiap fase ada di akhir masing-masing section di bawah.**

---

## 3. Fase 1 — Skeleton & Domain Layer

### 3.1 Struktur direktori (buat persis seperti ini)

```
pr-summarizer/
├── cmd/pr-summarizer/main.go
├── internal/
│   ├── domain/
│   │   ├── diff.go
│   │   └── summary.go
│   ├── usecase/
│   │   ├── summarize.go
│   │   └── summarize_test.go
│   ├── repository/
│   │   ├── diff/
│   │   │   ├── extractor.go
│   │   │   └── git_extractor.go
│   │   ├── llm/
│   │   │   ├── summarizer.go
│   │   │   ├── mimo_client.go
│   │   │   └── mock_summarizer.go        # generated, jangan edit manual
│   │   ├── cache/
│   │   │   ├── cache.go
│   │   │   └── local_cache.go
│   │   └── vcs/
│   │       ├── client.go
│   │       └── github_client.go
│   ├── classifier/
│   │   ├── risk.go
│   │   └── risk_test.go
│   ├── chunker/
│   │   ├── chunker.go
│   │   └── chunker_test.go
│   └── config/
│       └── config.go
├── pkg/tokenutil/estimate.go
├── configs/risk_rules.yaml
├── .github/workflows/pr-summary.yml
├── go.mod
└── Makefile
```

### 3.2 Task list

- [x] `go mod init` dengan module path final (tentukan nama repo dulu — masuk ke Decision Log kalau berbeda dari asumsi `pr-summarizer`)
- [x] `internal/domain/diff.go` — definisikan struct berikut persis:
  ```go
  package domain

  type RiskLevel string

  const (
      RiskHigh   RiskLevel = "high"
      RiskMedium RiskLevel = "medium"
      RiskLow    RiskLevel = "low"
  )

  type DiffFile struct {
      Path      string
      Status    string // "added" | "modified" | "deleted" | "renamed"
      Content   string // unified diff, hunk-level
      Risk      RiskLevel
      RiskReason string
  }

  type DiffChunk struct {
      FilePath string
      Content  string
      Risk     RiskLevel
  }

  func (c DiffChunk) Hash() string // sha256(FilePath + Content), hex-encoded
  ```
- [x] `internal/domain/summary.go`:
  ```go
  package domain

  type FileSummary struct {
      FilePath string
      Summary  string
      Category string // "feat" | "fix" | "refactor" | "chore" | "docs"
      Concerns []string
      Risk     RiskLevel
      FromCache bool
      FromLLM   bool // false = heuristic fallback dipakai
  }

  type PRSummary struct {
      Files       []FileSummary
      HighRiskCount int
      TokensUsed    int
      CacheHits     int
      CacheMisses   int
  }

  func MergeSummaries(results []FileSummary) PRSummary
  func (s PRSummary) RenderMarkdown() string // format comment final
  ```
- [x] `internal/config/config.go` — pakai `github.com/caarlos0/env/v10`, fail-fast (`log.Fatal` kalau required env kosong):
  ```go
  type Config struct {
      GitHubToken  string        `env:"GITHUB_TOKEN,required"`
      MimoAPIKey   string        `env:"MIMO_API_KEY,required"`
      BaseRef      string        `env:"GITHUB_BASE_REF,required"`
      HeadRef      string        `env:"GITHUB_HEAD_REF,required"`
      RepoOwner    string        `env:"GITHUB_REPOSITORY_OWNER,required"`
      RepoName     string        `env:"GITHUB_REPO_NAME,required"`
      PRNumber     int           `env:"PR_NUMBER,required"`
      TokenBudget  int           `env:"TOKEN_BUDGET" envDefault:"8000"`
      Concurrency  int           `env:"LLM_CONCURRENCY" envDefault:"3"`
      Timeout      time.Duration `env:"TIMEOUT" envDefault:"120s"`
      CacheDir     string        `env:"CACHE_DIR" envDefault:".pr-summary-cache"`
      RiskRulesPath string       `env:"RISK_RULES_PATH" envDefault:"configs/risk_rules.yaml"`
      GateEnabled  bool          `env:"GATE_ENABLED" envDefault:"false"`
  }

  func Load() (Config, error)
  ```
- [x] `configs/risk_rules.yaml` — isi awal (tim review dulu sebelum final, lihat Open Items):
  ```yaml
  high:
    - pattern: "**/migrations/**"
      reason: "database schema change"
    - pattern: "**/auth/**"
      reason: "authentication/authorization logic"
    - pattern: "**/payment/**"
      reason: "payment logic"
  medium:
    - pattern: "**/middleware/**"
      reason: "cross-cutting request handling"
    - pattern: "go.mod"
      reason: "dependency change"
    - pattern: "go.sum"
      reason: "dependency lockfile"
  low:
    - pattern: "**/*_test.go"
      reason: "test file"
    - pattern: "**/mocks/**"
      reason: "generated mock"
    - pattern: "**/*.pb.go"
      reason: "generated protobuf code"
    - pattern: "docs/**"
      reason: "documentation"
  ```
- [x] `Makefile` dengan target minimal: `build`, `test`, `lint`, `mocks` (jalanin `go generate ./...`)
- [x] `cmd/pr-summarizer/main.go` — boleh sementara cuma `log.Print("skeleton ok")`, wiring lengkap masuk di fase 2.

### 3.3 Definition of Done Fase 1

- [x] `go build ./...` sukses tanpa error.
- [x] `go vet ./...` bersih.
- [x] Config loader punya unit test yang assert `log.Fatal`/error kalau `GITHUB_TOKEN` kosong.
- [ ] `risk_rules.yaml` sudah direview minimal oleh satu orang lain (bukan cuma yang nulis) — karena ini jadi dasar keputusan gating di fase 5.

---

## 4. Fase 2 — Heuristic-only Pipeline (Tanpa LLM)

Tujuan fase ini: buktikan pipeline end-to-end jalan dan value dasarnya kerasa (risk tag + file list otomatis muncul di PR), **sebelum** nambah kompleksitas LLM. Kalau fase ini gagal deliver value, jangan lanjut ke fase 3.

### 4.1 Task list

- [x] `internal/classifier/risk.go`:
  ```go
  package classifier

  type RiskRules struct {
      High   []Rule `yaml:"high"`
      Medium []Rule `yaml:"medium"`
      Low    []Rule `yaml:"low"`
  }
  type Rule struct {
      Pattern string `yaml:"pattern"`
      Reason  string `yaml:"reason"`
  }

  func LoadRules(path string) (RiskRules, error)
  func Classify(files []domain.DiffFile, rules RiskRules) []domain.DiffFile
  ```
  **Acceptance criteria**: pure function, tidak ada I/O di `Classify` (I/O cuma di `LoadRules`). File yang tidak match rule apapun default ke `RiskLevel` kosong/medium — putuskan default ini secara eksplisit dan tulis di komentar kode.
- [x] `internal/classifier/risk_test.go` — table-driven test, minimal cover: file match high, file match low, file tidak match apapun, path dengan wildcard nested (`internal/auth/handler/login.go` harus match `**/auth/**`).
- [x] `internal/repository/diff/extractor.go`:
  ```go
  package diff

  type DiffExtractor interface {
      Extract(ctx context.Context) ([]domain.DiffFile, error)
  }
  ```
- [x] `internal/repository/diff/git_extractor.go` — implementasi via `os/exec` manggil `git diff base...head --name-status` lalu `git diff base...head` untuk content. **Acceptance criteria**: pakai three-dot diff (bukan two-dot), robust terhadap file yang di-rename (status `R`).
- [x] `internal/chunker/chunker.go`:
  ```go
  package chunker

  type Chunker interface {
      Chunk(files []domain.DiffFile) []domain.DiffChunk
  }
  ```
  Fase ini chunker cukup 1:1 (satu file = satu chunk), token budget belum dipakai — logic budget masuk fase 4.
- [x] `internal/repository/vcs/client.go`:
  ```go
  package vcs

  type VCSClient interface {
      PostOrUpdateComment(ctx context.Context, prNumber int, body string) error
  }
  ```
- [x] `internal/repository/vcs/github_client.go` — pakai `github.com/google/go-github/v66`. **Acceptance criteria**: idempotent — cari comment existing dengan marker `<!-- pr-summarizer-bot -->` di awal body, `PATCH` kalau ketemu, `POST` kalau belum ada.
- [x] `internal/usecase/summarize.go` — versi awal tanpa LLM/cache (interface LLM & Cache di-inject tapi boleh no-op implementation dulu):
  ```go
  type Summarizer struct {
      extractor  diff.DiffExtractor
      classifier func([]domain.DiffFile, classifier.RiskRules) []domain.DiffFile
      rules      classifier.RiskRules
      chunker    chunker.Chunker
      vcs        vcs.VCSClient
      logger     zerolog.Logger
  }

  func (s *Summarizer) Run(ctx context.Context, prNumber int) error
  ```
- [x] `internal/usecase/summarize_test.go` — mock `DiffExtractor` dan `VCSClient`, assert `Run` menghasilkan comment body yang mengandung marker dan risk count yang benar.
- [x] `.github/workflows/pr-summary.yml` — versi minimal (lihat 4.2).

### 4.2 CI Workflow (versi fase 2)

```yaml
name: pr-summary
on:
  pull_request:
    types: [opened, synchronize, reopened]

permissions:
  contents: read
  pull-requests: write

concurrency:
  group: pr-summary-${{ github.event.pull_request.number }}
  cancel-in-progress: true

jobs:
  summarize:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - run: go build -o pr-summarizer ./cmd/pr-summarizer
      - run: ./pr-summarizer
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GITHUB_BASE_REF: ${{ github.base_ref }}
          GITHUB_HEAD_REF: ${{ github.head_ref }}
          GITHUB_REPOSITORY_OWNER: ${{ github.repository_owner }}
          GITHUB_REPO_NAME: ${{ github.event.repository.name }}
          PR_NUMBER: ${{ github.event.pull_request.number }}
          MIMO_API_KEY: "unused-in-phase-2"
```

**Catatan keamanan**: trigger `pull_request` (bukan `pull_request_target`) — wajib, jangan diubah demi kemudahan testing dari fork.

### 4.3 Definition of Done Fase 2

- [ ] Buka PR test (boleh PR dummy di repo test), comment otomatis muncul berisi: daftar file berubah + risk tag per file. *(butuh PR sungguhan di GitHub — belum divalidasi end-to-end)*
- [ ] Push kedua ke branch yang sama → comment ter-**update**, bukan comment baru (validasi idempotency). *(logic idempotent sudah diimplementasi di `github_client.go`, belum divalidasi live)*
- [ ] Push ke PR dari branch yang menyentuh file di `configs/risk_rules.yaml` pattern high → comment menunjukkan `HighRiskCount > 0`. *(sudah dicover unit test `usecase`, belum divalidasi live)*
- [x] Unit test `classifier` dan `usecase` lulus di CI (`go test ./...`).

---

## 5. Fase 3 — LLM Layer (MiMo)

### 5.1 Task list

- [ ] `internal/repository/llm/summarizer.go`:
  ```go
  package llm

  type LLMSummarizer interface {
      Summarize(ctx context.Context, chunk domain.DiffChunk) (domain.FileSummary, error)
  }
  ```
- [ ] Generate mock: tambahkan `//go:generate mockgen -source=summarizer.go -destination=mock_summarizer.go -package=llm` di atas `summarizer.go`, jalankan `make mocks`.
- [ ] `internal/repository/llm/mimo_client.go` — implementasi sesuai draf yang sudah direview (lihat bagian 5.2). **Sebelum dipakai di CI, wajib lolos Task Validasi 5.3 dulu.**
- [ ] Update `internal/usecase/summarize.go`: inject `LLMSummarizer`, tambahkan logic fallback:
  ```go
  summary, err := s.llm.Summarize(ctx, chunk)
  if err != nil {
      s.logger.Warn().Err(err).Str("file", chunk.FilePath).Msg("llm summarize failed, using heuristic fallback")
      summary = heuristicFallback(chunk) // FromLLM: false
  }
  ```
- [ ] `heuristicFallback(chunk)` — fungsi baru di `usecase/`, hasilnya minimal: `Summary: "<risk> risk change in <file>"`, `Category` ditebak dari conventional commit prefix kalau ada, kalau tidak default `"chore"`.
- [ ] Concurrency: worker pool di `usecase.Run` dengan limit dari `cfg.Concurrency` (default 3) — pakai `golang.org/x/sync/errgroup` dengan `SetLimit`.
- [ ] Update CI workflow: isi `MIMO_API_KEY: ${{ secrets.MIMO_API_KEY }}` beneran, tambahkan secret di repo settings.

### 5.2 Spesifikasi `mimo_client.go` (final, siap-implement)

```go
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/rs/zerolog"

	"pr-summarizer/internal/domain"
)

const (
	mimoModel   = "mimo-v2.5-pro"
	mimoBaseURL = "https://api.xiaomimimo.com/anthropic"
)

type MimoClient struct {
	client  anthropic.Client
	timeout time.Duration
	logger  zerolog.Logger
}

func NewMimoClient(apiKey string, timeout time.Duration, logger zerolog.Logger) *MimoClient {
	client := anthropic.NewClient(
		option.WithBaseURL(mimoBaseURL),
		option.WithHeader("api-key", apiKey),
	)
	return &MimoClient{client: client, timeout: timeout, logger: logger}
}

func (m *MimoClient) Summarize(ctx context.Context, chunk domain.DiffChunk) (domain.FileSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	resp, err := m.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     mimoModel,
		MaxTokens: 1024,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(buildPrompt(chunk))),
		},
		Temperature: anthropic.Float(0.2),
	})
	if err != nil {
		return domain.FileSummary{}, fmt.Errorf("mimo summarize %s: %w", chunk.FilePath, err)
	}
	return parseStructuredOutput(resp, chunk)
}

const systemPrompt = `You are a senior backend engineer reviewing a git diff.
Output ONLY valid JSON matching this schema, no prose outside JSON, no markdown code fence:
{"summary": string, "category": "feat|fix|refactor|chore|docs", "concerns": [string]}`

func buildPrompt(chunk domain.DiffChunk) string {
	return fmt.Sprintf("File: %s\nDiff:\n%s", chunk.FilePath, chunk.Content)
}

func parseStructuredOutput(resp *anthropic.Message, chunk domain.DiffChunk) (domain.FileSummary, error) {
	if len(resp.Content) == 0 {
		return domain.FileSummary{}, fmt.Errorf("empty response from mimo for %s", chunk.FilePath)
	}
	text := strings.TrimSpace(resp.Content[0].Text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var parsed struct {
		Summary  string   `json:"summary"`
		Category string   `json:"category"`
		Concerns []string `json:"concerns"`
	}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return domain.FileSummary{}, fmt.Errorf("parse mimo output for %s: %w", chunk.FilePath, err)
	}
	return domain.FileSummary{
		FilePath: chunk.FilePath,
		Summary:  parsed.Summary,
		Category: parsed.Category,
		Concerns: parsed.Concerns,
		Risk:     chunk.Risk,
		FromLLM:  true,
	}, nil
}
```

### 5.3 Task Validasi Wajib (sebelum masuk CI production)

- [ ] **Test auth header manual** — buat 1 script/test kecil terpisah (`cmd/mimo-smoke-test/main.go`, boleh dihapus setelah divalidasi) yang manggil `Messages.New` dengan API key asli, pastikan tidak 401. Kalau 401, coba ganti pendekatan header (`option.WithAPIKey` vs `option.WithHeader` manual) sampai jelas mana yang benar-benar dipakai SDK.
- [ ] **Test fallback path** — matikan network / pakai API key salah sengaja, pastikan `usecase.Run` tetap menghasilkan comment (via `heuristicFallback`), tidak crash, tidak bikin CI job merah.
- [ ] **Test markdown fence stripping** — kirim prompt yang sengaja memancing model membalas dengan ```` ```json ```` wrapper, pastikan parser tetap berhasil.
- [ ] **Cek rate limit resmi** — buka `https://mimo.mi.com/docs/en-US/api/guidance/rate-limit` dan `.../price/pay-as-you-go`, catat angka rate limit & pricing di bagian 9 (Decision Log), sesuaikan `LLM_CONCURRENCY` default kalau perlu.

### 5.4 Definition of Done Fase 3

- [ ] Comment di PR test berisi narasi ringkasan per file (bukan cuma risk tag mentah).
- [ ] Simulasi API key salah tetap menghasilkan comment (fallback jalan, ada log warning, CI job tidak gagal karena ini).
- [ ] Concurrency limit teruji tidak melebihi `LLM_CONCURRENCY` (bisa divalidasi via log timestamp overlap).

---

## 6. Fase 4 — Cost Control

### 6.1 Task list

- [ ] `internal/repository/cache/cache.go`:
  ```go
  package cache

  type Cache interface {
      Get(ctx context.Context, key string) (domain.FileSummary, bool)
      Set(ctx context.Context, key string, value domain.FileSummary) error
  }
  ```
- [ ] `internal/repository/cache/local_cache.go` — implementasi file-based (JSON per key di `CacheDir`), key = `chunk.Hash()`.
- [ ] Update `.github/workflows/pr-summary.yml`: tambahkan step `actions/cache@v4` dengan `path: .pr-summary-cache`, `key` berbasis PR number + fallback restore-key tanpa PR number (biar cache bisa reused lintas PR untuk file yang sama).
- [ ] Update `chunker.Chunk`: file dengan pattern low-risk yang match daftar `skip_llm_patterns` (lockfile, generated code, vendor) langsung dapat `FileSummary` heuristic tanpa lewat LLM sama sekali — tambahkan field ini ke `risk_rules.yaml` atau file config terpisah `configs/skip_patterns.yaml`.
- [ ] Tambahkan deteksi whitespace-only diff: kalau `git diff -w base...head` kosong, skip seluruh LLM call untuk PR itu, langsung heuristic-only comment.
- [ ] Update `usecase.Run`: cek cache dulu sebelum manggil `llm.Summarize`, log cache hit/miss count ke `domain.PRSummary`.
- [ ] Sematkan metrics ke comment (HTML comment tersembunyi): `<!-- tokens_used: N, cache_hit: X/Y -->`.

### 6.2 Definition of Done Fase 4

- [ ] Push kedua ke PR yang sama tanpa perubahan di sebagian besar file → cache hit rate terlihat di comment (via HTML comment metric).
- [ ] PR yang cuma mengubah `go.sum` tidak memicu LLM call sama sekali (cek log — 0 call ke `Summarize`).
- [ ] PR whitespace-only tidak memicu LLM call.

---

## 7. Fase 5 — Gate Check (Opsional, Tunda Sampai Tim Yakin)

**Prasyarat sebelum mulai fase ini**: minimal 2 minggu berjalan di mode observasi (fase 2-4), dengan false-positive rate risk classifier yang dianggap acceptable oleh tim.

- [ ] Job terpisah `gate-check` di workflow, `needs: summarize`.
- [ ] Logic: exit non-zero kalau `PRSummary.HighRiskCount > 0` dan belum ada approval dari reviewer yang terdaftar di CODEOWNERS untuk path terkait.
- [ ] Set sebagai required check di branch protection rule — **ini keputusan tim, bukan keputusan teknis, perlu sign-off eksplisit sebelum diaktifkan.**

---

## 8. Fase 6 — Pre-commit Guard (Tool 1)

Scope v1: 2 analyzer dengan dampak terbesar dan risiko false-positive terendah.

### 8.1 Task list

- [ ] Project terpisah (atau subpackage), `analyzers/ctxbackground/analyzer.go` — deteksi `context.Background()` di luar `main()`/file yang eksplisit di-allowlist.
- [ ] `analyzers/ormunscoped/analyzer.go` — deteksi pemanggilan `.Save()` GORM tanpa `.Where()` mendahului di chain yang sama.
- [ ] Tiap analyzer: unit test dengan `analysistest.Run` (package resmi `golang.org/x/tools/go/analysis/analysistest`) — wajib ada test case positif (harus flag) dan negatif (tidak boleh flag, termasuk kasus di allowlist).
- [ ] Integrasi sebagai CI step terpisah (`golangci-lint` custom plugin, atau `go vet -vettool=` kalau lebih simpel) — **keputusan pendekatan integrasi masuk Decision Log saat dieksekusi**, karena ada trade-off antara custom `golangci-lint` plugin (lebih terintegrasi tapi setup lebih ribet) vs binary analyzer mandiri.

### 8.2 Definition of Done Fase 6

- [ ] Kedua analyzer lulus test positif & negatif.
- [ ] CI step baru tidak false-positive di codebase existing (`backend-web-ciputra-staging`) — jalankan analyzer terhadap codebase saat ini sebagai smoke test, review manual semua temuan sebelum diaktifkan sebagai required check.

---

## 9. Decision Log

Isi tabel ini setiap kali ada keputusan yang menyimpang dari asumsi awal di plan ini.

| Tanggal | Keputusan | Alasan |
|---|---|---|
| 2026-07-31 | Module path final: `github.com/YugaAI/PR-Summarizer-CLI`, bukan asumsi `pr-summarizer`. | Mengikuti nama repo GitHub aktual (`origin` remote: `github.com/YugaAI/PR-Summarizer-CLI`) sesuai §10.1. Nama folder/package di internal (`cmd/pr-summarizer`, dst.) tetap seperti di plan — hanya module path root yang berubah. |
| 2026-07-31 | CI workflow (§4.2): `GITHUB_BASE_REF` diisi `origin/${{ github.base_ref }}` (bukan `${{ github.base_ref }}` polos), `GITHUB_HEAD_REF` diisi literal `HEAD` (bukan `${{ github.head_ref }}`). `go-version` di `actions/setup-go` dinaikkan ke `1.25` (bukan `1.23`). | `actions/checkout` untuk event `pull_request` checkout PR head secara detached — branch base tidak ada sebagai local branch bernama itu, cuma tersedia sebagai `origin/<base>`; `git diff <base>...<head>` akan gagal kalau `<base>` bukan ref yang valid secara lokal. Ini juga konsisten dengan draf brainstorming awal (`git diff origin/base...HEAD`). `go-version` dinaikkan karena `go.mod` sudah mencatat `go 1.25.2` (toolchain lokal saat `go mod init`), lebih baru dari asumsi `1.23` di draf. |
| 2026-07-31 | Rate limit `mimo-v2.5-pro`: 100 RPM / 10.000.000 TPM per akun (agregat semua API key). Pricing overseas: input cache-miss $0.435/M, input cache-hit $0.0036/M (~120x lebih murah), output $0.87/M. Cache write gratis untuk waktu terbatas (bisa berubah). | Dicek langsung dari `mimo.mi.com/docs/en-US/api/guidance/rate-limit` dan `.../price/pay-as-you-go`, update terakhir per dokumentasi: Juni-Juli 2026. `LLM_CONCURRENCY` default 3 dikonfirmasi aman jauh di bawah limit 100 RPM — tidak perlu disesuaikan. Prioritas cache di Fase 4 naik karena selisih harga cache-hit vs cache-miss besar. |

---

## 10. Open Items — Status & Rencana Eksekusi

### 10.1 Nama final module Go / repo
**Status: RESOLVED** — repo standalone sudah dibuat. Module path Go mengikuti nama repo GitHub yang sudah dibuat (isi field ini di `go.mod` sesuai nama repo aktual, sesuaikan semua import path internal di task 3.2 kalau berbeda dari asumsi `pr-summarizer` di draf kode sebelumnya).

### 10.2 Review `configs/risk_rules.yaml`
**Status: RESOLVED (pendekatan diputuskan)** — `risk_rules.yaml` akan masuk sebagai **PR pertama** di repo baru. Ini otomatis:
- Memaksa file itu lewat review process normal (reviewer kedua terlibat lewat mekanisme PR, bukan sesi diskusi terpisah).
- Jadi validasi awal bahwa branch protection & CI repo baru sudah berfungsi (kalau PR pertama ini nggak bisa di-merge tanpa review/CI hijau, berarti setup dasar repo sudah benar sebelum kode lain masuk).
- **Task turunan**: pastikan branch protection rule (minimal 1 approval) sudah aktif di repo **sebelum** PR pertama ini dibuka — kalau belum, aktifkan itu duluan sebagai commit/config pertama.

### 10.3 Rate limit & pricing resmi MiMo API
**Status: RESOLVED** — lihat Decision Log baris 2026-07-31. Data rate limit (100 RPM / 10M TPM) dan pricing sudah final.
**Task turunan yang tetap perlu jalan**: Task Validasi 5.3 (smoke test) tetap wajib dieksekusi apa adanya — dokumentasi provider tidak selalu match perilaku aktual API, terutama provider yang relatif baru. Log response/error saat smoke test untuk konfirmasi angka ini akurat di praktik, baru benar-benar ditutup.

### 10.4 Pendekatan integrasi analyzer (`golangci-lint` plugin vs standalone)
**Rekomendasi: mulai dari standalone (`go vet -vettool=`)**, bukan `golangci-lint` custom plugin, dengan alasan:
- Effort setup rendah → sinyal cepat apakah analyzer berguna atau banyak false-positive, sebelum invest ke plugin architecture.
- Plugin API `golangci-lint` terikat versi ketat — risiko maintenance tambahan yang belum tentu worth di tahap awal.
- Migrasi ke plugin approach nanti bersifat **additive** (logic `analysis.Analyzer` tetap sama, cuma wiring yang beda) — tidak ada sunk cost kalau mulai dari standalone.

**Task turunan (5 menit, bukan diskusi panjang)**: cek langsung apakah `.golangci.yml`/`.golangci.yaml` sudah ada dan aktif dipakai di CI `backend-web-ciputra-staging`.
- [ ] Kalau **ada dan aktif** → balik ke pendekatan custom plugin (analyzer terpisah dari linter yang sudah jadi kebiasaan tim gampang keabaikan).
- [ ] Kalau **belum ada** → lanjut standalone sesuai rekomendasi di atas.

### 10.5 Sign-off untuk Gate Check (Fase 5)
**Rekomendasi proses proporsional** (tetap ada checkpoint, tidak seberat proses enterprise):
1. **Kriteria observasi terukur** — tentukan angka konkret sebelum Fase 4-5 dimulai, misal: dari N PR terakhir yang di-comment, berapa yang risk-tag-nya keliru menurut penilaian reviewer manusia. Sarankan threshold: false-positive rate **>15% → gate check ditunda, rules direvisi dulu**.
2. **Approval lightweight** — bukan dokumen formal; cukup satu pesan eksplisit ke channel tim/lead ("mau aktifkan gate check untuk PR high-risk mulai [tanggal], berdasarkan data observasi ini — ada keberatan?") dengan window waktu sebelum benar-benar diaktifkan.
3. **Selalu sediakan jalur override manual** — pastikan admin repo tetap bisa bypass required check untuk kasus darurat, supaya tidak ada situasi semua orang stuck karena bug di tool sendiri.

**Status**: tetap **terbuka** sampai Fase 4 selesai dan ada data false-positive rate nyata untuk dibawa ke checkpoint di atas — jangan diaktifkan tanpa data, meski proses approval-nya sendiri sudah didesain ringan.