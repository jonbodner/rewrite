package main

import (
	"bytes"
	"fmt"
	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/dave/dst/decorator/resolver/guess"
	"github.com/dave/dst/dstutil"
	"reflect"
	"strings"
)

func main() {
	const src = `package main

import "net/http"

func main() {
	http.HandleFunc("/hi", func(rw http.ResponseWriter, r *http.Request) {
		rw.Write([]byte("hello"))
	})

	http.ListenAndServe(":8080", nil)
}
`

	sc, _ := decorator.Parse(src)

	v := simpleVisitor{}

	dstutil.Apply(sc, v.pre, v.post)

	res := decorator.NewRestorerWithImports("main.go", guess.New())
	var out bytes.Buffer
	res.Fprint(&out, sc)
	fmt.Println(out.String())
}

type simpleVisitor struct {
	depth int
}

func (v *simpleVisitor) pre(cursor *dstutil.Cursor) bool {
	v.depth++
	curNode := cursor.Node()
	var curNodeType reflect.Type
	if curNode != nil {
		curNodeType = reflect.ValueOf(curNode).Elem().Type()
	}
	var info string
	switch curNode := curNode.(type) {
	case *dst.Ident:
		info = ": " + curNode.Name
	case *dst.GenDecl:
		info = ": " + curNode.Tok.String()
	case *dst.BasicLit:
		info = ": " + curNode.Kind.String() + "(" + curNode.Value + ")"
	}
	fmt.Printf("%s%s: %v %s\n", strings.Repeat("    ", v.depth-1), cursor.Name(), curNodeType, info)
	return true
}

func (v *simpleVisitor) post(_ *dstutil.Cursor) bool {
	v.depth--
	return true
}
