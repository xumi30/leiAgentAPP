package openclawskill

import "testing"

func TestPickClawdRegistryPackagePrefersExactID(t *testing.T) {
	pkg, ok := pickClawdRegistryPackage([]clawdSkillPackage{
		{ID: "irre-nnn/avatar-image-generator", Name: "avatar-image-generator"},
		{ID: "irre-nnn/3d-image-generator", Name: "3d-image-generator"},
	}, "irre-nnn/3d-image-generator")
	if !ok {
		t.Fatal("pickClawdRegistryPackage did not find a package")
	}
	if pkg.ID != "irre-nnn/3d-image-generator" {
		t.Fatalf("picked ID = %q, want irre-nnn/3d-image-generator", pkg.ID)
	}
}

func TestPickClawdRegistryPackageFallsBackToName(t *testing.T) {
	pkg, ok := pickClawdRegistryPackage([]clawdSkillPackage{
		{ID: "irre-nnn/3d-image-generator", Name: "3d-image-generator"},
	}, "3d-image-generator")
	if !ok {
		t.Fatal("pickClawdRegistryPackage did not find a package")
	}
	if pkg.ID != "irre-nnn/3d-image-generator" {
		t.Fatalf("picked ID = %q, want irre-nnn/3d-image-generator", pkg.ID)
	}
}
