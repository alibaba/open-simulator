package main

import (
	"fmt"
	"os"

	goflag "flag"

	"github.com/alibaba/open-simulator/cmd/apply"
	"github.com/alibaba/open-simulator/cmd/version"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	cliflag "k8s.io/component-base/cli/flag"
)

const (
	EnvLogLevel = "LogLevel"
	LogPanic    = "Panic"
	LogFatal    = "Fatal"
	LogError    = "Error"
	LogWarn     = "Warn"
	LogInfo     = "Info"
	LogDebug    = "Debug"
	LogTrace    = "Trace"
)

var SimonCmd = &cobra.Command{
	Use:   "simon",
	Short: "Simon is a simulator, which will simulate a cluster and simulate workload scheduling.",
}

func main() {
	if err := SimonCmd.Execute(); err != nil {
		fmt.Printf("start with error: %s", err.Error())
		os.Exit(1)
	}
}

func init() {
	addCommands()
	SimonCmd.SetGlobalNormalizationFunc(cliflag.WordSepNormalizeFunc)
	SimonCmd.Flags().AddGoFlagSet(goflag.CommandLine)
	logLevel := os.Getenv(EnvLogLevel)
	switch logLevel {
	case LogPanic:
		log.SetLevel(log.PanicLevel)
	case LogFatal:
		log.SetLevel(log.FatalLevel)
	case LogError:
		log.SetLevel(log.ErrorLevel)
	case LogWarn:
		log.SetLevel(log.WarnLevel)
	case LogInfo:
		log.SetLevel(log.InfoLevel)
	case LogDebug:
		log.SetLevel(log.DebugLevel)
	case LogTrace:
		log.SetLevel(log.TraceLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}
}

func addCommands() {
	SimonCmd.AddCommand(
		version.VersionCmd,
		apply.ApplyCmd,
	)
}
