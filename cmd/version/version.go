package version

import (
	"fmt"

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
		fmt.Printf("Version: %s\n", VERSION)
		fmt.Printf("Commit: %s\n", COMMITID)
	},
}
