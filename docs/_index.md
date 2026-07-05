---
title: exe.dev
meta_desc: A Pulumi provider for creating and managing exe.dev VMs.
layout: overview
---

The exe.dev provider for Pulumi lets you provision and manage
[exe.dev](https://exe.dev) VMs as infrastructure as code.

## Example

{{< chooser language "typescript,go,yaml" >}}

{{% choosable language typescript %}}

```typescript
import * as exedev from "@firstatlast/exedev";

const vm = new exedev.Vm("dev", {
    image: "ubuntu:22.04",
    cpu: 2,
    memory: "4GB",
    disk: "20GB",
    tags: ["dev"],
});

export const url = vm.httpsUrl;
export const ssh = vm.sshDest;
```

{{% /choosable %}}

{{% choosable language go %}}

```go
package main

import (
	"github.com/firstatlast/pulumi-exedev/sdk/go/exedev"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		vm, err := exedev.NewVm(ctx, "dev", &exedev.VmArgs{
			Image:  pulumi.String("ubuntu:22.04"),
			Cpu:    pulumi.Int(2),
			Memory: pulumi.String("4GB"),
			Disk:   pulumi.String("20GB"),
		})
		if err != nil {
			return err
		}
		ctx.Export("url", vm.HttpsUrl)
		ctx.Export("ssh", vm.SshDest)
		return nil
	})
}
```

{{% /choosable %}}

{{% choosable language yaml %}}

```yaml
resources:
  dev:
    type: exedev:index:Vm
    properties:
      image: ubuntu:22.04
      cpu: 2
      memory: 4GB
      disk: 20GB
outputs:
  url: ${dev.httpsUrl}
  ssh: ${dev.sshDest}
```

{{% /choosable %}}

{{< /chooser >}}
