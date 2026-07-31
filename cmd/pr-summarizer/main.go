package main

import (
	"context"
	"log"
	"os"

	"github.com/rs/zerolog"

	"github.com/YugaAI/PR-Summarizer-CLI/internal/chunker"
	"github.com/YugaAI/PR-Summarizer-CLI/internal/classifier"
	"github.com/YugaAI/PR-Summarizer-CLI/internal/config"
	"github.com/YugaAI/PR-Summarizer-CLI/internal/repository/diff"
	"github.com/YugaAI/PR-Summarizer-CLI/internal/repository/vcs"
	"github.com/YugaAI/PR-Summarizer-CLI/internal/usecase"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	rules, err := classifier.LoadRules(cfg.RiskRulesPath)
	if err != nil {
		log.Fatalf("load risk rules: %v", err)
	}

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	extractor := diff.NewGitExtractor(cfg.BaseRef, cfg.HeadRef)
	vcsClient := vcs.NewGitHubClient(cfg.GitHubToken, cfg.RepoOwner, cfg.RepoName)

	summarizer := usecase.NewSummarizer(extractor, classifier.Classify, rules, chunker.NewFileChunker(), vcsClient, logger)

	if err := summarizer.Run(context.Background(), cfg.PRNumber); err != nil {
		log.Fatalf("run: %v", err)
	}
}
