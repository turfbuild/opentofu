// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package funcs

import (
	"path/filepath"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// MakeAbsPathFunc constructs AbsPathFunc for a base directory: a relative path
// is joined to baseDir before it is made absolute, as file() and the other
// filesystem functions join theirs. With "." -- OpenTofu's own BaseDir -- it is
// AbsPathFunc exactly. A downstream host that evaluates in a process whose
// working directory is not the configuration's opts in through
// Scope.AbsPathFromBaseDir, so abspath agrees with the functions beside it.
func MakeAbsPathFunc(baseDir string) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{
				Name: "path",
				Type: cty.String,
			},
		},
		Type:         function.StaticReturnType(cty.String),
		RefineResult: refineNotNull,
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			path := args[0].AsString()
			if !filepath.IsAbs(path) {
				path = filepath.Join(baseDir, path)
			}
			absPath, err := filepath.Abs(path)
			return cty.StringVal(filepath.ToSlash(absPath)), err
		},
	})
}
