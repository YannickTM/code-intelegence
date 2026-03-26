package extractors

import (
	"context"
	"testing"

	"myjungle/backend-worker/internal/parser"
)

// parseAndExtractImports is a test helper that parses source code and extracts imports.
func parseAndExtractImports(t *testing.T, langID, filePath, source string) []parser.Import {
	t.Helper()
	grammar := parser.GetGrammar(langID)
	if grammar == nil {
		t.Fatalf("no grammar for %s", langID)
	}
	pool := parser.NewPool(1)
	defer pool.Shutdown()

	content := []byte(source)
	tree, err := pool.Parse(context.Background(), content, grammar)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return ExtractImports(tree.RootNode(), content, filePath, langID)
}

// findImport returns the first import with the given ImportName, or nil.
func findImport(imports []parser.Import, name string) *parser.Import {
	for i := range imports {
		if imports[i].ImportName == name {
			return &imports[i]
		}
	}
	return nil
}

// assertImportExists checks that an import with the given name exists and returns it.
func assertImportExists(t *testing.T, imports []parser.Import, name string) *parser.Import {
	t.Helper()
	imp := findImport(imports, name)
	if imp == nil {
		names := make([]string, len(imports))
		for i, im := range imports {
			names[i] = im.ImportName + "(" + im.ImportType + ")"
		}
		t.Errorf("expected import %q not found; have: %v", name, names)
	}
	return imp
}

// ---------------------------------------------------------------------------
// JavaScript tests
// ---------------------------------------------------------------------------

func TestExtractImports_JavaScript(t *testing.T) {
	t.Run("named import", func(t *testing.T) {
		imports := parseAndExtractImports(t, "javascript", "src/app.js",
			`import { foo, bar } from 'lodash';`)
		imp := assertImportExists(t, imports, "lodash")
		if imp != nil && imp.ImportType != "EXTERNAL" {
			t.Errorf("type = %q, want EXTERNAL", imp.ImportType)
		}
	})

	t.Run("default import", func(t *testing.T) {
		imports := parseAndExtractImports(t, "javascript", "src/app.js",
			`import React from 'react';`)
		imp := assertImportExists(t, imports, "react")
		if imp != nil && imp.ImportType != "EXTERNAL" {
			t.Errorf("type = %q, want EXTERNAL", imp.ImportType)
		}
	})

	t.Run("namespace import", func(t *testing.T) {
		imports := parseAndExtractImports(t, "javascript", "src/app.js",
			`import * as path from 'path';`)
		imp := assertImportExists(t, imports, "path")
		if imp != nil && imp.ImportType != "STDLIB" {
			t.Errorf("type = %q, want STDLIB", imp.ImportType)
		}
	})

	t.Run("side-effect import", func(t *testing.T) {
		imports := parseAndExtractImports(t, "javascript", "src/app.js",
			`import './styles.css';`)
		imp := assertImportExists(t, imports, "./styles.css")
		if imp != nil {
			if imp.ImportType != "INTERNAL" {
				t.Errorf("type = %q, want INTERNAL", imp.ImportType)
			}
			if imp.TargetFilePath != "src/styles.css" {
				t.Errorf("TargetFilePath = %q, want src/styles.css", imp.TargetFilePath)
			}
		}
	})

	t.Run("stdlib node module", func(t *testing.T) {
		imports := parseAndExtractImports(t, "javascript", "src/app.js",
			`import fs from 'fs';`)
		imp := assertImportExists(t, imports, "fs")
		if imp != nil && imp.ImportType != "STDLIB" {
			t.Errorf("type = %q, want STDLIB", imp.ImportType)
		}
	})

	t.Run("stdlib with node: prefix", func(t *testing.T) {
		imports := parseAndExtractImports(t, "javascript", "src/app.js",
			`import crypto from 'node:crypto';`)
		imp := assertImportExists(t, imports, "node:crypto")
		if imp != nil && imp.ImportType != "STDLIB" {
			t.Errorf("type = %q, want STDLIB", imp.ImportType)
		}
	})

	t.Run("internal relative import", func(t *testing.T) {
		imports := parseAndExtractImports(t, "javascript", "src/components/App.js",
			`import { helper } from '../utils/helper';`)
		imp := assertImportExists(t, imports, "../utils/helper")
		if imp != nil {
			if imp.ImportType != "INTERNAL" {
				t.Errorf("type = %q, want INTERNAL", imp.ImportType)
			}
			if imp.TargetFilePath != "src/utils/helper" {
				t.Errorf("TargetFilePath = %q, want src/utils/helper", imp.TargetFilePath)
			}
		}
	})

	t.Run("require call", func(t *testing.T) {
		imports := parseAndExtractImports(t, "javascript", "src/app.js",
			`const lodash = require('lodash');`)
		imp := assertImportExists(t, imports, "lodash")
		if imp != nil && imp.ImportType != "EXTERNAL" {
			t.Errorf("type = %q, want EXTERNAL", imp.ImportType)
		}
	})

	t.Run("dynamic import literal", func(t *testing.T) {
		imports := parseAndExtractImports(t, "javascript", "src/app.js",
			`const mod = import('./lazy-module');`)
		imp := assertImportExists(t, imports, "./lazy-module")
		if imp != nil && imp.ImportType != "INTERNAL" {
			t.Errorf("type = %q, want INTERNAL", imp.ImportType)
		}
	})

	t.Run("dynamic import non-literal", func(t *testing.T) {
		imports := parseAndExtractImports(t, "javascript", "src/app.js",
			`const mod = import(variable);`)
		assertImportExists(t, imports, "<dynamic>")
	})

	t.Run("interpolated template require is ignored", func(t *testing.T) {
		imports := parseAndExtractImports(t, "javascript", "src/app.js",
			"const m = require(`./modules/${name}`);")
		// Interpolated template strings are dynamic, not static imports.
		if findImport(imports, "./modules/${name}") != nil {
			t.Error("interpolated template string was treated as a static import")
		}
	})

	t.Run("plain template require is accepted", func(t *testing.T) {
		imports := parseAndExtractImports(t, "javascript", "src/app.js",
			"const m = require(`./utils`);")
		assertImportExists(t, imports, "./utils")
	})

	t.Run("interpolated template dynamic import", func(t *testing.T) {
		imports := parseAndExtractImports(t, "javascript", "src/app.js",
			"const m = import(`./pages/${page}`);")
		assertImportExists(t, imports, "<dynamic>")
		if findImport(imports, "./pages/${page}") != nil {
			t.Error("interpolated template string was treated as a static import")
		}
	})

	t.Run("nested import inside call expression", func(t *testing.T) {
		imports := parseAndExtractImports(t, "javascript", "src/app.js",
			`const mods = Promise.all([import('./a'), import('./b')]);`)
		assertImportExists(t, imports, "./a")
		assertImportExists(t, imports, "./b")
	})

	t.Run("deduplication", func(t *testing.T) {
		imports := parseAndExtractImports(t, "javascript", "src/app.js",
			`import { foo } from 'react';
import { bar } from 'react';`)
		count := 0
		for _, imp := range imports {
			if imp.ImportName == "react" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("expected 1 import for 'react', got %d", count)
		}
	})

	t.Run("multiple imports ordered", func(t *testing.T) {
		imports := parseAndExtractImports(t, "javascript", "src/app.js",
			`import React from 'react';
import { useState } from 'react';
import lodash from 'lodash';
import fs from 'fs';`)
		if len(imports) != 3 {
			t.Fatalf("expected 3 unique imports, got %d", len(imports))
		}
		// First occurrence order: react, lodash, fs
		if imports[0].ImportName != "react" {
			t.Errorf("imports[0] = %q, want react", imports[0].ImportName)
		}
		if imports[1].ImportName != "lodash" {
			t.Errorf("imports[1] = %q, want lodash", imports[1].ImportName)
		}
		if imports[2].ImportName != "fs" {
			t.Errorf("imports[2] = %q, want fs", imports[2].ImportName)
		}
	})
}

