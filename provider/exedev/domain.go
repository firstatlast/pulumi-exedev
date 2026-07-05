package exedev

import (
	"context"
	"fmt"
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type Domain struct{}

type DomainArgs struct {
	Vm       string `pulumi:"vm"`
	Hostname string `pulumi:"hostname"`
	Wildcard *bool  `pulumi:"wildcard,optional"`
}

type DomainState struct {
	DomainArgs
	Verified bool `pulumi:"verified"`
}

var (
	_ infer.CustomResource[DomainArgs, DomainState] = (*Domain)(nil)
	_ infer.CustomRead[DomainArgs, DomainState]     = (*Domain)(nil)
	_ infer.CustomDelete[DomainState]               = (*Domain)(nil)
	_ infer.CustomDiff[DomainArgs, DomainState]     = (*Domain)(nil)
	_ infer.Annotated                               = (*Domain)(nil)
)

func (d *Domain) Annotate(a infer.Annotator) {
	a.Describe(&d, "A custom domain linked to an exe.dev VM.")
	a.SetToken("index", "Domain")
}

func (a *DomainArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Vm, "Name of the VM to attach the domain to. Changing this replaces the domain.")
	an.Describe(&a.Hostname, "The custom hostname (a CNAME you already point at the VM). Changing this replaces the domain.")
	an.Describe(&a.Wildcard, "Issue a wildcard (*.<parent>) certificate via DNS-01 delegation.")
}

func domainID(vm, hostname string) string { return vm + "/" + hostname }

func splitDomainID(id string) (vm, hostname string, err error) {
	i := strings.IndexByte(id, '/')
	if i < 0 {
		return "", "", fmt.Errorf("invalid domain id %q (expected <vm>/<hostname>)", id)
	}
	return id[:i], id[i+1:], nil
}

func (Domain) Create(ctx context.Context, req infer.CreateRequest[DomainArgs]) (infer.CreateResponse[DomainState], error) {
	in := req.Inputs
	state := DomainState{DomainArgs: in}
	id := domainID(in.Vm, in.Hostname)

	if req.DryRun {
		return infer.CreateResponse[DomainState]{ID: id, Output: state}, nil
	}

	client := clientFromContext(ctx)
	if err := client.DomainAdd(ctx, in.Vm, in.Hostname, in.Wildcard != nil && *in.Wildcard); err != nil {
		return infer.CreateResponse[DomainState]{}, err
	}
	if dom, ok, err := client.DomainGet(ctx, in.Vm, in.Hostname); err == nil && ok {
		state.Verified = dom.Verified
	}
	return infer.CreateResponse[DomainState]{ID: id, Output: state}, nil
}

func (Domain) Read(ctx context.Context, req infer.ReadRequest[DomainArgs, DomainState]) (infer.ReadResponse[DomainArgs, DomainState], error) {
	vm, hostname := req.State.Vm, req.State.Hostname
	if vm == "" || hostname == "" {
		var err error
		if vm, hostname, err = splitDomainID(req.ID); err != nil {
			return infer.ReadResponse[DomainArgs, DomainState]{}, err
		}
	}

	client := clientFromContext(ctx)
	dom, ok, err := client.DomainGet(ctx, vm, hostname)
	if err != nil {
		return infer.ReadResponse[DomainArgs, DomainState]{}, err
	}
	if !ok {
		return infer.ReadResponse[DomainArgs, DomainState]{}, nil
	}

	state := req.State
	state.Vm = vm
	state.Hostname = hostname
	state.Verified = dom.Verified
	return infer.ReadResponse[DomainArgs, DomainState]{
		ID:     domainID(vm, hostname),
		Inputs: state.DomainArgs,
		State:  state,
	}, nil
}

func (Domain) Delete(ctx context.Context, req infer.DeleteRequest[DomainState]) (infer.DeleteResponse, error) {
	client := clientFromContext(ctx)
	return infer.DeleteResponse{}, client.DomainRemove(ctx, req.State.Vm, req.State.Hostname)
}

// Diff: every input is immutable (no mutating CLI command), so any change replaces.
func (Domain) Diff(_ context.Context, req infer.DiffRequest[DomainArgs, DomainState]) (infer.DiffResponse, error) {
	in := req.Inputs
	old := req.State.DomainArgs
	diff := map[string]p.PropertyDiff{}
	if in.Vm != old.Vm {
		diff["vm"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if in.Hostname != old.Hostname {
		diff["hostname"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if boolPtr(in.Wildcard) != boolPtr(old.Wildcard) {
		diff["wildcard"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}

	// Replace on the same vm+hostname would collide on re-add, so delete first.
	deleteBeforeReplace := in.Vm == old.Vm && in.Hostname == old.Hostname && len(diff) > 0

	return infer.DiffResponse{
		HasChanges:          len(diff) > 0,
		DetailedDiff:        diff,
		DeleteBeforeReplace: deleteBeforeReplace,
	}, nil
}

func boolPtr(b *bool) bool { return b != nil && *b }
