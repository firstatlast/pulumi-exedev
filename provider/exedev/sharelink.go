package exedev

import (
	"context"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type ShareLink struct{}

type ShareLinkArgs struct {
	Vm string `pulumi:"vm"`
}

type ShareLinkState struct {
	ShareLinkArgs
	Token string `pulumi:"token"`
	Url   string `pulumi:"url"`
}

var (
	_ infer.CustomResource[ShareLinkArgs, ShareLinkState] = (*ShareLink)(nil)
	_ infer.CustomRead[ShareLinkArgs, ShareLinkState]     = (*ShareLink)(nil)
	_ infer.CustomDelete[ShareLinkState]                  = (*ShareLink)(nil)
	_ infer.CustomDiff[ShareLinkArgs, ShareLinkState]     = (*ShareLink)(nil)
	_ infer.Annotated                                     = (*ShareLink)(nil)
)

func (l *ShareLink) Annotate(a infer.Annotator) {
	a.Describe(&l, "A shareable HTTPS access link for a VM.")
	a.SetToken("index", "ShareLink")
}

func (a *ShareLinkArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Vm, "Name of the VM to create a share link for. Changing this replaces the link.")
}

func (ShareLink) Create(ctx context.Context, req infer.CreateRequest[ShareLinkArgs]) (infer.CreateResponse[ShareLinkState], error) {
	in := req.Inputs
	state := ShareLinkState{ShareLinkArgs: in}
	if req.DryRun {
		return infer.CreateResponse[ShareLinkState]{Output: state}, nil
	}
	client := clientFromContext(ctx)
	token, url, err := client.ShareAddLink(ctx, in.Vm)
	if err != nil {
		return infer.CreateResponse[ShareLinkState]{}, err
	}
	state.Token = token
	state.Url = url
	return infer.CreateResponse[ShareLinkState]{ID: shareID(in.Vm, token), Output: state}, nil
}

func (ShareLink) Read(ctx context.Context, req infer.ReadRequest[ShareLinkArgs, ShareLinkState]) (infer.ReadResponse[ShareLinkArgs, ShareLinkState], error) {
	vm, token := req.State.Vm, req.State.Token
	if vm == "" || token == "" {
		var err error
		if vm, token, err = splitShareID(req.ID); err != nil {
			return infer.ReadResponse[ShareLinkArgs, ShareLinkState]{}, err
		}
	}
	client := clientFromContext(ctx)
	_, ok, err := client.ShareLinkByToken(ctx, vm, token)
	if err != nil {
		return infer.ReadResponse[ShareLinkArgs, ShareLinkState]{}, err
	}
	if !ok {
		return infer.ReadResponse[ShareLinkArgs, ShareLinkState]{}, nil
	}
	state := req.State
	state.Vm = vm
	state.Token = token
	return infer.ReadResponse[ShareLinkArgs, ShareLinkState]{ID: shareID(vm, token), Inputs: state.ShareLinkArgs, State: state}, nil
}

func (ShareLink) Delete(ctx context.Context, req infer.DeleteRequest[ShareLinkState]) (infer.DeleteResponse, error) {
	client := clientFromContext(ctx)
	return infer.DeleteResponse{}, client.ShareRemoveLink(ctx, req.State.Vm, req.State.Token)
}

// Diff: vm is the only input and is immutable, so any change replaces.
func (ShareLink) Diff(_ context.Context, req infer.DiffRequest[ShareLinkArgs, ShareLinkState]) (infer.DiffResponse, error) {
	diff := map[string]p.PropertyDiff{}
	if req.Inputs.Vm != req.State.Vm {
		diff["vm"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	return infer.DiffResponse{HasChanges: len(diff) > 0, DetailedDiff: diff}, nil
}