// ---------------------------------------------------------------------------
// TypeScript tests
// ---------------------------------------------------------------------------

func TestExtractImports_TypeScript(t *testing.T) {
	t.Run("import type", func(t *testing.T) {
		imports := parseAndExtractImports(t, "typescript", "src/app.ts",
			`import type { Config } from './config';`)
		imp := assertImportExists(t, imports, "./config")
		if imp != nil && imp.ImportType != "INTERNAL" {
			t.Errorf("type = %q, want INTERNAL", imp.ImportType)
		}
	})

	t.Run("re-export", func(t *testing.T) {
		imports := parseAndExtractImports(t, "typescript", "src/index.ts",
			`export { foo } from './module';`)
		imp := assertImportExists(t, imports, "./module")
		if imp != nil {
			if imp.ImportType != "INTERNAL" {
				t.Errorf("type = %q, want INTERNAL", imp.ImportType)
			}
			if imp.TargetFilePath != "src/module" {
				t.Errorf("TargetFilePath = %q, want src/module", imp.TargetFilePath)
			}
		}
	})

	t.Run("export all from", func(t *testing.T) {
		imports := parseAndExtractImports(t, "typescript", "src/index.ts",
			`export * from './utils';`)
		assertImportExists(t, imports, "./utils")
	})
}

// ---------------------------------------------------------------------------
// JSX tests
// ---------------------------------------------------------------------------

func TestExtractImports_JSX(t *testing.T) {
	t.Run("jsx import", func(t *testing.T) {
		imports := parseAndExtractImports(t, "jsx", "src/App.jsx",
			`import React from 'react';
import { Button } from './components/Button';`)
		assertImportExists(t, imports, "react")
		imp := assertImportExists(t, imports, "./components/Button")
		if imp != nil && imp.ImportType != "INTERNAL" {
			t.Errorf("type = %q, want INTERNAL", imp.ImportType)
		}
	})
}

// ---------------------------------------------------------------------------
// Python tests
// ---------------------------------------------------------------------------

