package exedev

import (
	"context"
	"fmt"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type SshKey struct{}

type SshKeyArgs struct {
	PublicKey string  `pulumi:"publicKey"`
	Tag       *string `pulumi:"tag,optional"`
}

type SshKeyState struct {
	SshKeyArgs
	Name        string `pulumi:"name"`
	Fingerprint string `pulumi:"fingerprint"`
}

var (
	_ infer.CustomResource[SshKeyArgs, SshKeyState] = (*SshKey)(nil)
	_ infer.CustomRead[SshKeyArgs, SshKeyState]     = (*SshKey)(nil)
	_ infer.CustomDelete[SshKeyState]               = (*SshKey)(nil)
	_ infer.CustomDiff[SshKeyArgs, SshKeyState]     = (*SshKey)(nil)
	_ infer.Annotated                               = (*SshKey)(nil)
)

func (k *SshKey) Annotate(a infer.Annotator) {
	a.Describe(&k, "An SSH public key registered on your exe.dev account.")
	a.SetToken("index", "SshKey")
}

func (a *SshKeyArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.PublicKey, "The SSH public key (e.g. 'ssh-ed25519 AAAA... comment'). The comment sets the key name. Changing this replaces the key.")
	an.Describe(&a.Tag, "Scope the key to VMs carrying this tag. Changing this replaces the key.")
}

func (SshKey) Create(ctx context.Context, req infer.CreateRequest[SshKeyArgs]) (infer.CreateResponse[SshKeyState], error) {
	in := req.Inputs
	state := SshKeyState{SshKeyArgs: in}

	if req.DryRun {
		return infer.CreateResponse[SshKeyState]{Output: state}, nil
	}

	client := clientFromContext(ctx)
	if err := client.SshKeyAdd(ctx, in.PublicKey, deref(in.Tag)); err != nil {
		return infer.CreateResponse[SshKeyState]{}, err
	}
	key, ok, err := client.SshKeyByMaterial(ctx, in.PublicKey)
	if err != nil {
		return infer.CreateResponse[SshKeyState]{}, err
	}
	if !ok {
		return infer.CreateResponse[SshKeyState]{}, fmt.Errorf("exe.dev: ssh key not found after add")
	}
	state.Name = key.Name
	state.Fingerprint = key.Fingerprint
	return infer.CreateResponse[SshKeyState]{ID: key.Fingerprint, Output: state}, nil
}

func (SshKey) Read(ctx context.Context, req infer.ReadRequest[SshKeyArgs, SshKeyState]) (infer.ReadResponse[SshKeyArgs, SshKeyState], error) {
	client := clientFromContext(ctx)
	key, ok, err := client.SshKeyByFingerprint(ctx, req.ID)
	if err != nil {
		return infer.ReadResponse[SshKeyArgs, SshKeyState]{}, err
	}
	if !ok {
		return infer.ReadResponse[SshKeyArgs, SshKeyState]{}, nil
	}
	state := req.State
	if state.PublicKey == "" {
		state.PublicKey = key.PublicKey
	}
	state.Name = key.Name
	state.Fingerprint = key.Fingerprint
	return infer.ReadResponse[SshKeyArgs, SshKeyState]{ID: key.Fingerprint, Inputs: state.SshKeyArgs, State: state}, nil
}

func (SshKey) Delete(ctx context.Context, req infer.DeleteRequest[SshKeyState]) (infer.DeleteResponse, error) {
	client := clientFromContext(ctx)
	ref := req.State.Fingerprint
	if ref == "" {
		ref = req.ID
	}
	return infer.DeleteResponse{}, client.SshKeyRemove(ctx, ref)
}

// Diff: no edit command exists, so any input change replaces.
func (SshKey) Diff(_ context.Context, req infer.DiffRequest[SshKeyArgs, SshKeyState]) (infer.DiffResponse, error) {
	in := req.Inputs
	old := req.State.SshKeyArgs
	diff := map[string]p.PropertyDiff{}
	sameKey := keyMaterial(in.PublicKey) == keyMaterial(old.PublicKey)
	if !sameKey {
		diff["publicKey"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if !strEq(in.Tag, old.Tag) {
		diff["tag"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	// Re-adding the same key would collide, so delete first when the key is unchanged.
	return infer.DiffResponse{
		HasChanges:          len(diff) > 0,
		DetailedDiff:        diff,
		DeleteBeforeReplace: sameKey && len(diff) > 0,
	}, nil
}
