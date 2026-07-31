package config

import (
	"time"

	"github.com/caarlos0/env/v10"
)

type Config struct {
	GitHubToken      string        `env:"GITHUB_TOKEN,required"`
	MimoAPIKey       string        `env:"MIMO_API_KEY,required"`
	BaseRef          string        `env:"GITHUB_BASE_REF,required"`
	HeadRef          string        `env:"GITHUB_HEAD_REF,required"`
	RepoOwner        string        `env:"GITHUB_REPOSITORY_OWNER,required"`
	RepoName         string        `env:"GITHUB_REPO_NAME,required"`
	PRNumber         int           `env:"PR_NUMBER,required"`
	TokenBudget      int           `env:"TOKEN_BUDGET" envDefault:"8000"`
	Concurrency      int           `env:"LLM_CONCURRENCY" envDefault:"3"`
	Timeout          time.Duration `env:"TIMEOUT" envDefault:"120s"`
	CacheDir         string        `env:"CACHE_DIR" envDefault:".pr-summary-cache"`
	RiskRulesPath    string        `env:"RISK_RULES_PATH" envDefault:"configs/risk_rules.yaml"`
	SkipPatternsPath string        `env:"SKIP_PATTERNS_PATH" envDefault:"configs/skip_patterns.yaml"`
	GateEnabled      bool          `env:"GATE_ENABLED" envDefault:"false"`
}

func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