func TestExtractImports_Python(t *testing.T) {
	t.Run("import stdlib", func(t *testing.T) {
		imports := parseAndExtractImports(t, "python", "src/main.py",
			`import os`)
		imp := assertImportExists(t, imports, "os")
		if imp != nil && imp.ImportType != "STDLIB" {
			t.Errorf("type = %q, want STDLIB", imp.ImportType)
		}
	})

	t.Run("from import stdlib", func(t *testing.T) {
		imports := parseAndExtractImports(t, "python", "src/main.py",
			`from os.path import join`)
		imp := assertImportExists(t, imports, "os.path")
		if imp != nil && imp.ImportType != "STDLIB" {
			t.Errorf("type = %q, want STDLIB", imp.ImportType)
		}
	})

	t.Run("relative import", func(t *testing.T) {
		imports := parseAndExtractImports(t, "python", "src/app/views.py",
			`from .utils import helper`)
		imp := assertImportExists(t, imports, ".utils")
		if imp != nil && imp.ImportType != "INTERNAL" {
			t.Errorf("type = %q, want INTERNAL", imp.ImportType)
		}
	})

	t.Run("parent relative import", func(t *testing.T) {
		imports := parseAndExtractImports(t, "python", "src/app/views.py",
			`from ..models import User`)
		imp := assertImportExists(t, imports, "..models")
		if imp != nil && imp.ImportType != "INTERNAL" {
			t.Errorf("type = %q, want INTERNAL", imp.ImportType)
		}
	})

	t.Run("external package", func(t *testing.T) {
		imports := parseAndExtractImports(t, "python", "src/main.py",
			`import requests`)
		imp := assertImportExists(t, imports, "requests")
		if imp != nil && imp.ImportType != "EXTERNAL" {
			t.Errorf("type = %q, want EXTERNAL", imp.ImportType)
		}
	})

	t.Run("multiple imports", func(t *testing.T) {
		imports := parseAndExtractImports(t, "python", "src/main.py",
			`import os
import sys
import requests
from .utils import foo`)
		if len(imports) != 4 {
			t.Fatalf("expected 4 imports, got %d", len(imports))
		}
		assertImportExists(t, imports, "os")
		assertImportExists(t, imports, "sys")
		assertImportExists(t, imports, "requests")
		assertImportExists(t, imports, ".utils")
	})
}

func TestExtractImports_Python_ResolvePath(t *testing.T) {
	t.Run("single dot relative", func(t *testing.T) {
		imports := parseAndExtractImports(t, "python", "src/app/views.py",
			`from .utils import helper`)
		imp := assertImportExists(t, imports, ".utils")
		if imp == nil {
			return
		}
		if imp.ImportType != "INTERNAL" {
			t.Errorf("type = %q, want INTERNAL", imp.ImportType)
		}
		if imp.TargetFilePath != "src/app/utils" {
			t.Errorf("TargetFilePath = %q, want %q", imp.TargetFilePath, "src/app/utils")
		}
	})

	t.Run("double dot relative", func(t *testing.T) {
		imports := parseAndExtractImports(t, "python", "src/app/views.py",
			`from ..models import User`)
		imp := assertImportExists(t, imports, "..models")
		if imp == nil {
			return
		}
		if imp.TargetFilePath != "src/models" {
			t.Errorf("TargetFilePath = %q, want %q", imp.TargetFilePath, "src/models")
		}
	})

	t.Run("bare dot import (package dir)", func(t *testing.T) {
		imports := parseAndExtractImports(t, "python", "src/app/views.py",
			`from . import something`)
		imp := assertImportExists(t, imports, ".")
		if imp == nil {
			return
		}
		if imp.TargetFilePath != "src/app" {
			t.Errorf("TargetFilePath = %q, want %q", imp.TargetFilePath, "src/app")
		}
	})

	t.Run("dotted remainder", func(t *testing.T) {
		imports := parseAndExtractImports(t, "python", "src/app/views.py",
			`from .models.user import User`)
		imp := assertImportExists(t, imports, ".models.user")
		if imp == nil {
			return
		}
		if imp.TargetFilePath != "src/app/models/user" {
			t.Errorf("TargetFilePath = %q, want %q", imp.TargetFilePath, "src/app/models/user")
		}
	})

	t.Run("absolute import has no target path", func(t *testing.T) {
		imports := parseAndExtractImports(t, "python", "src/main.py",
			`import requests`)
		imp := assertImportExists(t, imports, "requests")
		if imp != nil && imp.TargetFilePath != "" {
			t.Errorf("absolute import should have empty TargetFilePath, got %q", imp.TargetFilePath)
		}
	})
}

