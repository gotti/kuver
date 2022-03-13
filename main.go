package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/olekukonko/tablewriter"
	"gopkg.in/yaml.v2"
)

var (
	errInvalidKubernetesManifest = errors.New("Invalid kubernetes manifest, no kind and apiVersion")
	errUnknownExtentions         = errors.New("Unknown extentions, skipping")
)

var extentionsToRead = []string{".yaml", ".yml"}

func matchExtentions(filename string) bool {
	for _, e := range extentionsToRead {
		if strings.HasSuffix(filename, e) {
			return true
		}
	}
	return false
}

var rawyamlfiles struct {
	files []string
}

type kubernetesManifest struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
}

func checkIfManifestHaveValidKeys(y string) (bool, error) {
	var m kubernetesManifest
	err := yaml.Unmarshal([]byte(y), &m)
	if err != nil {
		return false, fmt.Errorf("failed to unmarshal kubernetes manifest")
	}
	if m.APIVersion != "" && m.Kind != "" {
		return true, nil
	}
	return false, nil
}

type manifest string

func newManifest(m string) (*manifest, error) {
	if b, err := checkIfManifestHaveValidKeys(m); err != nil {
		return nil, errInvalidKubernetesManifest
	} else if !b {
		return nil, errInvalidKubernetesManifest
	}
	ret := manifest(m)
	return &ret, nil
}
func (m manifest) String() string {
	return string(m)
}

var manifests []*manifest

func openYamlFile(path string) error {
	if !matchExtentions(filepath.Base(path)) {
		return errUnknownExtentions
	}
	f, err := ioutil.ReadFile(path)
	if err != nil {
		log.Printf("opening file: %s", err.Error())
	}
	yamls := strings.Split(string(f), "---")
	for _, y := range yamls {
		m, err := newManifest(y)
		if err == errInvalidKubernetesManifest {
			log.Printf("opening file=%s, err=%s", path, err)
			continue
		} else if err != nil {
			fmt.Println("skipping")
			return fmt.Errorf("opening file=%s, err=%w", path, err)
		}
		manifests = append(manifests, m)
	}
	return nil
}

type versionDiff struct {
	name       string
	currentVer string
	latestVer  string
	deprecated bool
}

func getLatestVersion(versions []string) *version.Version {
	vs := make([]*version.Version, len(versions))
	for i, r := range versions {
		v, _ := version.NewVersion(r)
		vs[i] = v
	}
	sort.Sort(version.Collection(vs))
	return vs[len(vs)-1]
}

var rules = []func(y []*manifest) (vdiff []versionDiff){
	searchHelmRelease,
	findOldImage,
}

func main() {
	r := flag.String("dir", "./", "./path/to/dir/for/searching")
	flag.Parse()

	err := filepath.Walk(*r, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("failed filepath.Walk: %w", err)
		}
		if info.IsDir() {
			return nil
		}
		openYamlFile(path)
		return nil
	})

	if err != nil {
		log.Fatal(err)
	}

	deprecated := false

	var output [][]string

	for _, r := range rules {
		vdiff := r(manifests)
		for _, v := range vdiff {
			if v.deprecated {
				deprecated = true
			}
			fmt.Printf("%s %s %s\n", v.name, v.currentVer, v.latestVer)
			oldText := "latest"
			if v.deprecated {
				oldText = "old"
			}
			output = append(output, []string{oldText, v.name, v.currentVer, v.latestVer})
		}
	}
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"using", "repo/name", "current Version", "Latest Version"})

	for _, v := range output {
		table.Append(v)
	}
	table.Render() // Send output
	if deprecated {
		os.Exit(1)
	}
}
