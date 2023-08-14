package rewrite

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/dave/dst/dstutil"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"
)

func BuildRulePairs(rules []string) []RulePair {
	var rulePairs []RulePair
	for _, v := range rules {
		in, out, _ := strings.Cut(v, "->")
		rulePairs = append(rulePairs, RulePair{
			in:  strings.TrimSpace(in),
			out: strings.TrimSpace(out),
		})
	}
	return rulePairs
}

func ParseRuleFile(ruleFile string) ([]string, []string, error) {
	f, err := os.Open(ruleFile)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Split(bufio.ScanLines)
	var vars []string
	var rules []string
	type stage int
	const (
		inVar  stage = 1
		inRule stage = 2
	)
	var curStage stage
	for s.Scan() {
		curLine := strings.TrimSpace(s.Text())
		switch curLine {
		case "":
			// ignore empty lines
		case "vars:":
			curStage = inVar
		case "rules:":
			curStage = inRule
		default:
			switch curStage {
			case inVar:
				vars = append(vars, curLine)
			case inRule:
				rules = append(rules, curLine)
			}
		}
	}
	return vars, rules, nil
}

type RulePair struct {
	in  string
	out string
}

func BuildPackageMap(packages []string) map[string]string {
	out := map[string]string{}
	for _, v := range packages {
		imp, pkg, _ := strings.Cut(v, " ")
		out[imp] = pkg
	}
	return out
}

func Process(varTypes []string, rules []RulePair, sampleCode string) ([]byte, error) {
	ri, err := processInput(varTypes, rules)
	if err != nil {
		panic(err)
	}

	deco := decorator.NewDecorator(token.NewFileSet())
	sc, err := deco.Parse(sampleCode)
	if err != nil {
		panic(err)
	}

	applyRules(sc, ri)

	//for k, v := range packages {
	//	sc.Imports = append(sc.Imports, &dst.ImportSpec{
	//		Path: &dst.BasicLit{Kind: token.STRING, Value: fmt.Sprintf(`"%s"`, k)},
	//		Name: &dst.Ident{Name: v},
	//	})
	//}
	sc.Decls = append([]dst.Decl{
		&dst.GenDecl{
			Tok: token.IMPORT,
			Specs: []dst.Spec{
				&dst.ImportSpec{
					Path: &dst.BasicLit{
						Kind:  token.STRING,
						Value: fmt.Sprintf(`"%s"`, "github.com/jonbodner/orchestrion/instrument"),
					},
				},
			},
			Decs: dst.GenDeclDecorations{
				NodeDecs: dst.NodeDecs{
					Before: dst.NewLine,
					Start:  dst.Decorations{dd_startinstrument},
					After:  dst.NewLine,
					End:    dst.Decorations{"\n", dd_endinstrument},
				},
			},
		},
	}, sc.Decls...)
	out, err := printResult(sc)
	return out, err
}

func printResult(sc *dst.File) ([]byte, error) {
	res := decorator.NewRestorer()
	var out bytes.Buffer
	err := res.Fprint(&out, sc)
	if err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}

func processInput(varTypes []string, rulePairs []RulePair) (RuleInfo, error) {
	typesMap := map[string]dst.Node{}
	newDecorator := decorator.NewDecorator(token.NewFileSet())
	for _, v := range varTypes {
		name, typ, _ := strings.Cut(v, " ")
		expr, err := parser.ParseExpr(typ)
		if err != nil {
			return RuleInfo{}, err
		}
		newNode, err := newDecorator.DecorateNode(expr)
		if err != nil {
			return RuleInfo{}, err
		}
		typesMap[name] = maybeFixNode(newNode)
	}

	var rules []Rule
	for _, v := range rulePairs {
		p1, err := parser.ParseExpr(v.in)
		if err != nil {
			return RuleInfo{}, err
		}
		dp1, err := newDecorator.DecorateNode(p1)
		if err != nil {
			return RuleInfo{}, err
		}
		p2, err := parser.ParseExpr(v.out)
		if err != nil {
			return RuleInfo{}, err
		}
		dp2, err := newDecorator.DecorateNode(p2)
		if err != nil {
			return RuleInfo{}, err
		}

		funcName, params := buildPreInfo(dp1)
		toFuncName, toParams := buildPostInfo(dp2)
		paramMap := buildParamMap(params, toParams)
		rules = append(rules, Rule{
			inFuncName:  funcName,
			inParams:    params,
			outFuncName: toFuncName,
			outParams:   toParams,
			paramMap:    paramMap,
		})
	}

	ri := RuleInfo{
		rules:    rules,
		typesMap: typesMap,
	}

	return ri, nil
}