func TestExtractImports_Rust_ResolvePath(t *testing.T) {
	t.Run("crate import", func(t *testing.T) {
		imports := parseAndExtractImports(t, "rust", "src/main.rs",
			`use crate::utils::helper;`)
		imp := assertImportExists(t, imports, "crate::utils::helper")
		if imp == nil {
			return
		}
		if imp.ImportType != "INTERNAL" {
			t.Errorf("type = %q, want INTERNAL", imp.ImportType)
		}
		if imp.TargetFilePath != "src/utils/helper" {
			t.Errorf("TargetFilePath = %q, want %q", imp.TargetFilePath, "src/utils/helper")
		}
	})

	t.Run("super from file module", func(t *testing.T) {
		// auth.rs is module crate::handlers::auth; super = crate::handlers
		imports := parseAndExtractImports(t, "rust", "src/handlers/auth.rs",
			`use super::models;`)
		imp := assertImportExists(t, imports, "super::models")
		if imp == nil {
			return
		}
		if imp.TargetFilePath != "src/handlers/models" {
			t.Errorf("TargetFilePath = %q, want %q", imp.TargetFilePath, "src/handlers/models")
		}
	})

	t.Run("super from mod.rs", func(t *testing.T) {
		// mod.rs is module crate::handlers; super = crate (src/)
		imports := parseAndExtractImports(t, "rust", "src/handlers/mod.rs",
			`use super::models;`)
		imp := assertImportExists(t, imports, "super::models")
		if imp == nil {
			return
		}
		if imp.TargetFilePath != "src/models" {
			t.Errorf("TargetFilePath = %q, want %q", imp.TargetFilePath, "src/models")
		}
	})

	t.Run("chained super from file module", func(t *testing.T) {
		// auth.rs is module crate::handlers::auth
		// super::super = crate → src/
		imports := parseAndExtractImports(t, "rust", "src/handlers/auth.rs",
			`use super::super::foo;`)
		imp := assertImportExists(t, imports, "super::super::foo")
		if imp == nil {
			return
		}
		if imp.TargetFilePath != "src/foo" {
			t.Errorf("TargetFilePath = %q, want %q", imp.TargetFilePath, "src/foo")
		}
	})

	t.Run("self from file module", func(t *testing.T) {
		// auth.rs is module crate::handlers::auth; self::child = crate::handlers::auth::child
		imports := parseAndExtractImports(t, "rust", "src/handlers/auth.rs",
			`use self::child;`)
		imp := assertImportExists(t, imports, "self::child")
		if imp == nil {
			return
		}
		if imp.TargetFilePath != "src/handlers/auth/child" {
			t.Errorf("TargetFilePath = %q, want %q", imp.TargetFilePath, "src/handlers/auth/child")
		}
	})

	t.Run("self from mod.rs", func(t *testing.T) {
		// mod.rs is module crate::handlers; self::child = crate::handlers::child
		imports := parseAndExtractImports(t, "rust", "src/handlers/mod.rs",
			`use self::child;`)
		imp := assertImportExists(t, imports, "self::child")
		if imp == nil {
			return
		}
		if imp.TargetFilePath != "src/handlers/child" {
			t.Errorf("TargetFilePath = %q, want %q", imp.TargetFilePath, "src/handlers/child")
		}
	})

	t.Run("nested crate path", func(t *testing.T) {
		imports := parseAndExtractImports(t, "rust", "src/main.rs",
			`use crate::handlers::auth;`)
		imp := assertImportExists(t, imports, "crate::handlers::auth")
		if imp == nil {
			return
		}
		if imp.TargetFilePath != "src/handlers/auth" {
			t.Errorf("TargetFilePath = %q, want %q", imp.TargetFilePath, "src/handlers/auth")
		}
	})

	t.Run("nested main.rs is file module", func(t *testing.T) {
		// src/handlers/main.rs is a file module (main), not a crate root.
		// self::child = crate::handlers::main::child
		imports := parseAndExtractImports(t, "rust", "src/handlers/main.rs",
			`use self::child;`)
		imp := assertImportExists(t, imports, "self::child")
		if imp == nil {
			return
		}
		if imp.TargetFilePath != "src/handlers/main/child" {
			t.Errorf("TargetFilePath = %q, want %q", imp.TargetFilePath, "src/handlers/main/child")
		}
	})

	t.Run("std import has no target path", func(t *testing.T) {
		imports := parseAndExtractImports(t, "rust", "src/main.rs",
			`use std::io::Read;`)
		imp := assertImportExists(t, imports, "std::io::Read")
		if imp != nil && imp.TargetFilePath != "" {
			t.Errorf("stdlib import should have empty TargetFilePath, got %q", imp.TargetFilePath)
		}
	})
}

// ---------------------------------------------------------------------------
// Go tests
// ---------------------------------------------------------------------------

func TestExtractImports_Go(t *testing.T) {
	t.Run("single import", func(t *testing.T) {
		imports := parseAndExtractImports(t, "go", "main.go",
			`package main
import "fmt"`)
		imp := assertImportExists(t, imports, "fmt")
		if imp != nil && imp.ImportType != "STDLIB" {
			t.Errorf("type = %q, want STDLIB", imp.ImportType)
		}
	})

	t.Run("grouped imports", func(t *testing.T) {
		imports := parseAndExtractImports(t, "go", "main.go",
			`package main
import (
	"fmt"
	"os"
	"github.com/pkg/errors"
)`)
		fmtImp := assertImportExists(t, imports, "fmt")
		if fmtImp != nil && fmtImp.ImportType != "STDLIB" {
			t.Errorf("fmt type = %q, want STDLIB", fmtImp.ImportType)
		}
		osImp := assertImportExists(t, imports, "os")
		if osImp != nil && osImp.ImportType != "STDLIB" {
			t.Errorf("os type = %q, want STDLIB", osImp.ImportType)
		}
		errImp := assertImportExists(t, imports, "github.com/pkg/errors")
		if errImp != nil && errImp.ImportType != "EXTERNAL" {
			t.Errorf("errors type = %q, want EXTERNAL", errImp.ImportType)
		}
	})

	t.Run("stdlib net/http", func(t *testing.T) {
		imports := parseAndExtractImports(t, "go", "main.go",
			`package main
import "net/http"`)
		imp := assertImportExists(t, imports, "net/http")
		if imp != nil && imp.ImportType != "STDLIB" {
			t.Errorf("type = %q, want STDLIB", imp.ImportType)
		}
	})
}

