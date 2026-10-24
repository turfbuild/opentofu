// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package funcs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zclconf/go-cty/cty"
)

func TestMakeAbsPathFunc(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	abs := filepath.Join(base, "elsewhere")
	tests := []struct {
		baseDir, path, want string
	}{
		// "." is OpenTofu's own BaseDir, and abspath is AbsPathFunc there.
		{".", "modules/x", filepath.Join(cwd, "modules", "x")},
		{".", ".", cwd},
		{base, "modules/x", filepath.Join(base, "modules", "x")},
		{base, ".", base},
		{base, "../sibling", filepath.Join(filepath.Dir(base), "sibling")},
		{base, abs, abs},
		// An empty base directory is the process working directory, as it is
		// for the other filesystem functions.
		{"", "modules/x", filepath.Join(cwd, "modules", "x")},
	}
	for _, tc := range tests {
		got, err := MakeAbsPathFunc(tc.baseDir).Call([]cty.Value{cty.StringVal(tc.path)})
		if err != nil {
			t.Errorf("abspath(%q) under %q: %s", tc.path, tc.baseDir, err)
			continue
		}
		if want := cty.StringVal(filepath.ToSlash(tc.want)); !got.RawEquals(want) {
			t.Errorf("abspath(%q) under %q = %#v; want %#v", tc.path, tc.baseDir, got, want)
		}
	}
}
