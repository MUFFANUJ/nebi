package service

import (
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/projectfs"
)

// WithProjectWriteAccess permits a Nebi operation to modify managed specs.
// Local projects belong to the user and keep their normal filesystem permissions.
func (s *ProjectService) WithProjectWriteAccess(project *models.Project, write func() error) error {
	if project.Source == "local" {
		return write()
	}
	return projectfs.WithWriteAccess(s.executor.GetProjectPath(project), write)
}

// ProtectManagedProjects applies protection to existing saved projects at startup.
func (s *ProjectService) ProtectManagedProjects() error {
	var projects []models.Project
	if err := s.db.Where("source <> ? OR source IS NULL", "local").Find(&projects).Error; err != nil {
		return err
	}
	for i := range projects {
		if err := s.WithProjectWriteAccess(&projects[i], func() error { return nil }); err != nil {
			return err
		}
	}
	return nil
}
