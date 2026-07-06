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

// isTransientConnect reports whether err is a "VM not reachable yet" failure —
// share operations need the VM's SSH up, which lags behind status=running.
func isTransientConnect(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusUnprocessableEntity {
		b := strings.ToLower(apiErr.Body)
		return strings.Contains(b, "failed to connect") || strings.Contains(b, "handshake failed")
	}
	return false
}

// retryTransient retries fn while it fails with a transient connect error, up to
// a deadline, so freshly-created VMs settle before share operations run.
func retryTransient(ctx context.Context, fn func() error) error {
	deadline := time.Now().Add(120 * time.Second)
	for {
		err := fn()
		if err == nil || !isTransientConnect(err) || time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(4 * time.Second):
		}
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
	body, err := c.Exec(ctx, cmd.String())
	if err != nil {
		return err
	}
	// domain add reports "DNS does not point to ..." as HTTP 200 with an error body.
	return bodyError(body)
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

// ShareReceiveEmail toggles inbound email for a VM (share receive-email <vm> on|off).
func (c *Client) ShareReceiveEmail(ctx context.Context, vm string, on bool) error {
	cmd := newCmd("share")
	cmd.raw("receive-email")
	cmd.literal(vm)
	if on {
		cmd.raw("on")
	} else {
		cmd.raw("off")
	}
	cmd.raw("--json")
	_, err := c.Exec(ctx, cmd.String())
	return err
}

// ShareAccess toggles team SSH/Shelley/web access (share access allow|disallow <vm>).
func (c *Client) ShareAccess(ctx context.Context, vm string, allow bool) error {
	cmd := newCmd("share")
	cmd.raw("access")
	if allow {
		cmd.raw("allow")
	} else {
		cmd.raw("disallow")
	}
	cmd.literal(vm)
	cmd.raw("--json")
	_, err := c.Exec(ctx, cmd.String())
	return err
}

// SshKeyInfo is the JSON shape from `ssh-key list`. The stored public_key drops
// the comment; name is derived from the comment at add time.
type SshKeyInfo struct {
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
	Name        string `json:"name"`
	Current     bool   `json:"current"`
}

// keyMaterial returns the "type base64" prefix of a public key, ignoring the comment.
func keyMaterial(pubKey string) string {
	f := strings.Fields(pubKey)
	if len(f) >= 2 {
		return f[0] + " " + f[1]
	}
	return strings.TrimSpace(pubKey)
}

func (c *Client) SshKeyAdd(ctx context.Context, publicKey, tag string) error {
	cmd := newCmd("ssh-key")
	cmd.raw("add")
	if tag != "" {
		cmd.flag("tag", tag)
	}
	cmd.literal(publicKey)
	cmd.raw("--json")
	body, err := c.Exec(ctx, cmd.String())
	if err != nil {
		return err
	}
	return bodyError(body)
}

func (c *Client) SshKeyRemove(ctx context.Context, ref string) error {
	cmd := newCmd("ssh-key")
	cmd.raw("remove")
	cmd.literal(ref)
	cmd.raw("--json")
	_, err := c.Exec(ctx, cmd.String())
	var apiErr *APIError
	if err != nil && errors.As(err, &apiErr) && apiErr.Status == http.StatusUnprocessableEntity &&
		strings.Contains(strings.ToLower(apiErr.Body), "not found") {
		return nil
	}
	return err
}

func (c *Client) SshKeyList(ctx context.Context) ([]SshKeyInfo, error) {
	body, err := c.Exec(ctx, "ssh-key list --json")
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Keys []SshKeyInfo `json:"ssh_keys"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, fmt.Errorf("exe.dev: parsing ssh-key list: %w: %s", err, string(body))
	}
	return wrap.Keys, nil
}

// SshKeyByFingerprint finds a key by fingerprint.
func (c *Client) SshKeyByFingerprint(ctx context.Context, fp string) (*SshKeyInfo, bool, error) {
	keys, err := c.SshKeyList(ctx)
	if err != nil {
		return nil, false, err
	}
	for i := range keys {
		if keys[i].Fingerprint == fp {
			return &keys[i], true, nil
		}
	}
	return nil, false, nil
}

// SshKeyByMaterial finds a key by its type+base64 material, ignoring the comment.
func (c *Client) SshKeyByMaterial(ctx context.Context, publicKey string) (*SshKeyInfo, bool, error) {
	keys, err := c.SshKeyList(ctx)
	if err != nil {
		return nil, false, err
	}
	want := keyMaterial(publicKey)
	for i := range keys {
		if keyMaterial(keys[i].PublicKey) == want {
			return &keys[i], true, nil
		}
	}
	return nil, false, nil
}

// IntegrationInfo is the JSON shape from `integrations list`. config is
// type-specific; header values are masked, so they are not readable here.
type IntegrationInfo struct {
	Name        string         `json:"name"`
	Type        string         `json:"type"`
	Comment     string         `json:"comment"`
	Attachments []string       `json:"attachments"`
	Config      map[string]any `json:"config"`
}

func (i IntegrationInfo) Target() string {
	if v, ok := i.Config["target"].(string); ok {
		return v
	}
	return ""
}

// IntegrationSpec are the inputs to add/edit an http-proxy integration.
type IntegrationSpec struct {
	Name        string
	Type        string
	Target      string
	Headers     map[string]string
	Bearer      string
	NoAuth      bool
	Comment     string
	Attachments []string
}

// integrations add/edit accept no --json and return an empty body on success.
func (c *Client) IntegrationAdd(ctx context.Context, s IntegrationSpec) error {
	cmd := newCmd("integrations")
	cmd.raw("add")
	cmd.literal(s.Type)
	cmd.flag("name", s.Name)
	if s.Target != "" {
		cmd.flag("target", s.Target)
	}
	for _, k := range sortedKeys(s.Headers) {
		cmd.flag("header", k+":"+s.Headers[k])
	}
	if s.Bearer != "" {
		cmd.flag("bearer", s.Bearer)
	}
	if s.NoAuth {
		cmd.raw("--no-auth")
	}
	if s.Comment != "" {
		cmd.flag("comment", s.Comment)
	}
	for _, a := range s.Attachments {
		cmd.flag("attach", a)
	}
	body, err := c.Exec(ctx, cmd.String())
	if err != nil {
		return err
	}
	return bodyError(body)
}

// IntegrationEdit re-applies the mutable http-proxy fields.
func (c *Client) IntegrationEdit(ctx context.Context, s IntegrationSpec) error {
	cmd := newCmd("integrations")
	cmd.raw("edit")
	cmd.literal(s.Name)
	if s.Target != "" {
		cmd.flag("target", s.Target)
	}
	for _, k := range sortedKeys(s.Headers) {
		cmd.flag("header", k+":"+s.Headers[k])
	}
	if s.Bearer != "" {
		cmd.flag("bearer", s.Bearer)
	}
	if s.NoAuth {
		cmd.raw("--no-auth")
	}
	cmd.flag("comment", s.Comment)
	body, err := c.Exec(ctx, cmd.String())
	if err != nil {
		return err
	}
	return bodyError(body)
}

func (c *Client) IntegrationRemove(ctx context.Context, name string) error {
	cmd := newCmd("integrations")
	cmd.raw("remove")
	cmd.literal(name)
	_, err := c.Exec(ctx, cmd.String())
	var apiErr *APIError
	if err != nil && errors.As(err, &apiErr) && apiErr.Status == http.StatusUnprocessableEntity &&
		strings.Contains(strings.ToLower(apiErr.Body), "not found") {
		return nil
	}
	return err
}

func (c *Client) IntegrationList(ctx context.Context) ([]IntegrationInfo, error) {
	body, err := c.Exec(ctx, "integrations list --json")
	if err != nil {
		return nil, err
	}
	var list []IntegrationInfo
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("exe.dev: parsing integrations list: %w: %s", err, string(body))
	}
	return list, nil
}

func (c *Client) IntegrationGet(ctx context.Context, name string) (*IntegrationInfo, bool, error) {
	list, err := c.IntegrationList(ctx)
	if err != nil {
		return nil, false, err
	}
	for i := range list {
		if list[i].Name == name {
			return &list[i], true, nil
		}
	}
	return nil, false, nil
}

func (c *Client) IntegrationAttach(ctx context.Context, name, spec string) error {
	cmd := newCmd("integrations")
	cmd.raw("attach")
	cmd.literal(name)
	cmd.literal(spec)
	_, err := c.Exec(ctx, cmd.String())
	return err
}

func (c *Client) IntegrationDetach(ctx context.Context, name, spec string) error {
	cmd := newCmd("integrations")
	cmd.raw("detach")
	cmd.literal(name)
	cmd.literal(spec)
	_, err := c.Exec(ctx, cmd.String())
	return err
}

// TeamMemberInfo is the JSON shape from `team members`. Field names are parsed
// leniently: the exact shape is unverified (requires a team account).
type TeamMemberInfo struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (c *Client) TeamAdd(ctx context.Context, email, role string) error {
	cmd := newCmd("team")
	cmd.raw("add")
	cmd.literal(email)
	if role != "" {
		cmd.literal(role)
	}
	cmd.raw("--json")
	body, err := c.Exec(ctx, cmd.String())
	if err != nil {
		return err
	}
	return bodyError(body)
}

func (c *Client) TeamRole(ctx context.Context, email, role string) error {
	cmd := newCmd("team")
	cmd.raw("role")
	cmd.literal(email)
	cmd.literal(role)
	cmd.raw("--json")
	body, err := c.Exec(ctx, cmd.String())
	if err != nil {
		return err
	}
	return bodyError(body)
}

func (c *Client) TeamRemove(ctx context.Context, email string) error {
	cmd := newCmd("team")
	cmd.raw("remove")
	cmd.literal(email)
	cmd.raw("--json")
	_, err := c.Exec(ctx, cmd.String())
	var apiErr *APIError
	if err != nil && errors.As(err, &apiErr) && apiErr.Status == http.StatusUnprocessableEntity &&
		strings.Contains(strings.ToLower(apiErr.Body), "not found") {
		return nil
	}
	return err
}

func (c *Client) TeamMembers(ctx context.Context) ([]TeamMemberInfo, error) {
	body, err := c.Exec(ctx, "team members --json")
	if err != nil {
		return nil, err
	}
	return parseTeamMembers(body)
}

func (c *Client) TeamMemberByEmail(ctx context.Context, email string) (*TeamMemberInfo, bool, error) {
	members, err := c.TeamMembers(ctx)
	if err != nil {
		return nil, false, err
	}
	for i := range members {
		if members[i].Email == email {
			return &members[i], true, nil
		}
	}
	return nil, false, nil
}

// ShareState is the subset of `share show` used by the share resources.
type ShareState struct {
	Links []ShareLinkInfo `json:"links"`
	Users []ShareUserInfo `json:"users"`
}

type ShareLinkInfo struct {
	Token    string `json:"token"`
	UseCount int    `json:"use_count"`
}

type ShareUserInfo struct {
	Email  string `json:"email"`
	Status string `json:"status"`
}

func (c *Client) ShareShow(ctx context.Context, vm string) (*ShareState, error) {
	cmd := newCmd("share")
	cmd.raw("show")
	cmd.literal(vm)
	cmd.raw("--json")
	body, err := c.Exec(ctx, cmd.String())
	if err != nil {
		return nil, err
	}
	var s ShareState
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, fmt.Errorf("exe.dev: parsing share show: %w: %s", err, string(body))
	}
	return &s, nil
}

// ShareAddLink creates a share link and returns its token and URL.
func (c *Client) ShareAddLink(ctx context.Context, vm string) (token, url string, err error) {
	cmd := newCmd("share")
	cmd.raw("add-link")
	cmd.literal(vm)
	cmd.raw("--json")
	body, err := c.Exec(ctx, cmd.String())
	if err != nil {
		return "", "", err
	}
	var r struct {
		Token string `json:"token"`
		URL   string `json:"url"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", "", fmt.Errorf("exe.dev: parsing add-link: %w: %s", err, string(body))
	}
	return r.Token, r.URL, nil
}

