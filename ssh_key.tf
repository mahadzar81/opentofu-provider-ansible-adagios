resource "aws_key_pair" "auth_ephemeral_node" {
  # NOTE: kept as a fixed name (rather than key_name_prefix) so re-running
  # this module in the same account/region reliably targets the same key
  # pair, and so existing tooling/tests that assert on the key pair name
  # keep working. If you need to run multiple copies of this module
  # concurrently in one account, override this via a future variable
  # rather than hardcoding a second literal here.
  key_name = "user_ssh"

  # pathexpand() so "~/.ssh/id_rsa.pub"-style paths work; file() itself does
  # not do shell tilde-expansion.
  public_key = file(pathexpand(var.key_pair_id))
}
