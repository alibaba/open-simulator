package doc

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

type GenerateDoc struct {
	DocCmd    *cobra.Command
	outputDir string
}

var GenDoc = &GenerateDoc{}

func init() {
	GenDoc.DocCmd = &cobra.Command{
		Use:           "gen-doc",
		Short:         "Generate markdown document for your project",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return GenDoc.generateDocument()
		},
	}

	GenDoc.DocCmd.Flags().StringVarP(&GenDoc.outputDir, "output-directory", "d", "./docs/commandline", "assign a directory to store documents")
}

func (c *GenerateDoc) generateDocument() error {
	if _, err := os.Stat(c.outputDir); err != nil {
		return fmt.Errorf("Invalid output directory(%s) ", c.outputDir)
	}
	return doc.GenMarkdownTree(c.DocCmd.Parent(), c.outputDir)
}
