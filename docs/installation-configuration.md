---
title: exe.dev Installation & Configuration
meta_desc: How to install and configure the exe.dev Pulumi provider.
layout: installation
---

## Installation

Add the provider to a project with:

```bash
pulumi package add exedev
```

This installs the plugin (from the provider's GitHub Releases via the schema's
`pluginDownloadURL`) and generates a local SDK in your project language — TypeScript,
Python, Go, C#, Java or YAML. The exe.dev provider is a native provider, so the SDK for
any supported language is produced from the package schema on demand; no per-language
package needs to be published to a language registry.

The Go SDK is also importable directly at
`github.com/firstatlast/pulumi-exedev/sdk/go/exedev` and the TypeScript SDK is generated
under the `@firstatlast/exedev` name.

## Configuration

| Config | Environment variable | Secret | Description |
|--------|----------------------|--------|-------------|
| `exedev:token` | `EXEDEV_TOKEN` | yes | Bearer API token. |
| `exedev:endpoint` | `EXEDEV_ENDPOINT` | no | Exec endpoint override (default `https://exe.dev/exec`). |

### API token

Generate a token scoped to the commands the resource lifecycle needs. A default
token only permits create/read; the full lifecycle requires `rm`, `resize`, `tag`,
`comment` and `rename`:

```bash
ssh exe.dev ssh-key generate-api-key --label=pulumi \
  --cmds=new,ls,rm,resize,tag,comment,rename,domain,share --exp=30d
```

Set it as a secret on the stack:

```bash
pulumi config set --secret exedev:token exe1.YOURTOKEN
```

or export it in the environment:

```bash
export EXEDEV_TOKEN=exe1.YOURTOKEN
```
