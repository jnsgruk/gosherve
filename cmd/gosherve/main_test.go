package main

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestInvalidWebrootFailsBeforeFetchingRedirects(t *testing.T) {
	previousLogger := slog.Default()
	previousWebroot := viper.Get("webroot")
	previousRedirects := viper.Get("redirect_map_url")
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
		viper.Set("webroot", previousWebroot)
		viper.Set("redirect_map_url", previousRedirects)
	})
	viper.Set("webroot", filepath.Join(t.TempDir(), "missing"))
	viper.Set("redirect_map_url", "http://invalid.example/redirects")

	err := rootCmd.RunE(rootCmd, nil)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("got error %v, want a missing webroot error", err)
	}
}
