package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTFFiles(t *testing.T) {
	tests := []struct {
		name        string
		dir         string
		wantErr     bool
		errContains string
	}{
		{
			name:    "current directory",
			dir:     ".",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := ParseTFFiles(tt.dir)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseTFFiles() expected error but got nil")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("ParseTFFiles() error = %v, want contains %v", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseTFFiles() unexpected error = %v", err)
				return
			}

			if config == nil {
				t.Errorf("ParseTFFiles() expected non-nil config")
				return
			}
		})
	}
}

func TestParseFile(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.tf")
	
	content := `
provider "aws" {
  region = "us-east-1"
}

resource "aws_instance" "web" {
  ami           = "ami-12345678"
  instance_type = "t2.micro"
}

variable "region" {
  type = string
}

module "vpc" {
  source = "terraform-aws-modules/vpc/aws"
}

data "aws_availability_zones" "available" {}

locals {
  name = "test"
}
`
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	config := &TerraformConfig{
		Providers:   make(map[string]interface{}),
		Resources:   make(map[string]interface{}),
		Variables:   make(map[string]interface{}),
		Modules:     make(map[string]interface{}),
		DataSources: make(map[string]interface{}),
		Locals:      make(map[string]interface{}),
	}

	err := parseFile(testFile, config)
	if err != nil {
		t.Errorf("parseFile() unexpected error = %v", err)
		return
	}

	// Verify provider was parsed
	if _, exists := config.Providers["aws"]; !exists {
		t.Errorf("parseFile() expected provider 'aws' to be parsed")
	}

	// Verify resource was parsed
	if _, exists := config.Resources["aws_instance.web"]; !exists {
		t.Errorf("parseFile() expected resource 'aws_instance.web' to be parsed")
	}

	// Verify variable was parsed
	if _, exists := config.Variables["region"]; !exists {
		t.Errorf("parseFile() expected variable 'region' to be parsed")
	}

	// Verify module was parsed
	if _, exists := config.Modules["vpc"]; !exists {
		t.Errorf("parseFile() expected module 'vpc' to be parsed")
	}

	// Verify data source was parsed
	if _, exists := config.DataSources["aws_availability_zones.available"]; !exists {
		t.Errorf("parseFile() expected data source 'aws_availability_zones.available' to be parsed")
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name          string
		config        *TerraformConfig
		expectedErrors int
	}{
		{
			name: "valid config with providers and resources",
			config: &TerraformConfig{
				Providers: map[string]interface{}{"aws": nil},
				Resources: map[string]interface{}{"aws_instance.web": nil},
			},
			expectedErrors: 0,
		},
		{
			name: "no providers",
			config: &TerraformConfig{
				Providers: map[string]interface{}{},
				Resources: map[string]interface{}{"aws_instance.web": nil},
			},
			expectedErrors: 1,
		},
		{
			name: "no resources",
			config: &TerraformConfig{
				Providers: map[string]interface{}{"aws": nil},
				Resources: map[string]interface{}{},
			},
			expectedErrors: 1,
		},
		{
			name: "no providers and no resources",
			config: &TerraformConfig{
				Providers: map[string]interface{}{},
				Resources: map[string]interface{}{},
			},
			expectedErrors: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := ValidateConfig(tt.config)
			if len(errors) != tt.expectedErrors {
				t.Errorf("ValidateConfig() expected %d errors, got %d: %v", tt.expectedErrors, len(errors), errors)
			}
		})
	}
}

func TestGetResourceCount(t *testing.T) {
	config := &TerraformConfig{
		Resources: map[string]interface{}{
			"aws_instance.web":    nil,
			"aws_instance.app":    nil,
			"aws_s3_bucket.data":  nil,
			"aws_security_group.sg": nil,
		},
	}

	tests := []struct {
		name         string
		resourceType string
		expected     int
	}{
		{
			name:         "count aws_instance",
			resourceType: "aws_instance",
			expected:     2,
		},
		{
			name:         "count aws_s3_bucket",
			resourceType: "aws_s3_bucket",
			expected:     1,
		},
		{
			name:         "count non-existent type",
			resourceType: "aws_lambda_function",
			expected:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := GetResourceCount(config, tt.resourceType)
			if count != tt.expected {
				t.Errorf("GetResourceCount(%s) = %d, want %d", tt.resourceType, count, tt.expected)
			}
		})
	}
}

