package authcontracts_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestNativeAuthenticationDoesNotImportTheCommunityModule(t *testing.T) {
	root := projectRoot(t)
	for _, directory := range []string{"app", "bootstrap", "config", "database", "routes", "tests"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, imported := range file.Imports {
				name, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					return err
				}
				if name == "github.com/arandu-io/framework/modules/auth" {
					t.Errorf("%s imports the community-module namespace", relative(root, path))
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", directory, err)
		}
	}
}

func TestCredentialVerificationHasNoSessionMutationSurface(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "app", "Services", "UserService.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parsing UserService: %v", err)
	}

	found := false
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "VerifyCredentials" {
			continue
		}
		found = true
		ast.Inspect(function.Body, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.Ident:
				if value.Name == "SessionGuard" {
					t.Errorf("VerifyCredentials names %s", value.Name)
				}
			case *ast.SelectorExpr:
				switch value.Sel.Name {
				case "Login", "Rotate", "Regenerate", "Start":
					t.Errorf("VerifyCredentials mutates session state through %s", value.Sel.Name)
				}
			}
			return true
		})
	}
	if !found {
		t.Fatal("UserService has no VerifyCredentials seam")
	}
}

func TestApplicationCodeDoesNotNameSessionGuard(t *testing.T) {
	root := projectRoot(t)
	for _, directory := range []string{"app", "bootstrap", "config"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(node ast.Node) bool {
				identifier, ok := node.(*ast.Ident)
				if ok && identifier.Name == "SessionGuard" {
					t.Errorf("%s names legacy SessionGuard", relative(root, path))
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", directory, err)
		}
	}
}

func TestSecondFactorPersistenceKeepsAtomicWritesTenantScoped(t *testing.T) {
	path := filepath.Join(projectRoot(t), "app", "Repositories", "TwoFactorRepository.go")
	tests := []struct {
		method    string
		fragments []string
		counts    map[string]int
	}{
		{
			method: "Find",
			fragments: []string{
				"FROM user_two_factor WHERE user_id = ? AND tenant_id = ?",
			},
		},
		{
			method: "Enrol",
			fragments: []string{
				"DELETE FROM user_two_factor WHERE user_id = ? AND tenant_id = ? AND confirmed_at IS NULL",
				"INSERT INTO user_two_factor (user_id, tenant_id, secret, confirmed_at, last_used_step, created_at)",
			},
		},
		{
			method: "Confirm",
			fragments: []string{
				"WHERE user_id = ? AND tenant_id = ? AND confirmed_at IS NULL",
			},
		},
		{
			method: "Disable",
			counts: map[string]int{
				"WHERE user_id = ? AND tenant_id = ?": 2,
			},
		},
		{
			method: "SpendStep",
			fragments: []string{
				"WHERE user_id = ? AND tenant_id = ? AND last_used_step < ?",
			},
		},
		{
			method: "ReplaceRecoveryCodes",
			fragments: []string{
				"DELETE FROM user_recovery_codes WHERE user_id = ? AND tenant_id = ?",
				"INSERT INTO user_recovery_codes (id, tenant_id, user_id, code_hash, used_at, created_at)",
			},
		},
		{
			method: "ConsumeRecoveryCode",
			fragments: []string{
				"WHERE user_id = ? AND tenant_id = ? AND used_at IS NULL",
				"WHERE id = ? AND tenant_id = ? AND used_at IS NULL",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.method, func(t *testing.T) {
			literals := methodStringLiterals(t, path, test.method)
			for _, fragment := range test.fragments {
				if !strings.Contains(literals, fragment) {
					t.Errorf("%s does not preserve the SQL contract %q", test.method, fragment)
				}
			}
			for fragment, want := range test.counts {
				if got := strings.Count(literals, fragment); got != want {
					t.Errorf("%s contains %q %d times, want %d", test.method, fragment, got, want)
				}
			}
		})
	}
}

func TestSecondFactorRequiredChecksTheReadGrantBeforeDelegating(t *testing.T) {
	path := filepath.Join(projectRoot(t), "app", "Repositories", "TwoFactorRepository.go")
	method := methodDeclaration(t, path, "Required")
	if len(method.Body.List) == 0 {
		t.Fatal("TwoFactorRepository.Required has no body")
	}

	checksGrant := false
	checksReadAction := false
	ast.Inspect(method.Body.List[0], func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || expressionName(call.Fun) != "grant.Check" {
			return true
		}
		checksGrant = true
		if len(call.Args) == 1 && expressionName(call.Args[0]) == "policies.ActionTwoFactorRead" {
			checksReadAction = true
		}
		return true
	})
	if !checksGrant || !checksReadAction {
		t.Fatal("TwoFactorRepository.Required does not start by checking ActionTwoFactorRead")
	}
}

func TestSecondFactorServicesReauthorizeTheLoadedEnrollment(t *testing.T) {
	path := filepath.Join(projectRoot(t), "app", "Services", "TwoFactorService.go")
	for _, methodName := range []string{"Confirm", "RegenerateRecoveryCodes"} {
		t.Run(methodName, func(t *testing.T) {
			method := methodDeclaration(t, path, methodName)
			rowPosition := token.NoPos
			authorized := map[string]bool{}
			ast.Inspect(method.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch expressionName(call.Fun) {
				case "s.repository.Find":
					rowPosition = call.Pos()
				case "security.Authorize":
					if rowPosition == token.NoPos || call.Pos() < rowPosition || len(call.Args) != 5 {
						return true
					}
					if expressionName(call.Args[4]) != "enrolment" {
						return true
					}
					authorized[expressionName(call.Args[3])] = true
				}
				return true
			})

			if rowPosition == token.NoPos {
				t.Fatal("the service does not load the enrollment")
			}
			for _, action := range []string{
				"policies.ActionTwoFactorRead",
				"policies.ActionTwoFactorManage",
			} {
				if !authorized[action] {
					t.Errorf("the loaded enrollment is not reauthorized for %s", action)
				}
			}
		})
	}
}

func projectRoot(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime did not report the test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
}

func relative(root, path string) string {
	name, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return name
}

func methodStringLiterals(t *testing.T, path, method string) string {
	t.Helper()

	function := methodDeclaration(t, path, method)
	var literals []string
	ast.Inspect(function.Body, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err != nil {
			t.Fatalf("unquoting a literal in %s: %v", method, err)
		}
		literals = append(literals, value)
		return true
	})
	return strings.Join(strings.Fields(strings.Join(literals, " ")), " ")
}

func methodDeclaration(t *testing.T, path, method string) *ast.FuncDecl {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", relative(projectRoot(t), path), err)
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != method || function.Recv == nil {
			continue
		}
		return function
	}
	t.Fatalf("%s has no %s method", relative(projectRoot(t), path), method)
	return nil
}

func expressionName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		prefix := expressionName(value.X)
		if prefix == "" {
			return value.Sel.Name
		}
		return prefix + "." + value.Sel.Name
	}
	return ""
}
