package cmd

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/adrg/xdg"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pelletier/go-toml/v2"
	"github.com/urfave/cli/v2"
	varibale "github.com/yorukot/superfile/src/config"
	internal "github.com/yorukot/superfile/src/internal"
)

func Run(content embed.FS) {

	internal.LoadAllDefaultConfig(content)

	app := &cli.App{
		Name:        "superfile",
		Version:     varibale.CurrentVersion,
		Description: "Pretty fancy and modern terminal file manager ",
		ArgsUsage:   "[path]",
		Commands: []*cli.Command{
			{
				Name:    "path-list",
				Aliases: []string{"pl"},
				Usage:   "Print the path to the configuration and directory",
				Action: func(c *cli.Context) error {
					fmt.Printf("%-*s %s\n", 55, lipgloss.NewStyle().Foreground(lipgloss.Color("#66b2ff")).Render("[Configuration file path]"), varibale.ConfigFilea)
					fmt.Printf("%-*s %s\n", 55, lipgloss.NewStyle().Foreground(lipgloss.Color("#ffcc66")).Render("[Hotkeys file path]"), varibale.HotkeysFilea)
					fmt.Printf("%-*s %s\n", 55, lipgloss.NewStyle().Foreground(lipgloss.Color("#66ff66")).Render("[Log file path]"), varibale.LogFilea)
					fmt.Printf("%-*s %s\n", 55, lipgloss.NewStyle().Foreground(lipgloss.Color("#ff9999")).Render("[Configuration directory path]"), varibale.SuperFileMainDir)
					fmt.Printf("%-*s %s\n", 55, lipgloss.NewStyle().Foreground(lipgloss.Color("#ff66ff")).Render("[Data directory path]"), varibale.SuperFileDataDir)
					return nil
				},
			},
		},
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "fix-hotkeys",
				Aliases: []string{"fh"},
				Usage:   "Adds any missing hotkeys to the hotkey config file",
				Value:   false,
			},
		},
		Action: func(c *cli.Context) error {
			path := ""
			if c.Args().Present() {
				path = c.Args().First()
			}

			InitConfigFile()

			varibale.FixHotkeys = c.Bool("fix-hotkeys")

			firstUse := checkFirstUse()

			p := tea.NewProgram(internal.InitialModel(path, firstUse), tea.WithAltScreen(), tea.WithMouseCellMotion())
			if _, err := p.Run(); err != nil {
				log.Fatalf("Alas, there's been an error: %v", err)
				os.Exit(1)
			}

			CheckForUpdates()
			return nil
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatalln(err)
	}
}

func InitConfigFile() {
	// Create directories
	if err := createDirectories(
		varibale.SuperFileMainDir,
		varibale.SuperFileDataDir,
		varibale.SuperFileStateDir,
		varibale.ThemeFoldera,
	); err != nil {
		log.Fatalln("Error creating directories:", err)
	}

	// Create trash directories
	if runtime.GOOS != "darwin" {
		if err := createDirectories(
			xdg.DataHome+varibale.TrashDirectory,
			xdg.DataHome+varibale.TrashDirectoryFiles,
			xdg.DataHome+varibale.TrashDirectoryInfo,
		); err != nil {
			log.Fatalln("Error creating directories:", err)
		}
	}

	// Create files
	if err := createFiles(
		varibale.PinnedFilea,
		varibale.ToggleDotFilea,
		varibale.LogFilea,
		varibale.ThemeFileVersiona,
	); err != nil {
		log.Fatalln("Error creating files:", err)
	}

	// Write config file
	if err := writeConfigFile(varibale.ConfigFilea, internal.ConfigTomlString); err != nil {
		log.Fatalln("Error writing config file:", err)
	}

	if err := writeConfigFile(varibale.HotkeysFilea, internal.HotkeysTomlString); err != nil {
		log.Fatalln("Error writing config file:", err)
	}
}

// Helper functions
func createDirectories(dirs ...string) error {
	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			// Directory doesn't exist, create it
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
		} else if err != nil {
			// Some other error occurred while checking if the directory exists
			return fmt.Errorf("failed to check directory status %s: %w", dir, err)
		}
		// else: directory already exists
	}
	return nil
}

func createFiles(files ...string) error {
	for _, file := range files {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			if err := os.WriteFile(file, nil, 0644); err != nil {
				return fmt.Errorf("failed to create file %s: %w", file, err)
			}
		}
	}
	return nil
}

func checkFirstUse() bool {
	file := varibale.FirstUseChecka
	firstUse := false
	if _, err := os.Stat(file); os.IsNotExist(err) {
		firstUse = true
		if err := os.WriteFile(file, nil, 0644); err != nil {
			log.Fatalln("failed to create file: %w", err)
		}
	}
	return firstUse
}
func writeConfigFile(path, data string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			return fmt.Errorf("failed to write config file %s: %w", path, err)
		}
	}
	return nil
}

func CheckForUpdates() {
	var Config internal.ConfigType

	data, err := os.ReadFile(varibale.ConfigFilea)
	if err != nil {
		log.Fatalf("Config file doesn't exist: %v", err)
	}

	err = toml.Unmarshal(data, &Config)
	if err != nil {
		log.Fatalf("Error decoding config file ( your config file may be misconfigured ): %v", err)
	}

	if !Config.AutoCheckUpdate {
		return
	}

	lastTime, err := readLastTimeCheckVersionFromFile(varibale.LastCheckVersiona)
	if err != nil && !os.IsNotExist(err) {
		fmt.Println("Error reading from file:", err)
		return
	}

	currentTime := time.Now()

	if lastTime.IsZero() || currentTime.Sub(lastTime) >= 24*time.Hour {
		resp, err := http.Get(varibale.LatestVersionURL)
		if err != nil {
			fmt.Println("Error checking for updates:", err)
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return
		}

		type GitHubRelease struct {
			TagName string `json:"tag_name"`
		}

		var release GitHubRelease
		if err := json.Unmarshal(body, &release); err != nil {
			return
		}

		if versionToNumber(release.TagName) > versionToNumber(varibale.CurrentVersion) {
			fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF69E1")).Render("┃ ") +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#FFBA52")).Bold(true).Render("A new version ") + 
			lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFF2")).Bold(true).Italic(true).Render(release.TagName) + 
			lipgloss.NewStyle().Foreground(lipgloss.Color("#FFBA52")).Bold(true).Render(" is available."))
			
			fmt.Printf(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF69E1")).Render("┃ ") + "Please update.\n┏\n\n      => %s\n\n", varibale.LatestVersionGithub)
			fmt.Printf("                                                               ┛\n")
		}

		timeStr := currentTime.Format(time.RFC3339)
		err = writeToFile(varibale.LastCheckVersiona, timeStr)
		if err != nil {
			log.Println("Error writing to file:", err)
			return
		}
	}
}

func versionToNumber(version string) int {
	version = strings.ReplaceAll(version, "v", "")
	version = strings.ReplaceAll(version, ".", "")

	num, _ := strconv.Atoi(version)
	return num
}

func readLastTimeCheckVersionFromFile(filename string) (time.Time, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return time.Time{}, err
	}
	if len(content) == 0 {
		return time.Time{}, nil
	}
	lastTime, err := time.Parse(time.RFC3339, string(content))
	if err != nil {
		return time.Time{}, err
	}

	return lastTime, nil
}

func writeToFile(filename, content string) error {
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(content)
	if err != nil {
		return err
	}

	return nil
}