var fixedNodes = map[string]dst.Node{
	"string": &dst.BasicLit{Kind: token.STRING},
}

func maybeFixNode(node dst.Node) dst.Node {
	if identNode, ok := node.(*dst.Ident); ok {
		if outNode, ok := fixedNodes[identNode.Name]; ok {
			return outNode
		}
	}
	return node
}

type Rule struct {
	inFuncName  string
	inParams    []string
	outFuncName string
	outParams   []dst.Expr
	paramMap    ParamMap
}

type RuleInfo struct {
	rules    []Rule
	typesMap map[string]dst.Node
}

func applyRules(sc dst.Node, ri RuleInfo) {
	dstutil.Apply(sc, func(cursor *dstutil.Cursor) bool {
		ceCurNode, ok := cursor.Node().(*dst.CallExpr)
		if !ok {
			return true
		}
		ceCurNodeFunc, ok := ceCurNode.Fun.(*dst.SelectorExpr)
		if !ok {
			return true
		}
		// check every rule
		for _, rule := range ri.rules {
			applied := checkApplyRule(ceCurNodeFunc, rule, ceCurNode, ri)
			if applied {
				return false
			}
		}
		return true
	}, nil)
}

func checkApplyRule(ceCurNodeFunc *dst.SelectorExpr, rule Rule, ceCurNode *dst.CallExpr, ri RuleInfo) bool {
	// check the function name, the number of args, and the types
	if ceCurNodeFunc.Sel.Name != rule.inFuncName ||
		len(ceCurNode.Args) != len(rule.inParams) ||
		!match(ceCurNode.Args, rule.inParams, ri.typesMap) {
		return false
	}
	// convert!
	// fix name
	ceCurNodeFunc.Sel.Name = rule.outFuncName
	// walk the parameters, find the ones with matching names, use them to reassign parameters
	newArgs := make([]dst.Expr, len(ceCurNode.Args))
	for i, argToPlace := range ceCurNode.Args {
		newLocation := rule.paramMap.positions[i]
		if newLocation == nil {
			// this means that an incoming parameter isn't used in the output...
			continue
		}
		toParam := dst.Clone(rule.outParams[newLocation[0]]).(dst.Expr)
		newArgs[newLocation[0]] = buildResultExpr(argToPlace, newLocation, toParam)
	}
	ceCurNode.Args = newArgs
	// wrap with comments
	ceCurNode.Decs = dst.CallExprDecorations{
		NodeDecs: dst.NodeDecs{
			Before: dst.NewLine,
			Start:  dst.Decorations{dd_startinstrument},
			After:  dst.NewLine,
			End:    dst.Decorations{"\n", dd_endinstrument},
		}}
	return true
}

