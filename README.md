# tfunfold

[![CI](https://github.com/winebarrel/tfunfold/actions/workflows/ci.yml/badge.svg)](https://github.com/winebarrel/tfunfold/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/winebarrel/tfunfold/branch/main/graph/badge.svg)](https://codecov.io/gh/winebarrel/tfunfold)
[![AI Generated](https://img.shields.io/badge/AI%20Generated-Claude-orange?logo=anthropic)](https://claude.ai/claude-code)

`tfunfold` rewrites Terraform `resource` and `module` blocks that use `for_each` or `count`, splitting each one into a separate block per instance and emitting a `moved` block so that `terraform apply` does not re-create existing infrastructure.

Instance keys come from a `terraform.tfstate` file. The tool does not call `terraform`, but the state needs to exist, so `terraform init` and `terraform apply` (or `terraform state pull`) must have been run first.

## Installation

```
brew install winebarrel/tfunfold/tfunfold
```

## Usage

```
Usage: tfunfold [<dir>] [flags]

Expand Terraform for_each / count into individual resources or modules with moved blocks.

Arguments:
  [<dir>]    Directory containing *.tf files (default: ".").

Flags:
  -h, --help            Show help.
  -s, --state=STRING    Path to the terraform.tfstate file (default: <dir>/terraform.tfstate).
  -i, --in-place        Write changes back to files instead of stdout.
      --version
```

By default the rewritten files are printed to stdout. Pass `-i` to overwrite files on disk.

## Example

```hcl
# main.tf
resource "null_resource" "x" {
  for_each = toset(["a", "b"])
  triggers = { name = "x-${each.key}" }
}

resource "null_resource" "ref" {
  triggers = { parent = null_resource.x["a"].id }
}
```

```sh
tfunfold -i .
```

```hcl
# main.tf (rewritten)
resource "null_resource" "ref" {
  triggers = { parent = null_resource.x_a.id }
}

moved {
  from = null_resource.x["a"]
  to   = null_resource.x_a
}
resource "null_resource" "x_a" {
  triggers = { name = "x-a" }
}

moved {
  from = null_resource.x["b"]
  to   = null_resource.x_b
}
resource "null_resource" "x_b" {
  triggers = { name = "x-b" }
}
```

After running `terraform plan`, the output should report `0 to add, 0 to change, 0 to destroy` with `has moved to` entries for every former instance.

## How it works

1. `*.tf` files in the target directory are parsed with `hclwrite`.
2. `terraform.tfstate` is read directly as JSON. For each `resource` or `module` block that has `for_each` or `count`, the tool collects the instance keys from state.
3. The original block is replaced by one block per key. The new block label is `<original>_<sanitized-key>`. Within the body, `each.key`, `each.value`, and `count.index` are substituted with their literal values.
4. References to the expanded blocks anywhere in the directory (`<type>.<name>["k"]`, `module.<name>[0]`, ...) are rewritten to the new identifiers.
5. A `moved` block is inserted directly before each new block so Terraform keeps the existing state.

## Module expansion

When a `module` block is expanded, the children inside the module follow automatically. Only one `moved` per module instance is emitted; child resources are migrated by Terraform's own follow-through behaviour. Plan output will list one `has moved to` line per child resource.

## Limitations

- Only `*.tf` files directly in the target directory are scanned. Subdirectories are not recursed.
- The state must already contain the instances for every expanded target. Drift or missing instances are reported as errors.
- References that cannot be statically rewritten cause an error rather than a partial rewrite. This includes dynamic keys (`aws_x.y[var.k]`) and whole-collection access (`aws_x.y` without a subscript, or expressions like `for v in aws_x.y : ...`).
- The new resource label is `<original>_<sanitized-key>`, where any character outside `[A-Za-z0-9_-]` is replaced with `_`. If two keys sanitize to the same name, or the resulting name collides with an existing block, the run errors out.
- `each.value` is substituted with the same string as `each.key`. This is correct for set-style for_each (`for_each = toset(...)`). For map-style for_each, `each.value` is not the value of the map entry but the key itself; verify the result if you rely on it.
- Remote backends are not handled directly. Run `terraform state pull > local.tfstate` first and pass it with `--state`.
- `moved` blocks emitted by the tool stay in place after `terraform apply`. Remove them once you no longer need the migration path.

## Verification

After running the tool, `terraform init && terraform plan` should report no resource changes, only `has moved to` lines.
