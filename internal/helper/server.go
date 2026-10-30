// Package helper implements the strict, short-lived protocol used by the
// privileged privacybit-helper process.
package helper

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Nomadcxx/privacybit/internal/catalog"
)

const (
	Protocol    = 1
	TokenWindow = 5 * time.Minute
	maxLine     = 64 << 10
)

// buildVersion is injected by package builds; source builds report "devel".
var buildVersion = "devel"

var packageNamePattern = regexp.MustCompile(`^[A-Za-z0-9@._+:-]+$`)

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

type Options struct {
	Catalog *catalog.Set
	Runner  CommandRunner
	Now     func() time.Time
	Token   func() (string, error)
	IsRoot  func() bool
	Log     func(AuditRecord) error
}

type Request struct {
	Op               string `json:"op"`
	Entry            string `json:"entry,omitempty"`
	Unit             string `json:"unit,omitempty"`
	State            string `json:"state,omitempty"`
	Token            string `json:"token,omitempty"`
	AuditGeneratedAt string `json:"audit_generated_at,omitempty"`
}

type Removal struct {
	Package        string   `json:"package"`
	RequiredBy     []string `json:"required_by,omitempty"`
	InstalledBytes int64    `json:"installed_bytes"`
}

type Plan struct {
	Entry               string    `json:"entry"`
	AuditGeneratedAt    string    `json:"audit_generated_at,omitempty"`
	PlannedAt           time.Time `json:"planned_at"`
	ExpiresAt           time.Time `json:"expires_at"`
	Removals            []Removal `json:"removals"`
	ReverseDependencies []string  `json:"reverse_dependencies"`
	ReclaimBytes        int64     `json:"reclaim_bytes"`
	Token               string    `json:"token"`
}

type AuditRecord struct {
	At       time.Time `json:"at"`
	Op       string    `json:"op"`
	Entry    string    `json:"entry,omitempty"`
	Unit     string    `json:"unit,omitempty"`
	State    string    `json:"state,omitempty"`
	Packages []string  `json:"packages,omitempty"`
	Result   string    `json:"result"`
	Error    string    `json:"error,omitempty"`
}

type Event struct {
	Type       string `json:"t"`
	Protocol   int    `json:"protocol,omitempty"`
	CatalogRev string `json:"catalog_rev,omitempty"`
	Version    string `json:"version,omitempty"`
	Op         string `json:"op,omitempty"`
	OK         bool   `json:"ok,omitempty"`
	Error      string `json:"error,omitempty"`
	Plan       *Plan  `json:"plan,omitempty"`
}

type pendingPlan struct {
	entry     string
	auditAt   string
	hash      string
	token     string
	expiresAt time.Time
}

// Serve runs one helper session. The readiness event is a transport handshake;
// requests remain untrusted until strict decoding and validation complete.
func Serve(ctx context.Context, input io.Reader, output io.Writer, opts Options) error {
	if opts.Catalog == nil {
		return errors.New("embedded catalogue is required")
	}
	if opts.Runner == nil {
		opts.Runner = ExecRunner{}
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Token == nil {
		opts.Token = randomToken
	}
	if opts.IsRoot == nil {
		opts.IsRoot = func() bool { return os.Geteuid() == 0 }
	}

	enc := json.NewEncoder(output)
	if err := enc.Encode(Event{Type: "ready", Protocol: Protocol, CatalogRev: opts.Catalog.Revision}); err != nil {
		return err
	}

	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), maxLine)
	var pending *pendingPlan
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		request, err := decodeRequest(scanner.Bytes())
		if err != nil {
			_ = enc.Encode(Event{Type: "error", Error: err.Error()})
			return err
		}
		result, err := handle(ctx, request, opts, &pending)
		if err != nil {
			result = Event{Type: "result", Op: request.Op, Error: err.Error()}
		}
		if err := enc.Encode(result); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read helper request: %w", err)
	}
	return nil
}

