package lib

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strings"
)

// ReferencedAsset holds the information for an asset referenced
// by the user
type ReferencedAsset struct {
	Name      string
	AssetPath string
	Group     *Group
}

// Group holds information relating to a group
type Group struct {
	Name      string
	LocalPath string
	FullPath  string
}

// ReferencedAssets is a collection of assets referenced from a file
type ReferencedAssets struct {
	Caller      string
	PackageName string
	BaseDir     string
	Assets      []*ReferencedAsset
	Groups      []*Group
}

// HasAsset returns true if the given asset name has already been processed
// for this asset group
func (r *ReferencedAssets) HasAsset(name string) bool {
	for _, asset := range r.Assets {
		if asset.Name == name {
			return true
		}
	}
	return false
}

// GetReferencedAssets gets a list of referenced assets from the AST
func GetReferencedAssets(filenames []string) ([]*ReferencedAssets, error) {

	var result []*ReferencedAssets
	assetMap := make(map[string]*ReferencedAssets)

	groups := make(map[string]*Group)

	for _, filename := range filenames {
		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, filename, nil, parser.AllErrors)
		if err != nil {
			return nil, err
		}

		var packageName string

		// Normalise per directory imports
		var baseDir = filepath.Dir(filename)
		var thisAssetBundle = assetMap[baseDir]
		if thisAssetBundle == nil {
			thisAssetBundle = &ReferencedAssets{Caller: filename, BaseDir: baseDir}
			assetMap[baseDir] = thisAssetBundle
		}

		ast.Inspect(node, func(node ast.Node) bool {
			switch x := node.(type) {
			case *ast.File:
				packageName = x.Name.Name
				thisAssetBundle.PackageName = packageName

			case *ast.AssignStmt:
				thisAsset := ParseAssignment(x)
				if thisAsset != nil {
					objName := thisAsset.RHS.Obj
					if objName == "mewn" {
						switch thisAsset.RHS.Method {
						case "Group":
							baseDir := filepath.Dir(filename)
							fullPath, err := filepath.Abs(filepath.Join(baseDir, thisAsset.RHS.Path))
							if err != nil {
								return false
							}
							thisGroup := &Group{Name: thisAsset.LHS, LocalPath: thisAsset.RHS.Path, FullPath: fullPath}
							thisAssetBundle.Groups = append(thisAssetBundle.Groups, thisGroup)
							groups[thisAsset.LHS] = thisGroup
						case "String", "MustString", "Bytes", "MustBytes":
							newAsset := &ReferencedAsset{Name: thisAsset.RHS.Path, Group: nil, AssetPath: thisAsset.RHS.Path}
							thisAssetBundle.Assets = append(thisAssetBundle.Assets, newAsset)
						default:
							err = fmt.Errorf("unknown call to mewn.%s", thisAsset.RHS.Method)
							return false
						}
					} else {
						// Check if we have a call on a group
						group, exists := groups[objName]
						if exists {
							// We have a group call!
							newAsset := &ReferencedAsset{Name: thisAsset.RHS.Path, Group: group, AssetPath: thisAsset.RHS.Path}
							thisAssetBundle.Assets = append(thisAssetBundle.Assets, newAsset)
						}
					}
				}
			}
			return true
		})
		result = append(result, thisAssetBundle)
	}
	return result, nil
}

// AssignStmt holds data about an assignment
type AssignStmt struct {
	LHS string
	RHS *CallStmt
}

func (a *AssignStmt) String() string {
	return fmt.Sprintf("%s = %s", a.LHS, a.RHS)
}

// ParseAssignment parses an assignment statement
func ParseAssignment(astmt *ast.AssignStmt) *AssignStmt {
	var lhs string
	var result *AssignStmt

	if len(astmt.Lhs) == 1 && reflect.TypeOf(astmt.Lhs[0]).String() == "*ast.Ident" {
		lhs = astmt.Lhs[0].(*ast.Ident).String()
	}

	if len(astmt.Rhs) == 1 && reflect.TypeOf(astmt.Rhs[0]).String() == "*ast.CallExpr" {
		t := astmt.Rhs[0].(*ast.CallExpr)
		call := ParseCallExpr(t)
		if call != nil {
			result = &AssignStmt{LHS: lhs, RHS: call}
		}
	}

	return result
}

// CallStmt holds data about a call statement
type CallStmt struct {
	Obj    string
	Method string
	Path   string
}

func (c *CallStmt) String() string {
	return fmt.Sprintf("{ obj: '%s', method: '%s', path: '%s' }", c.Obj, c.Method, c.Path)
}

// ParseCallExpr parses a call expression for mewn related statements
func ParseCallExpr(callstmt *ast.CallExpr) *CallStmt {
	var result *CallStmt

	if len(callstmt.Args) != 1 {
		return nil
	}

	switch fn := callstmt.Fun.(type) {
	case *ast.SelectorExpr:
		if reflect.TypeOf(fn.X).String() != "*ast.Ident" {
			return nil
		}
		obj := fn.X.(*ast.Ident).String()

		if reflect.TypeOf(fn.Sel).String() != "*ast.Ident" {
			return nil
		}
		fnCallName := fn.Sel.String()

		if reflect.TypeOf(callstmt.Args[0]).String() != "*ast.BasicLit" {
			return nil
		}

		assetPath := strings.Replace(callstmt.Args[0].(*ast.BasicLit).Value, "\"", "", -1)

		result = &CallStmt{Obj: obj, Method: fnCallName, Path: assetPath}

	}
	return result
}
