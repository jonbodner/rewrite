package main

import (
	"fmt"
	"net/http"
)

func main() {
	result := test(1, "hello", []string{"nope", "hello", "goodbye"})
	fmt.Println(result)

	http.HandleFunc("/hi", func(rw http.ResponseWriter, r *http.Request) {
		rw.Write([]byte("hello"))
	})

	http.HandleFunc("/bye", myHandlerFunc)

	http.Handle("/another", MyHandle{"another"})
	mh := MyHandle{"var_val"}
	http.Handle("/var_val", mh)

	http.ListenAndServe(":8080", nil)
}

func myHandlerFunc(rw http.ResponseWriter, r *http.Request) {
	rw.Write([]byte("goodbye"))
}

type MyHandle struct {
	msg string
}

func (mh MyHandle) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	rw.Write([]byte(mh.msg))
}

func test(a int, b string, c []string) bool {
	if len(c) > a {
		return c[a] == b
	}
	return false
}
