package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-exec/tfexec"
)

// TerraformConfig holds parsed configuration data
type TerraformConfig struct {
	Providers  map[string]interface{}
	Resources  map[string]interface{}
	Variables  map[string]interface{}
	Modules    map[string]interface{}
	DataSources map[string]interface{}
	Locals     map[string]interface{}
}

// ParseTFFiles parses all .tf files in a directory and extracts configuration
func ParseTFFiles(dir string) (*TerraformConfig, error) {
	config := &TerraformConfig{
		Providers:   make(map[string]interface{}),
		Resources:   make(map[string]interface{}),
		Variables:   make(map[string]interface{}),
		Modules:     make(map[string]interface{}),
		DataSources: make(map[string]interface{}),
		Locals:      make(map[string]interface{}),
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.tf"))
	if err != nil {
		return nil, fmt.Errorf("failed to glob tf files: %w", err)
	}

	for _, file := range files {
		if err := parseFile(file, config); err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", file, err)
		}
	}

	return config, nil
}

func parseFile(filename string, config *TerraformConfig) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	file, diags := hclsyntax.ParseConfig(data, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return fmt.Errorf("failed to parse HCL: %v", diags.Error())
	}

	if file == nil {
		return fmt.Errorf("failed to parse HCL file")
	}

	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return fmt.Errorf("unexpected body type")
	}

	for _, block := range body.Blocks {
		switch block.Type {
		case "provider":
			if len(block.Labels) > 0 {
				name := block.Labels[0]
				config.Providers[name] = block
			}
		case "resource":
			if len(block.Labels) >= 2 {
				key := fmt.Sprintf("%s.%s", block.Labels[0], block.Labels[1])
				config.Resources[key] = block
			}
		case "variable":
			if len(block.Labels) >= 1 {
				config.Variables[block.Labels[0]] = block
			}
		case "module":
			if len(block.Labels) >= 1 {
				config.Modules[block.Labels[0]] = block
			}
		case "data":
			if len(block.Labels) >= 2 {
				key := fmt.Sprintf("%s.%s", block.Labels[0], block.Labels[1])
				config.DataSources[key] = block
			}
		case "locals":
			config.Locals["locals"] = block
		case "terraform":
			// Handle terraform block separately if needed
			config.Locals["terraform_backend"] = block
		}
	}

	return nil
}

// ValidateConfig checks if the configuration has required elements
func ValidateConfig(config *TerraformConfig) []string {
	var errors []string

	if len(config.Providers) == 0 {
		errors = append(errors, "no providers defined")
	}

	if len(config.Resources) == 0 {
		errors = append(errors, "no resources defined")
	}

	return errors
}

// GetResourceCount returns the number of resources by type
func GetResourceCount(config *TerraformConfig, resourceType string) int {
	count := 0
	prefix := resourceType + "."
	for key := range config.Resources {
		if strings.HasPrefix(key, prefix) {
			count++
		}
	}
	return count
}

// HasVariable checks if a variable is defined
func HasVariable(config *TerraformConfig, varName string) bool {
	_, exists := config.Variables[varName]
	return exists
}

// HasModule checks if a module is defined
func HasModule(config *TerraformConfig, moduleName string) bool {
	_, exists := config.Modules[moduleName]
	return exists
}

// HasDataSource checks if a data source is defined
func HasDataSource(config *TerraformConfig, dataSourceType, name string) bool {
	key := fmt.Sprintf("%s.%s", dataSourceType, name)
	_, exists := config.DataSources[key]
	return exists
}

// InitializeTerraform initializes a new Terraform working directory
func InitializeTerraform(workingDir string, execPath string) (*tfexec.Terraform, error) {
	// If execPath is empty, use default
	if execPath == "" {
		execPath = "tofu" // OpenTofu binary
	}

	// Check if binary exists
	if _, err := os.Stat(execPath); os.IsNotExist(err) {
		// Try alternative names
		alternatives := []string{"terraform", "/usr/local/bin/tofu", "/usr/local/bin/terraform"}
		found := false
		for _, alt := range alternatives {
			if _, err := os.Stat(alt); err == nil {
				execPath = alt
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("neither tofu nor terraform binary found")
		}
	}

	tf, err := tfexec.NewTerraform(workingDir, execPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create Terraform instance: %w", err)
	}

	return tf, nil
}

// RunValidate runs terraform/tofu validate command
func RunValidate(ctx context.Context, tf *tfexec.Terraform) error {
	_, err := tf.Validate(ctx)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	return nil
}

// RunFormatCheck checks if files are properly formatted
func RunFormatCheck(ctx context.Context, tf *tfexec.Terraform) (bool, error) {
	// Note: FormatIsChanged may not be available in older versions
	// This is a placeholder for format checking functionality
	// In practice, you would use: return tf.FormatIsChanged(ctx)
	return true, nil
}

func main() {
	// Example usage
	dir := "."
	config, err := ParseTFFiles(dir)
	if err != nil {
		fmt.Printf("Error parsing config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d providers\n", len(config.Providers))
	fmt.Printf("Found %d resources\n", len(config.Resources))
	fmt.Printf("Found %d variables\n", len(config.Variables))
	fmt.Printf("Found %d modules\n", len(config.Modules))
	fmt.Printf("Found %d data sources\n", len(config.DataSources))

	errors := ValidateConfig(config)
	if len(errors) > 0 {
		fmt.Println("Validation errors:")
		for _, e := range errors {
			fmt.Printf("  - %s\n", e)
		}
	} else {
		fmt.Println("Configuration validation passed!")
	}
}
