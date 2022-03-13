package main

import (
	"regexp"
	"strings"
)

func findOldImage(y []*manifest) (vdiff []versionDiff) {
	r := regexp.MustCompile(`(image: \S*)`)
	for _, f := range y {
		res := r.FindAllString(f.String(), -1)
		var imgs []string
		for _, re := range res {
			imgs = append(imgs, strings.Split(re, " ")[1])
		}
		//img = append(img, imgs...)
	}
	return
}
