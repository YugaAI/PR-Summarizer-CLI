package classifier

import (
	"os"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"

	"github.com/YugaAI/PR-Summarizer-CLI/internal/domain"
)

type RiskRules struct {
	High   []Rule `yaml:"high"`
	Medium []Rule `yaml:"medium"`
	Low    []Rule `yaml:"low"`
}

type Rule struct {
	Pattern string `yaml:"pattern"`
	Reason  string `yaml:"reason"`
}

func LoadRules(path string) (RiskRules, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RiskRules{}, err
	}
	var rules RiskRules
	if err := yaml.Unmarshal(data, &rules); err != nil {
		return RiskRules{}, err
	}
	return rules, nil
}

// Classify is a pure function, no I/O. A file that doesn't match any rule
// defaults to RiskMedium: an unclassified change is safer to over-flag for
// review than to silently treat as low risk.
func Classify(files []domain.DiffFile, rules RiskRules) []domain.DiffFile {
	result := make([]domain.DiffFile, len(files))
	for i, f := range files {
		f.Risk, f.RiskReason = classifyOne(f.Path, rules)
		result[i] = f
	}
	return result
}

func classifyOne(path string, rules RiskRules) (domain.RiskLevel, string) {
	if rule, ok := match(path, rules.High); ok {
		return domain.RiskHigh, rule.Reason
	}
	if rule, ok := match(path, rules.Medium); ok {
		return domain.RiskMedium, rule.Reason
	}
	if rule, ok := match(path, rules.Low); ok {
		return domain.RiskLow, rule.Reason
	}
	return domain.RiskMedium, "no matching rule; defaulted to medium"
}

func match(path string, rules []Rule) (Rule, bool) {
	for _, r := range rules {
		if ok, _ := doublestar.Match(r.Pattern, path); ok {
			return r, true
		}
	}
	return Rule{}, false
}
