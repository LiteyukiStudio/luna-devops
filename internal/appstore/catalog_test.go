package appstore

import (
	"bytes"
	"testing"
)

func TestCatalogUsesTypedDataVolumeDeclarations(t *testing.T) {
	raw, err := templateFS.ReadFile("templates.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, legacy := range [][]byte{[]byte(`"dataRetentionEnabled"`), []byte(`"dataMountPath"`), []byte(`"dataCapacity"`)} {
		if bytes.Contains(raw, legacy) {
			t.Fatalf("catalog still contains legacy volume property %s", legacy)
		}
	}

	templates, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}
	persistentTemplates := 0
	for _, template := range templates {
		for _, dataVolume := range template.DataVolumes {
			if dataVolume.SourceType == "projectVolume" {
				persistentTemplates++
				if dataVolume.LogicalName == "" || dataVolume.MountPath == "" {
					t.Fatalf("template %s has an incomplete projectVolume declaration: %#v", template.ID, dataVolume)
				}
			}
		}
	}
	if persistentTemplates == 0 {
		t.Fatal("catalog has no persistent templates to exercise project volume installation")
	}
}

func TestValidateTemplateDataVolumesRejectsAmbiguousProjectVolumeBindings(t *testing.T) {
	template := Template{ID: "invalid", DataVolumes: []DataVolume{
		{LogicalName: "data", SourceType: "projectVolume", MountPath: "/data"},
		{LogicalName: "backup", SourceType: "projectVolume", MountPath: "/backup"},
	}}
	if err := validateTemplateDataVolumes(template); err == nil {
		t.Fatal("multiple projectVolume declarations were accepted even though install accepts one projectVolumeId")
	}
}