// ---------------------------------------------------------------------------
// Rust tests
// ---------------------------------------------------------------------------

func TestExtractImports_Rust(t *testing.T) {
	t.Run("use std", func(t *testing.T) {
		imports := parseAndExtractImports(t, "rust", "src/main.rs",
			`use std::io::Read;`)
		imp := assertImportExists(t, imports, "std::io::Read")
		if imp != nil && imp.ImportType != "STDLIB" {
			t.Errorf("type = %q, want STDLIB", imp.ImportType)
		}
	})

	t.Run("use crate", func(t *testing.T) {
		imports := parseAndExtractImports(t, "rust", "src/main.rs",
			`use crate::utils::helper;`)
		imp := assertImportExists(t, imports, "crate::utils::helper")
		if imp != nil && imp.ImportType != "INTERNAL" {
			t.Errorf("type = %q, want INTERNAL", imp.ImportType)
		}
	})

	t.Run("use external", func(t *testing.T) {
		imports := parseAndExtractImports(t, "rust", "src/main.rs",
			`use serde::Serialize;`)
		imp := assertImportExists(t, imports, "serde::Serialize")
		if imp != nil && imp.ImportType != "EXTERNAL" {
			t.Errorf("type = %q, want EXTERNAL", imp.ImportType)
		}
	})

	t.Run("use super", func(t *testing.T) {
		imports := parseAndExtractImports(t, "rust", "src/main.rs",
			`use super::models;`)
		imp := assertImportExists(t, imports, "super::models")
		if imp != nil && imp.ImportType != "INTERNAL" {
			t.Errorf("type = %q, want INTERNAL", imp.ImportType)
		}
	})
}

// ---------------------------------------------------------------------------
// Java tests
// ---------------------------------------------------------------------------

func TestExtractImports_Java(t *testing.T) {
	t.Run("stdlib import", func(t *testing.T) {
		imports := parseAndExtractImports(t, "java", "src/Main.java",
			`import java.util.List;
class Main {}`)
		imp := assertImportExists(t, imports, "java.util.List")
		if imp != nil && imp.ImportType != "STDLIB" {
			t.Errorf("type = %q, want STDLIB", imp.ImportType)
		}
	})

	t.Run("external import", func(t *testing.T) {
		imports := parseAndExtractImports(t, "java", "src/Main.java",
			`import com.example.MyClass;
class Main {}`)
		imp := assertImportExists(t, imports, "com.example.MyClass")
		if imp != nil && imp.ImportType != "EXTERNAL" {
			t.Errorf("type = %q, want EXTERNAL", imp.ImportType)
		}
	})

	t.Run("javax stdlib", func(t *testing.T) {
		imports := parseAndExtractImports(t, "java", "src/Main.java",
			`import javax.swing.JFrame;
class Main {}`)
		imp := assertImportExists(t, imports, "javax.swing.JFrame")
		if imp != nil && imp.ImportType != "STDLIB" {
			t.Errorf("type = %q, want STDLIB", imp.ImportType)
		}
	})
}

// ---------------------------------------------------------------------------
// Kotlin tests
// ---------------------------------------------------------------------------

func TestExtractImports_Kotlin(t *testing.T) {
	t.Run("kotlin stdlib", func(t *testing.T) {
		imports := parseAndExtractImports(t, "kotlin", "src/Main.kt",
			`import kotlin.collections.List
fun main() {}`)
		imp := assertImportExists(t, imports, "kotlin.collections.List")
		if imp != nil && imp.ImportType != "STDLIB" {
			t.Errorf("type = %q, want STDLIB", imp.ImportType)
		}
	})

	t.Run("external import", func(t *testing.T) {
		imports := parseAndExtractImports(t, "kotlin", "src/Main.kt",
			`import com.example.MyClass
fun main() {}`)
		imp := assertImportExists(t, imports, "com.example.MyClass")
		if imp != nil && imp.ImportType != "EXTERNAL" {
			t.Errorf("type = %q, want EXTERNAL", imp.ImportType)
		}
	})
}

// ---------------------------------------------------------------------------
// C tests
// ---------------------------------------------------------------------------

