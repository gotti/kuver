package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/olekukonko/tablewriter"
	"gopkg.in/yaml.v2"
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

func checkIfManifestHaveValidKeys(y string) bool {
	var m kubernetesManifest
	err := yaml.Unmarshal([]byte(y), &m)
	if err != nil {
		log.Printf("failed to unmarshal kubernetes manifest %s", y)
	}
	if m.APIVersion != "" && m.Kind != "" {
		return true
	}
	return false
}

var manifests []string

func openYamlFile(path string) {
	if !matchExtentions(filepath.Base(path)) {
		return
	}
	f, err := ioutil.ReadFile(path)
	if err != nil {
		log.Printf("opening file: %s", err.Error())
	}
	yamls := strings.Split(string(f), "---")
	for _, y := range yamls {
		if !checkIfManifestHaveValidKeys(y) {
			log.Printf("Invalid kubernetes manifest, skipping... filename=%s\n", path)
			continue
		}
		manifests = append(manifests, y)
	}
}

type versionDiff struct {
	name       string
	currentVer string
	latestVer  string
	deprecated bool
}

func findOldImage(y []string) (vdiff []versionDiff) {
	r := regexp.MustCompile(`(image: \S*)`)
	for _, f := range y {
		res := r.FindAllString(f, -1)
		var imgs []string
		for _, re := range res {
			imgs = append(imgs, strings.Split(re, " ")[1])
		}
		//img = append(img, imgs...)
	}
	return
}

type helmRelease struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
	Spec struct {
		Interval   string `yaml:"interval"`
		ValuesFrom []struct {
			Kind string `yaml:"kind"`
			Name string `yaml:"name"`
		} `yaml:"valuesFrom"`
		TargetNamespace string `yaml:"targetNamespace"`
		Chart           struct {
			Spec struct {
				Chart     string `yaml:"chart"`
				Version   string `yaml:"version"`
				SourceRef struct {
					Kind      string `yaml:"kind"`
					Name      string `yaml:"name"`
					Namespace string `yaml:"namespace"`
				} `yaml:"sourceRef"`
			} `yaml:"spec"`
		} `yaml:"chart"`
	} `yaml:"spec"`
}

type helmRepository struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
	Spec struct {
		URL      string `yaml:"url"`
		Interval string `yaml:"interval"`
	} `yaml:"spec"`
}

type helmIndex struct {
	APIVersion string                      `yaml:"apiVersion"`
	Entries    map[string][]helmIndexEntry `yaml:"entries"`
}

type helmIndexEntry struct {
	APIVersion   string `yaml:"apiVersion"`
	AppVersion   string `yaml:"appVersion"`
	Dependencies []struct {
		Condition  string   `yaml:"condition"`
		Name       string   `yaml:"name"`
		Repository string   `yaml:"repository"`
		Tags       []string `yaml:"tags,omitempty"`
		Version    string   `yaml:"version"`
	} `yaml:"dependencies"`
	Description string   `yaml:"description"`
	Digest      string   `yaml:"digest"`
	Home        string   `yaml:"home"`
	Icon        string   `yaml:"icon"`
	Keywords    []string `yaml:"keywords"`
	Maintainers []struct {
		Email string `yaml:"email"`
		Name  string `yaml:"name"`
	} `yaml:"maintainers"`
	Name    string   `yaml:"name"`
	Sources []string `yaml:"sources"`
	Urls    []string `yaml:"urls"`
	Version string   `yaml:"version"`
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

func searchHelmRelease(y []string) (vdiff []versionDiff) {
	for _, f := range y {
		var hrel helmRelease
		err := yaml.Unmarshal([]byte(f), &hrel)
		if err != nil {
			log.Printf("failed to unmarshal helmRelease %s", err)
		}
		if !(hrel.APIVersion == "helm.toolkit.fluxcd.io/v2beta1" && hrel.Kind == "HelmRelease") {
			continue
		}
		for _, f2 := range y {
			var hrep helmRepository
			err := yaml.Unmarshal([]byte(f2), &hrep)
			if err != nil {
				log.Printf("failed to unmarshal helmRepository %s", err)
			}
			if hrep.APIVersion == "source.toolkit.fluxcd.io/v1beta1" && hrep.Kind == "HelmRepository" && hrep.Metadata.Name == hrel.Spec.Chart.Spec.SourceRef.Name && hrep.Metadata.Namespace == hrel.Spec.Chart.Spec.SourceRef.Namespace && hrep.Kind == hrel.Spec.Chart.Spec.SourceRef.Kind {
				fmt.Println(hrel)
				u, err := url.Parse(hrep.Spec.URL)
				if err != nil {
					log.Printf("failed to parse helm url: url=%s, %s\n", hrep.Spec.URL, err)
				}
				u.Path = path.Join(u.Path, "index.yaml")
				ym, err := http.Get(u.String())
				if err != nil {
					log.Printf("failed to fetch helm repository: url=%s, %s\n", hrep.Spec.URL, err)
				}
				defer ym.Body.Close()
				b, err := io.ReadAll(ym.Body)
				if err != nil {
					log.Printf("failed to load helm repository body: url=%s, %s\n", hrep.Spec.URL, err)
				}
				var helmIdx helmIndex
				if err := yaml.Unmarshal(b, &helmIdx); err != nil {
					log.Printf("failed to parse helm repository index yaml: url=%s, %s", hrep.Spec.URL, err)
				}
				entries := helmIdx.Entries[hrel.Spec.Chart.Spec.Chart]
				var vs []string
				for _, e := range entries {
					vs = append(vs, e.Version)
				}
				latest := getLatestVersion(vs)
				con, err := version.NewConstraint(hrel.Spec.Chart.Spec.Version)
				if err != nil {
					log.Printf("failedt to parse version: v=%s, %s", hrel.Spec.Chart.Spec.Version, err)
				}
				vd := versionDiff{name: hrel.Spec.Chart.Spec.Chart, currentVer: con.String(), latestVer: latest.String()}
				if !con.Check(latest) {
					log.Printf("using older version of %s: current=%s, latest=%s\n", hrel.Spec.Chart.Spec.Chart, hrel.Spec.Chart.Spec.Version, latest.String())
					vd.deprecated = true
				}
				vdiff = append(vdiff, vd)
			}
		}
	}
	return vdiff
}

var rules = []func(y []string) (vdiff []versionDiff){
	searchHelmRelease,
	//  findOldImage,
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
