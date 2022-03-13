package main

import (
	"errors"
	"regexp"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/go-version"
)

var (
	errInvalidDockerTag = errors.New("invalid docker url, should be hogehoge:huga")
	errTagIsNotSemver   = errors.New("tag is not semver")
)

func findOldImage(y []*manifest) (vdiff []versionDiff) {
	r := regexp.MustCompile(`(image: \S*)`)
	for _, f := range y {
		res := r.FindAllString(f.String(), -1)
		for _, re := range res {
			img := strings.Split(re, " ")[1]
			tag := strings.Split(img, ":")
			if len(tag) != 2 {
				panic("invalid")
			}
			err := ifIsDockerTagSemVer(tag[1])
			if err == errTagIsNotSemver {
				continue
			} else if err != nil {
				//TODO
				panic(err)
			}
			latest := getLatestVersion(fetchDockerImageVersions(img))
			c, err := version.NewConstraint(tag[1])
			if err != nil {
				panic(err)
			}
			vd := versionDiff{
				detector:   "docker image",
				name:       img,
				currentVer: c.String(),
				latestVer:  latest.String(),
			}
			if !c.Check(latest) {
				vd.deprecated = true
			}
			vdiff = append(vdiff, vd)
		}
	}
	return vdiff
}

func ifIsDockerTagSemVer(tag string) error {
	_, err := version.NewSemver(tag)
	if err != nil {
		return errTagIsNotSemver
	}
	return nil
}

func fetchDockerImageVersions(dockerurl string) []string {
	ref, err := name.ParseReference(dockerurl)
	if err != nil {
		panic(err)
	}
	tags, err := remote.List(ref.Context())
	if err != nil {
		panic(err)
	}
	return tags
}