func TestExtractImports_C(t *testing.T) {
	t.Run("system include stdlib", func(t *testing.T) {
		imports := parseAndExtractImports(t, "c", "src/main.c",
			`#include <stdio.h>
int main() { return 0; }`)
		imp := assertImportExists(t, imports, "stdio.h")
		if imp != nil && imp.ImportType != "STDLIB" {
			t.Errorf("type = %q, want STDLIB", imp.ImportType)
		}
	})

	t.Run("quoted include internal", func(t *testing.T) {
		imports := parseAndExtractImports(t, "c", "src/main.c",
			`#include "mylib.h"
int main() { return 0; }`)
		imp := assertImportExists(t, imports, "mylib.h")
		if imp != nil {
			if imp.ImportType != "INTERNAL" {
				t.Errorf("type = %q, want INTERNAL", imp.ImportType)
			}
			if imp.TargetFilePath != "src/mylib.h" {
				t.Errorf("TargetFilePath = %q, want src/mylib.h", imp.TargetFilePath)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// C++ tests
// ---------------------------------------------------------------------------

func TestExtractImports_CPP(t *testing.T) {
	t.Run("system include", func(t *testing.T) {
		imports := parseAndExtractImports(t, "cpp", "src/main.cpp",
			`#include <iostream>
int main() { return 0; }`)
		imp := assertImportExists(t, imports, "iostream")
		if imp != nil && imp.ImportType != "STDLIB" {
			t.Errorf("type = %q, want STDLIB", imp.ImportType)
		}
	})

	t.Run("quoted include", func(t *testing.T) {
		imports := parseAndExtractImports(t, "cpp", "src/main.cpp",
			`#include "myheader.h"
int main() { return 0; }`)
		imp := assertImportExists(t, imports, "myheader.h")
		if imp != nil && imp.ImportType != "INTERNAL" {
			t.Errorf("type = %q, want INTERNAL", imp.ImportType)
		}
	})
}

// ---------------------------------------------------------------------------
// C# tests
// ---------------------------------------------------------------------------

func TestExtractImports_CSharp(t *testing.T) {
	t.Run("system using", func(t *testing.T) {
		imports := parseAndExtractImports(t, "csharp", "src/Program.cs",
			`using System.Collections.Generic;
class Program {}`)
		imp := assertImportExists(t, imports, "System.Collections.Generic")
		if imp != nil && imp.ImportType != "STDLIB" {
			t.Errorf("type = %q, want STDLIB", imp.ImportType)
		}
	})

	t.Run("external using", func(t *testing.T) {
		imports := parseAndExtractImports(t, "csharp", "src/Program.cs",
			`using Newtonsoft.Json;
class Program {}`)
		imp := assertImportExists(t, imports, "Newtonsoft.Json")
		if imp != nil && imp.ImportType != "EXTERNAL" {
			t.Errorf("type = %q, want EXTERNAL", imp.ImportType)
		}
	})

	t.Run("microsoft stdlib", func(t *testing.T) {
		imports := parseAndExtractImports(t, "csharp", "src/Program.cs",
			`using Microsoft.Extensions.DependencyInjection;
class Program {}`)
		imp := assertImportExists(t, imports, "Microsoft.Extensions.DependencyInjection")
		if imp != nil && imp.ImportType != "STDLIB" {
			t.Errorf("type = %q, want STDLIB", imp.ImportType)
		}
	})

	t.Run("global using", func(t *testing.T) {
		imports := parseAndExtractImports(t, "csharp", "src/GlobalUsings.cs",
			`global using System.IO;
class Program {}`)
		imp := assertImportExists(t, imports, "System.IO")
		if imp != nil && imp.ImportType != "STDLIB" {
			t.Errorf("type = %q, want STDLIB", imp.ImportType)
		}
	})

	t.Run("root System namespace is stdlib", func(t *testing.T) {
		imports := parseAndExtractImports(t, "csharp", "src/Program.cs",
			`using System;
class Program {}`)
		imp := assertImportExists(t, imports, "System")
		if imp != nil && imp.ImportType != "STDLIB" {
			t.Errorf("type = %q, want STDLIB", imp.ImportType)
		}
	})
}

// ---------------------------------------------------------------------------
// Swift tests
// ---------------------------------------------------------------------------

func TestExtractImports_Swift(t *testing.T) {
	t.Run("stdlib import", func(t *testing.T) {
		imports := parseAndExtractImports(t, "swift", "Sources/main.swift",
			`import Foundation`)
		imp := assertImportExists(t, imports, "Foundation")
		if imp != nil && imp.ImportType != "STDLIB" {
			t.Errorf("type = %q, want STDLIB", imp.ImportType)
		}
	})

	t.Run("external import", func(t *testing.T) {
		imports := parseAndExtractImports(t, "swift", "Sources/main.swift",
			`import Alamofire`)
		imp := assertImportExists(t, imports, "Alamofire")
		if imp != nil && imp.ImportType != "EXTERNAL" {
			t.Errorf("type = %q, want EXTERNAL", imp.ImportType)
		}
	})

	t.Run("testable import", func(t *testing.T) {
		imports := parseAndExtractImports(t, "swift", "Tests/MyTests.swift",
			`@testable import MyModule`)
		assertImportExists(t, imports, "MyModule")
		if findImport(imports, "@testable import MyModule") != nil {
			t.Error("@testable prefix leaked into ImportName")
		}
	})
}

// ---------------------------------------------------------------------------
// Ruby tests
// ---------------------------------------------------------------------------

func TestExtractImports_Ruby(t *testing.T) {
	t.Run("require stdlib", func(t *testing.T) {
		imports := parseAndExtractImports(t, "ruby", "lib/app.rb",
			`require 'json'`)
		imp := assertImportExists(t, imports, "json")
		if imp != nil && imp.ImportType != "STDLIB" {
			t.Errorf("type = %q, want STDLIB", imp.ImportType)
		}
	})

	t.Run("require external", func(t *testing.T) {
		imports := parseAndExtractImports(t, "ruby", "lib/app.rb",
			`require 'sinatra'`)
		imp := assertImportExists(t, imports, "sinatra")
		if imp != nil && imp.ImportType != "EXTERNAL" {
			t.Errorf("type = %q, want EXTERNAL", imp.ImportType)
		}
	})

	t.Run("require_relative", func(t *testing.T) {
		imports := parseAndExtractImports(t, "ruby", "lib/app.rb",
			`require_relative './helper'`)
		imp := assertImportExists(t, imports, "./helper")
		if imp != nil {
			if imp.ImportType != "INTERNAL" {
				t.Errorf("type = %q, want INTERNAL", imp.ImportType)
			}
			if imp.TargetFilePath != "lib/helper" {
				t.Errorf("TargetFilePath = %q, want lib/helper", imp.TargetFilePath)
			}
		}
	})

	t.Run("require_relative bare name", func(t *testing.T) {
		imports := parseAndExtractImports(t, "ruby", "lib/app.rb",
			`require_relative 'helper'`)
		imp := assertImportExists(t, imports, "./helper")
		if imp != nil {
			if imp.ImportType != "INTERNAL" {
				t.Errorf("type = %q, want INTERNAL", imp.ImportType)
			}
			if imp.TargetFilePath != "lib/helper" {
				t.Errorf("TargetFilePath = %q, want %q", imp.TargetFilePath, "lib/helper")
			}
		}
	})
}

// ---------------------------------------------------------------------------
// PHP tests
// ---------------------------------------------------------------------------

func TestExtractImports_PHP(t *testing.T) {
	t.Run("namespace use", func(t *testing.T) {
		imports := parseAndExtractImports(t, "php", "src/Controller.php",
			`<?php
use App\Models\User;
class Controller {}`)
		assertImportExists(t, imports, "App\\Models\\User")
	})

	t.Run("comma-separated use", func(t *testing.T) {
		imports := parseAndExtractImports(t, "php", "src/Controller.php",
			`<?php
use App\Models\User, App\Models\Post;
class Controller {}`)
		assertImportExists(t, imports, "App\\Models\\User")
		assertImportExists(t, imports, "App\\Models\\Post")
	})

	t.Run("use with alias", func(t *testing.T) {
		imports := parseAndExtractImports(t, "php", "src/Controller.php",
			`<?php
use App\Models\User as U;
class Controller {}`)
		assertImportExists(t, imports, "App\\Models\\User")
		// Verify the leaked form does not appear.
		if findImport(imports, "App\\Models\\User as U") != nil {
			t.Error("alias text leaked into ImportName")
		}
	})

	t.Run("require_once", func(t *testing.T) {
		imports := parseAndExtractImports(t, "php", "src/index.php",
			`<?php
require_once 'vendor/autoload.php';`)
		assertImportExists(t, imports, "vendor/autoload.php")
	})

	t.Run("include_once", func(t *testing.T) {
		imports := parseAndExtractImports(t, "php", "src/index.php",
			`<?php
include_once 'config.php';`)
		assertImportExists(t, imports, "config.php")
	})

	t.Run("require with parentheses", func(t *testing.T) {
		imports := parseAndExtractImports(t, "php", "src/index.php",
			`<?php
require('bootstrap.php');`)
		assertImportExists(t, imports, "bootstrap.php")
	})

	t.Run("require_once with parentheses", func(t *testing.T) {
		imports := parseAndExtractImports(t, "php", "src/index.php",
			`<?php
require_once('vendor/autoload.php');`)
		assertImportExists(t, imports, "vendor/autoload.php")
	})
}

// ---------------------------------------------------------------------------
// Bash tests
// ---------------------------------------------------------------------------

func TestExtractImports_Bash(t *testing.T) {
	t.Run("source relative", func(t *testing.T) {
		imports := parseAndExtractImports(t, "bash", "scripts/deploy.sh",
			`#!/bin/bash
source ./lib.sh`)
		imp := assertImportExists(t, imports, "./lib.sh")
		if imp != nil {
			if imp.ImportType != "INTERNAL" {
				t.Errorf("type = %q, want INTERNAL", imp.ImportType)
			}
			if imp.TargetFilePath != "scripts/lib.sh" {
				t.Errorf("TargetFilePath = %q, want scripts/lib.sh", imp.TargetFilePath)
			}
		}
	})

	t.Run("dot source", func(t *testing.T) {
		imports := parseAndExtractImports(t, "bash", "scripts/deploy.sh",
			`#!/bin/bash
. /etc/profile`)
		imp := assertImportExists(t, imports, "/etc/profile")
		if imp != nil && imp.ImportType != "EXTERNAL" {
			t.Errorf("type = %q, want EXTERNAL", imp.ImportType)
		}
	})
}

// ---------------------------------------------------------------------------
// HCL tests
// ---------------------------------------------------------------------------

func TestExtractImports_HCL(t *testing.T) {
	t.Run("module source", func(t *testing.T) {
		imports := parseAndExtractImports(t, "hcl", "main.tf",
			`module "vpc" {
  source = "terraform-aws-modules/vpc/aws"
  version = "3.0"
}`)
		assertImportExists(t, imports, "terraform-aws-modules/vpc/aws")
	})

	t.Run("module local source", func(t *testing.T) {
		imports := parseAndExtractImports(t, "hcl", "main.tf",
			`module "local" {
  source = "./modules/local"
}`)
		imp := assertImportExists(t, imports, "./modules/local")
		if imp != nil && imp.ImportType != "INTERNAL" {
			t.Errorf("type = %q, want INTERNAL", imp.ImportType)
		}
	})
}

// ---------------------------------------------------------------------------
// Dockerfile tests
// ---------------------------------------------------------------------------

func TestExtractImports_Dockerfile(t *testing.T) {
	t.Run("from instruction", func(t *testing.T) {
		imports := parseAndExtractImports(t, "dockerfile", "Dockerfile",
			`FROM golang:1.21-alpine
RUN echo hello`)
		if len(imports) == 0 {
			t.Fatal("expected at least 1 import")
		}
		// Should extract the image name (golang or golang:1.21-alpine base).
		if imports[0].ImportType != "EXTERNAL" {
			t.Errorf("type = %q, want EXTERNAL", imports[0].ImportType)
		}
	})

	t.Run("from registry with port", func(t *testing.T) {
		imports := parseAndExtractImports(t, "dockerfile", "Dockerfile",
			`FROM registry.example.com:5000/myimage:latest
RUN echo hello`)
		if len(imports) == 0 {
			t.Fatal("expected at least 1 import")
		}
		// Must preserve registry:port/path, only strip the tag.
		imp := imports[0]
		want := "registry.example.com:5000/myimage"
		if imp.ImportName != want {
			t.Errorf("ImportName = %q, want %q", imp.ImportName, want)
		}
	})
}

// ---------------------------------------------------------------------------
// CSS tests
// ---------------------------------------------------------------------------

func TestExtractImports_CSS(t *testing.T) {
	t.Run("import statement", func(t *testing.T) {
		imports := parseAndExtractImports(t, "css", "src/styles.css",
			`@import './reset.css';
body { margin: 0; }`)
		imp := assertImportExists(t, imports, "./reset.css")
		if imp != nil && imp.ImportType != "INTERNAL" {
			t.Errorf("type = %q, want INTERNAL", imp.ImportType)
		}
	})

	t.Run("import with media query", func(t *testing.T) {
		imports := parseAndExtractImports(t, "css", "src/styles.css",
			`@import "print.css" print;
body { margin: 0; }`)
		assertImportExists(t, imports, "print.css")
		if findImport(imports, `"print.css" print`) != nil {
			t.Error("media query leaked into ImportName")
		}
	})

	t.Run("import url with media query", func(t *testing.T) {
		imports := parseAndExtractImports(t, "css", "src/styles.css",
			`@import url("landscape.css") screen and (orientation: landscape);
body { margin: 0; }`)
		assertImportExists(t, imports, "landscape.css")
	})

	t.Run("import url with parens in path", func(t *testing.T) {
		imports := parseAndExtractImports(t, "css", "src/styles.css",
			`@import url("fonts/subset(latin).css");
body { margin: 0; }`)
		assertImportExists(t, imports, "fonts/subset(latin).css")
	})
}

// ---------------------------------------------------------------------------
// Edge case tests
// ---------------------------------------------------------------------------

func TestExtractImports_EdgeCases(t *testing.T) {
	t.Run("nil root", func(t *testing.T) {
		imports := ExtractImports(nil, nil, "", "javascript")
		if imports != nil {
			t.Errorf("expected nil, got %v", imports)
		}
	})

	t.Run("unknown language", func(t *testing.T) {
		// Use a real parsed tree so nil-root doesn't short-circuit.
		grammar := parser.GetGrammar("javascript")
		pool := parser.NewPool(1)
		defer pool.Shutdown()
		content := []byte("var x = 1;")
		tree, err := pool.Parse(context.Background(), content, grammar)
		if err != nil {
			t.Fatal(err)
		}
		imports := ExtractImports(tree.RootNode(), content, "file.xyz", "unknown-lang")
		if imports != nil {
			t.Errorf("expected nil, got %v", imports)
		}
	})

	t.Run("empty file", func(t *testing.T) {
		imports := parseAndExtractImports(t, "javascript", "src/empty.js", "")
		if len(imports) != 0 {
			t.Errorf("expected 0 imports, got %d", len(imports))
		}
	})

	t.Run("language with no import node types", func(t *testing.T) {
		imports := parseAndExtractImports(t, "yaml", "config.yaml", `key: value`)
		if len(imports) != 0 {
			t.Errorf("expected 0 imports for YAML, got %d", len(imports))
		}
	})

	t.Run("source file path preserved", func(t *testing.T) {
		imports := parseAndExtractImports(t, "javascript", "src/app.js",
			`import lodash from 'lodash';`)
		if len(imports) > 0 && imports[0].SourceFilePath != "src/app.js" {
			t.Errorf("SourceFilePath = %q, want src/app.js", imports[0].SourceFilePath)
		}
	})

	t.Run("deterministic order", func(t *testing.T) {
		src := `import a from 'alpha';
import b from 'beta';
import c from 'gamma';`
		imports1 := parseAndExtractImports(t, "javascript", "app.js", src)
		imports2 := parseAndExtractImports(t, "javascript", "app.js", src)
		if len(imports1) != len(imports2) {
			t.Fatal("non-deterministic count")
		}
		for i := range imports1 {
			if imports1[i].ImportName != imports2[i].ImportName {
				t.Errorf("order mismatch at %d: %q vs %q", i, imports1[i].ImportName, imports2[i].ImportName)
			}
		}
	})
}
