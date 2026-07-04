provider "aws" {
  region = var.region
}

terraform {
  # Partial backend configuration.
  #
  # Bucket/key/region/dynamodb_table are intentionally NOT hardcoded here.
  # OpenTofu/Terraform backend blocks cannot reference input variables, so
  # a fully-hardcoded block (the previous version of this file) meant the
  # backend_bucket/backend_key/backend_dynamodb_table variables in
  # modvars.tf were declared but silently unused, and every consumer of
  # this module was stuck writing to the same state file.
  #
  # Supply the real values at init time, e.g.:
  #   tofu init -backend-config=backend.hcl
  # or:
  #   tofu init \
  #     -backend-config="bucket=$TF_VAR_backend_bucket" \
  #     -backend-config="key=$TF_VAR_backend_key" \
  #     -backend-config="region=ap-southeast-1" \
  #     -backend-config="dynamodb_table=$TF_VAR_backend_dynamodb_table" \
  #     -backend-config="encrypt=true"
  #
  # See backend.hcl.example. Terratest does the equivalent via
  # terraform.Options.BackendConfig.
  backend "s3" {}
}

data "aws_availability_zones" "available" {}

locals {
  name     = "-ephemeral-node"
  vpc_cidr = var.vpc_cidr
  azs      = slice(data.aws_availability_zones.available.names, 0, 3)

  tags = {
    Name       = local.name
    Example    = local.name
    Repository = "https://github.com/mahadzar81/opentofu-provider-ansible-adagios"
  }
}

################################################################################
# EC2 ephemeral / jump node
################################################################################

resource "aws_instance" "ec2-ephemeral-node" {
  count = var.count_instance

  ami           = var.ami
  instance_type = var.instance_type
  key_name      = aws_key_pair.auth_ephemeral_node.key_name
  subnet_id     = element(module.vpc.public_subnets, 0)

  vpc_security_group_ids      = [module.security_group_ssh.security_group_id]
  associate_public_ip_address = true
  source_dest_check           = false

  root_block_device {
    volume_size           = var.root_volume_size
    volume_type           = var.root_volume_type
    encrypted             = true
    delete_on_termination = true
  }

  tags = local.tags

  connection {
    type        = "ssh"
    host        = self.public_ip
    private_key = file(pathexpand(var.private_key))
    user        = var.user
    timeout     = "10m" # give cloud-init / SSH daemon startup room on cold boot
  }

  provisioner "remote-exec" {
    inline     = var.command
    on_failure = continue
  }
}

resource "ansible_host" "web_host" {
  # One ansible_host per EC2 instance created above.
  count = length(aws_instance.ec2-ephemeral-node)

  name   = aws_instance.ec2-ephemeral-node[count.index].public_dns
  groups = ["web", "production"]

  variables = {
    ansible_user = var.user
    ansible_ssh_private_key_file = var.private_key
    custom_app_port               = var.app_port
  }
}

################################################################################
# Supporting Resources
################################################################################

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 6.0"

  name = local.name
  cidr = local.vpc_cidr

  azs            = local.azs
  public_subnets = [for k, v in local.azs : cidrsubnet(local.vpc_cidr, 8, k)]

  tags = local.tags
}

module "security_group_ssh" {
  source  = "terraform-aws-modules/security-group/aws"
  version = "~> 5.0"

  name        = local.name
  description = "Allow SSH, ICMP, and HTTP/HTTPS access to the ephemeral node"
  vpc_id      = module.vpc.vpc_id

  ingress_cidr_blocks = var.ssh_allowed_cidr_blocks
  ingress_rules       = ["http-80-tcp", "https-443-tcp", "all-icmp", "ssh-tcp"]
  egress_rules        = ["all-all"]

  ingress_with_cidr_blocks = [
    {
      from_port   = 4730
      to_port     = 4730
      protocol    = "tcp"
      description = "mod-gearman port"
      cidr_blocks = join(",", var.ssh_allowed_cidr_blocks)
    },
  ]

  tags = local.tags
}