func match(args []dst.Expr, params []string, typesMap map[string]dst.Node) bool {
	for i, arg := range args {
		curType, ok := typesMap[params[i]]
		if !ok {
			return false
		}
		switch arg := arg.(type) {
		case *dst.BasicLit:
			curType, ok := curType.(*dst.BasicLit)
			if !ok {
				return false
			}
			return curType.Kind == arg.Kind
		case *dst.Ident:
			switch arg.Obj.Kind {
			case dst.Var:
				kind := arg.Obj.Decl.(*dst.ValueSpec).Values[0].(*dst.BasicLit).Kind
				curTypeLit, ok := curType.(*dst.BasicLit)
				if !ok {
					return false
				}
				if curTypeLit.Kind != kind {
					return false
				}
			case dst.Fun:
				typ := arg.Obj.Decl.(*dst.FuncDecl).Type
				curTypeFunc, ok := curType.(*dst.FuncType)
				if !ok {
					return false
				}
				// check input params
				if typ.Params != nil {
					for i := range typ.Params.List {
						curCompareParam := curTypeFunc.Params.List[i]
						curParam := typ.Params.List[i]
						if !equalField(curCompareParam.Type, curParam.Type) {
							return false
						}
					}
				}
				// check return values
				if typ.Results != nil {
					for i := range typ.Results.List {
						curCompareParam := curTypeFunc.Results.List[i]
						curParam := typ.Results.List[i]
						if !equalField(curCompareParam.Type, curParam.Type) {
							return false
						}
					}
				}
				// check type params
				if typ.TypeParams != nil {
					for i = range typ.TypeParams.List {
						curCompareParam := curTypeFunc.TypeParams.List[i]
						curParam := typ.TypeParams.List[i]
						if !equalField(curCompareParam.Type, curParam.Type) {
							return false
						}
					}
				}
			}
		default:
			fmt.Println(reflect.TypeOf(arg))
		}
	}
	return true
}

func equalField(curCompareParamType dst.Expr, curParamType dst.Expr) bool {
	switch compSel := curCompareParamType.(type) {
	case *dst.SelectorExpr:
		sel, ok := curParamType.(*dst.SelectorExpr)
		if !ok {
			return false
		}
		if compSel.X.(*dst.Ident).Name != sel.X.(*dst.Ident).Name || compSel.Sel.Name != sel.Sel.Name {
			return false
		}
	case *dst.StarExpr:
		sel, ok := curParamType.(*dst.StarExpr)
		if !ok {
			return false
		}
		return equalField(compSel.X, sel.X)
	default:
		fmt.Println(curCompareParamType)
		fmt.Println(curParamType)
	}
	return true
}

const (
	dd_startinstrument = "//dd:startinstrument"
	dd_endinstrument   = "//dd:endinstrument"
)

func buildResultExpr(argToPlace dst.Expr, locations []int, toParam dst.Expr) dst.Expr {
	switch tp := toParam.(type) {
	case *dst.Ident:
		// if you're just an identifier, then found the location, put the value here
		// also len(newLocation) should be 1
		if len(locations) != 1 {
			fmt.Fprintln(os.Stderr, "this should have been length 1...", locations)
		}
		return argToPlace
	case *dst.CallExpr:
		// if you're a function call, then need to recurse (and there should be > 1 value in newLocation)
		if len(locations) == 0 {
			fmt.Fprintln(os.Stderr, "this should have been length > 0...", locations)
		}
		newLocations := locations[1:]
		nextParam := tp.Args[newLocations[0]]
		tp.Args[newLocations[0]] = buildResultExpr(argToPlace, newLocations, nextParam)
		return tp
	}
	return nil
}

func buildPreInfo(dp1 dst.Node) (string, []string) {
	cedb1 := dp1.(*dst.CallExpr)
	funcName := cedb1.Fun.(*dst.SelectorExpr).Sel.Name
	params := make([]string, len(cedb1.Args))
	for i, v := range cedb1.Args {
		params[i] = v.(*dst.Ident).Name
	}
	return funcName, params
}

func buildPostInfo(dp2 dst.Node) (string, []dst.Expr) {
	cedb2 := dp2.(*dst.CallExpr)
	funcName := cedb2.Fun.(*dst.SelectorExpr).Sel.Name
	return funcName, cedb2.Args
}

type ParamMap struct {
	positions map[int][]int
}

func buildParamMap(inParams []string, outParams []dst.Expr) ParamMap {
	//from in pos to out pos, and outpos is a slice of positions so it can be nested in function calls
	out := ParamMap{positions: map[int][]int{}}
	for i, v := range inParams {
		location := buildLocation(v, outParams)
		out.positions[i] = location
	}
	return out
}

func buildLocation(ident string, args []dst.Expr) []int {
	for j := 0; j < len(args); j++ {
		switch p := args[j].(type) {
		case *dst.Ident:
			if p.Name == ident {
				return []int{j}
			}
		case *dst.CallExpr:
			result := buildLocation(ident, p.Args)
			if result == nil {
				return nil
			}
			return append([]int{j}, result...)
		}
	}
	return nil
}
