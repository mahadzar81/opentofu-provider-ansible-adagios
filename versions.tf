terraform {
  # Pin the CLI itself. This module uses OpenTofu-specific idioms (e.g. a
  # partial S3 backend configured via `-backend-config`), so pin to a
  # version known to support it rather than leaving this unconstrained.
  required_version = ">= 1.6.0"

  required_providers {
    # The Cloud Provider (e.g., AWS)
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
    # The Ansible Provider
    ansible = {
      source  = "ansible/ansible"
      version = "~> 1.3.0"
    }
  }
}
