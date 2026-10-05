package scaffold

import (
	"path/filepath"
	"runtime/debug"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/lint"
)

// Framework is the framework an app is created against: a released Version, or a local
// checkout Dir.
type Framework struct{ Version, Dir string }

// unreleased is the version a go.mod requires next to a replace line to a checkout.
const unreleased = "v0.0.0-00010101000000-000000000000"

// Detect returns the framework this aicoded was built with: the version in its build info,
// or checkout when it was installed from one. It fails with E-CLI-003 when it knows neither.
func Detect(checkout string) (Framework, error) {
	info, _ := debug.ReadBuildInfo()
	if v := released(info); v != "" {
		return Framework{Version: v}, nil
	}
	if checkout == "" {
		return Framework{}, noFramework()
	}
	dir, err := filepath.Abs(checkout)
	if err != nil {
		return Framework{}, err
	}
	return Framework{Dir: dir}, nil
}

// released returns the version of the framework in info when it is a released one: not
// replaced, and neither the version of a checkout nor (devel).
func released(info *debug.BuildInfo) string {
	if info == nil {
		return ""
	}
	for _, d := range info.Deps {
		if d.Path == lint.Framework && d.Replace == nil && d.Version != unreleased && d.Version != "(devel)" {
			return d.Version
		}
	}
	return ""
}

func noFramework() error {
	return errs.New("E-CLI-003", "this aicoded knows no framework version to create apps with",
		"install a released aicoded, or run make install in the framework checkout")
}
