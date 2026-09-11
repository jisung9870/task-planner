package cli

import (
	"encoding/json"
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// Build metadata, stamped by the linker (see scripts/install.sh).
//
// Version falls back to the module's own build info so a binary produced by a
// plain `go build` or `go install` still reports something truthful rather than
// claiming to be a release.
var (
	Version   = ""
	Commit    = ""
	BuildDate = ""
)

// BuildInfo is what `tp version --json` emits. Machine-readable output exists so
// the install script can identify a binary before replacing or deleting it.
type BuildInfo struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	BuildDate string `json:"build_date,omitempty"`
	Go        string `json:"go"`
	Platform  string `json:"platform"`
}

func buildInfo() BuildInfo {
	version, commit := Version, Commit
	if version == "" || commit == "" {
		// go build / go install stamps VCS data into the binary; use it rather
		// than reporting an empty or invented version.
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, s := range bi.Settings {
				switch s.Key {
				case "vcs.revision":
					if commit == "" && len(s.Value) >= 7 {
						commit = s.Value[:7]
					}
				case "vcs.modified":
					if s.Value == "true" && commit != "" {
						commit += "-dirty"
					}
				}
			}
		}
	}
	if version == "" {
		if commit != "" {
			version = commit
		} else {
			version = "dev"
		}
	}
	return BuildInfo{
		Name:      "task-planner",
		Version:   version,
		Commit:    commit,
		BuildDate: BuildDate,
		Go:        runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
	}
}

// VersionString is the one-line form used by `tp --version`.
func VersionString() string {
	bi := buildInfo()
	s := bi.Version
	if bi.Commit != "" && bi.Commit != bi.Version {
		s += " (" + bi.Commit + ")"
	}
	return s
}

func newVersionCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "버전 및 빌드 정보",
		RunE: func(cmd *cobra.Command, args []string) error {
			bi := buildInfo()
			if asJSON {
				raw, err := json.Marshal(bi)
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "task-planner %s\n", bi.Version)
			if bi.Commit != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  commit    %s\n", bi.Commit)
			}
			if bi.BuildDate != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  built     %s\n", bi.BuildDate)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  go        %s\n  platform  %s\n", bi.Go, bi.Platform)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "기계 판독용 JSON 출력 (설치 스크립트가 사용)")
	return cmd
}
