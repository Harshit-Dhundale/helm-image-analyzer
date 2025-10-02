package kube

import (
	"strings"

	"sigs.k8s.io/yaml"
)


func ExtractImages(rendered string) ([]string, error) {
	if strings.TrimSpace(rendered) == "" {
		return []string{}, nil
	}

	var out []string
	seen := map[string]struct{}{}

	for _, doc := range splitYAMLDocs(rendered) {
		var m map[string]interface{}
		if err := yaml.Unmarshal([]byte(doc), &m); err != nil || m == nil {
			continue
		}

	
		specs := [][]string{
			{"spec"},                                           
			{"spec", "template", "spec"},                      
			{"spec", "jobTemplate", "spec", "template", "spec"},
		}
		for _, p := range specs {
			if spec, ok := getMap(m, p...); ok {
				collectContainers(spec, &out, seen)
			}
		}
	}

	return out, nil
}

func splitYAMLDocs(s string) []string {
	raw := strings.Split(s, "\n---")
	var docs []string
	for _, d := range raw {
		t := strings.TrimSpace(d)
		if t != "" {
			docs = append(docs, t)
		}
	}
	return docs
}

func getMap(m map[string]interface{}, path ...string) (map[string]interface{}, bool) {
	cur := m
	for _, k := range path {
		v, ok := cur[k]
		if !ok {
			return nil, false
		}
		nm, ok := v.(map[string]interface{})
		if !ok {
			return nil, false
		}
		cur = nm
	}
	return cur, true
}

func collectContainers(podSpec map[string]interface{}, out *[]string, seen map[string]struct{}) {
	fields := []string{"containers", "initContainers", "ephemeralContainers"}
	for _, f := range fields {
		if v, ok := podSpec[f]; ok {
			arr, ok := v.([]interface{})
			if !ok {
				continue
			}
			for _, item := range arr {
				cmap, _ := item.(map[string]interface{})
				if cmap == nil {
					continue
				}
				if img, ok := cmap["image"].(string); ok && strings.TrimSpace(img) != "" {
					if _, dup := seen[img]; !dup {
						seen[img] = struct{}{}
						*out = append(*out, img)
					}
				}
			}
		}
	}
}
