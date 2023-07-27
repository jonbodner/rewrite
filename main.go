package main

import (
	"bytes"
	"fmt"
	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/dave/dst/decorator/resolver/guess"
	"github.com/dave/dst/dstutil"
	"go/parser"
	"go/token"
	"os"
)

func main() {
	part1 := `http.HandleFunc(path, handlerFunc)`
	part2 := `http.NewHandleFunc(path, orchestrion.Wrap(handlerFunc))`
	sampleCode := `package main


func main() {
	http.HandleFunc("/hi", func(rw http.ResponseWriter, r *http.Request) {
		rw.Write([]byte("hello"))
	})

	http.HandleFunc("/bye", myHandlerFunc)

	http.ListenAndServe(":8080", nil)
}

func myHandlerFunc(rw http.ResponseWriter, r *http.Request) {
		rw.Write([]byte("goodbye"))
}
`
	p1, err := parser.ParseExpr(part1)
	if err != nil {
		panic(err)
	}
	dp1, err := decorator.NewDecorator(token.NewFileSet()).DecorateNode(p1)
	if err != nil {
		panic(err)
	}
	p2, err := parser.ParseExpr(part2)
	if err != nil {
		panic(err)
	}
	dp2, err := decorator.NewDecorator(token.NewFileSet()).DecorateNode(p2)
	if err != nil {
		panic(err)
	}
	sc, err := decorator.Parse(sampleCode)
	if err != nil {
		panic(err)
	}

	funcName, params := buildPreInfo(dp1)
	toFuncName, toParams := buildPostInfo(dp2)
	paramMap := buildParamMap(params, toParams)

	fmt.Println(paramMap)
	dstutil.Apply(sc, func(cursor *dstutil.Cursor) bool {
		curNode := cursor.Node()
		if ceCurNode, ok := curNode.(*dst.CallExpr); ok {
			if ceCurNodeFunc, ok := ceCurNode.Fun.(*dst.SelectorExpr); ok {
				if ceCurNodeFunc.Sel.Name == funcName && len(ceCurNode.Args) == len(params) {
					// convert!
					// fix name
					ceCurNodeFunc.Sel.Name = toFuncName
					// walk the parameters, find the ones with matching names, use them to reassign parameters
					newArgs := make([]dst.Expr, len(ceCurNode.Args))
					for i, argToPlace := range ceCurNode.Args {
						newLocation := paramMap.positions[i]
						if newLocation == nil {
							// this means that an incoming parameter isn't used in the output...
							continue
						}
						toParam := dup(toParams[newLocation[0]])
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
					return false
				}
			}
		}
		return true
	}, nil)

	res := decorator.NewRestorerWithImports("sample.go", guess.New())
	var out bytes.Buffer
	err = res.Fprint(&out, sc)
	if err != nil {
		panic(err)
	}
	fmt.Println(out.String())
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

func dup(in dst.Expr) dst.Expr {
	switch in := in.(type) {
	case *dst.Ident:
		return &dst.Ident{Name: in.Name, Path: in.Path}
	case *dst.CallExpr:
		newArgs := make([]dst.Expr, len(in.Args))
		for i, v := range in.Args {
			newArgs[i] = dup(v)
		}
		return &dst.CallExpr{
			Fun:      dup(in.Fun),
			Args:     newArgs,
			Ellipsis: in.Ellipsis,
		}
	case *dst.SelectorExpr:
		return &dst.SelectorExpr{
			X:   dup(in.X),
			Sel: dup(in.Sel).(*dst.Ident),
		}
	case *dst.IndexExpr:
		return &dst.IndexExpr{
			X:     dup(in.X),
			Index: dup(in.Index),
		}
	case *dst.BasicLit:
		return &dst.BasicLit{Kind: in.Kind, Value: in.Value}
	default:
		return &dst.BasicLit{Kind: token.STRING, Value: "unknown"}
	}
}

func buildPreInfo(dp1 dst.Node) (string, []string) {
	cedb1 := dp1.(*dst.CallExpr)
	fmt.Println(cedb1.Fun)
	funcName := cedb1.Fun.(*dst.SelectorExpr).Sel.Name
	params := make([]string, len(cedb1.Args))
	for i, v := range cedb1.Args {
		params[i] = v.(*dst.Ident).Name
	}
	return funcName, params
}

func buildPostInfo(dp2 dst.Node) (string, []dst.Expr) {
	cedb2 := dp2.(*dst.CallExpr)
	fmt.Println(cedb2.Fun)
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
