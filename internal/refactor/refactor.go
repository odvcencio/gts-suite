package refactor

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gts-suite/internal/model"
	"gts-suite/internal/query"
)

type Options struct {
	Write           bool
	UpdateCallsites bool
}

type Edit struct {
	File     string `json:"file"`
	Kind     string `json:"kind"`
	Category string `json:"category"`
	OldName  string `json:"old_name"`
	NewName  string `json:"new_name"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Offset   int    `json:"offset"`
	Applied  bool   `json:"applied"`
	Skipped  bool   `json:"skipped,omitempty"`
	SkipNote string `json:"skip_note,omitempty"`
}

type Report struct {
	Root             string `json:"root"`
	Selector         string `json:"selector"`
	NewName          string `json:"new_name"`
	Write            bool   `json:"write"`
	UpdateCallsites  bool   `json:"update_callsites"`
	MatchCount       int    `json:"match_count"`
	PlannedEdits     int    `json:"planned_edits"`
	PlannedDeclEdits int    `json:"planned_declaration_edits"`
	PlannedUseEdits  int    `json:"planned_callsite_edits"`
	AppliedEdits     int    `json:"applied_edits"`
	ChangedFiles     int    `json:"changed_files"`
	Edits            []Edit `json:"edits,omitempty"`
}

func RenameDeclarations(idx *model.Index, selector query.Selector, newName string, opts Options) (Report, error) {
	if idx == nil {
		return Report{}, fmt.Errorf("index is nil")
	}
	newName = strings.TrimSpace(newName)
	if !token.IsIdentifier(newName) {
		return Report{}, fmt.Errorf("new name %q is not a valid Go identifier", newName)
	}

	report := Report{
		Root:            idx.Root,
		Selector:        selector.Raw,
		NewName:         newName,
		Write:           opts.Write,
		UpdateCallsites: opts.UpdateCallsites,
	}

	targetsByFile := make(map[string][]model.Symbol)
	for _, file := range idx.Files {
		for _, symbol := range file.Symbols {
			if !selector.Match(symbol) {
				continue
			}
			report.MatchCount++
			if !supportsDeclarationRename(symbol.Kind) {
				report.Edits = append(report.Edits, Edit{
					File:     symbol.File,
					Kind:     symbol.Kind,
					Category: "declaration",
					OldName:  symbol.Name,
					NewName:  newName,
					Line:     symbol.StartLine,
					Column:   1,
					Skipped:  true,
					SkipNote: "unsupported kind for declaration rename",
				})
				continue
			}
			if symbol.Name == newName {
				report.Edits = append(report.Edits, Edit{
					File:     symbol.File,
					Kind:     symbol.Kind,
					Category: "declaration",
					OldName:  symbol.Name,
					NewName:  newName,
					Line:     symbol.StartLine,
					Column:   1,
					Skipped:  true,
					SkipNote: "already has target name",
				})
				continue
			}
			targetsByFile[symbol.File] = append(targetsByFile[symbol.File], symbol)
		}
	}

	groups, err := buildPackageGroups(idx, targetsByFile, opts.UpdateCallsites)
	if err != nil {
		return report, err
	}

	plannedByFile := map[string][]Edit{}
	absByFile := map[string]string{}
	sourceByFile := map[string][]byte{}

	for _, group := range groups {
		edits, skips, err := planGroupEdits(group, newName, opts.UpdateCallsites)
		if err != nil {
			return report, err
		}
		report.Edits = append(report.Edits, skips...)
		for _, edit := range edits {
			plannedByFile[edit.File] = append(plannedByFile[edit.File], edit)
			absByFile[edit.File] = group.absByRel[edit.File]
			sourceByFile[edit.File] = group.sourceByRel[edit.File]
			if edit.Category == "declaration" {
				report.PlannedDeclEdits++
			} else {
				report.PlannedUseEdits++
			}
		}
	}
	report.PlannedEdits = report.PlannedDeclEdits + report.PlannedUseEdits

	fileKeys := make([]string, 0, len(plannedByFile))
	for file := range plannedByFile {
		fileKeys = append(fileKeys, file)
	}
	sort.Strings(fileKeys)

	editIndexesByFile := map[string][]int{}
	for _, relPath := range fileKeys {
		edits := append([]Edit(nil), plannedByFile[relPath]...)
		sort.Slice(edits, func(i, j int) bool {
			if edits[i].Offset == edits[j].Offset {
				return edits[i].Category < edits[j].Category
			}
			return edits[i].Offset < edits[j].Offset
		})

		for _, edit := range edits {
			report.Edits = append(report.Edits, edit)
			editIndexesByFile[relPath] = append(editIndexesByFile[relPath], len(report.Edits)-1)
		}

		if !opts.Write || len(edits) == 0 {
			continue
		}

		updated, applied, err := applySourceEdits(sourceByFile[relPath], edits)
		if err != nil {
			return report, err
		}
		if applied == 0 {
			continue
		}
		if err := os.WriteFile(absByFile[relPath], updated, 0o644); err != nil {
			return report, err
		}
		report.ChangedFiles++
		report.AppliedEdits += applied
		for _, idx := range editIndexesByFile[relPath] {
			report.Edits[idx].Applied = true
		}
	}

	sort.Slice(report.Edits, func(i, j int) bool {
		if report.Edits[i].File == report.Edits[j].File {
			if report.Edits[i].Line == report.Edits[j].Line {
				if report.Edits[i].Column == report.Edits[j].Column {
					return report.Edits[i].Category < report.Edits[j].Category
				}
				return report.Edits[i].Column < report.Edits[j].Column
			}
			return report.Edits[i].Line < report.Edits[j].Line
		}
		return report.Edits[i].File < report.Edits[j].File
	})

	return report, nil
}

func supportsDeclarationRename(kind string) bool {
	switch kind {
	case "function_definition", "method_definition", "type_definition":
		return true
	default:
		return false
	}
}

type declKey struct {
	kind string
	name string
	line int
}

type packageGroup struct {
	dir         string
	packageName string
	targets     []model.Symbol
	fset        *token.FileSet
	astByRel    map[string]*ast.File
	absByRel    map[string]string
	sourceByRel map[string][]byte
	relByAbs    map[string]string
	info        *types.Info
}

func buildPackageGroups(idx *model.Index, targetsByFile map[string][]model.Symbol, withTypeInfo bool) ([]*packageGroup, error) {
	if len(targetsByFile) == 0 {
		return nil, nil
	}

	targetDirs := map[string]bool{}
	for file := range targetsByFile {
		targetDirs[filepath.ToSlash(filepath.Dir(filepath.Clean(file)))] = true
	}

	groups := make([]*packageGroup, 0, len(targetDirs))
	for dir := range targetDirs {
		fset := token.NewFileSet()
		buckets := map[string]*packageGroup{}

		for _, fileSummary := range idx.Files {
			fileDir := filepath.ToSlash(filepath.Dir(filepath.Clean(fileSummary.Path)))
			if fileDir != dir {
				continue
			}

			absPath := filepath.Join(idx.Root, filepath.FromSlash(fileSummary.Path))
			source, err := os.ReadFile(absPath)
			if err != nil {
				return nil, err
			}

			parsed, err := parser.ParseFile(fset, absPath, source, parser.ParseComments)
			if err != nil {
				return nil, err
			}

			packageName := parsed.Name.Name
			group, ok := buckets[packageName]
			if !ok {
				group = &packageGroup{
					dir:         dir,
					packageName: packageName,
					fset:        fset,
					astByRel:    map[string]*ast.File{},
					absByRel:    map[string]string{},
					sourceByRel: map[string][]byte{},
					relByAbs:    map[string]string{},
				}
				buckets[packageName] = group
			}

			group.astByRel[fileSummary.Path] = parsed
			group.absByRel[fileSummary.Path] = absPath
			group.sourceByRel[fileSummary.Path] = source
			group.relByAbs[filepath.Clean(absPath)] = fileSummary.Path
		}

		for relPath, symbols := range targetsByFile {
			fileDir := filepath.ToSlash(filepath.Dir(filepath.Clean(relPath)))
			if fileDir != dir {
				continue
			}
			for _, group := range buckets {
				if _, ok := group.astByRel[relPath]; ok {
					group.targets = append(group.targets, symbols...)
					break
				}
			}
		}

		for _, group := range buckets {
			if len(group.targets) == 0 {
				continue
			}
			if withTypeInfo {
				info, err := typeCheckGroup(group)
				if err != nil {
					return nil, err
				}
				group.info = info
			}
			groups = append(groups, group)
		}
	}

	sort.Slice(groups, func(i, j int) bool {
		if groups[i].dir == groups[j].dir {
			return groups[i].packageName < groups[j].packageName
		}
		return groups[i].dir < groups[j].dir
	})
	return groups, nil
}

func typeCheckGroup(group *packageGroup) (*types.Info, error) {
	files := make([]*ast.File, 0, len(group.astByRel))
	for _, file := range group.astByRel {
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool {
		left := group.fset.Position(files[i].Pos()).Filename
		right := group.fset.Position(files[j].Pos()).Filename
		return left < right
	})

	info := &types.Info{
		Defs: map[*ast.Ident]types.Object{},
		Uses: map[*ast.Ident]types.Object{},
	}
	config := &types.Config{
		Importer: importer.Default(),
		Error:    func(error) {},
	}
	_, _ = config.Check(group.packageName, group.fset, files, info)
	return info, nil
}

func planGroupEdits(group *packageGroup, newName string, withCallsites bool) ([]Edit, []Edit, error) {
	planned := make([]Edit, 0, len(group.targets)*2)
	skipped := make([]Edit, 0, 4)
	seen := map[string]bool{}

	for _, target := range group.targets {
		fileAST, ok := group.astByRel[target.File]
		if !ok {
			skipped = append(skipped, Edit{
				File:     target.File,
				Kind:     target.Kind,
				Category: "declaration",
				OldName:  target.Name,
				NewName:  newName,
				Line:     target.StartLine,
				Column:   1,
				Skipped:  true,
				SkipNote: "file not loaded in package context",
			})
			continue
		}

		declIdent := findDeclarationIdent(group.fset, fileAST, target)
		if declIdent == nil {
			skipped = append(skipped, Edit{
				File:     target.File,
				Kind:     target.Kind,
				Category: "declaration",
				OldName:  target.Name,
				NewName:  newName,
				Line:     target.StartLine,
				Column:   1,
				Skipped:  true,
				SkipNote: "declaration node not found by structural key",
			})
			continue
		}

		declPos := group.fset.Position(declIdent.Pos())
		declEdit := Edit{
			File:     target.File,
			Kind:     target.Kind,
			Category: "declaration",
			OldName:  declIdent.Name,
			NewName:  newName,
			Line:     declPos.Line,
			Column:   declPos.Column,
			Offset:   declPos.Offset,
		}
		key := editKey(declEdit)
		if !seen[key] {
			planned = append(planned, declEdit)
			seen[key] = true
		}

		if !withCallsites {
			continue
		}
		if group.info == nil {
			skipped = append(skipped, Edit{
				File:     target.File,
				Kind:     target.Kind,
				Category: "callsite",
				OldName:  target.Name,
				NewName:  newName,
				Line:     target.StartLine,
				Column:   1,
				Skipped:  true,
				SkipNote: "type information unavailable",
			})
			continue
		}

		object := group.info.Defs[declIdent]
		if object == nil {
			skipped = append(skipped, Edit{
				File:     target.File,
				Kind:     target.Kind,
				Category: "callsite",
				OldName:  target.Name,
				NewName:  newName,
				Line:     target.StartLine,
				Column:   1,
				Skipped:  true,
				SkipNote: "failed to resolve declaration object",
			})
			continue
		}

		for ident, useObj := range group.info.Uses {
			if useObj != object {
				continue
			}
			pos := group.fset.Position(ident.Pos())
			relPath := group.relByAbs[filepath.Clean(pos.Filename)]
			if relPath == "" {
				continue
			}
			callEdit := Edit{
				File:     relPath,
				Kind:     target.Kind,
				Category: "callsite",
				OldName:  ident.Name,
				NewName:  newName,
				Line:     pos.Line,
				Column:   pos.Column,
				Offset:   pos.Offset,
			}
			if callEdit.OldName == newName {
				continue
			}
			key := editKey(callEdit)
			if seen[key] {
				continue
			}
			planned = append(planned, callEdit)
			seen[key] = true
		}
	}

	sort.Slice(planned, func(i, j int) bool {
		if planned[i].File == planned[j].File {
			if planned[i].Offset == planned[j].Offset {
				return planned[i].Category < planned[j].Category
			}
			return planned[i].Offset < planned[j].Offset
		}
		return planned[i].File < planned[j].File
	})
	return planned, skipped, nil
}

func findDeclarationIdent(fset *token.FileSet, file *ast.File, symbol model.Symbol) *ast.Ident {
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			kind := "function_definition"
			if d.Recv != nil && len(d.Recv.List) > 0 {
				kind = "method_definition"
			}
			if kind != symbol.Kind {
				continue
			}
			if d.Name.Name != symbol.Name {
				continue
			}
			if fset.Position(d.Pos()).Line != symbol.StartLine {
				continue
			}
			return d.Name
		case *ast.GenDecl:
			if symbol.Kind != "type_definition" || d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if typeSpec.Name.Name != symbol.Name {
					continue
				}
				if fset.Position(typeSpec.Pos()).Line != symbol.StartLine {
					continue
				}
				return typeSpec.Name
			}
		}
	}
	return nil
}

func editKey(edit Edit) string {
	return edit.File + ":" + fmt.Sprintf("%d", edit.Offset)
}

func applySourceEdits(source []byte, edits []Edit) ([]byte, int, error) {
	if len(edits) == 0 {
		return source, 0, nil
	}

	sort.Slice(edits, func(i, j int) bool {
		return edits[i].Offset > edits[j].Offset
	})

	updated := append([]byte(nil), source...)
	applied := 0
	for _, edit := range edits {
		if edit.Offset < 0 || edit.Offset+len(edit.OldName) > len(updated) {
			return nil, 0, fmt.Errorf("invalid edit offset %d for %s", edit.Offset, edit.File)
		}
		current := string(updated[edit.Offset : edit.Offset+len(edit.OldName)])
		if current != edit.OldName {
			return nil, 0, fmt.Errorf(
				"source mismatch at %s:%d:%d: expected %q, found %q",
				edit.File,
				edit.Line,
				edit.Column,
				edit.OldName,
				current,
			)
		}

		updated = append(updated[:edit.Offset], append([]byte(edit.NewName), updated[edit.Offset+len(edit.OldName):]...)...)
		applied++
	}
	return updated, applied, nil
}
