package version

import (
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var (
	// VERSION is version of CSI Pangu Driver
	VERSION = ""
	// COMMITID is commit ID of code
	COMMITID = ""
)

var VersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of simon",
	Run: func(cmd *cobra.Command, args []string) {
		pterm.FgLightWhite.Printf("Version: %s\n", VERSION)
		pterm.FgLightWhite.Printf("Commit: %s\n", COMMITID)
	},
}
