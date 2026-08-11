package model

import "testing"

func TestAIViewerRoleTemplate(t *testing.T) {
	viewer := RoleTemplates["viewer"]
	aiviewer := RoleTemplates["aiviewer"]
	if viewer == nil || aiviewer == nil {
		t.Fatal("missing role templates")
	}
	if viewer.HasPermission("aiops", "view") {
		t.Fatal("viewer must not have aiops:view")
	}
	if viewer.HasPermission("aiops_config", "view") {
		t.Fatal("viewer must not have aiops_config:view")
	}
	if !aiviewer.HasPermission("aiops", "view") {
		t.Fatal("aiviewer must have aiops:view")
	}
	if !aiviewer.HasPermission("aiops_config", "view") {
		t.Fatal("aiviewer must have aiops_config:view")
	}
	if aiviewer.HasPermission("aiops", "execute") {
		t.Fatal("aiviewer must not have aiops:execute")
	}
	if aiviewer.HasPermission("aiops_config", "edit") {
		t.Fatal("aiviewer must not have aiops_config:edit")
	}
	// baseline cluster view should still exist
	if !aiviewer.HasPermission("pods", "view") {
		t.Fatal("aiviewer should keep viewer pod view")
	}
	if !aiviewer.HasPermission("clusters", "view") {
		t.Fatal("aiviewer must have clusters:view")
	}
}
