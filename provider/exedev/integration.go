package exedev

import (
	"context"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type Integration struct{}

type IntegrationArgs struct {
	Name        string            `pulumi:"name"`
	Type        *string           `pulumi:"type,optional"`
	Target      *string           `pulumi:"target,optional"`
	Headers     map[string]string `pulumi:"headers,optional" provider:"secret"`
	Bearer      *string           `pulumi:"bearer,optional" provider:"secret"`
	NoAuth      *bool             `pulumi:"noAuth,optional"`
	Comment     *string           `pulumi:"comment,optional"`
	Attachments []string          `pulumi:"attachments,optional"`
}

type IntegrationState struct {
	IntegrationArgs
	ResolvedType string `pulumi:"resolvedType"`
}

var (
	_ infer.CustomResource[IntegrationArgs, IntegrationState] = (*Integration)(nil)
	_ infer.CustomRead[IntegrationArgs, IntegrationState]     = (*Integration)(nil)
	_ infer.CustomUpdate[IntegrationArgs, IntegrationState]   = (*Integration)(nil)
	_ infer.CustomDelete[IntegrationState]                    = (*Integration)(nil)
	_ infer.CustomDiff[IntegrationArgs, IntegrationState]     = (*Integration)(nil)
	_ infer.Annotated                                         = (*Integration)(nil)
)

const defaultIntegrationType = "http-proxy"

func (i *Integration) Annotate(a infer.Annotator) {
	a.Describe(&i, "An exe.dev integration. Currently models the http-proxy type, which injects auth/headers into requests from attached VMs to a target URL.")
	a.SetToken("index", "Integration")
}

func (a *IntegrationArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Name, "Integration name (unique). Changing this replaces the integration.")
	an.Describe(&a.Type, "Integration type. Defaults to http-proxy. Changing this replaces the integration.")
	an.Describe(&a.Target, "Target URL the http-proxy forwards to.")
	an.Describe(&a.Headers, "Headers to inject, as name->value.")
	an.Describe(&a.Bearer, "Bearer token to inject (shorthand for an Authorization: Bearer header).")
	an.Describe(&a.NoAuth, "Create the http-proxy with no injected authentication.")
	an.Describe(&a.Comment, "Free-form comment stored with the integration.")
	an.Describe(&a.Attachments, "Where the integration is mounted: vm:<name>, tag:<name>, or auto:all.")
	an.SetDefault(&a.Type, defaultIntegrationType)
}

func (a IntegrationArgs) toSpec() IntegrationSpec {
	t := defaultIntegrationType
	if a.Type != nil && *a.Type != "" {
		t = *a.Type
	}
	return IntegrationSpec{
		Name:        a.Name,
		Type:        t,
		Target:      deref(a.Target),
		Headers:     a.Headers,
		Bearer:      deref(a.Bearer),
		NoAuth:      a.NoAuth != nil && *a.NoAuth,
		Comment:     deref(a.Comment),
		Attachments: a.Attachments,
	}
}

func (Integration) Create(ctx context.Context, req infer.CreateRequest[IntegrationArgs]) (infer.CreateResponse[IntegrationState], error) {
	in := req.Inputs
	spec := in.toSpec()
	state := IntegrationState{IntegrationArgs: in, ResolvedType: spec.Type}

	if req.DryRun {
		return infer.CreateResponse[IntegrationState]{ID: in.Name, Output: state}, nil
	}

	client := clientFromContext(ctx)
	if err := client.IntegrationAdd(ctx, spec); err != nil {
		return infer.CreateResponse[IntegrationState]{}, err
	}
	return infer.CreateResponse[IntegrationState]{ID: in.Name, Output: state}, nil
}

