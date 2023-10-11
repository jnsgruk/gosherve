package cmd

import (
	"fmt"
	"os"

	"github.com/jnsgruk/gosherve/internal/logging"
	"github.com/jnsgruk/gosherve/internal/manager"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "gosherve",
	Short: "gosherve - file server & URL shortening/redirect service",
	Long: `
A simple HTTP file server with some basic URL shortening/redirect functionality

This project is a simple web server written in Go that will:

	- Serve files from a specified directory
	- Serve redirects specified in a file hosted at a URL

The only configuration necessary to start gosherve is set through the
'GOSHERVE_REDIRECT_MAP_URL' environment variable, which must point to
a url containing alias/URL pairs. For example:

		github https://github.com/jnsgruk
		linkedin https://linkedin.com/in/jnsgruk
		something https://somelink.com
		wow https://www.ohmygoodness.com

For more information, visit the homepage at: https://github.com/jnsgruk/gosherve
	`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := logging.GetRootLogger()

		if viper.GetString("redirect_map_url") == "" {
			logger.Error("GOSHERVE_REDIRECT_MAP_URL environment variable not set")
			os.Exit(1)
		}

		mgr := manager.NewGosherveManager(logger)
		logger.Info("gosherve manager initialised; fetching redirects")

		// Hydrate the redirects map
		err := mgr.RefreshRedirects()
		if err != nil {
			// Since this is the first hydration, exit if unable to fetch redirects.
			// At this point, without the redirects to begin with the server is
			// quite useless.
			logger.Error("error fetching redirect map")
			os.Exit(1)
		}

		logger.Info(fmt.Sprintf("fetched %d redirects, starting server", len(mgr.Redirects)))
		mgr.Start()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute(version string) {
	rootCmd.Version = version
	rootCmd.SetVersionTemplate("gosherve\nversion: {{.Version}}")

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// All env vars should be prefixed GOSHERVE_
	viper.SetEnvPrefix("gosherve")
	// Configure viper to look for the right env vars
	viper.MustBindEnv("redirect_map_url")
	viper.BindEnv("webroot")
	viper.BindEnv("log_level")
}
