package exedev

import (
	"context"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// TeamMember manages membership of your exe.dev team.
//
// NOTE: This resource is grounded in the `team` CLI docs but has not been
// verified against a live team (the test account has no team). The
// `team members` JSON shape in particular is parsed leniently.
type TeamMember struct{}

const defaultTeamRole = "user"

type TeamMemberArgs struct {
	Email string  `pulumi:"email"`
	Role  *string `pulumi:"role,optional"`
}

type TeamMemberState struct {
	TeamMemberArgs
}

var (
	_ infer.CustomResource[TeamMemberArgs, TeamMemberState] = (*TeamMember)(nil)
	_ infer.CustomRead[TeamMemberArgs, TeamMemberState]     = (*TeamMember)(nil)
	_ infer.CustomUpdate[TeamMemberArgs, TeamMemberState]   = (*TeamMember)(nil)
	_ infer.CustomDelete[TeamMemberState]                   = (*TeamMember)(nil)
	_ infer.CustomDiff[TeamMemberArgs, TeamMemberState]     = (*TeamMember)(nil)
	_ infer.Annotated                                       = (*TeamMember)(nil)
)

func (m *TeamMember) Annotate(a infer.Annotator) {
	a.Describe(&m, "Membership of your exe.dev team. Note: not yet verified against a live team.")
	a.SetToken("index", "TeamMember")
}

func (a *TeamMemberArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Email, "Email of the team member. Changing this replaces the membership.")
	an.Describe(&a.Role, "Role: user, admin, or billing_owner. Defaults to user.")
	an.SetDefault(&a.Role, defaultTeamRole)
}

func teamRole(r *string) string {
	if r == nil || *r == "" {
		return defaultTeamRole
	}
	return *r
}

func (TeamMember) Create(ctx context.Context, req infer.CreateRequest[TeamMemberArgs]) (infer.CreateResponse[TeamMemberState], error) {
	in := req.Inputs
	state := TeamMemberState{TeamMemberArgs: in}
	if req.DryRun {
		return infer.CreateResponse[TeamMemberState]{ID: in.Email, Output: state}, nil
	}
	client := clientFromContext(ctx)
	if err := client.TeamAdd(ctx, in.Email, teamRole(in.Role)); err != nil {
		return infer.CreateResponse[TeamMemberState]{}, err
	}
	return infer.CreateResponse[TeamMemberState]{ID: in.Email, Output: state}, nil
}

func (TeamMember) Read(ctx context.Context, req infer.ReadRequest[TeamMemberArgs, TeamMemberState]) (infer.ReadResponse[TeamMemberArgs, TeamMemberState], error) {
	email := req.ID
	if email == "" {
		email = req.State.Email
	}
	client := clientFromContext(ctx)
	member, ok, err := client.TeamMemberByEmail(ctx, email)
	if err != nil {
		return infer.ReadResponse[TeamMemberArgs, TeamMemberState]{}, err
	}
	if !ok {
		return infer.ReadResponse[TeamMemberArgs, TeamMemberState]{}, nil
	}
	state := req.State
	state.Email = member.Email
	if member.Role != "" {
		r := member.Role
		state.Role = &r
	}
	return infer.ReadResponse[TeamMemberArgs, TeamMemberState]{ID: member.Email, Inputs: state.TeamMemberArgs, State: state}, nil
}

func (TeamMember) Update(ctx context.Context, req infer.UpdateRequest[TeamMemberArgs, TeamMemberState]) (infer.UpdateResponse[TeamMemberState], error) {
	in := req.Inputs
	state := TeamMemberState{TeamMemberArgs: in}
	if req.DryRun {
		return infer.UpdateResponse[TeamMemberState]{Output: state}, nil
	}
	client := clientFromContext(ctx)
	if err := client.TeamRole(ctx, in.Email, teamRole(in.Role)); err != nil {
		return infer.UpdateResponse[TeamMemberState]{}, err
	}
	return infer.UpdateResponse[TeamMemberState]{Output: state}, nil
}

func (TeamMember) Delete(ctx context.Context, req infer.DeleteRequest[TeamMemberState]) (infer.DeleteResponse, error) {
	email := req.State.Email
	if email == "" {
		email = req.ID
	}
	client := clientFromContext(ctx)
	return infer.DeleteResponse{}, client.TeamRemove(ctx, email)
}

func (TeamMember) Diff(_ context.Context, req infer.DiffRequest[TeamMemberArgs, TeamMemberState]) (infer.DiffResponse, error) {
	in := req.Inputs
	old := req.State.TeamMemberArgs
	diff := map[string]p.PropertyDiff{}
	if in.Email != old.Email {
		diff["email"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if teamRole(in.Role) != teamRole(old.Role) {
		diff["role"] = p.PropertyDiff{Kind: p.Update}
	}
	return infer.DiffResponse{
		HasChanges:          len(diff) > 0,
		DetailedDiff:        diff,
		DeleteBeforeReplace: in.Email == old.Email && hasReplace(diff),
	}, nil
}
