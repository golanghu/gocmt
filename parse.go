package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
)

// parseFile parses and modifies the input file if necessary. Returns AST represents of (new) source, a boolean
// to report whether the source file was modified, and any error if occurred.
func parseFile(fset *token.FileSet, filePath, template string) (af *ast.File, modified bool, err error) {
	af, err = parser.ParseFile(fset, filePath, nil, parser.ParseComments|parser.AllErrors)
	if err != nil {
		return
	}

	// Inject first comment to prevent nil comment map
	if len(af.Comments) == 0 {
		af.Comments = []*ast.CommentGroup{{List: []*ast.Comment{{Slash: -1, Text: "// gocmt"}}}}
		defer func() {
			// Remove the injected comment
			af.Comments = af.Comments[1:]
		}()
	}

	//commentTemplate := commentBase + template
	commentTemplate := template

	originalCommentSign := ""
	for _, c := range af.Comments {
		originalCommentSign += c.Text()
	}

	cmap := ast.NewCommentMap(fset, af, af.Comments)

	skipped := make(map[ast.Node]bool)
	ast.Inspect(af, func(n ast.Node) bool {
		switch typ := n.(type) {
		case *ast.FuncDecl:
			if skipped[typ] || !typ.Name.IsExported() {
				return true
			}
			addFuncDeclComment(typ, commentTemplate)
			cmap[typ] = appendCommentGroup(cmap[typ], typ.Doc)

			//case *ast.DeclStmt:
			//	skipped[typ.Decl] = true

			//case *ast.GenDecl:
			//	switch typ.Tok {
			//	case token.CONST, token.VAR:
			//		if !(typ.Lparen == token.NoPos && typ.Rparen == token.NoPos) {
			//			// if there's a () and parenComment is true, add comment for each sub entry
			//			if *parenComment {
			//				for _, spec := range typ.Specs {
			//					vs := spec.(*ast.ValueSpec)
			//					if !vs.Names[0].IsExported() {
			//						continue
			//					}
			//					addParenValueSpecComment(vs, commentTemplate)
			//					cmap[vs] = appendCommentGroup(cmap[vs], vs.Doc)
			//				}
			//				return true
			//			}
			//		}
			//
			//		// empty var block
			//		if len(typ.Specs) == 0 {
			//			return true
			//		}
			//
			//		vs := typ.Specs[0].(*ast.ValueSpec)
			//		if skipped[typ] || !vs.Names[0].IsExported() {
			//			return true
			//		}
			//		addValueSpecComment(typ, vs, commentTemplate)
			//
			//	case token.TYPE:
			//		ts := typ.Specs[0].(*ast.TypeSpec)
			//		if skipped[typ] || !ts.Name.IsExported() {
			//			return true
			//		}
			//		addTypeSpecComment(typ, ts, commentTemplate)
			//	default:
			//		return true
			//	}
			//	cmap[typ] = appendCommentGroup(cmap[typ], typ.Doc)
		}
		return true
	})

	// Rebuild comments
	af.Comments = cmap.Filter(af).Comments()

	currentCommentSign := ""
	for _, c := range af.Comments {
		currentCommentSign += c.Text()
	}

	modified = currentCommentSign != originalCommentSign
	return
}

// Helper function to get string representation of an ast.Expr safely.
func getTypeString(expr ast.Expr) string {
	if expr == nil {
		return "unknown"
	}
	var sb strings.Builder
	if err := printer.Fprint(&sb, token.NewFileSet(), expr); err != nil {
		return "unknown"
	}
	return sb.String()
}

