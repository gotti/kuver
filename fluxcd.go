package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"

	"github.com/hashicorp/go-version"
	"gopkg.in/yaml.v2"
)

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

var (
	errNotFoundHelmRepository = errors.New("Not found such helm repository")
)

func searchHelmRelease(y []*manifest) (vdiff []versionDiff) {
	for _, f := range y {
		var hrel helmRelease
		err := yaml.Unmarshal([]byte(f.String()), &hrel)
		if err != nil {
			log.Printf("failed to unmarshal helmRelease %s\n", err)
		}
		if !(hrel.APIVersion == "helm.toolkit.fluxcd.io/v2beta1" && hrel.Kind == "HelmRelease") {
			continue
		}
		hrep, err := searchHelmRepository(y, hrel)
		if err != nil {
			log.Printf("error searching helmrepository=%s, err=%s\n", hrel.Metadata.Name, err)
			continue
		}
		vs, err := fetchHelmVersions(hrep.Spec.URL, hrel.Spec.Chart.Spec.Chart)
		if err != nil {
			log.Printf("error fetching versions from repository url=%s, err=%s", hrep.Spec.URL, err)
		}
		latest := getLatestVersion(vs)
		con, err := version.NewConstraint(hrel.Spec.Chart.Spec.Version)
		if err != nil {
			log.Printf("failed to parse version: v=%s, %s", hrel.Spec.Chart.Spec.Version, err)
		}
		vd := versionDiff{detector: "helm(fluxcd)", name: hrel.Spec.Chart.Spec.Chart, currentVer: con.String(), latestVer: latest.String()}
		if !con.Check(latest) {
			log.Printf("using older version of %s: current=%s, latest=%s\n", hrel.Spec.Chart.Spec.Chart, hrel.Spec.Chart.Spec.Version, latest.String())
			vd.deprecated = true
		}
		vdiff = append(vdiff, vd)
	}
	return vdiff
}

func searchHelmRepository(y []*manifest, h helmRelease) (*helmRepository, error) {
	for _, f := range y {
		var hrep helmRepository
		if err := yaml.Unmarshal([]byte(f.String()), &hrep); err != nil {
			return nil, fmt.Errorf("failed to unmarshal helmRepository %w", err)
		}
		if hrep.APIVersion == "source.toolkit.fluxcd.io/v1beta1" && hrep.Kind == "HelmRepository" && hrep.Metadata.Name == h.Spec.Chart.Spec.SourceRef.Name && hrep.Metadata.Namespace == h.Spec.Chart.Spec.SourceRef.Namespace && hrep.Kind == h.Spec.Chart.Spec.SourceRef.Kind {
			return &hrep, nil
		}
	}
	return nil, errNotFoundHelmRepository
}

func fetchHelmVersions(helmurl, chart string) ([]string, error) {
	u, err := url.Parse(helmurl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse helm url: url=%s, %w", helmurl, err)
	}
	u.Path = path.Join(u.Path, "index.yaml")
	ym, err := http.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch helm repository: url=%s, %w", helmurl, err)
	}
	defer ym.Body.Close()
	b, err := io.ReadAll(ym.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to load helm repository body: url=%s, %w", helmurl, err)
	}
	var helmIdx helmIndex
	if err := yaml.Unmarshal(b, &helmIdx); err != nil {
		return nil, fmt.Errorf("failed to parse helm repository index yaml: url=%s, %w", helmurl, err)
	}
	entries := helmIdx.Entries[chart]
	var vs []string
	for _, e := range entries {
		vs = append(vs, e.Version)
	}
	return vs, nil
}
