package lib_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"netbird-terraformer/lib"
)

// Go randomizes map iteration order, so generated files must sort their keys.
// Without this, every regeneration reshuffles attributes and produces a huge
// diff even when nothing changed in NetBird.
func TestWriteResourceFileIsDeterministic(t *testing.T) {
	res := []lib.TerraformResource{
		{
			Type: "policy",
			Name: "sample",
			Attributes: map[string]any{
				"id":          "abc123",
				"name":        "sample",
				"description": "some description",
				"enabled":     true,
				"metric":      9999,
				"masquerade":  false,
				"groups":      []string{"netbird_group.a.id", "netbird_group.b.id"},
				"rules": []any{
					map[string]any{
						"name":          "r1",
						"action":        "accept",
						"protocol":      "tcp",
						"enabled":       true,
						"bidirectional": false,
						"ports":         []string{"80", "443"},
						"sources":       []string{"netbird_group.a.id"},
						"destinations":  []string{"netbird_group.b.id"},
						"destination_resource": map[string]any{
							"id":   "res1",
							"type": "host",
						},
					},
				},
			},
		},
	}

	var first string
	for i := 0; i < 50; i++ {
		dir := t.TempDir()
		tg := lib.NewTerraformGenerator(dir, &lib.Config{})
		if err := tg.WriteResourceFile("policy", res); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		out, err := os.ReadFile(filepath.Join(dir, "policy.tf"))
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if i == 0 {
			first = string(out)
			continue
		}
		if string(out) != first {
			t.Fatalf("run %d differs from run 0:\n--- run 0 ---\n%s\n--- run %d ---\n%s", i, first, i, out)
		}
	}
}

// Resource order must not depend on the order the NetBird API returned them.
func TestWriteResourceFileSortsResources(t *testing.T) {
	names := []string{"zulu", "alpha", "mike"}
	res := make([]lib.TerraformResource, 0, len(names))
	for _, n := range names {
		res = append(res, lib.TerraformResource{
			Type:       "group",
			Name:       n,
			Attributes: map[string]any{"name": n},
		})
	}

	dir := t.TempDir()
	tg := lib.NewTerraformGenerator(dir, &lib.Config{})
	if err := tg.WriteResourceFile("group", res); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(filepath.Join(dir, "group.tf"))
	if err != nil {
		t.Fatal(err)
	}

	wantOrder := []string{"alpha", "mike", "zulu"}
	prev := -1
	for _, n := range wantOrder {
		at := strings.Index(string(out), `"`+n+`"`)
		if at < 0 {
			t.Fatalf("resource %q missing from output:\n%s", n, out)
		}
		if at < prev {
			t.Fatalf("resources not sorted, %q came too early:\n%s", n, out)
		}
		prev = at
	}

	// The caller's slice must not be reordered.
	if res[0].Name != "zulu" {
		t.Fatalf("caller slice was reordered: got %q as first element", res[0].Name)
	}
}

// "id" is read-only on the resource itself, but required inside nested blocks
// such as source_resource / destination_resource.
func TestWriteResourceFileKeepsNestedID(t *testing.T) {
	res := []lib.TerraformResource{
		{
			Type: "policy",
			Name: "sample",
			Attributes: map[string]any{
				"id":   "top-level-id",
				"name": "sample",
				"rules": []any{
					map[string]any{
						"name": "r1",
						"destination_resource": map[string]any{
							"id":   "nested-id",
							"type": "host",
						},
					},
				},
			},
		},
	}

	dir := t.TempDir()
	tg := lib.NewTerraformGenerator(dir, &lib.Config{})
	if err := tg.WriteResourceFile("policy", res); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(filepath.Join(dir, "policy.tf"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(out), `id = "nested-id"`) {
		t.Errorf("nested id was dropped:\n%s", out)
	}
	if strings.Contains(string(out), "top-level-id") {
		t.Errorf("top-level id must not be written:\n%s", out)
	}
}

// RawValue items in a list matched no case in the list writer and were dropped
// without error. Any resolved reference placed in a list -- a data source id,
// a locals lookup -- would have silently vanished from the generated file.
func TestWriteResourceFileKeepsRawValueListItems(t *testing.T) {
	res := []lib.TerraformResource{
		{
			Type: "policy",
			Name: "sample",
			Attributes: map[string]any{
				"name": "sample",
				"sources": []any{
					lib.RawValue("data.netbird_peer.nat_tools.id"),
					"netbird_group.infrastructure.id",
					"literal-value",
				},
			},
		},
	}

	dir := t.TempDir()
	tg := lib.NewTerraformGenerator(dir, &lib.Config{})
	if err := tg.WriteResourceFile("policy", res); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(filepath.Join(dir, "policy.tf"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)

	for _, want := range []string{
		"data.netbird_peer.nat_tools.id,",
		"netbird_group.infrastructure.id,",
		`"literal-value",`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// A quoted reference becomes a literal string and silently breaks the graph.
	if strings.Contains(got, `"data.netbird_peer`) {
		t.Errorf("reference was quoted:\n%s", got)
	}
}

// Group peer membership is owned by NetBird: it is derived from each user's
// auto_groups via groups_propagation. Emitting it would make Terraform
// authoritative over a list the server rewrites on its own.
func TestWriteResourceFileSkipsGroupPeers(t *testing.T) {
	res := []lib.TerraformResource{
		{
			Type: "group",
			Name: "nat",
			Attributes: map[string]any{
				"name":  "nat",
				"peers": []any{"peer-id-1", "peer-id-2"},
			},
		},
	}

	dir := t.TempDir()
	tg := lib.NewTerraformGenerator(dir, &lib.Config{})
	if err := tg.WriteResourceFile("group", res); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(filepath.Join(dir, "group.tf"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "peers") {
		t.Errorf("group membership must stay computed:\n%s", out)
	}
}
