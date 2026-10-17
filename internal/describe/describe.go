// Package describe is ContextManager. It finds the configuration of a
// project, loads the policy engine, and gives the layers, the registered
// services, and the rules. sv context writes them for an agent. It does not
// read or write stdio.
package describe

import (
	"context"

	"github.com/tarrball/specval/internal/config"
	"github.com/tarrball/specval/internal/policy"
)

// Service is a registered service. See policy.Service.
type Service = policy.Service

// RuleDoc is the documentation of a rule. See policy.RuleDoc.
type RuleDoc = policy.RuleDoc

// Project holds what an agent must know to write a spec for a project.
type Project struct {
	// SpecsDir is the specs directory, relative to the project root.
	SpecsDir     string
	Architecture policy.Architecture
	Services     map[string]Service
	Rules        []RuleDoc
}

// Find describes the project that contains dir.
func Find(ctx context.Context, dir string) (*Project, error) {
	cfg, err := config.Find(dir)
	if err != nil {
		return nil, err
	}
	eng, err := policy.New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	arch, err := eng.Architecture()
	if err != nil {
		return nil, err
	}
	services, err := eng.Services()
	if err != nil {
		return nil, err
	}
	return &Project{
		SpecsDir:     cfg.SpecsDir,
		Architecture: arch,
		Services:     services,
		Rules:        eng.RuleDocs(),
	}, nil
}