func addFuncDeclComment(fd *ast.FuncDecl, commentTemplate string) {
	if fd.Doc == nil || strings.TrimSpace(fd.Doc.Text()) == fd.Name.Name {
		// 获取参数类型的描述
		paramDescriptions := []string{}
		if fd.Type.Params != nil { // 确保参数不为nil
			for _, param := range fd.Type.Params.List {
				paramType := getTypeString(param.Type) // 获取参数类型的字符串表示
				for _, name := range param.Names {
					paramDescriptions = append(paramDescriptions, fmt.Sprintf("%s %s", name.Name, paramType))
				}
			}
		}
		paramStr := strings.Join(paramDescriptions, ", ")

		// 获取返回值类型的描述
		returnDescriptions := []string{}
		if fd.Type.Results != nil { // 确保返回值不为nil
			for _, result := range fd.Type.Results.List {
				returnType := getTypeString(result.Type) // 获取返回值类型的字符串表示
				// 判断是否有名称
				if len(result.Names) > 0 {
					for _, name := range result.Names {
						returnDescriptions = append(returnDescriptions, fmt.Sprintf("%s %s", name.Name, returnType))
					}
				} else {
					// 如果没有被命名，则只返回类型
					returnDescriptions = append(returnDescriptions, returnType)
				}
			}
		}
		returnStr := strings.Join(returnDescriptions, ", ")

		// 创建注释文本
		commentTemplate = strings.ReplaceAll(commentTemplate, "\\n", "\n")
		newText := fmt.Sprintf(commentTemplate, fd.Name.Name, paramStr, returnStr)

		// 设置注释位置
		pos := fd.Pos() - token.Pos(1)
		if fd.Doc != nil {
			pos = fd.Doc.Pos()
			// 将新生成的注释添加到原注释后面，并保留原注释
			originalText := strings.TrimSpace(fd.Doc.Text())
			// 确保不会重复空格或换行
			if originalText != "" {
				newText = fmt.Sprintf("%s\n%s", originalText, newText)
			}
		}
		fd.Doc = &ast.CommentGroup{List: []*ast.Comment{{Slash: pos, Text: newText}}}
		return
	}

	if fd.Doc != nil && isLineComment(fd.Doc) && !hasCommentPrefix(fd.Doc, fd.Name.Name) {
		modifyComment(fd.Doc, fd.Name.Name)
		return
	}
}

func addValueSpecComment(gd *ast.GenDecl, vs *ast.ValueSpec, commentTemplate string) {
	if gd.Doc == nil || strings.TrimSpace(gd.Doc.Text()) == vs.Names[0].Name {
		text := fmt.Sprintf(commentTemplate, vs.Names[0].Name)
		pos := gd.Pos() - token.Pos(1)
		if gd.Doc != nil {
			pos = gd.Doc.Pos()
		}
		gd.Doc = &ast.CommentGroup{List: []*ast.Comment{{Slash: pos, Text: text}}}
		return
	}
	if gd.Doc != nil && isLineComment(gd.Doc) && !hasCommentPrefix(gd.Doc, vs.Names[0].Name) {
		modifyComment(gd.Doc, vs.Names[0].Name)
		return
	}
}

func addParenValueSpecComment(vs *ast.ValueSpec, commentTemplate string) {
	if vs.Doc == nil || strings.TrimSpace(vs.Doc.Text()) == vs.Names[0].Name {
		commentTemplate = strings.Replace(commentTemplate, commentBase, commentIndentedBase, 1)
		text := fmt.Sprintf(commentTemplate, vs.Names[0].Name)
		pos := vs.Pos() - token.Pos(1)
		if vs.Doc != nil {
			pos = vs.Doc.Pos()
		}
		vs.Doc = &ast.CommentGroup{List: []*ast.Comment{{Slash: pos, Text: text}}}
		return
	}
	if vs.Doc != nil && isLineComment(vs.Doc) && !hasCommentPrefix(vs.Doc, vs.Names[0].Name) {
		modifyComment(vs.Doc, vs.Names[0].Name)
		return
	}
}

func addTypeSpecComment(gd *ast.GenDecl, ts *ast.TypeSpec, commentTemplate string) {
	if gd.Doc == nil || strings.TrimSpace(gd.Doc.Text()) == ts.Name.Name {
		text := fmt.Sprintf(commentTemplate, ts.Name.Name)
		pos := gd.Pos() - token.Pos(1)
		if gd.Doc != nil {
			pos = gd.Doc.Pos()
		}
		gd.Doc = &ast.CommentGroup{List: []*ast.Comment{{Slash: pos, Text: text}}}
		return
	}
	if gd.Doc != nil && isLineComment(gd.Doc) && !hasCommentPrefix(gd.Doc, ts.Name.Name) {
		modifyComment(gd.Doc, ts.Name.Name)
		return
	}
}

func modifyComment(comment *ast.CommentGroup, prefix string) {
	//commentTemplate := commentBase + *template
	commentTemplate := *template

	first := comment.List[0].Text
	if strings.HasPrefix(first, "//") && !strings.HasPrefix(first, "// ") {
		text := fmt.Sprintf(commentTemplate, prefix)
		comment.List = append([]*ast.Comment{{Text: text, Slash: comment.Pos()}}, comment.List...)
		return
	}
	first = strings.TrimPrefix(first, "// ")
	first = fmt.Sprintf(commentBase+"%s", prefix, first)
	comment.List[0].Text = first
}
