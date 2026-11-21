//go:build ignore

// Generate the website download assets from their CLI-owned source files.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

func main() {
	if len(os.Args) != 2 {
		panic("usage: go run scripts/site-assets.go <website-public-directory>")
	}
	root := filepath.Join(os.Args[1], "cli")
	releaseBytes, err := os.ReadFile("release.json")
	if err != nil {
		panic(err)
	}
	var release struct {
		Skill struct {
			Version    string `json:"version"`
			MinimumCLI string `json:"minimum_cli"`
		} `json:"skill"`
	}
	if json.Unmarshal(releaseBytes, &release) != nil || !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(release.Skill.Version) || !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(release.Skill.MinimumCLI) {
		panic("invalid release metadata")
	}
	skillDir := filepath.Join(root, "skills", release.Skill.Version)
	if e := os.MkdirAll(skillDir, 0755); e != nil {
		panic(e)
	}
	for _, name := range []string{"install.sh", "install.ps1"} {
		b, e := os.ReadFile(filepath.Join("scripts", name))
		if e != nil {
			panic(e)
		}
		if e = os.WriteFile(filepath.Join(root, name), b, 0644); e != nil {
			panic(e)
		}
	}
	b, e := os.ReadFile("skills/inkspect/SKILL.md")
	if e != nil {
		panic(e)
	}
	if e = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), b, 0644); e != nil {
		panic(e)
	}
	m := map[string]any{"version": release.Skill.Version, "minimum_cli": release.Skill.MinimumCLI, "files": map[string]string{"SKILL.md": fmt.Sprintf("%x", sha256.Sum256(b))}}
	b, e = json.MarshalIndent(m, "", "  ")
	if e != nil {
		panic(e)
	}
	if e = os.WriteFile(filepath.Join(skillDir, "manifest.json"), append(b, '\n'), 0644); e != nil {
		panic(e)
	}
	if e = os.WriteFile(filepath.Join(root, "releases.json"), releaseBytes, 0644); e != nil {
		panic(e)
	}
	fmt.Println("Website installers and skill manifest generated.")
}
