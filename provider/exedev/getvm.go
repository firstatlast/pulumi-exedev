package exedev

import (
	"context"
	"fmt"

	"github.com/pulumi/pulumi-go-provider/infer"
)

type GetVm struct{}

type GetVmArgs struct {
	Name string `pulumi:"name"`
}

type GetVmResult struct {
	VmName        string   `pulumi:"vmName"`
	HttpsUrl      string   `pulumi:"httpsUrl"`
	SshDest       string   `pulumi:"sshDest"`
	Region        string   `pulumi:"region"`
	RegionDisplay string   `pulumi:"regionDisplay"`
	Status        string   `pulumi:"status"`
	Image         string   `pulumi:"image"`
	Comment       string   `pulumi:"comment"`
	Tags          []string `pulumi:"tags"`
	AllocatedCpus int      `pulumi:"allocatedCpus"`
	MemoryBytes   int64    `pulumi:"memoryBytes"`
	DiskBytes     int64    `pulumi:"diskBytes"`
}

var (
	_ infer.Fn[GetVmArgs, GetVmResult] = (*GetVm)(nil)
	_ infer.Annotated                  = (*GetVm)(nil)
)

func (g *GetVm) Annotate(a infer.Annotator) {
	a.Describe(&g, "Look up an existing exe.dev VM by name.")
	a.SetToken("index", "getVm")
}

func (a *GetVmArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Name, "Name of the VM to look up.")
}

func (GetVm) Invoke(ctx context.Context, req infer.FunctionRequest[GetVmArgs]) (infer.FunctionResponse[GetVmResult], error) {
	client := clientFromContext(ctx)
	vm, ok, err := client.Get(ctx, req.Input.Name)
	if err != nil {
		return infer.FunctionResponse[GetVmResult]{}, err
	}
	if !ok {
		return infer.FunctionResponse[GetVmResult]{}, fmt.Errorf("exe.dev: no VM named %q", req.Input.Name)
	}
	return infer.FunctionResponse[GetVmResult]{Output: GetVmResult{
		VmName:        vm.Name,
		HttpsUrl:      vm.HTTPSURL,
		SshDest:       vm.SSHDest,
		Region:        vm.Region,
		RegionDisplay: vm.RegionDisplay,
		Status:        vm.Status,
		Image:         vm.Image,
		Comment:       vm.Comment,
		Tags:          vm.Tags,
		AllocatedCpus: vm.AllocatedCPUs,
		MemoryBytes:   vm.MemoryBytes,
		DiskBytes:     vm.DiskBytes,
	}}, nil
}
