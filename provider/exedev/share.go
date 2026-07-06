package exedev

import (
	"context"
	"fmt"
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

func shareID(vm, key string) string { return vm + "/" + key }

func splitShareID(id string) (vm, key string, err error) {
	i := strings.IndexByte(id, '/')
	if i < 0 {
		return "", "", fmt.Errorf("invalid share id %q (expected <vm>/<key>)", id)
	}
	return id[:i], id[i+1:], nil
}

type Share struct{}

type ShareArgs struct {
	Vm      string  `pulumi:"vm"`
	Email   string  `pulumi:"email"`
	Message *string `pulumi:"message,optional"`
}

type ShareStateResource struct {
	ShareArgs
	Status string `pulumi:"status"`
}

var (
	_ infer.CustomResource[ShareArgs, ShareStateResource] = (*Share)(nil)
	_ infer.CustomRead[ShareArgs, ShareStateResource]     = (*Share)(nil)
	_ infer.CustomDelete[ShareStateResource]              = (*Share)(nil)
	_ infer.CustomDiff[ShareArgs, ShareStateResource]     = (*Share)(nil)
	_ infer.Annotated                                     = (*Share)(nil)
)

func (s *Share) Annotate(a infer.Annotator) {
	a.Describe(&s, "Shares HTTPS access to a VM with a user by email.")
	a.SetToken("index", "Share")
}

func (a *ShareArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Vm, "Name of the VM to share. Changing this replaces the share.")
	an.Describe(&a.Email, "Email of the user to share with. Changing this replaces the share.")
	an.Describe(&a.Message, "Optional message included in the invitation. Applied only at create time.")
}

func (Share) Create(ctx context.Context, req infer.CreateRequest[ShareArgs]) (infer.CreateResponse[ShareStateResource], error) {
	in := req.Inputs
	state := ShareStateResource{ShareArgs: in}
	if req.DryRun {
		return infer.CreateResponse[ShareStateResource]{ID: shareID(in.Vm, in.Email), Output: state}, nil
	}
	client := clientFromContext(ctx)
	if err := client.ShareAddUser(ctx, in.Vm, in.Email, deref(in.Message)); err != nil {
		return infer.CreateResponse[ShareStateResource]{}, err
	}
	if u, ok, err := client.ShareUserByEmail(ctx, in.Vm, in.Email); err == nil && ok {
		state.Status = u.Status
	}
	return infer.CreateResponse[ShareStateResource]{ID: shareID(in.Vm, in.Email), Output: state}, nil
}

func (Share) Read(ctx context.Context, req infer.ReadRequest[ShareArgs, ShareStateResource]) (infer.ReadResponse[ShareArgs, ShareStateResource], error) {
	vm, email := req.State.Vm, req.State.Email
	if vm == "" || email == "" {
		var err error
		if vm, email, err = splitShareID(req.ID); err != nil {
			return infer.ReadResponse[ShareArgs, ShareStateResource]{}, err
		}
	}
	client := clientFromContext(ctx)
	u, ok, err := client.ShareUserByEmail(ctx, vm, email)
	if err != nil {
		return infer.ReadResponse[ShareArgs, ShareStateResource]{}, err
	}
	if !ok {
		return infer.ReadResponse[ShareArgs, ShareStateResource]{}, nil
	}
	state := req.State
	state.Vm = vm
	state.Email = email
	state.Status = u.Status
	return infer.ReadResponse[ShareArgs, ShareStateResource]{ID: shareID(vm, email), Inputs: state.ShareArgs, State: state}, nil
}

func (Share) Delete(ctx context.Context, req infer.DeleteRequest[ShareStateResource]) (infer.DeleteResponse, error) {
	client := clientFromContext(ctx)
	return infer.DeleteResponse{}, client.ShareRemoveUser(ctx, req.State.Vm, req.State.Email)
}

// Diff: vm and email are immutable; message is create-only. Any change replaces.
func (Share) Diff(_ context.Context, req infer.DiffRequest[ShareArgs, ShareStateResource]) (infer.DiffResponse, error) {
	in := req.Inputs
	old := req.State.ShareArgs
	diff := map[string]p.PropertyDiff{}
	if in.Vm != old.Vm {
		diff["vm"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if in.Email != old.Email {
		diff["email"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if !strEq(in.Message, old.Message) {
		diff["message"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	deleteBeforeReplace := in.Vm == old.Vm && in.Email == old.Email && len(diff) > 0
	return infer.DiffResponse{HasChanges: len(diff) > 0, DetailedDiff: diff, DeleteBeforeReplace: deleteBeforeReplace}, nil
}
