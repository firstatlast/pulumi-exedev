// Package exedev is a Pulumi provider for exe.dev VMs. The exe.dev API is a single
// endpoint, POST https://exe.dev/exec, whose body is a CLI command run with bearer auth.
package exedev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DefaultEndpoint is the exe.dev exec endpoint.
const DefaultEndpoint = "https://exe.dev/exec"

// Client is a thin wrapper over the exe.dev /exec HTTPS API.
type Client struct {
	http     *http.Client
	endpoint string
	token    string
}

// NewClient builds a Client. endpoint may be empty to use DefaultEndpoint.
func NewClient(token, endpoint string) *Client {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	return &Client{
		http:     &http.Client{Timeout: 45 * time.Second},
		endpoint: endpoint,
		token:    token,
	}
}

// APIError is returned when the /exec endpoint responds with a non-2xx status.
type APIError struct {
	Status  int
	Command string
	Body    string
}

func (e *APIError) Error() string {
	msg := strings.TrimSpace(e.Body)
	switch e.Status {
	case http.StatusUnauthorized:
		return fmt.Sprintf("exe.dev: invalid or expired token (401): %s", msg)
	case http.StatusForbidden:
		return fmt.Sprintf("exe.dev: command %q not allowed by token permissions (403): %s", e.Command, msg)
	case http.StatusNotFound:
		return fmt.Sprintf("exe.dev: unknown command %q (404): %s", e.Command, msg)
	case http.StatusUnprocessableEntity:
		return fmt.Sprintf("exe.dev: command failed (422): %s", msg)
	case http.StatusGatewayTimeout:
		return fmt.Sprintf("exe.dev: command timed out (504): %s", msg)
	case http.StatusTooManyRequests:
		return fmt.Sprintf("exe.dev: rate limited (429): %s", msg)
	default:
		return fmt.Sprintf("exe.dev: request failed (%d): %s", e.Status, msg)
	}
}

func (c *Client) Exec(ctx context.Context, command string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewBufferString(command))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exe.dev: %q: %w", command, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{Status: resp.StatusCode, Command: command, Body: string(body)}
	}
	return body, nil
}

// VM is the JSON shape exe.dev returns from ls/new. Byte-based size fields are
// surfaced as outputs only; their GB convention is inconsistent across VMs.
type VM struct {
	Name          string   `json:"vm_name"`
	HTTPSURL      string   `json:"https_url"`
	SSHDest       string   `json:"ssh_dest"`
	Region        string   `json:"region"`
	RegionDisplay string   `json:"region_display"`
	Status        string   `json:"status"`
	Image         string   `json:"image,omitempty"`
	Comment       string   `json:"comment,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	AllocatedCPUs int      `json:"allocated_cpus,omitempty"`
	MemoryBytes   int64    `json:"memory_capacity_bytes,omitempty"`
	DiskBytes     int64    `json:"disk_capacity_bytes,omitempty"`
}

func (c *Client) List(ctx context.Context, pattern string) ([]VM, error) {
	cmd := newCmd("ls")
	if pattern != "" {
		cmd.literal(pattern)
	}
	cmd.raw("--json")
	body, err := c.Exec(ctx, cmd.String())
	if err != nil {
		return nil, err
	}
	var wrap struct {
		VMs []VM `json:"vms"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, fmt.Errorf("exe.dev: parsing ls response: %w: %s", err, string(body))
	}
	return wrap.VMs, nil
}

// Get returns the VM with the exact name, or (nil, false) if it does not exist.
func (c *Client) Get(ctx context.Context, name string) (*VM, bool, error) {
	vms, err := c.List(ctx, name)
	if err != nil {
		return nil, false, err
	}
	for i := range vms {
		if vms[i].Name == name {
			return &vms[i], true, nil
		}
	}
	return nil, false, nil
}

type CreateArgs struct {
	Name         string
	Image        string
	CPU          *int
	Memory       string
	Disk         string
	Tags         []string
	Env          map[string]string
	SetupScript  string
	Command      string
	Comment      string
	Integrations []string
	Prompt       string
	RegistryAuth string
	NoEmail      bool
}

