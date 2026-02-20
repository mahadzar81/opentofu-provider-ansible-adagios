provider "aws" {
  region = var.region
}

terraform {
  backend "s3" {
    encrypt        = true
    bucket         = "maza-remote-state-storage-s3"
    region         = "ap-southeast-1"
    key            = "infrastructure/tfremote/terraform.tfstate"
    dynamodb_table = "terraform-state-lock-dynamo"
  }
}

data "aws_availability_zones" "available" {}

locals {
  name   = "-ephemeral-node"
  vpc_cidr = "10.0.0.0/16"
  azs      = slice(data.aws_availability_zones.available.names, 0, 3)
  domain_name = "terraform-aws-modules.modules.tf" # trimsuffix(data.aws_route53_zone.this.name, ".")
  subdomain   = "complete-http"

  tags = {
    Name       = local.name
    Example    = local.name
    Repository = "https://github.com/mahadzar81/terraform-aws-serverless"
  }
}

################################################################################
# ec2 jump node
################################################################################

resource "aws_instance" "ec2-ephemeral-node"  {
  ami           = var.ami
  instance_type = var.instance_type
  key_name      = aws_key_pair.auth_ephemeral_node.key_name
  subnet_id = element(module.vpc.public_subnets, 0)
  vpc_security_group_ids      = [module.security_group_ssh.security_group_id]
  
  associate_public_ip_address = true
  source_dest_check           = false
  count   = var.count_instance

  ebs_block_device {
    device_name           = "/dev/xvda"
    volume_size           = "30"
    volume_type           = "standard"
    delete_on_termination = true
  }

  tags = {
    Name       = local.name
    Example    = local.name
    Repository = "https://github.com/terraform-aws-modules/terraform-aws-ec2-instance"
  }

  connection {
    type        = "ssh"
    host        = self.public_ip
    private_key = file(var.private_key)
    user        = var.user
    # timeout     = "15m"
  }
  # install prerequisites package
  provisioner "remote-exec" {
    inline = var.command
    on_failure = continue

  }
}

resource "ansible_host" "web_host" {
  # Add count to match your EC2 instances
  count = length(aws_instance.ec2-ephemeral-node)

  # Use the index [count.index] to pick the right DNS
  name  = aws_instance.ec2-ephemeral-node[count.index].public_dns 
  
  groups = ["web", "production"]

  variables = {
    ansible_user                 = var.user
    ansible_ssh_private_key_file = "~/.ssh/id_rsa"
    custom_app_port              = 80
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

  azs              = local.azs
  public_subnets   = [for k, v in local.azs : cidrsubnet(local.vpc_cidr, 8, k)]
  /* private_subnets  = [for k, v in local.azs : cidrsubnet(local.vpc_cidr, 8, k + 3)]
  database_subnets = [for k, v in local.azs : cidrsubnet(local.vpc_cidr, 8, k + 6)]
  create_database_subnet_group = true */

  tags = local.tags
}

module "security_group" {
  source  = "terraform-aws-modules/security-group/aws"
  version = "~> 5.0"

  name        = local.name
  description = "S3 import VPC example security group"
  vpc_id      = module.vpc.vpc_id

  # ingress
  ingress_with_self = [
    {
      rule        = "https-443-tcp"
      description = "Allow all internal HTTPs"
    },
  ]
  # egress
  computed_egress_with_self = [
    {
      rule        = "https-443-tcp"
      description = "Allow all internal HTTPs"
    },
  ]
  number_of_computed_egress_with_self = 1

  egress_cidr_blocks = ["0.0.0.0/0"]
  egress_rules       = ["all-all"]
  
  tags = local.tags
}

module "security_group_ssh" {
  source  = "terraform-aws-modules/security-group/aws"
  version = "~> 5.0"

  name        = local.name
  description = "Allow ssh, ICMP, HTTP/HTTPS access for All"
  vpc_id      = module.vpc.vpc_id

  ingress_cidr_blocks = ["0.0.0.0/0"]
  ingress_rules       = ["http-80-tcp", "https-443-tcp", "all-icmp", "ssh-tcp"]
  egress_rules        = ["all-all"]

  ingress_with_cidr_blocks = [
    {
      from_port   = 4730
      to_port     = 4730
      protocol    = "tcp"
      description = "mod-gearman port"
      cidr_blocks = "0.0.0.0/0"
    },
  ]

  tags = local.tags
}
