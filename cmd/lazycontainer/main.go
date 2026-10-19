// Command lazycontainer is a lazydocker-style TUI for Apple's container CLI.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/owenrumney/container-tui/internal/container"
	"github.com/owenrumney/container-tui/internal/ui"
)

var version = "dev"

func main() {
	root := &cobra.Command{
		Use:           "lazycontainer",
		Short:         "A lazydocker-style TUI for Apple's container CLI",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(*cobra.Command, []string) error {
			client, err := container.New()
			if err != nil {
				return err
			}
			p := tea.NewProgram(ui.New(client), tea.WithAltScreen(), tea.WithMouseCellMotion())
			if _, err := p.Run(); err != nil {
				return fmt.Errorf("run tui: %w", err)
			}
			return nil
		},
	}
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