func (c *Client) ShareRemoveLink(ctx context.Context, vm, token string) error {
	cmd := newCmd("share")
	cmd.raw("remove-link")
	cmd.literal(vm)
	cmd.literal(token)
	cmd.raw("--json")
	_, err := c.Exec(ctx, cmd.String())
	var apiErr *APIError
	if err != nil && errors.As(err, &apiErr) && apiErr.Status == http.StatusUnprocessableEntity &&
		strings.Contains(strings.ToLower(apiErr.Body), "not found") {
		return nil
	}
	return err
}

func (c *Client) ShareLinkByToken(ctx context.Context, vm, token string) (*ShareLinkInfo, bool, error) {
	s, err := c.ShareShow(ctx, vm)
	if err != nil {
		return nil, false, err
	}
	for i := range s.Links {
		if s.Links[i].Token == token {
			return &s.Links[i], true, nil
		}
	}
	return nil, false, nil
}

func (c *Client) ShareAddUser(ctx context.Context, vm, email, message string) error {
	cmd := newCmd("share")
	cmd.raw("add")
	cmd.literal(vm)
	cmd.literal(email)
	if message != "" {
		cmd.flag("message", message)
	}
	cmd.raw("--json")
	body, err := c.Exec(ctx, cmd.String())
	if err != nil {
		return err
	}
	return bodyError(body)
}

