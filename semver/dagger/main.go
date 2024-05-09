// Based on https://semver.org/

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ohler55/ojg/jp"
	"github.com/ohler55/ojg/oj"
	"regexp"
	"strconv"
	"strings"
)
import "github.com/antchfx/xmlquery"
import "gobn.github.io/coalesce"

type Semver struct{}

type Version struct {
	Maj        int
	Min        int
	Patch      int
	Prerelease string
	Build      string
}

func (m *Version) Json() (string, error) {
	if m == nil {
		return "", errors.New("cannot get Version")
	}
	//return "Hello", nil
	b, err := json.Marshal(*m)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return string(b), nil
}

func (m *Semver) GetFull(ctx context.Context,
	// Path to the source directory
	src *Directory,

	// min.maj.patch version
	// +optional
	version string,

	// Build version
	// +optional
	build string,
) (string, error) {

	var err error

	if build == "" {
		build, err = m.GetBuild(ctx, src, false, false)
		if err != nil {
			return "", err
		}
	}

	if version == "" {
		version, err = m.DetectVersion(ctx, src)
		if err != nil {
			return "", err
		}
	}

	fullVer := m.ConcatVersion(version, build)
	if m.Validate(fullVer) {
		return fullVer, nil
	} else {
		return "", errors.New(fmt.Sprintf("Invalid SemVer: %s", fullVer))
	}
}

func (m *Semver) GetBuild(ctx context.Context,
	// Path to the source directory
	src *Directory,
	// +optional
	noTs bool,
	// +optional
	noCommit bool,
) (string, error) {
	if noCommit && noTs {
		return "", nil
	}
	return dag.Container().From("alpine:latest").
		WithExec([]string{"apk", "add", "git"}).
		WithMountedDirectory("/src/", src).
		WithWorkdir("/src").
		WithEnvVariable("NO_TS", fmt.Sprintf("%t", noTs)).
		WithEnvVariable("NO_COMMIT", fmt.Sprintf("%t", noCommit)).
		WithExec([]string{"sh", "-c", `
			parts=""
			if [ "${NO_TS}" = "false" ] ; then ts=$(TZ=UTC date '+%Y%m%dT%H%M%S'); parts="${parts} ${ts} " ; fi
			if [ "${NO_COMMIT}" = "false" ] ; then commit=$(git rev-parse --short HEAD); parts="${parts} ${commit} " ; fi
			
			for part in ${parts} ; do ver="${ver:+${ver}-}${part}"; done
			echo "${ver}" | tr -d $'\n' >>/tmp/BUILD
		`}).File("/tmp/BUILD").
		Contents(ctx)
}

func (r *Semver) DetectVersion(ctx context.Context, src *Directory) (string, error) {
	version := coalesce.String(
		r.getVersionFromPomXml(ctx, src),
		r.getVersionFromPackageJson(ctx, src),
	)

	if version != nil {
		return *version, nil
	} else {
		return "", errors.New("Cannot detect version")
	}
}

func (r *Semver) getVersionFromPomXml(ctx context.Context, src *Directory) *string {
	contents, err := src.File("pom.xml").Contents(ctx)
	if err != nil {
		fmt.Println("Cannot find a pom.xml")
		return nil
	}
	root, err := xmlquery.Parse(strings.NewReader(contents))
	if err != nil {
		fmt.Println("Cannot parse pom.xml")
		fmt.Println(err)
		return nil
	}

	version := xmlquery.FindOne(root, "//project/version")
	str := version.InnerText()
	return &str
}

func (r *Semver) getVersionFromPackageJson(ctx context.Context, src *Directory) *string {
	contents, err := src.File("package.json").Contents(ctx)
	if err != nil {
		fmt.Println("Cannot find a package.json")
		return nil
	}
	root, err := oj.ParseString(contents)
	if err != nil {
		fmt.Println("Cannot parse package.json")
		fmt.Println(err)
		return nil
	}

	x, err := jp.ParseString(`$.version`)
	if err != nil {
		fmt.Println("Cannot parse version jsonpath")
		fmt.Println(err)
		return nil
	}

	ver := x.First(root).(string)
	return &ver
}

func (r *Semver) Validate(ver string) bool {
	parsed, err := r.Parse(ver)

	if err != nil {
		fmt.Printf("Version not valid: %s", ver)
		fmt.Println(err)
		return false
	}

	return parsed != nil
}

func (r *Semver) ConcatVersion(version string, build string) string {
	return fmt.Sprintf("%s+%s", version, build)
}

func (r *Semver) Build(
	maj int,
	min int,
	patch int,
	// +optional
	prerelease string,
	// +optional
	build string) string {
	if prerelease != "" {
		prerelease = "-" + prerelease
	}
	if build != "" {
		build = "+" + build
	}
	return fmt.Sprintf("%d.%d.%d%s%s", maj, min, patch, prerelease, build)
}

func (r *Semver) Parse(ver string) (*Version, error) {
	pattern := regexp.MustCompile("^(?P<major>0|[1-9]\\d*)\\.(?P<minor>0|[1-9]\\d*)\\.(?P<patch>0|[1-9]\\d*)(?:-(?P<prerelease>(?:0|[1-9]\\d*|\\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\\.(?:0|[1-9]\\d*|\\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\\+(?P<buildmetadata>[0-9a-zA-Z-]+(?:\\.[0-9a-zA-Z-]+)*))?$")
	groups := extractGroups(pattern, ver)
	if len(groups) == 0 {
		return nil, errors.New(fmt.Sprintf("Cannot parse SemVer in %s", ver))
	}

	fmt.Printf("Found groups: %s.%s.%s-%s+%s\n", groups["major"], groups["minor"], groups["patch"], groups["prerelease"], groups["buildmetadata"])

	maj, err := strconv.Atoi(groups["major"])
	if err != nil {
		fmt.Printf("Cannot extract Major version from %s\n", ver)
		return nil, err
	}
	min, err := strconv.Atoi(groups["minor"])
	if err != nil {
		fmt.Printf("Cannot extract Minor version from %s\n", ver)
		return nil, err
	}
	patch, err := strconv.Atoi(groups["patch"])
	if err != nil {
		fmt.Printf("Cannot extract Patch version from %s\n", ver)
		return nil, err
	}

	return &Version{
		Maj:        maj,
		Min:        min,
		Patch:      patch,
		Prerelease: groups["prerelease"],
		Build:      groups["buildmetadata"],
	}, nil
}

func extractGroups(pattern *regexp.Regexp, line string) map[string]string {
	result := make(map[string]string)
	if pattern.MatchString(line) {
		match := pattern.FindStringSubmatch(line)
		for i, name := range pattern.SubexpNames() {
			if i != 0 && name != "" {
				result[name] = match[i]
			}
		}
	} else {
		fmt.Println("Cannot find any groups")
	}
	return result
}
