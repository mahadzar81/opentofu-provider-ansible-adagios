terraform {
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