func (c *Client) ShareRemoveUser(ctx context.Context, vm, email string) error {
	cmd := newCmd("share")
	cmd.raw("remove")
	cmd.literal(vm)
	cmd.literal(email)
	cmd.raw("--json")
	_, err := c.Exec(ctx, cmd.String())
	var apiErr *APIError
	if err != nil && errors.As(err, &apiErr) && apiErr.Status == http.StatusUnprocessableEntity &&
		strings.Contains(strings.ToLower(apiErr.Body), "not found") {
		return nil
	}
	return err
}

func (c *Client) ShareUserByEmail(ctx context.Context, vm, email string) (*ShareUserInfo, bool, error) {
	s, err := c.ShareShow(ctx, vm)
	if err != nil {
		return nil, false, err
	}
	for i := range s.Users {
		if s.Users[i].Email == email {
			return &s.Users[i], true, nil
		}
	}
	return nil, false, nil
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

// parseTeamMembers tolerates {"members":[..]} or a bare array. The shape is
// unverified (requires a team account), so parsing is intentionally lenient.
func parseTeamMembers(data []byte) ([]TeamMemberInfo, error) {
	var wrap struct {
		Members []TeamMemberInfo `json:"members"`
	}
	if err := json.Unmarshal(data, &wrap); err == nil && wrap.Members != nil {
		return wrap.Members, nil
	}
	var arr []TeamMemberInfo
	if err := json.Unmarshal(data, &arr); err == nil {
		return arr, nil
	}
	return nil, fmt.Errorf("exe.dev: could not parse team members from response: %s", string(data))
}

// bodyError surfaces soft failures exe.dev returns as HTTP 200 with an
// {"error": "..."} field instead of a non-2xx status.
func bodyError(body []byte) error {
	var r struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &r) == nil && r.Error != "" {
		return fmt.Errorf("exe.dev: %s", r.Error)
	}
	return nil
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
