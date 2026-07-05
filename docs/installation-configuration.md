---
title: exe.dev Installation & Configuration
meta_desc: How to install and configure the exe.dev Pulumi provider.
layout: installation
---

## Installation

The exe.dev provider is distributed as the [`@firstatlast/exedev`](https://www.npmjs.com/package/@firstatlast/exedev)
npm package and the `github.com/firstatlast/pulumi-exedev/sdk/go/exedev` Go module.
Add it to a project with:

```bash
pulumi package add exedev
```

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
  --cmds=new,ls,rm,resize,tag,comment,rename --exp=30d
```

Set it as a secret on the stack:

```bash
pulumi config set --secret exedev:token exe1.YOURTOKEN
```

or export it in the environment:

```bash
export EXEDEV_TOKEN=exe1.YOURTOKEN
```