func (Integration) Read(ctx context.Context, req infer.ReadRequest[IntegrationArgs, IntegrationState]) (infer.ReadResponse[IntegrationArgs, IntegrationState], error) {
	name := req.ID
	if name == "" {
		name = req.State.Name
	}
	client := clientFromContext(ctx)
	info, ok, err := client.IntegrationGet(ctx, name)
	if err != nil {
		return infer.ReadResponse[IntegrationArgs, IntegrationState]{}, err
	}
	if !ok {
		return infer.ReadResponse[IntegrationArgs, IntegrationState]{}, nil
	}

	// Refresh readable fields only; header/bearer values are masked by the API and
	// stay authoritative from prior state.
	state := req.State
	state.Name = info.Name
	state.ResolvedType = info.Type
	if info.Target() != "" {
		t := info.Target()
		state.Target = &t
	}
	if info.Comment != "" {
		cm := info.Comment
		state.Comment = &cm
	}
	state.Attachments = info.Attachments
	return infer.ReadResponse[IntegrationArgs, IntegrationState]{ID: info.Name, Inputs: state.IntegrationArgs, State: state}, nil
}

func (Integration) Update(ctx context.Context, req infer.UpdateRequest[IntegrationArgs, IntegrationState]) (infer.UpdateResponse[IntegrationState], error) {
	in := req.Inputs
	old := req.State
	state := IntegrationState{IntegrationArgs: in, ResolvedType: old.ResolvedType}

	if req.DryRun {
		return infer.UpdateResponse[IntegrationState]{Output: state}, nil
	}

	client := clientFromContext(ctx)
	if err := client.IntegrationEdit(ctx, in.toSpec()); err != nil {
		return infer.UpdateResponse[IntegrationState]{}, err
	}

	add, remove := diffTags(old.Attachments, in.Attachments)
	for _, spec := range add {
		if err := client.IntegrationAttach(ctx, in.Name, spec); err != nil {
			return infer.UpdateResponse[IntegrationState]{}, err
		}
	}
	for _, spec := range remove {
		if err := client.IntegrationDetach(ctx, in.Name, spec); err != nil {
			return infer.UpdateResponse[IntegrationState]{}, err
		}
	}
	return infer.UpdateResponse[IntegrationState]{Output: state}, nil
}

func (Integration) Delete(ctx context.Context, req infer.DeleteRequest[IntegrationState]) (infer.DeleteResponse, error) {
	name := req.State.Name
	if name == "" {
		name = req.ID
	}
	client := clientFromContext(ctx)
	return infer.DeleteResponse{}, client.IntegrationRemove(ctx, name)
}

func (Integration) Diff(_ context.Context, req infer.DiffRequest[IntegrationArgs, IntegrationState]) (infer.DiffResponse, error) {
	in := req.Inputs
	old := req.State.IntegrationArgs
	diff := map[string]p.PropertyDiff{}

	if in.Name != old.Name {
		diff["name"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if integrationType(in.Type) != integrationType(old.Type) {
		diff["type"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if !strEq(in.Target, old.Target) {
		diff["target"] = p.PropertyDiff{Kind: p.Update}
	}
	if !strEq(in.Comment, old.Comment) {
		diff["comment"] = p.PropertyDiff{Kind: p.Update}
	}
	if !strEq(in.Bearer, old.Bearer) {
		diff["bearer"] = p.PropertyDiff{Kind: p.Update}
	}
	if boolPtr(in.NoAuth) != boolPtr(old.NoAuth) {
		diff["noAuth"] = p.PropertyDiff{Kind: p.Update}
	}
	if !envEq(in.Headers, old.Headers) {
		diff["headers"] = p.PropertyDiff{Kind: p.Update}
	}
	if !setEq(in.Attachments, old.Attachments) {
		diff["attachments"] = p.PropertyDiff{Kind: p.Update}
	}

	deleteBeforeReplace := in.Name == old.Name
	return infer.DiffResponse{
		HasChanges:          len(diff) > 0,
		DetailedDiff:        diff,
		DeleteBeforeReplace: deleteBeforeReplace && hasReplace(diff),
	}, nil
}

func integrationType(t *string) string {
	if t == nil || *t == "" {
		return defaultIntegrationType
	}
	return *t
}

func hasReplace(diff map[string]p.PropertyDiff) bool {
	for _, d := range diff {
		if d.Kind == p.UpdateReplace || d.Kind == p.AddReplace || d.Kind == p.DeleteReplace {
			return true
		}
	}
	return false
}
