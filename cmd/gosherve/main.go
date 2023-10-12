package main

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"

	"github.com/jnsgruk/gosherve/internal/logging"
	"github.com/jnsgruk/gosherve/internal/server"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	version string = "dev"
	commit  string = "dev"
	date    string
)

var shortDesc = "A file server & URL shortening/redirect service"
var longDesc string = `A file server & URL shortening/redirect service

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
`

var rootCmd = &cobra.Command{
	Use:           "gosherve",
	Version:       buildVersion(version, commit, date),
	Short:         shortDesc,
	Long:          longDesc,
	SilenceErrors: false,
	SilenceUsage:  true,

	RunE: func(cmd *cobra.Command, args []string) error {
		// Create a default slog logger with the correct handlers
		logging.SetupLogger(viper.GetString("log_level"))

		webroot := viper.GetString("webroot")
		redirect_map_url := viper.GetString("redirect_map_url")

		if redirect_map_url == "" {
			// Application cannot function without a redirect map url.
			return fmt.Errorf("GOSHERVE_REDIRECT_MAP_URL environment variable not set")
		}

		// Instantiate a new Gosherve server
		s := server.NewServer(webroot, redirect_map_url)
		slog.Info("gosherve", "version", version, "commit", commit, "build_date", date)

		// Hydrate the redirects map
		err := s.RefreshRedirects()
		if err != nil {
			// Since this is the first hydration, exit if unable to fetch redirects.
			// At this point, without the redirects to begin with the server is
			// quite useless.
			return fmt.Errorf("error fetching redirect map")
		}

		slog.Info(fmt.Sprintf("fetched %d redirects, starting server", s.NumRedirects()))
		s.Start()
		return nil
	},
}

// buildVersion writes a multiline version string from the specified
// version variables
func buildVersion(version, commit, date string) string {
	result := version
	if commit != "" {
		result = fmt.Sprintf("%s\ncommit: %s", result, commit)
	}
	if date != "" {
		result = fmt.Sprintf("%s\nbuilt at: %s", result, date)
	}
	result = fmt.Sprintf("%s\ngoos: %s\ngoarch: %s", result, runtime.GOOS, runtime.GOARCH)
	return result
}

func main() {
	viper.SetEnvPrefix("gosherve")
	viper.MustBindEnv("redirect_map_url")
	viper.BindEnv("webroot")
	viper.BindEnv("log_level")

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