func decodeRequest(line []byte) (Request, error) {
	dec := json.NewDecoder(bytes.NewReader(line))
	token, err := dec.Token()
	if err != nil {
		return Request{}, fmt.Errorf("parse helper request: %w", err)
	}
	if token != json.Delim('{') {
		return Request{}, errors.New("helper request must be a JSON object")
	}
	values := make(map[string]json.RawMessage, 5)
	for dec.More() {
		token, err = dec.Token()
		if err != nil {
			return Request{}, fmt.Errorf("parse helper request key: %w", err)
		}
		key, ok := token.(string)
		if !ok {
			return Request{}, errors.New("helper request key must be a string")
		}
		if _, exists := values[key]; exists {
			return Request{}, fmt.Errorf("duplicate helper request field %q", key)
		}
		if !knownField(key) {
			return Request{}, fmt.Errorf("unknown field %q", key)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return Request{}, fmt.Errorf("parse helper request field %q: %w", key, err)
		}
		values[key] = raw
	}
	if _, err := dec.Token(); err != nil {
		return Request{}, fmt.Errorf("close helper request: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Request{}, errors.New("helper request has trailing JSON")
		}
		return Request{}, fmt.Errorf("parse trailing helper request data: %w", err)
	}

	var request Request
	for key, raw := range values {
		if len(raw) == 0 || raw[0] != '"' {
			return Request{}, fmt.Errorf("helper request field %q must be a string", key)
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return Request{}, fmt.Errorf("helper request field %q must be a string", key)
		}
		switch key {
		case "op":
			request.Op = value
		case "entry":
			request.Entry = value
		case "unit":
			request.Unit = value
		case "state":
			request.State = value
		case "token":
			request.Token = value
		case "audit_generated_at":
			request.AuditGeneratedAt = value
		}
	}
	if request.Op == "" {
		return Request{}, errors.New("helper request is missing op")
	}
	if err := validateRequestFields(request, values); err != nil {
		return Request{}, err
	}
	return request, nil
}

func knownField(key string) bool {
	switch key {
	case "op", "entry", "unit", "state", "token", "audit_generated_at":
		return true
	default:
		return false
	}
}

func validateRequestFields(request Request, fields map[string]json.RawMessage) error {
	allowed := map[string]bool{"op": true}
	switch request.Op {
	case "ping", "status":
	case "set-unit":
		allowed["unit"], allowed["state"] = true, true
	case "plan-purge":
		allowed["entry"], allowed["audit_generated_at"] = true, true
	case "purge-entry":
		allowed["entry"], allowed["token"], allowed["audit_generated_at"] = true, true, true
	default:
		return fmt.Errorf("unknown helper op %q", request.Op)
	}
	for key := range fields {
		if !allowed[key] {
			return fmt.Errorf("field %q is not valid for op %q", key, request.Op)
		}
	}
	switch request.Op {
	case "set-unit":
		if request.Unit == "" || request.State == "" {
			return errors.New("set-unit requires unit and state")
		}
	case "plan-purge":
		if request.Entry == "" || request.AuditGeneratedAt == "" {
			return errors.New("plan-purge requires entry and audit_generated_at")
		}
	case "purge-entry":
		if request.Entry == "" || request.Token == "" || request.AuditGeneratedAt == "" {
			return errors.New("purge-entry requires entry, token, and audit_generated_at")
		}
		if _, err := time.Parse(time.RFC3339Nano, request.AuditGeneratedAt); err != nil {
			return errors.New("audit_generated_at must be RFC3339")
		}
	}
	if request.Op == "plan-purge" {
		if _, err := time.Parse(time.RFC3339Nano, request.AuditGeneratedAt); err != nil {
			return errors.New("audit_generated_at must be RFC3339")
		}
	}
	return nil
}

func handle(ctx context.Context, request Request, opts Options, pending **pendingPlan) (Event, error) {
	switch request.Op {
	case "ping":
		return Event{Type: "result", Op: request.Op, OK: true}, nil
	case "status":
		return Event{Type: "result", Op: request.Op, OK: true, Version: buildVersion, CatalogRev: opts.Catalog.Revision}, nil
	case "set-unit":
		if !opts.IsRoot() {
			return Event{}, errors.New("set-unit requires root")
		}
		if request.Unit != "fwupd-refresh.timer" {
			return Event{}, fmt.Errorf("unit %q is not allowlisted", request.Unit)
		}
		verb := "disable"
		if request.State == "enabled" {
			verb = "enable"
		} else if request.State != "disabled" {
			return Event{}, fmt.Errorf("unknown unit state %q", request.State)
		}
		args := []string{"--system", verb, "--now", "fwupd-refresh.timer"}
		return runMutation(ctx, request, opts, nil, "systemctl", args...)
	case "plan-purge":
		if !opts.IsRoot() {
			return Event{}, errors.New("plan-purge requires root")
		}
		plan, hash, err := makePlan(ctx, request, opts)
		if err != nil {
			return Event{}, err
		}
		token, err := opts.Token()
		if err != nil {
			return Event{}, fmt.Errorf("create review token: %w", err)
		}
		plan.Token = token
		*pending = &pendingPlan{entry: request.Entry, auditAt: request.AuditGeneratedAt,
			hash: hash, token: token, expiresAt: plan.ExpiresAt}
		return Event{Type: "result", Op: request.Op, OK: true, Plan: &plan}, nil
	case "purge-entry":
		if !opts.IsRoot() {
			return Event{}, errors.New("purge-entry requires root")
		}
		plan := *pending
		if plan == nil || request.Token != plan.token || request.Entry != plan.entry || request.AuditGeneratedAt != plan.auditAt {
			return Event{}, errors.New("review token is missing or does not match this session plan")
		}
		*pending = nil // A valid token is one-use even when revalidation fails.
		if !opts.Now().Before(plan.expiresAt) {
			return Event{}, errors.New("review token expired; create a new plan")
		}
		current, hash, err := makePlan(ctx, request, opts)
		if err != nil {
			return Event{}, err
		}
		if hash != plan.hash {
			return Event{}, errors.New("package plan changed; create a new plan")
		}
		packages := make([]string, 0, len(current.Removals))
		for _, removal := range current.Removals {
			packages = append(packages, removal.Package)
		}
		args := append([]string{"-Rn", "--noconfirm", "--"}, packages...)
		return runMutation(ctx, request, opts, packages, "pacman", args...)
	default:
		return Event{}, fmt.Errorf("unknown helper op %q", request.Op)
	}
}

