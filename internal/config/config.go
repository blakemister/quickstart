package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bcmister/quickstart/internal/monitor"
	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Version  int                `yaml:"version"`
	Profiles map[string]Profile `yaml:"profiles"`
}

// Profile represents a quickstart profile
type Profile struct {
	ProjectsDir       string          `yaml:"projectsDir"`
	PostSelectCommand string          `yaml:"postSelectCommand"`
	Monitors          []MonitorConfig `yaml:"monitors"`
}

// MonitorConfig represents configuration for a single monitor
type MonitorConfig struct {
	Name    string `yaml:"name"`
	Windows int    `yaml:"windows"`
	Layout  string `yaml:"layout"` // grid, vertical, horizontal, full
}

// DefaultConfigPath returns the default configuration file path
func DefaultConfigPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".quickstart", "config.yaml")
}

// Load reads the configuration from a file
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}

// Save writes the configuration to a file
func Save(cfg *Config, path string) error {
	if path == "" {
		path = DefaultConfigPath()
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	err = os.WriteFile(path, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// RunInitWizard runs the interactive configuration wizard
func RunInitWizard() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Welcome to Quickstart! Let's set up your development environment.")
	fmt.Println()

	// Get projects directory
	homeDir, _ := os.UserHomeDir()
	defaultProjectsDir := filepath.Join(homeDir, ".1dev")

	// Display user's home folder for easy copying
	fmt.Println("Your home folder:")
	fmt.Printf("  %s\\\n", homeDir)
	fmt.Println()

	fmt.Printf("Where are your projects located? [%s]: ", defaultProjectsDir)
	projectsDir, _ := reader.ReadString('\n')
	projectsDir = strings.TrimSpace(projectsDir)
	if projectsDir == "" {
		projectsDir = defaultProjectsDir
	}

	// Validate projects directory exists
	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		fmt.Printf("Warning: Directory '%s' does not exist. Create it? [Y/n]: ", projectsDir)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer == "" || answer == "y" || answer == "yes" {
			os.MkdirAll(projectsDir, 0755)
			fmt.Println("Directory created.")
		}
	}

	// Get post-select command
	defaultCommand := "claude --dangerously-skip-permissions"
	fmt.Printf("\nWhat command should run after selecting a project?\n[%s]: ", defaultCommand)
	postCommand, _ := reader.ReadString('\n')
	postCommand = strings.TrimSpace(postCommand)
	if postCommand == "" {
		postCommand = defaultCommand
	}

	// Detect monitors
	fmt.Println("\nDetecting monitors...")
	monitors, err := monitor.Detect()
	if err != nil {
		return fmt.Errorf("failed to detect monitors: %w", err)
	}

	fmt.Printf("Found %d monitor(s):\n", len(monitors))
	for i, m := range monitors {
		primary := ""
		if m.Primary {
			primary = " (Primary)"
		}
		fmt.Printf("  %d: %dx%d at (%d, %d)%s\n", i+1, m.Width, m.Height, m.X, m.Y, primary)
	}

	// Configure monitors
	fmt.Println("\nLet's configure how many terminals on each monitor.")

	monitorConfigs := make([]MonitorConfig, 0)
	for i, m := range monitors {
		fmt.Printf("\nMonitor %d (%dx%d):\n", i+1, m.Width, m.Height)

		// Get window count
		fmt.Print("  How many terminal windows? [1]: ")
		windowsStr, _ := reader.ReadString('\n')
		windowsStr = strings.TrimSpace(windowsStr)
		windows := 1
		if windowsStr != "" {
			windows, _ = strconv.Atoi(windowsStr)
			if windows < 1 {
				windows = 1
			}
		}

		// Get layout
		layout := "full"
		if windows > 1 {
			fmt.Print("  Layout (grid/vertical/horizontal) [grid]: ")
			layoutInput, _ := reader.ReadString('\n')
			layout = strings.TrimSpace(strings.ToLower(layoutInput))
			if layout == "" {
				layout = "grid"
			}
		}

		monitorConfigs = append(monitorConfigs, MonitorConfig{
			Name:    fmt.Sprintf("%d", i+1),
			Windows: windows,
			Layout:  layout,
		})
	}

	// Create config
	cfg := &Config{
		Version: 1,
		Profiles: map[string]Profile{
			"default": {
				ProjectsDir:       projectsDir,
				PostSelectCommand: postCommand,
				Monitors:          monitorConfigs,
			},
		},
	}

	// Save config
	configPath := DefaultConfigPath()
	err = Save(cfg, configPath)
	if err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Create scripts directory and picker script
	scriptsDir := filepath.Join(filepath.Dir(configPath), "scripts")
	err = os.MkdirAll(scriptsDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create scripts directory: %w", err)
	}

	err = createPickerScript(scriptsDir)
	if err != nil {
		return fmt.Errorf("failed to create picker script: %w", err)
	}

	fmt.Printf("\nConfiguration saved to: %s\n", configPath)
	fmt.Println("\nYou can now run 'quickstart' to launch your development environment!")
	fmt.Println("\nTip: Make sure you have 'fzf' installed for the project picker.")
	fmt.Println("     Install with: winget install junegunn.fzf")

	return nil
}

func createPickerScript(scriptsDir string) error {
	scriptContent := `# Quickstart Project Picker Script
param(
    [string]$ProjectsDir,
    [string]$PostCommand
)

# Change to projects directory
Set-Location $ProjectsDir

# Get list of project directories
$projects = Get-ChildItem -Directory | Select-Object -ExpandProperty Name

# Check if fzf is available
$fzfPath = Get-Command fzf -ErrorAction SilentlyContinue

if ($fzfPath) {
    # Use fzf for selection
    $selected = $projects | fzf --height=40% --reverse --border --prompt="Select project: "
} else {
    # Fallback to simple menu
    Write-Host "Available projects:" -ForegroundColor Cyan
    Write-Host ""

    for ($i = 0; $i -lt $projects.Count; $i++) {
        Write-Host "  [$($i + 1)] $($projects[$i])"
    }

    Write-Host ""
    $selection = Read-Host "Enter number"

    $index = [int]$selection - 1
    if ($index -ge 0 -and $index -lt $projects.Count) {
        $selected = $projects[$index]
    }
}

if ($selected) {
    $projectPath = Join-Path $ProjectsDir $selected
    Set-Location $projectPath

    Write-Host ""
    Write-Host "Opening: $selected" -ForegroundColor Green
    Write-Host "Running: $PostCommand" -ForegroundColor Yellow
    Write-Host ""

    # Run the post-select command
    Invoke-Expression $PostCommand
} else {
    Write-Host "No project selected." -ForegroundColor Red
}
`

	scriptPath := filepath.Join(scriptsDir, "picker.ps1")
	return os.WriteFile(scriptPath, []byte(scriptContent), 0644)
}