func (c *Client) Create(ctx context.Context, a CreateArgs) (*VM, error) {
	cmd := newCmd("new")
	if a.Name != "" {
		cmd.flag("name", a.Name)
	}
	if a.Image != "" {
		cmd.flag("image", a.Image)
	}
	if a.CPU != nil {
		cmd.flag("cpu", strconv.Itoa(*a.CPU))
	}
	if a.Memory != "" {
		cmd.flag("memory", a.Memory)
	}
	if a.Disk != "" {
		cmd.flag("disk", a.Disk)
	}
	for _, t := range a.Tags {
		cmd.flag("tag", t)
	}
	for _, k := range sortedKeys(a.Env) { // sorted for stable commands
		cmd.flag("env", k+"="+a.Env[k])
	}
	if a.SetupScript != "" {
		cmd.flag("setup-script", a.SetupScript)
	}
	if a.Command != "" {
		cmd.flag("command", a.Command)
	}
	if a.Comment != "" {
		cmd.flag("comment", a.Comment)
	}
	for _, in := range a.Integrations {
		cmd.flag("integration", in)
	}
	if a.Prompt != "" {
		cmd.flag("prompt", a.Prompt)
	}
	if a.RegistryAuth != "" {
		cmd.flag("registry-auth", a.RegistryAuth)
	}
	if a.NoEmail {
		cmd.raw("--no-email")
	}
	cmd.raw("--json")

	body, err := c.Exec(ctx, cmd.String())
	if err != nil {
		return nil, err
	}
	return parseVM(body)
}

// Delete treats an already-absent VM as success.
func (c *Client) Delete(ctx context.Context, name string) error {
	cmd := newCmd("rm")
	cmd.literal(name)
	cmd.raw("--json")
	_, err := c.Exec(ctx, cmd.String())
	var apiErr *APIError
	if err != nil && errors.As(err, &apiErr) && apiErr.Status == http.StatusUnprocessableEntity &&
		strings.Contains(strings.ToLower(apiErr.Body), "not found") {
		return nil
	}
	return err
}

func (c *Client) Resize(ctx context.Context, name string, cpu *int, memory, disk string) error {
	cmd := newCmd("resize")
	cmd.literal(name)
	if cpu != nil {
		cmd.flag("cpu", strconv.Itoa(*cpu))
	}
	if memory != "" {
		cmd.flag("memory", memory)
	}
	if disk != "" {
		cmd.flag("disk", disk)
	}
	cmd.raw("--json")
	_, err := c.Exec(ctx, cmd.String())
	return err
}

func (c *Client) AddTags(ctx context.Context, name string, tags []string) error {
	if len(tags) == 0 {
		return nil
	}
	cmd := newCmd("tag")
	cmd.literal(name)
	for _, t := range tags {
		cmd.literal(t)
	}
	cmd.raw("--json")
	_, err := c.Exec(ctx, cmd.String())
	return err
}

func (c *Client) RemoveTags(ctx context.Context, name string, tags []string) error {
	if len(tags) == 0 {
		return nil
	}
	cmd := newCmd("tag")
	cmd.raw("-d")
	cmd.literal(name)
	for _, t := range tags {
		cmd.literal(t)
	}
	cmd.raw("--json")
	_, err := c.Exec(ctx, cmd.String())
	return err
}

// SetComment sets the comment; an empty text clears it.
func (c *Client) SetComment(ctx context.Context, name, text string) error {
	cmd := newCmd("comment")
	cmd.literal(name)
	cmd.literal(text)
	cmd.raw("--json")
	_, err := c.Exec(ctx, cmd.String())
	return err
}

// Domain is a custom domain attached to a VM. Field names are parsed leniently
// since the exact `domain ls --json` shape is not yet verified against the API.
type DomainInfo struct {
	VM       string `json:"vm_name,omitempty"`
	Domain   string `json:"domain,omitempty"`
	Hostname string `json:"hostname,omitempty"`
	Verified bool   `json:"verified,omitempty"`
	Wildcard bool   `json:"wildcard,omitempty"`
}

// name returns whichever field carries the hostname.
func (d DomainInfo) name() string {
	if d.Domain != "" {
		return d.Domain
	}
	return d.Hostname
}

func (c *Client) DomainAdd(ctx context.Context, vm, hostname string, wildcard bool) error {
	cmd := newCmd("domain")
	cmd.raw("add")
	if wildcard {
		cmd.raw("--wildcard")
	}
	cmd.literal(vm)
	cmd.literal(hostname)
	cmd.raw("--json")
	_, err := c.Exec(ctx, cmd.String())
	return err
}

func (c *Client) DomainRemove(ctx context.Context, vm, hostname string) error {
	cmd := newCmd("domain")
	cmd.raw("rm")
	cmd.literal(vm)
	cmd.literal(hostname)
	cmd.raw("--json")
	_, err := c.Exec(ctx, cmd.String())
	var apiErr *APIError
	if err != nil && errors.As(err, &apiErr) && apiErr.Status == http.StatusUnprocessableEntity &&
		strings.Contains(strings.ToLower(apiErr.Body), "not found") {
		return nil
	}
	return err
}

