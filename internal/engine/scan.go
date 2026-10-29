// Package engine connects inventory sources, grading, and the shared report.
package engine

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/Nomadcxx/privacybit/internal/audit"
	"github.com/Nomadcxx/privacybit/internal/catalog"
	"github.com/Nomadcxx/privacybit/internal/detect"
	"github.com/Nomadcxx/privacybit/internal/network"
	"github.com/Nomadcxx/privacybit/internal/report"
)

type SourceSpec struct {
	Type   catalog.DetectType
	Source detect.Source
}

type Options struct {
	Runner    detect.Runner
	Sources   []SourceSpec
	Network   bool
	Family    string
	Now       func() time.Time
	Reporting report.BuildOptions
}

func Run(ctx context.Context, set *catalog.Set, opts Options) (report.Snapshot, error) {
	if set == nil {
		return report.Snapshot{}, fmt.Errorf("catalogue is required")
	}
	if err := ctx.Err(); err != nil {
		return report.Snapshot{}, err
	}
	started := time.Now()
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Runner == nil {
		opts.Runner = detect.ExecRunner{}
	}
	if opts.Sources == nil {
		opts.Sources = defaultSources(set, opts.Runner)
	}
	if opts.Family == "" {
		opts.Family = detect.DistributionFamily("/etc/os-release")
		if opts.Family == "" {
			opts.Family = fallbackFamily(opts.Runner)
		}
	}

	var observations []detect.Observation
	coverage := make([]audit.SourceCoverage, 0, len(opts.Sources))
	for _, spec := range opts.Sources {
		if spec.Source == nil {
			continue
		}
		status := spec.Source.Availability()
		entry := audit.SourceCoverage{
			Name: spec.Source.Name(), Type: spec.Type, Availability: status,
			Capabilities: spec.Source.Capabilities(),
		}
		if status.Ready && !status.Empty {
			items, err := spec.Source.Inventory(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return report.Snapshot{}, ctx.Err()
				}
				entry.Availability = detect.NotReady(spec.Source.Name() + " inventory failed: " + err.Error())
			} else {
				entry.Observed = len(items)
				observations = append(observations, items...)
			}
		}
		coverage = append(coverage, entry)
	}
	result := audit.Evaluate(set, observations, coverage)
	var netResult network.Result
	if opts.Network {
		netResult = network.Inspect(ctx, opts.Runner)
	} else {
		netResult = network.UnknownResult("network checks disabled by --no-network")
	}
	applyNetwork(set, &result, netResult)
	family := opts.Family
	findings := audit.BuildFindings(set, result, family)
	findings = append(findings, networkFindings(netResult)...)
	sort.Slice(findings, func(i, j int) bool { return findings[i].ID < findings[j].ID })
	return report.Build(set, result, findings, netResult, coverage, opts.Now(), time.Since(started), opts.Reporting), nil
}

func defaultSources(set *catalog.Set, runner detect.Runner) []SourceSpec {
	return []SourceSpec{
		{Type: catalog.DetectPackage, Source: detect.NewPacman(runner, func(name string) bool {
			return set.Match(catalog.DetectPackage, name) != nil
		})},
		{Type: catalog.DetectPackage, Source: detect.NewDpkg(runner)},
		{Type: catalog.DetectFlatpak, Source: detect.NewFlatpak(runner)},
		{Type: catalog.DetectDesktop, Source: detect.NewDesktop()},
		{Type: catalog.DetectBinary, Source: detect.NewBinary()},
	}
}

func fallbackFamily(runner detect.Runner) string {
	for _, item := range []struct{ family, binary string }{
		{"arch", "pacman"}, {"debian", "dpkg-query"}, {"fedora", "dnf"},
		{"suse", "zypper"}, {"alpine", "apk"}, {"xbps", "xbps-query"},
	} {
		if _, err := runner.LookPath(item.binary); err == nil {
			return item.family
		}
	}
	return ""
}

func applyNetwork(set *catalog.Set, result *audit.Result, netResult network.Result) {
	if set == nil || result == nil {
		return
	}
	category := set.Category("vpn")
	if category == nil {
		return
	}
	installedExpectationMet := false
	for _, observed := range result.Categories {
		if observed.ID != category.ID {
			continue
		}
		for _, expectation := range observed.Expectations {
			if expectation.ID == "vpn.installed" && expectation.Met {
				installedExpectationMet = true
			}
		}
	}
	for i := range result.Categories {
		if result.Categories[i].ID != category.ID {
			continue
		}
		if netResult.State == network.Unknown && installedExpectationMet {
			result.Categories[i].Grade = catalog.GradeUnknown
		}
		if len(netResult.Defects) > 0 {
			result.Categories[i].Grade = catalog.Worse(result.Categories[i].Grade, catalog.GradeRed)
		}
	}
	if len(result.Categories) > 0 || result.Sandbox.Applicable {
		result.Overall = catalog.GradeGreen
		for _, item := range result.Categories {
			result.Overall = catalog.Worse(result.Overall, item.Grade)
		}
		if result.Sandbox.Applicable {
			result.Overall = catalog.Worse(result.Overall, result.Sandbox.Grade)
		}
	}
}

func networkFindings(result network.Result) []audit.Finding {
	findings := make([]audit.Finding, 0, len(result.Defects))
	for _, defect := range result.Defects {
		findings = append(findings, audit.Finding{
			ID: defect.ID, Severity: "high", Grade: catalog.GradeRed, Category: "vpn",
			Title: defect.Title, Detail: defect.Detail,
			Actions: []audit.Action{{Tier: 0, Kind: audit.ActionInform, Label: "Read evidence", Target: defect.ID}},
		})
	}
	return findings
}
