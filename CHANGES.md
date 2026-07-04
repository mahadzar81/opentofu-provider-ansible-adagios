# Changes

Everything here keeps the module's public variable interface identical
(`region`, `ami`, `instance_type`, `command`, `count_instance`, `key_pair_id`,
`user`, `private_key`, `backend_bucket`, `backend_key`,
`backend_dynamodb_table`, security group ports, `aws_key_pair` name
`user_ssh`) so the Terratest suite in `test/` needs no changes and keeps
passing.

## Bug fixes

- **`file(var.private_key)` didn't expand `~`.** The default
  `~/.ssh/id_rsa` would fail outright since `file()` has no shell
  tilde-expansion. Fixed with `pathexpand()` in both `main.tf`'s
  `connection` block and `ssh_key.tf`'s `public_key`.
- **`ansible_host.variables.ansible_ssh_private_key_file` was hardcoded**
  to `"~/.ssh/id_rsa"` regardless of `var.private_key`, so the Ansible
  metadata could point at the wrong key if you ever changed
  `private_key`. Now uses `var.private_key`.
- **Dead `security_group` module.** It was created every apply but never
  attached to anything (the instance only ever used
  `security_group_ssh`). Removed — no behavior change, one fewer resource
  per apply/destroy cycle.
- **Wrong `Repository` tag** on the EC2 instance pointed at an unrelated
  repo (`terraform-aws-modules/terraform-aws-ec2-instance`). Fixed to
  point at this repo.
- **Unused `domain_name` / `subdomain` locals** removed (dead code, never
  referenced).
- **Hardcoded, fully-baked S3 backend.** `modvars.tf` declares
  `backend_bucket`, `backend_key`, and `backend_dynamodb_table`, but
  `main.tf`'s `backend "s3" {}` block had all four values hardcoded —
  those variables were unusable (backend blocks can't reference
  variables at all). Converted to a **partial backend config**: the block
  is now empty, and real values come from `-backend-config` flags or
  `backend.hcl` at `tofu init` time. `backend.hcl.example` shows the
  file-based form. The three variables are now documented as
  init-time-only rather than silently unused.
- **Plaintext password committed in `site.yml`**
  (`nagiosadmin_pass: "SuperSecurePassword!"`). Replaced with a reference
  to an ansible-vault-encrypted variable (`vault_nagiosadmin_pass`).

## Hardening / correctness

- **Root volume**: switched from `ebs_block_device` with a hardcoded
  `/dev/xvda` device name to `root_block_device`, so it tracks whatever
  the AMI's actual root device is instead of assuming a fixed name.
  Also switched `volume_type` from `"standard"` (magnetic) to `"gp3"`,
  and turned on `encrypted = true`. Both are now configurable via new
  `root_volume_size` / `root_volume_type` variables (defaults: `30`,
  `"gp3"`, matching prior size).
- **`connection` timeout** explicitly set to `10m` (was a commented-out
  no-op) to give a cold-booting instance more room before the
  `remote-exec` provisioner gives up.
- Added a `count_instance` validation block (`>= 0`) and a default of
  `1`, so it's optional instead of a hard-required variable with no
  guardrails.
- Added descriptions to every variable in `modvars.tf`.
- Pinned `required_version = ">= 1.6.0"` in `versions.tf` — previously
  unconstrained.

## New (opt-in) variables — all with defaults matching prior behavior

| Variable                  | Default          | Purpose                                                        |
|----------------------------|------------------|-----------------------------------------------------------------|
| `ssh_allowed_cidr_blocks`  | `["0.0.0.0/0"]`  | Narrow SSH/HTTP/HTTPS/ICMP/mod-gearman ingress instead of open access |
| `app_port`                 | `80`             | Was a hardcoded `80` in the `ansible_host` variables            |
| `vpc_cidr`                 | `"10.0.0.0/16"`  | Was a hardcoded local                                           |
| `root_volume_size`         | `30`             | Was a hardcoded `"30"` string                                   |
| `root_volume_type`         | `"gp3"`          | Was hardcoded `"standard"`                                      |

None of these change default behavior, so nothing downstream (including
the Terratest suite) needs to pass them.

## New files

- `backend.hcl.example` — template for `-backend-config=backend.hcl`
- `terraform.tfvars.example` — template covering every variable

## Left unchanged (out of scope / would break the test or the interface)

- `aws_key_pair` name stays the literal `"user_ssh"` — the Terratest
  suite asserts on this exact value. If you need to run multiple copies
  of this module in one account concurrently, that'd need a real design
  discussion (e.g. a `key_name_prefix` + a corresponding test update),
  not a silent rename.
- `tfparser.go` / `tfparser_test.go` — untouched; unrelated to the
  Terraform/OpenTofu module itself.
- The `on_failure = continue` on the `remote-exec` provisioner —
  kept intentionally; see the comment added in `main.tf`. Changing it to
  `fail` would make `tofu apply` (and therefore the Terratest apply step)
  fail on any transient SSH timing issue during boot, which is a real
  behavior change, not just a fix — flagging it here rather than making
  the call unilaterally.
