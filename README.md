# pulumi-exedev

A [Pulumi](https://www.pulumi.com) provider for managing [exe.dev](https://exe.dev) VMs as infrastructure as code.

> Status: **early development** — API surface and resource schema are still stabilizing.

## What it manages

exe.dev exposes a single HTTPS endpoint (`POST https://exe.dev/exec`) that runs its
CLI commands with bearer-token auth. This provider wraps the VM lifecycle
(`new` / `ls` / `rm` / `resize` / `rename` / `tag` / `comment`) behind a declarative
Pulumi resource.

### Resources

- `exedev:index:Vm` — a persistent Linux VM.

## Configuration

| Config | Env var | Secret | Description |
|--------|---------|--------|-------------|
| `exedev:token` | `EXEDEV_TOKEN` | yes | Bearer API token (see below). |
| `exedev:endpoint` | `EXEDEV_ENDPOINT` | no | Override the exec endpoint (default `https://exe.dev/exec`). |

The token must be scoped to the commands the resource lifecycle uses — a
default token only permits Create/Read. Generate one with:

```bash
ssh exe.dev ssh-key generate-api-key --label=pulumi \
  --cmds=new,ls,rm,resize,tag,comment,rename --exp=30d
```

## Repository layout

```
provider/     Go module: the provider plugin (pulumi-resource-exedev)
  exedev/     client.go (exec API), config.go, vm.go (resource), provider.go
  cmd/        plugin entrypoint (main.go)
sdk/go/       generated + compilable Go SDK (own module)
sdk/nodejs/   generated + compilable TypeScript SDK (@firstatlast/exedev)
examples/     Go and TypeScript usage examples
```

## Development

```bash
make build      # build the provider plugin (pulumi-resource-exedev)
make schema     # emit the Pulumi schema to schema.json
make sdk        # generate Go + TypeScript SDKs
make install    # install the plugin locally for `pulumi up` testing
make test       # run provider unit tests
```

### Dependency versions

The `provider` module is pinned to `pulumi/sdk` + `pulumi/pkg` **v3.232.0**: the
latest `pulumi-go-provider` (v1.3.2) does not compile against newer pulumi releases
(the `schema.ResourceSpec` type changed). The generated SDKs are independent and
track the latest pulumi releases (Go SDK on `pulumi/sdk` v3.250.0; Node SDK on
`@pulumi/pulumi` ^3.250.0, TypeScript 6).

### Testing status

Unit tests cover command construction, shell quoting, size parsing, and response
parsing. The full create → update → refresh → destroy lifecycle has been verified
live against the exe.dev API (in-place tag/comment updates confirmed, refresh clean),
and a live create/destroy has been run through both the Go and TypeScript SDKs.
Not yet live-exercised: `resize` (cpu/memory/disk) and replacement paths, though they
use the same client machinery.

> TypeScript 6 note: consuming programs may need `"ignoreDeprecations": "6.0"` in
> `tsconfig.json` (the generated SDK uses node-style module resolution).

## License

Apache-2.0
