// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

//go:build js && wasm

package main

import (
	"bytes"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"syscall/js"

	"github.com/elastic/mito/lib"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"github.com/google/cel-go/common"

	"github.com/elastic/celfmt"
)

//go:generate cp "$GOROOT/lib/wasm/wasm_exec.js" "$PWD"

func compileAndFormat(dst io.Writer, src string) error {
	xml, err := lib.XML(nil, nil)
	if err != nil {
		return err
	}
	env, err := cel.NewEnv(
		cel.Declarations(decls.NewVar("state", decls.Dyn)),
		lib.Collections(),
		lib.Crypto(),
		lib.JSON(nil),
		lib.Time(),
		lib.Try(),
		lib.Debug(func(_ string, _ any) {}),
		lib.File(nil),
		lib.MIME(nil),
		lib.HTTP(nil, nil, nil),
		lib.Limit(nil),
		lib.Strings(),
		xml,
		cel.OptionalTypes(cel.OptionalTypesVersion(1)),
		cel.EnableMacroCallTracking(),
	)
	if err != nil {
		return fmt.Errorf("failed to create env: %w", err)
	}
	ast, iss := env.Compile(src)
	if iss != nil {
		return fmt.Errorf("failed to parse program: %v", iss)
	}
	return celfmt.Format(dst, ast.NativeRep(), common.NewTextSource(src), celfmt.Pretty(), celfmt.AlwaysComma())
}

func celFmt(this js.Value, args []js.Value) any {
	if len(args) != 1 {
		return map[string]any{
			"error": "celFmt requires one argument",
		}
	}
	if args[0].Type() != js.TypeString {
		return map[string]any{
			"error": "celFmt argument must be a string",
		}
	}
	src := args[0].String()

	out := new(bytes.Buffer)
	if err := compileAndFormat(out, src); err != nil {
		return map[string]any{
			"error": err.Error(),
		}
	}

	return map[string]any{
		"source": out.String(),
	}
}

func getBuildMetadata() (map[string]string, error) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return nil, fmt.Errorf("could not read build info")
	}

	meta := map[string]string{
		"go":     runtime.Version(),
		"celfmt": info.Main.Version,
	}

	for _, m := range info.Deps {
		switch m.Path {
		case "github.com/elastic/mito":
			meta["mito"] = m.Version
		case "github.com/google/cel-go":
			meta["cel-go"] = m.Version
		}
	}

	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			meta["commit"] = setting.Value
		case "vcs.time":
			meta["commit_time"] = setting.Value
		}
	}

	return meta, nil
}

func printBuildMetadata() {
	meta, err := getBuildMetadata()
	if err != nil {
		return
	}

	fmt.Println("celfmt build metadata:", meta)
}

func main() {
	printBuildMetadata()

	done := make(chan int, 0)
	js.Global().Set("celFmt", js.FuncOf(celFmt))
	<-done
}
