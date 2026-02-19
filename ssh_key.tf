resource "aws_key_pair" "auth_ephemeral_node" {
  key_name   = "user_ssh"
  public_key = file(var.key_pair_id)
}