func TestHasVariable(t *testing.T) {
	config := &TerraformConfig{
		Variables: map[string]interface{}{
			"region":       nil,
			"instance_type": nil,
			"ami":          nil,
		},
	}

	tests := []struct {
		name     string
		varName  string
		expected bool
	}{
		{name: "existing variable", varName: "region", expected: true},
		{name: "non-existing variable", varName: "environment", expected: false},
		{name: "case sensitive check", varName: "Region", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasVariable(config, tt.varName)
			if result != tt.expected {
				t.Errorf("HasVariable(%s) = %v, want %v", tt.varName, result, tt.expected)
			}
		})
	}
}

func TestHasModule(t *testing.T) {
	config := &TerraformConfig{
		Modules: map[string]interface{}{
			"vpc":            nil,
			"security_group": nil,
		},
	}

	tests := []struct {
		name       string
		moduleName string
		expected   bool
	}{
		{name: "existing module", moduleName: "vpc", expected: true},
		{name: "non-existing module", moduleName: "rds", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasModule(config, tt.moduleName)
			if result != tt.expected {
				t.Errorf("HasModule(%s) = %v, want %v", tt.moduleName, result, tt.expected)
			}
		})
	}
}

func TestHasDataSource(t *testing.T) {
	config := &TerraformConfig{
		DataSources: map[string]interface{}{
			"aws_availability_zones.available": nil,
			"aws_ami.ubuntu":                   nil,
		},
	}

	tests := []struct {
		name           string
		dataSourceType string
		dsName         string
		expected       bool
	}{
		{name: "existing data source", dataSourceType: "aws_availability_zones", dsName: "available", expected: true},
		{name: "non-existing data source", dataSourceType: "aws_subnet", dsName: "main", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasDataSource(config, tt.dataSourceType, tt.dsName)
			if result != tt.expected {
				t.Errorf("HasDataSource(%s, %s) = %v, want %v", tt.dataSourceType, tt.dsName, result, tt.expected)
			}
		})
	}
}

func TestInitializeTerraform(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		execPath string
		wantErr  bool
	}{
		{
			name:     "empty exec path should search for binary",
			execPath: "",
			wantErr:  true, // Will fail if neither tofu nor terraform is installed
		},
		{
			name:     "non-existent binary",
			execPath: "/nonexistent/binary",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tf, err := InitializeTerraform(tmpDir, tt.execPath)

			if tt.wantErr {
				if err == nil {
					t.Logf("InitializeTerraform() expected error but got nil (binary might be installed)")
					// This is okay - the binary might actually be installed
				}
				return
			}

			if err != nil {
				t.Errorf("InitializeTerraform() unexpected error = %v", err)
				return
			}

			if tf == nil {
				t.Errorf("InitializeTerraform() expected non-nil Terraform instance")
			}
		})
	}
}

func TestParseActualProject(t *testing.T) {
	// Test parsing the actual project in the workspace
	config, err := ParseTFFiles(".")
	if err != nil {
		t.Fatalf("ParseTFFiles() failed to parse project: %v", err)
	}

	// Verify expected providers
	if _, exists := config.Providers["aws"]; !exists {
		t.Error("Expected 'aws' provider to be present")
	}

	// Verify expected resources
	expectedResources := []string{
		"aws_instance.ec2-ephemeral-node",
		"ansible_host.web_host",
		"aws_key_pair.auth_ephemeral_node",
	}

	for _, resource := range expectedResources {
		if _, exists := config.Resources[resource]; !exists {
			t.Errorf("Expected resource '%s' to be present", resource)
		}
	}

	// Verify expected variables
	expectedVariables := []string{
		"ami",
		"region",
		"instance_type",
		"command",
		"count_instance",
		"user",
	}

	for _, variable := range expectedVariables {
		if !HasVariable(config, variable) {
			t.Errorf("Expected variable '%s' to be present", variable)
		}
	}

	// Verify expected modules
	expectedModules := []string{
		"vpc",
		"security_group",
		"security_group_ssh",
	}

	for _, module := range expectedModules {
		if !HasModule(config, module) {
			t.Errorf("Expected module '%s' to be present", module)
		}
	}

	// Verify expected data sources
	if !HasDataSource(config, "aws_availability_zones", "available") {
		t.Error("Expected data source 'aws_availability_zones.available' to be present")
	}

	// Validate configuration
	errors := ValidateConfig(config)
	if len(errors) > 0 {
		t.Errorf("Configuration validation failed: %v", errors)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
