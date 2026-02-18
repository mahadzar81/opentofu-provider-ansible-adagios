```markdown
# OpenTofu Provider for Ansible

The **OpenTofu Provider for Ansible** acts as a bridge between Infrastructure as Code (OpenTofu) and Configuration Management (Ansible). 

It eliminates the need for messy "glue code"—such as `local-exec` scripts with `sleep` commands or manual inventory updates—by allowing you to define Ansible resources (hosts, groups, and variables) directly within your OpenTofu code.

## 🚀 Core Concept

This provider does not "run" Ansible by default. Instead, it populates the **OpenTofu State File** with information about your infrastructure. You then use an Ansible Inventory Plugin to read that state file, effectively turning OpenTofu into a **Dynamic Inventory** for Ansible.

**Key Provider:** `ansible/ansible` (Community provider)

## 📋 Prerequisites

Before using this provider, ensure you have the following installed:

1.  **OpenTofu**
2.  **Ansible**
3.  **Ansible Collection:** You must install the collection that enables Ansible to parse the OpenTofu state file.

```bash
ansible-galaxy collection install cloud.terraform

```

---

## 🛠️ Usage Workflow 1: Dynamic Inventory (Recommended)

This is the standard best practice. You use OpenTofu to provision infrastructure and "tag" it for Ansible. Ansible is then run separately, reading the state file to discover hosts.

### 1. Configure OpenTofu (`main.tf`)

Define the provider and creating an `ansible_host` resource to register your server.

```hcl
terraform {
  required_providers {
    # The Cloud Provider (e.g., AWS)
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    # The Ansible Provider
    ansible = {
      source  = "ansible/ansible"
      version = "~> 1.3.0"
    }
  }
}

# 1. Create the Infrastructure
```
resource "aws_instance" "web_server" {
  ami           = "ami-0c55b159cbfafe1f0" # Example AMI
  instance_type = "t2.micro"
  tags = {
    Name = "Tofu-Web-Server"
  }
}
```
# 2. Register it for Ansible
```
resource "ansible_host" "web_host" {
  # The name Ansible will see in the inventory
  name   = aws_instance.web_server.public_dns 
  
  # Assign it to specific Ansible groups
  groups = ["web", "production"]

  # Define host variables Ansible can use
  variables = {
    ansible_user                 = "ubuntu"
    ansible_ssh_private_key_file = "~/.ssh/id_rsa"
    custom_app_port              = 8080
  }
}

```

### 2. Configure Ansible Inventory (`inventory.yml`)

Create a file named `inventory.yml` to tell Ansible to use the Terraform/OpenTofu plugin.

```yaml
plugin: cloud.terraform.terraform_provider

```

### 3. Execution

Provision your infrastructure and then run your playbook against the dynamic inventory.

```bash
# 1. Provision infrastructure and populate state
tofu init
tofu apply -auto-approve

# 2. Run Ansible (referencing the dynamic inventory)
ansible-playbook -i inventory.yml site.yml

```

**Benefits:**

* **Separation of Concerns:** OpenTofu handles the build; Ansible handles the configuration.
* **Scalability:** If you increase your instance count in Tofu, the `ansible_host` resource automatically registers the new hosts.

---

## ⚡ Usage Workflow 2: Direct Execution (Provisioning)

If you need OpenTofu to run a playbook *immediately* after a resource is created (replacing the `local-exec` pattern), use the `ansible_playbook` resource.

```hcl
resource "ansible_playbook" "web_setup" {
  # The playbook to run
  playbook   = "./playbooks/setup_webserver.yml"
  
  # The inventory to use (can be a file or a list of hosts)
  name       = aws_instance.web_server.public_ip
  
  # Groups the host belongs to
  groups     = ["web"]

  # Variables to pass to the playbook
  extra_vars = {
    http_port = 80
    env       = "prod"
  }
  
  # Ensure this runs ONLY after the server is ready
  depends_on = [aws_instance.web_server]
}

```

---

## 📄 License

MIT license

```

```