variable "ami" {
  description = "AMI ID to launch the ephemeral node from. Left with no usable default on purpose: AMI IDs are region- and time-specific, so resolve one per run (e.g. via an aws_ami data source, or the Terratest suite's own AMI lookup)."
  type        = string
  default     = null
}

variable "region" {
  description = "AWS region to provision the VPC and EC2 resources in."
  type        = string
}

variable "instance_type" {
  description = "EC2 instance type for the ephemeral node (e.g. \"t3.micro\")."
  type        = string
}

variable "command" {
  description = "Shell command(s) run on the instance during initial bootstrap via the remote-exec provisioner."
  type        = list(string)
}

variable "count_instance" {
  description = "Number of ephemeral node instances to create."
  type        = number
  default     = 1

  validation {
    condition     = var.count_instance >= 0
    error_message = "count_instance must be zero or a positive number."
  }
}

variable "key_pair_id" {
  description = "Path to the local SSH public key file used to create the AWS key pair (supports ~ expansion)."
  type        = string
}

variable "user" {
  description = "SSH/Ansible user used to connect to the instance (e.g. \"ubuntu\")."
  type        = string
}

variable "private_key" {
  description = "Path to the local SSH private key file matching key_pair_id (supports ~ expansion)."
  type        = string
  default     = "~/.ssh/id_rsa"
}

variable "app_port" {
  description = "Application port recorded as the custom_app_port Ansible host variable."
  type        = number
  default     = 80
}

variable "ssh_allowed_cidr_blocks" {
  description = "CIDR blocks allowed to reach the ephemeral node over SSH/HTTP/HTTPS/ICMP/mod-gearman. Defaults to 0.0.0.0/0 to preserve prior behavior -- narrow this for anything beyond short-lived testing."
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC created for the ephemeral node."
  type        = string
  default     = "10.0.0.0/16"
}

variable "root_volume_size" {
  description = "Root EBS volume size (GiB) for the ephemeral node."
  type        = number
  default     = 30
}

variable "root_volume_type" {
  description = "Root EBS volume type for the ephemeral node."
  type        = string
  default     = "gp3"
}

# --- Backend documentation-only variables -----------------------------------
# OpenTofu/Terraform backend blocks cannot reference variables, so these are
# NOT consumed directly by the `backend "s3" {}` block in main.tf (see the
# comment there). They exist so CI pipelines, Makefiles, or wrapper scripts
# have one source of truth to build `-backend-config` flags / a
# backend.hcl file from, e.g.:
#   tofu init -backend-config="bucket=${TF_VAR_backend_bucket}" ...
# See backend.hcl.example for the file-based equivalent.

variable "backend_bucket" {
  description = "S3 bucket for remote state. Passed via -backend-config at init time, not referenced directly in this file."
  type        = string
}

variable "backend_key" {
  description = "S3 key/path for this state file. Passed via -backend-config at init time, not referenced directly in this file."
  type        = string
}

variable "backend_dynamodb_table" {
  description = "DynamoDB table used for state locking. Passed via -backend-config at init time, not referenced directly in this file."
  type        = string
}