func makePlan(ctx context.Context, request Request, opts Options) (Plan, string, error) {
	entry := opts.Catalog.Entry(request.Entry)
	if entry == nil {
		return Plan{}, "", fmt.Errorf("unknown catalogue entry %q", request.Entry)
	}
	var candidates []string
	for _, detect := range entry.Detects {
		if detect.Type == catalog.DetectPackage {
			if !packageNamePattern.MatchString(detect.Name) {
				return Plan{}, "", fmt.Errorf("catalogue package name %q is invalid", detect.Name)
			}
			candidates = append(candidates, detect.Name)
		}
	}
	if len(candidates) == 0 {
		return Plan{}, "", errors.New("entry has no pacman package rule; purge is unsupported")
	}
	sort.Strings(candidates)
	installedOutput, err := opts.Runner.Run(ctx, "pacman", "-Qq")
	if err != nil {
		return Plan{}, "", fmt.Errorf("read pacman inventory: %w", err)
	}
	installed, err := parseNames(installedOutput, true)
	if err != nil {
		return Plan{}, "", fmt.Errorf("parse pacman inventory: %w", err)
	}
	installedSet := make(map[string]bool, len(installed))
	for _, name := range installed {
		installedSet[name] = true
	}
	var targets []string
	for _, candidate := range candidates {
		if installedSet[candidate] {
			targets = append(targets, candidate)
		}
	}
	if len(targets) == 0 {
		return Plan{}, "", errors.New("no package for this catalogue entry is installed")
	}
	removalSet := make(map[string]bool)
	for _, target := range targets {
		output, err := opts.Runner.Run(ctx, "pacman", "-Rs", "--print", "--print-format=%n", "--", target)
		if err != nil {
			return Plan{}, "", fmt.Errorf("pacman could not plan removal of %s: %w", target, err)
		}
		packages, err := parseNames(output, false)
		if err != nil {
			return Plan{}, "", fmt.Errorf("parse pacman removal plan: %w", err)
		}
		for _, name := range packages {
			if !installedSet[name] {
				return Plan{}, "", fmt.Errorf("pacman plan contains package %q absent from inventory", name)
			}
			removalSet[name] = true
		}
	}
	for _, target := range targets {
		if !removalSet[target] {
			return Plan{}, "", fmt.Errorf("pacman plan omitted requested package %q", target)
		}
	}
	packageNames := make([]string, 0, len(removalSet))
	for name := range removalSet {
		packageNames = append(packageNames, name)
	}
	sort.Strings(packageNames)
	plan := Plan{
		Entry: request.Entry, AuditGeneratedAt: request.AuditGeneratedAt,
		PlannedAt: opts.Now().UTC(), ExpiresAt: opts.Now().UTC().Add(TokenWindow),
	}
	for _, name := range packageNames {
		output, err := opts.Runner.Run(ctx, "pacman", "-Qi", "--", name)
		if err != nil {
			return Plan{}, "", fmt.Errorf("read pacman details for %s: %w", name, err)
		}
		bytes, requiredBy, err := parsePackageInfo(output)
		if err != nil {
			return Plan{}, "", fmt.Errorf("parse pacman details for %s: %w", name, err)
		}
		plan.Removals = append(plan.Removals, Removal{Package: name, RequiredBy: requiredBy, InstalledBytes: bytes})
		plan.ReclaimBytes += bytes
		plan.ReverseDependencies = append(plan.ReverseDependencies, requiredBy...)
	}
	plan.ReverseDependencies = uniqueSorted(plan.ReverseDependencies)
	hashData, err := json.Marshal(struct {
		Entry    string
		AuditAt  string
		Catalog  string
		Removals []Removal
	}{request.Entry, request.AuditGeneratedAt, opts.Catalog.Revision, plan.Removals})
	if err != nil {
		return Plan{}, "", err
	}
	hash := sha256.Sum256(hashData)
	return plan, hex.EncodeToString(hash[:]), nil
}

