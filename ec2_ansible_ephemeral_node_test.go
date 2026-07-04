// Package test contains a Terratest suite for the OpenTofu module in
// mahadzar81/opentofu-provider-ansible-adagios.
//
// The module (main.tf) provisions:
//   - a VPC + two security groups (terraform-aws-modules)
//   - an aws_key_pair built from a local public key file (ssh_key.tf)
//   - one or more aws_instance "ec2-ephemeral-node" resources, bootstrapped
//     via a remote-exec provisioner (on_failure = continue)
//   - an ansible_host resource (ansible/ansible provider) per instance,
//     registering it in groups ["web", "production"]
//
// The module defines no `output` blocks, so this test reads results back
// two ways:
//   1. Directly from AWS (EC2 SDK) using the instance's Name tag.
//   2. From `tofu show -json`, to confirm the ansible_host resource in
//      state was populated with the values we expect.
//
// Prerequisites:
//   - AWS credentials in the environment (must be able to create/destroy
//     VPCs, EC2 instances, security groups, key pairs, and to read/write
//     the hardcoded state backend: s3://maza-remote-state-storage-s3
//     with DynamoDB lock table terraform-state-lock-dynamo, both in
//     ap-southeast-1).
//   - The `tofu` binary on PATH (the module uses OpenTofu-specific
//     backend/provider syntax; terraform.Options.TerraformBinary is set
//     to "tofu" below).
//   - Network egress from the test runner to port 22 on the created
//     instance's public IP, for the SSH connectivity check.
//
// Run with:
//   cd test && go mod tidy && go test -v -timeout 45m -run TestEphemeralNodeAnsibleHost
//
// Set SKIP_SSH_CHECK=true to skip the live SSH validation (useful if the
// runner's network can't reach the instance's public IP, e.g. behind a
// corporate proxy).
package test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/gruntwork-io/terratest/modules/retry"
	ttssh "github.com/gruntwork-io/terratest/modules/ssh"
	"github.com/gruntwork-io/terratest/modules/terraform"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"
)

const (
	awsRegion            = "ap-southeast-1" // where the EC2/VPC resources are created
	sshUser              = "ubuntu"
	instanceTypeForTest  = "t3.micro"
	expectedInstanceName = "-ephemeral-node" // matches locals.name in main.tf
	sshCheckRetries      = 20
	sshCheckWaitBetween  = 15 * time.Second
)

