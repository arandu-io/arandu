package authcontracts_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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

func TestAuthControllerPlaceholderUsesTheInstallerPackage(t *testing.T) {
	path := filepath.Join(projectRoot(t), "app", "Http", "Controllers", "Auth", "doc.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.PackageClauseOnly)
	if err != nil {
		t.Fatalf("parsing the auth controller placeholder: %v", err)
	}
	if file.Name.Name != "authui" {
		t.Fatalf("the auth controller placeholder declares package %s, want authui", file.Name.Name)
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

// TestTheAccountRulesAreNotCopiedIntoTheApplication keeps the account rules
// where they are written once: hesape/auth/account, which bootstrap/app.go
// builds over this application's users. A copy of them in app/ is a second
// set of rules that a fix to the package never reaches -- which is how nine
// projects came to carry nine diverging copies, two with a hole in them.
//
// What stays here is the user model, the user policy and the users
// migrations. The files the copy lived in must not come back, and no package
// under app/ may declare what they declared.
func TestTheAccountRulesAreNotCopiedIntoTheApplication(t *testing.T) {
	root := projectRoot(t)
	for _, file := range []string{
		"app/Services/UserService.go",
		"app/Services/TwoFactorService.go",
		"app/Repositories/TwoFactorRepository.go",
		"app/Models/TwoFactor.go",
		"app/Models/TwoFactorQuery.go",
		"app/Models/RecoveryCodeQuery.go",
		"app/Policies/TwoFactorPolicy.go",
		"app/Events/User.go",
		"database/migrations/20260828_0003_create_two_factor.go",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(file))); err == nil {
			t.Errorf("%s is back: the account rules are hesape/auth/account's, built in bootstrap/app.go", file)
		}
	}

	// The types the copy declared, anywhere under app/.
	copied := map[string]bool{
		"UserService": true, "NewUserService": true, "TwoFactorService": true, "NewTwoFactorService": true,
		"TwoFactorRepository": true, "NewTwoFactorRepository": true, "TwoFactor": true, "RecoveryCode": true,
		"TwoFactorPolicy": true,
	}
	// The event names it declared, in app/Events only: elsewhere the same
	// word names something else, such as the mail the sign-in screens send.
	events := map[string]bool{
		"UserRegistered": true, "EmailVerified": true, "PasswordReset": true, "PasswordChanged": true,
		"ProfileUpdated": true, "AccountClosed": true, "TwoFactorEnabled": true, "TwoFactorDisabled": true,
		"RecoveryCodesRegenerated": true, "RecoveryCodeUsed": true,
	}
	eventsDirectory := filepath.Join(root, "app", "Events")
	err := filepath.WalkDir(filepath.Join(root, "app"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, declaration := range file.Decls {
			for _, name := range declaredNames(declaration) {
				if copied[name] || events[name] && filepath.Dir(path) == eventsDirectory {
					t.Errorf("%s declares %s, which hesape/auth/account owns", relative(root, path), name)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking app: %v", err)
	}
}

// declaredNames are the package-level names one declaration introduces: a
// function that is not a method, a type, a constant or a variable.
func declaredNames(declaration ast.Decl) []string {
	var names []string
	switch value := declaration.(type) {
	case *ast.FuncDecl:
		if value.Recv == nil {
			names = append(names, value.Name.Name)
		}
	case *ast.GenDecl:
		for _, spec := range value.Specs {
			switch spec := spec.(type) {
			case *ast.TypeSpec:
				names = append(names, spec.Name.Name)
			case *ast.ValueSpec:
				for _, name := range spec.Names {
					names = append(names, name.Name)
				}
			}
		}
	}
	return names
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
