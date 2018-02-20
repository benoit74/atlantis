package events

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/logging"
)

//go:generate pegomock generate -m --use-experimental-model-gen --package mocks -o mocks/mock_project_finder.go ProjectFinder

// ProjectFinder determines what are the terraform project(s) within a repo.
type ProjectFinder interface {
	// DetermineProjects returns the list of projects that were modified based on
	// the modifiedFiles. The list will be de-duplicated.
	DetermineProjects(log *logging.SimpleLogger, modifiedFiles []string, repoFullName string, repoDir string) []models.Project
}

// DefaultProjectFinder implements ProjectFinder.
type DefaultProjectFinder struct{}

var excludeList = []string{"terraform.tfstate", "terraform.tfstate.backup"}

// DetermineProjects returns the list of projects that were modified based on
// the modifiedFiles. The list will be de-duplicated.
func (p *DefaultProjectFinder) DetermineProjects(log *logging.SimpleLogger, modifiedFiles []string, repoFullName string, repoDir string) []models.Project {
	var projects []models.Project

	modifiedTerraformFiles := p.filterToTerraform(modifiedFiles)
	if len(modifiedTerraformFiles) == 0 {
		return projects
	}
	log.Info("filtered modified files to %d .tf files: %v",
		len(modifiedTerraformFiles), modifiedTerraformFiles)

	var paths []string
	for _, modifiedFile := range modifiedTerraformFiles {
		projectPath := p.getProjectPath(modifiedFile, repoDir)
		if projectPath != "" {
			paths = append(paths, projectPath)
		}
	}
	uniquePaths := p.unique(paths)
	for _, uniquePath := range uniquePaths {
		projects = append(projects, models.NewProject(repoFullName, uniquePath))
	}
	log.Info("there are %d modified project(s) at path(s): %v",
		len(projects), strings.Join(uniquePaths, ", "))
	return projects
}

func (p *DefaultProjectFinder) filterToTerraform(files []string) []string {
	var filtered []string
	for _, fileName := range files {
		if !p.isInExcludeList(fileName) && strings.Contains(fileName, ".tf") {
			filtered = append(filtered, fileName)
		}
	}
	return filtered
}

func (p *DefaultProjectFinder) isInExcludeList(fileName string) bool {
	for _, s := range excludeList {
		if strings.Contains(fileName, s) {
			return true
		}
	}
	return false
}

// getProjectPath attempts to determine based on the location of a modified
// file, where the root of the Terraform project is. It also attempts to verify
// if the root is valid by looking for a main.tf file. It returns a relative
// path. If the project is at the root returns ".". If modified file doesn't
// lead to a valid project path, returns an empty string.
func (p *DefaultProjectFinder) getProjectPath(modifiedFilePath string, repoDir string) string {
	dir := path.Dir(modifiedFilePath)
	if path.Base(dir) == "env" {
		// If the modified file was inside an env/ directory, we treat this
		// specially and run plan one level up. This supports directory structures
		// like:
		// root/
		//   main.tf
		//   env/
		//     dev.tfvars
		//     staging.tfvars
		return path.Dir(dir)
	}

	// Surrounding dir with /'s so we can match on /modules/ even if dir is
	// "modules" or "project1/modules"
	if strings.Contains("/"+dir+"/", "/modules/") {
		// We treat changes inside modules/ folders specially. There are two cases:
		// 1. modules folder inside project:
		// root/
		//   main.tf
		//     modules/
		//       ...
		// In this case, if we detect a change in modules/, we will determine
		// the project root to be at root/.
		//
		// 2. shared top-level modules folder
		// root/
		//  project1/
		//    main.tf # uses modules via ../modules
		//  project2/
		//    main.tf # uses modules via ../modules
		//  modules/
		//    ...
		// In this case, if we detect a change in modules/ we don't know which
		// project was using this module so we can't suggest a project root, but we
		// also detect that there's no main.tf in the parent folder of modules/
		// so we won't suggest that as a project. So in this case we return nothing.
		// The code below makes this happen.

		// Need to add a trailing slash before splitting on modules/ because if
		// the input was modules/file.tf then path.Dir will be "modules" and so our
		// split on "modules/" will fail.
		dirWithTrailingSlash := dir + "/"
		modulesSplit := strings.SplitN(dirWithTrailingSlash, "modules/", 2)
		modulesParent := modulesSplit[0]

		// Now we check whether there is a main.tf in the parent.
		if _, err := os.Stat(filepath.Join(repoDir, modulesParent, "main.tf")); os.IsNotExist(err) {
			return ""
		}
		return path.Clean(modulesParent)
	}

	// If it wasn't a modules directory, we assume we're in a project and return
	// this directory.
	return dir
}

func (p *DefaultProjectFinder) unique(strs []string) []string {
	hash := make(map[string]bool)
	var unique []string
	for _, s := range strs {
		if !hash[s] {
			unique = append(unique, s)
			hash[s] = true
		}
	}
	return unique
}
