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
parsing. The full lifecycle has been verified live against the exe.dev API:

- create → in-place update (tags/comment) → refresh (clean) → destroy
- create/destroy through both the Go and TypeScript SDKs
- in-place `resize` (disk grow confirmed live; cpu/memory resize issues the same
  command but is bounded by the account plan)
- replacement paths: disk-shrink and name-change decisions verified via preview;
  an immutable-field change (env) executed live, confirming delete-before-replace
  recreates the VM under the same name without collision

> TypeScript 6 note: consuming programs may need `"ignoreDeprecations": "6.0"` in
> `tsconfig.json` (the generated SDK uses node-style module resolution).

## Releasing

Plugin binaries are published to GitHub Releases and the schema's
`pluginDownloadURL` points there, so `pulumi package add exedev` (and the generated
SDKs) auto-install the correct plugin.

1. Bump versions (SDK `package.json`, any version references) and regenerate SDKs
   with `make sdk`.
2. Tag and push: `git tag v0.1.0 && git push origin v0.1.0`. The `release` workflow
   cross-compiles for linux/darwin/windows (amd64+arm64), packages
   `pulumi-resource-exedev-vX.Y.Z-<os>-<arch>.tar.gz`, and creates the GitHub Release.
3. To list on the Pulumi Registry, open a PR to
   [`pulumi/registry`](https://github.com/pulumi/registry) adding this package to
   `community-packages/package-list.json` (repo + `docs/` reference).

## License

Apache-2.0