// TestEphemeralNodeAnsibleHost applies the module end-to-end against a real
// AWS account, then validates the EC2 instance, its security groups, SSH
// reachability, and the resulting ansible_host resource in state.
func TestEphemeralNodeAnsibleHost(t *testing.T) {
	t.Parallel()

	uniqueID := strings.ToLower(random.UniqueId())

	// --- 1. Generate a throwaway SSH key pair for this test run ---------
	privateKeyPEM, publicKeyAuthorized, err := generateRSAKeyPair(2048)
	require.NoError(t, err, "failed to generate SSH key pair")

	tmpDir := t.TempDir()
	privateKeyPath := filepath.Join(tmpDir, "id_rsa")
	publicKeyPath := filepath.Join(tmpDir, "id_rsa.pub")

	require.NoError(t, os.WriteFile(privateKeyPath, []byte(privateKeyPEM), 0600))
	require.NoError(t, os.WriteFile(publicKeyPath, []byte(publicKeyAuthorized), 0644))

	// --- 2. Look up a current Ubuntu 22.04 AMI in the target region -----
	amiID := getLatestUbuntuAMI(t, awsRegion)

	// --- 3. Configure Terratest / OpenTofu options -----------------------
	terraformOptions := &terraform.Options{
		TerraformDir:   "../", // path to the module root (main.tf, modvars.tf, ssh_key.tf, versions.tf)
		TerraformBinary: "tofu",
		Vars: map[string]interface{}{
			"region":         awsRegion,
			"ami":            amiID,
			"instance_type":  instanceTypeForTest,
			"command":        []string{"echo terratest-bootstrap-ok"},
			"count_instance": 1,
			"key_pair_id":    publicKeyPath,
			"user":           sshUser,
			"private_key":    privateKeyPath,
			// These three vars exist in modvars.tf but aren't actually wired
			// into the hardcoded `backend "s3"` block in main.tf (OpenTofu/
			// Terraform backends can't reference variables). They're passed
			// through only so `tofu plan/apply` doesn't warn about unused
			// declared-but-required vars in case a future refactor wires
			// them up.
			"backend_bucket":         "maza-remote-state-storage-s3",
			"backend_key":            fmt.Sprintf("infrastructure/tfremote/terratest-%s.tfstate", uniqueID),
			"backend_dynamodb_table": "terraform-state-lock-dynamo",
		},
		// Override just the state file key so concurrent/parallel test runs
		// don't clobber each other or production state in the same bucket.
		BackendConfig: map[string]interface{}{
			"bucket":         "maza-remote-state-storage-s3",
			"key":            fmt.Sprintf("infrastructure/tfremote/terratest-%s.tfstate", uniqueID),
			"region":         "ap-southeast-1",
			"dynamodb_table": "terraform-state-lock-dynamo",
			"encrypt":        true,
		},
		Reconfigure: true,
		NoColor:     true,
		RetryableTerraformErrors: map[string]string{
			".*RequestLimitExceeded.*":        "Hit an AWS API rate limit, retrying.",
			".*ThrottlingException.*":         "Hit AWS throttling, retrying.",
			".*Error creating Ansible Host.*":  "Transient provider error, retrying.",
		},
		MaxRetries:         3,
		TimeBetweenRetries: 10 * time.Second,
	}

	// --- 4. Ensure resources (and the temp key files) are cleaned up ----
	defer terraform.Destroy(t, terraformOptions)

	// --- 5. Apply -----------------------------------------------------
	terraform.InitAndApply(t, terraformOptions)

	// --- 6. Validate the EC2 instance directly via AWS -------------------
	sess, err := session.NewSession(&awssdk.Config{Region: awssdk.String(awsRegion)})
	require.NoError(t, err)
	ec2Client := ec2.New(sess)

	instance := findRunningInstanceByName(t, ec2Client, expectedInstanceName)
	require.NotNil(t, instance, "expected a running EC2 instance tagged Name=%s", expectedInstanceName)

	assert.Equal(t, instanceTypeForTest, awssdk.StringValue(instance.InstanceType), "instance type should match what was requested")
	assert.Equal(t, "user_ssh", awssdk.StringValue(instance.KeyName), "instance should be launched with the key pair created by ssh_key.tf")
	assert.NotEmpty(t, awssdk.StringValue(instance.PublicIpAddress), "instance should have a public IP (associate_public_ip_address = true)")
	assert.NotEmpty(t, awssdk.StringValue(instance.PublicDnsName), "instance should have a public DNS name (used as the ansible_host name)")

	// --- 7. Validate the security group rules ----------------------------
	sgIDs := make([]*string, 0, len(instance.SecurityGroups))
	for _, sg := range instance.SecurityGroups {
		sgIDs = append(sgIDs, sg.GroupId)
	}
	assertSecurityGroupAllowsTCP(t, ec2Client, sgIDs, 22, "ssh-tcp ingress")
	assertSecurityGroupAllowsTCP(t, ec2Client, sgIDs, 80, "http-80-tcp ingress")
	assertSecurityGroupAllowsTCP(t, ec2Client, sgIDs, 443, "https-443-tcp ingress")
	assertSecurityGroupAllowsTCP(t, ec2Client, sgIDs, 4730, "mod-gearman ingress")

	// --- 8. Validate SSH reachability (skippable) -------------------------
	if strings.ToLower(os.Getenv("SKIP_SSH_CHECK")) != "true" {
		publicIP := awssdk.StringValue(instance.PublicIpAddress)
		host := ttssh.Host{
			Hostname:    publicIP,
			SshUserName: sshUser,
			SshKeyPair: &ttssh.KeyPair{
				PublicKey:  publicKeyAuthorized,
				PrivateKey: privateKeyPEM,
			},
		}

		retry.DoWithRetry(t, "SSH to ephemeral node", sshCheckRetries, sshCheckWaitBetween, func() (string, error) {
			return ttssh.CheckSshCommandE(t, host, "echo terratest-ssh-ok")
		})
	} else {
		t.Log("SKIP_SSH_CHECK=true, skipping live SSH connectivity check")
	}

	// --- 9. Validate the ansible_host resource via `tofu show -json` -----
	resources := getStateResources(t, terraformOptions)

	ansibleHost := findResourceByType(resources, "ansible_host")
	require.NotNil(t, ansibleHost, "expected an ansible_host resource in state")

	nameAttr, _ := ansibleHost.AttributeValues["name"].(string)
	assert.Equal(t, awssdk.StringValue(instance.PublicDnsName), nameAttr, "ansible_host name should be the instance's public DNS name")

	groupsRaw, _ := ansibleHost.AttributeValues["groups"].([]interface{})
	groups := make([]string, 0, len(groupsRaw))
	for _, g := range groupsRaw {
		if s, ok := g.(string); ok {
			groups = append(groups, s)
		}
	}
	sort.Strings(groups)
	assert.Equal(t, []string{"production", "web"}, groups, "ansible_host should be registered in the web and production groups")

	varsRaw, _ := ansibleHost.AttributeValues["variables"].(map[string]interface{})
	assert.Equal(t, sshUser, varsRaw["ansible_user"], "ansible_host variables.ansible_user should match var.user")

	awsInstanceRes := findResourceByType(resources, "aws_instance")
	require.NotNil(t, awsInstanceRes, "expected an aws_instance resource in state")
	assert.Equal(t, amiID, awsInstanceRes.AttributeValues["ami"], "aws_instance ami should match the AMI passed in")
}