// DomainGet returns the domain on vm matching hostname, or (nil, false).
func (c *Client) DomainGet(ctx context.Context, vm, hostname string) (*DomainInfo, bool, error) {
	cmd := newCmd("domain")
	cmd.raw("ls")
	cmd.literal(vm)
	cmd.raw("--json")
	body, err := c.Exec(ctx, cmd.String())
	if err != nil {
		return nil, false, err
	}
	domains, err := parseDomains(body)
	if err != nil {
		return nil, false, err
	}
	for i := range domains {
		if domains[i].name() == hostname {
			return &domains[i], true, nil
		}
	}
	return nil, false, nil
}

func (c *Client) SharePort(ctx context.Context, vm string, port int) error {
	cmd := newCmd("share")
	cmd.raw("port")
	cmd.literal(vm)
	cmd.literal(strconv.Itoa(port))
	cmd.raw("--json")
	_, err := c.Exec(ctx, cmd.String())
	return err
}

// ShareSetPublic flips the VM proxy between public and authenticated-only.
func (c *Client) ShareSetPublic(ctx context.Context, vm string, public bool) error {
	cmd := newCmd("share")
	if public {
		cmd.raw("set-public")
	} else {
		cmd.raw("set-private")
	}
	cmd.literal(vm)
	cmd.raw("--json")
	_, err := c.Exec(ctx, cmd.String())
	return err
}

// cmd accumulates shell-quoted command tokens.
type cmd struct{ parts []string }

func newCmd(sub string) *cmd { return &cmd{parts: []string{sub}} }

func (c *cmd) raw(tok string)     { c.parts = append(c.parts, tok) }
func (c *cmd) literal(val string) { c.parts = append(c.parts, shellQuote(val)) }
func (c *cmd) flag(name, val string) {
	c.parts = append(c.parts, "--"+name+"="+shellQuote(val))
}
func (c *cmd) String() string { return strings.Join(c.parts, " ") }

var shellSafe = regexp.MustCompile(`^[a-zA-Z0-9_./:@=,+-]+$`)

// shellQuote applies POSIX single-quote rules for the exe.dev shell-words parser.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// parseVM tolerates the shapes exe.dev may return: bare object, {"vm":..}, {"vms":[..]}.
func parseVM(data []byte) (*VM, error) {
	var wrap struct {
		VM  *VM  `json:"vm"`
		VMs []VM `json:"vms"`
	}
	if err := json.Unmarshal(data, &wrap); err == nil {
		if wrap.VM != nil && wrap.VM.Name != "" {
			return wrap.VM, nil
		}
		if len(wrap.VMs) > 0 {
			return &wrap.VMs[0], nil
		}
	}
	var vm VM
	if err := json.Unmarshal(data, &vm); err == nil && vm.Name != "" {
		return &vm, nil
	}
	return nil, fmt.Errorf("exe.dev: could not parse VM from response: %s", string(data))
}

// parseDomains tolerates {"domains":[..]} or a bare array from `domain ls`.
func parseDomains(data []byte) ([]DomainInfo, error) {
	var wrap struct {
		Domains []DomainInfo `json:"domains"`
	}
	if err := json.Unmarshal(data, &wrap); err == nil && wrap.Domains != nil {
		return wrap.Domains, nil
	}
	var arr []DomainInfo
	if err := json.Unmarshal(data, &arr); err == nil {
		return arr, nil
	}
	return nil, fmt.Errorf("exe.dev: could not parse domains from response: %s", string(data))
}

var sizeRe = regexp.MustCompile(`^\s*(\d+)\s*([a-zA-Z]*)\s*$`)

// SizeToGB parses "4", "4GB" or "8G" to an integer of gigabytes.
func SizeToGB(s string) (int, error) {
	m := sizeRe.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("invalid size %q (expected e.g. 20, 20GB, 20G)", s)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	unit := strings.ToUpper(m[2])
	switch unit {
	case "", "G", "GB":
		return n, nil
	default:
		return 0, fmt.Errorf("unsupported size unit in %q (only GB is supported)", s)
	}
}

// NormalizeSize canonicalizes a size to "<n>GB" so 4/4G/4GB diff as equal.
func NormalizeSize(s string) (string, error) {
	n, err := SizeToGB(s)
	if err != nil {
		return "", err
	}
	return strconv.Itoa(n) + "GB", nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// simple insertion sort to avoid importing sort for a tiny slice
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}
