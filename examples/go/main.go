package main

import (
	"github.com/firstatlast/pulumi-exedev/sdk/go/exedev"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		vm, err := exedev.NewVm(ctx, "dev", &exedev.VmArgs{
			Image:   pulumi.String("ubuntu:22.04"),
			Cpu:     pulumi.Int(2),
			Memory:  pulumi.String("4GB"),
			Disk:    pulumi.String("20GB"),
			Tags:    pulumi.StringArray{pulumi.String("dev"), pulumi.String("pulumi")},
			Comment: pulumi.String("managed by pulumi"),
		})
		if err != nil {
			return err
		}
		ctx.Export("name", vm.VmName)
		ctx.Export("url", vm.HttpsUrl)
		ctx.Export("ssh", vm.SshDest)
		ctx.Export("status", vm.Status)
		return nil
	})
}