// --- helpers --------------------------------------------------------------

// generateRSAKeyPair returns a PEM-encoded RSA private key and the
// corresponding OpenSSH "authorized_keys" formatted public key.
func generateRSAKeyPair(bits int) (privateKeyPEM string, publicKeyAuthorized string, err error) {
	priv, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return "", "", err
	}

	privDER := x509.MarshalPKCS1PrivateKey(priv)
	privBlock := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privDER,
	}
	privateKeyPEM = string(pem.EncodeToMemory(&privBlock))

	pub, err := gossh.NewPublicKey(&priv.PublicKey)
	if err != nil {
		return "", "", err
	}
	publicKeyAuthorized = string(gossh.MarshalAuthorizedKey(pub))

	return privateKeyPEM, publicKeyAuthorized, nil
}

// getLatestUbuntuAMI looks up the most recent Ubuntu 22.04 LTS (Canonical)
// AMI in the given region, since modvars.tf's `ami` var has no usable
// default.
func getLatestUbuntuAMI(t *testing.T, region string) string {
	sess, err := session.NewSession(&awssdk.Config{Region: awssdk.String(region)})
	require.NoError(t, err)
	client := ec2.New(sess)

	out, err := client.DescribeImages(&ec2.DescribeImagesInput{
		Owners: []*string{awssdk.String("099720109477")}, // Canonical
		Filters: []*ec2.Filter{
			{Name: awssdk.String("name"), Values: []*string{awssdk.String("ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-*")}},
			{Name: awssdk.String("virtualization-type"), Values: []*string{awssdk.String("hvm")}},
			{Name: awssdk.String("state"), Values: []*string{awssdk.String("available")}},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, out.Images, "no Ubuntu 22.04 AMIs found in %s", region)

	sort.Slice(out.Images, func(i, j int) bool {
		return awssdk.StringValue(out.Images[i].CreationDate) > awssdk.StringValue(out.Images[j].CreationDate)
	})

	return awssdk.StringValue(out.Images[0].ImageId)
}

// findRunningInstanceByName returns the first running EC2 instance whose
// Name tag matches expectedName, or nil if none is found.
func findRunningInstanceByName(t *testing.T, client *ec2.EC2, expectedName string) *ec2.Instance {
	out, err := client.DescribeInstances(&ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{Name: awssdk.String("tag:Name"), Values: []*string{awssdk.String(expectedName)}},
			{Name: awssdk.String("instance-state-name"), Values: []*string{awssdk.String("running")}},
		},
	})
	require.NoError(t, err)

	for _, res := range out.Reservations {
		for _, inst := range res.Instances {
			return inst
		}
	}
	return nil
}

// assertSecurityGroupAllowsTCP fails the test unless at least one of sgIDs
// has an ingress rule allowing the given TCP port.
func assertSecurityGroupAllowsTCP(t *testing.T, client *ec2.EC2, sgIDs []*string, port int64, description string) {
	if len(sgIDs) == 0 {
		t.Fatalf("no security groups attached to instance, cannot check %s", description)
	}

	out, err := client.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{GroupIds: sgIDs})
	require.NoError(t, err)

	for _, sg := range out.SecurityGroups {
		for _, perm := range sg.IpPermissions {
			if perm.FromPort == nil || perm.ToPort == nil {
				continue
			}
			if awssdk.StringValue(perm.IpProtocol) != "tcp" {
				continue
			}
			if *perm.FromPort <= port && port <= *perm.ToPort {
				assert.True(t, true, description) // rule found; record a passing assertion
				return
			}
		}
	}
	t.Errorf("no security group among %v allows TCP port %d (%s)", awssdk.StringValueSlice(sgIDs), port, description)
}

// getStateResources shells out to `tofu show -json` and returns the flat
// list of resources in the root module's current state.
func getStateResources(t *testing.T, options *terraform.Options) []*tfjson.StateResource {
	raw := terraform.RunTerraformCommand(t, options, "show", "-json")

	var state tfjson.State
	require.NoError(t, json.Unmarshal([]byte(raw), &state))

	if state.Values == nil || state.Values.RootModule == nil {
		return nil
	}
	return state.Values.RootModule.Resources
}

// findResourceByType returns the first resource of the given Terraform
// resource type (e.g. "aws_instance", "ansible_host"), or nil.
func findResourceByType(resources []*tfjson.StateResource, resourceType string) *tfjson.StateResource {
	for _, r := range resources {
		if r.Type == resourceType {
			return r
		}
	}
	return nil
}