func parseNames(output string, allowEmpty bool) ([]string, error) {
	var names []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(output, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		if !packageNamePattern.MatchString(name) {
			return nil, fmt.Errorf("invalid package name %q", name)
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate package name %q", name)
		}
		seen[name] = true
		names = append(names, name)
	}
	if len(names) == 0 && !allowEmpty {
		return nil, errors.New("pacman returned an empty package list")
	}
	sort.Strings(names)
	return names, nil
}

func parsePackageInfo(output string) (int64, []string, error) {
	lines := strings.Split(output, "\n")
	var sizeValue, requiredValue string
	for i, line := range lines {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "Installed Size":
			sizeValue = strings.TrimSpace(value)
		case "Required By":
			values := []string{strings.TrimSpace(value)}
			for j := i + 1; j < len(lines) && !strings.Contains(lines[j], ":"); j++ {
				values = append(values, strings.TrimSpace(lines[j]))
			}
			requiredValue = strings.Join(values, " ")
		}
	}
	if sizeValue == "" || requiredValue == "" {
		return 0, nil, errors.New("missing Installed Size or Required By field")
	}
	size, err := parseSize(sizeValue)
	if err != nil {
		return 0, nil, err
	}
	var required []string
	if requiredValue != "None" {
		for _, name := range strings.Fields(requiredValue) {
			if !packageNamePattern.MatchString(name) {
				return 0, nil, fmt.Errorf("invalid reverse dependency %q", name)
			}
			required = append(required, name)
		}
	}
	return size, uniqueSorted(required), nil
}

func parseSize(value string) (int64, error) {
	fields := strings.Fields(value)
	if len(fields) != 2 {
		return 0, fmt.Errorf("invalid installed size %q", value)
	}
	amount, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 {
		return 0, fmt.Errorf("invalid installed size %q", value)
	}
	multiplier := map[string]float64{
		"B": 1, "KiB": 1 << 10, "MiB": 1 << 20, "GiB": 1 << 30, "TiB": 1 << 40,
	}[fields[1]]
	if multiplier == 0 {
		return 0, fmt.Errorf("unsupported installed size %q", value)
	}
	bytes := amount * multiplier
	// int64's upper bound is exclusive; float64(math.MaxInt64) rounds up to 2^63.
	if math.IsInf(bytes, 0) || bytes >= 0x1p63 {
		return 0, fmt.Errorf("unsupported installed size %q", value)
	}
	return int64(math.Round(bytes)), nil
}

func runMutation(ctx context.Context, request Request, opts Options, packages []string, command string, args ...string) (Event, error) {
	if opts.Log == nil {
		return Event{}, errors.New("secure audit log is unavailable; refusing mutation")
	}
	record := AuditRecord{At: opts.Now().UTC(), Op: request.Op, Entry: request.Entry,
		Unit: request.Unit, State: request.State, Packages: append([]string(nil), packages...), Result: "started"}
	if err := opts.Log(record); err != nil {
		return Event{}, fmt.Errorf("write audit record before mutation: %w", err)
	}
	_, runErr := opts.Runner.Run(ctx, command, args...)
	if runErr != nil {
		record.At, record.Result, record.Error = opts.Now().UTC(), "failed", runErr.Error()
		_ = opts.Log(record)
		return Event{}, runErr
	}
	record.At, record.Result = opts.Now().UTC(), "completed"
	if err := opts.Log(record); err != nil {
		return Event{}, fmt.Errorf("mutation completed but final audit record failed: %w", err)
	}
	return Event{Type: "result", Op: request.Op, OK: true}, nil
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func randomToken() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

// ExecRunner limits helper commands to fixed system binaries and forces stable
// English output for the pacman fields used in the reviewed plan.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	path := map[string]string{"pacman": "/usr/bin/pacman", "systemctl": "/usr/bin/systemctl"}[name]
	if path == "" {
		return "", fmt.Errorf("helper command %q is not allowed", name)
	}
	command := exec.CommandContext(ctx, path, args...)
	command.Env = append(os.Environ(), "LC_ALL=C", "COLUMNS=1000")
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return stdout.String(), fmt.Errorf("%s: %s: %w", name, detail, err)
		}
		return stdout.String(), fmt.Errorf("%s: %w", name, err)
	}
	return stdout.String(), nil
}
